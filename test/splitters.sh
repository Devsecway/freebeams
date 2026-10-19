#!/usr/bin/env bash
# Splitter stress test: launches FreeWorship, builds a small schedule and puts
# a song live (so the monitors, slide grids and schedule rows hold real
# render.SlideViews), then drags every splitter back and forth through Godot's
# input pipeline (the drag-split control hook), with several mouse motions per
# frame as a trackpad delivers them. The app must survive every drag and the
# log must stay clean.
#
#   test/splitters.sh                       # needs gd (or bin/gd), curl, python3
#   SPLITTERS="resource" STEPS=60 BURST=6 test/splitters.sh
#
# The app log is kept in test/out/splitters.log. When a step fails while the
# app is still running (a hang), its main thread is sampled into
# test/out/splitters.sample.txt before the app is stopped.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/test/out"
mkdir -p "$OUT"
LOG="$OUT/splitters.log"
: >"$LOG"
rm -f "$OUT/splitters.sample.txt" "$OUT/splitters.fws"

LIB="$(mktemp -t freeworship-split)"
KEY="testkey"
STEPS="${STEPS:-40}"
BURST="${BURST:-3}"
SPLITTERS="${SPLITTERS:-main top preview-live preview live resource}"
APP_PID=""
BASE=""

log() { printf '[splitters] %s\n' "$*"; }

# descendants lists the given processes and everything under them. bin/gd
# execs `go tool gd`, which runs gd, which runs Godot as a grandchild: killing
# only the launcher's children left Godot running after a failed run, and the
# orphan then disturbed the next run's focus-dependent checks.
descendants() {
	local p
	for p in "$@"; do
		echo "$p"
		# shellcheck disable=SC2046
		descendants $(pgrep -P "$p" 2>/dev/null)
	done
}

app_pids() {
	[ -n "$APP_PID" ] || return 0
	descendants "$APP_PID"
}

# godot_pid finds the Godot process under the launcher (up to two levels down).
godot_pid() {
	local p q
	for p in $(app_pids) $(for q in $(app_pids); do pgrep -P "$q" 2>/dev/null; done); do
		if ps -o comm= -p "$p" 2>/dev/null | grep -q 'Godot'; then
			echo "$p"
			return
		fi
	done
}

app_running() {
	local p
	for p in $(app_pids); do
		kill -0 "$p" 2>/dev/null && return 0
	done
	return 1
}

cleanup() {
	if app_running; then
		log "stopping the app"
		for p in $(app_pids); do kill "$p" 2>/dev/null || true; done
		sleep 1
		for p in $(app_pids); do kill -9 "$p" 2>/dev/null || true; done
	fi
	rm -f "$LIB" "$LIB-wal" "$LIB-shm"
}
trap cleanup EXIT

fail() {
	printf '\n[splitters] FAIL: %s\n' "$1" >&2
	if [ -n "${2:-}" ]; then printf '[splitters] last response: %s\n' "$2" | head -c 2000 >&2; echo >&2; fi
	local g
	g="$(godot_pid)"
	if [ -n "$g" ] && kill -0 "$g" 2>/dev/null; then
		log "the app is still running (pid $g): sampling it into $OUT/splitters.sample.txt"
		sample "$g" 2 -file "$OUT/splitters.sample.txt" >/dev/null 2>&1 || true
	fi
	printf '[splitters] app log: %s (last lines below)\n' "$LOG" >&2
	tail -n 40 "$LOG" >&2
	exit 1
}

post() {
	curl -sS --max-time 45 -X POST -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
		-d "${2:-}" "$BASE/action/$1" || echo '{"ok":false,"error":"curl failed"}'
}
state() { curl -sS --max-time 45 -H "Authorization: Bearer $KEY" "$BASE/state" || echo '{"ok":false}'; }

check() {
	python3 - "$2" "$3" <<'PY' || fail "$1" "$2"
import json, sys
try:
    r = json.loads(sys.argv[1])
except Exception as e:
    print("not JSON:", e, file=sys.stderr); sys.exit(1)
d = r.get("data") or {}
s = d.get("state", d) if isinstance(d, dict) else {}
try:
    ok = eval(sys.argv[2], {"r": r, "s": s, "d": d})
except Exception as e:
    print("check error:", e, file=sys.stderr); ok = False
sys.exit(0 if ok else 1)
PY
	log "ok: $1"
}

act() {
	local resp
	resp="$(post "$2" "$3")"
	check "$1" "$resp" "$4"
	LAST="$resp"
}

# default_percent is a splitter's default position (ui.MainWindow constants).
default_percent() {
	case "$1" in
	main) echo 0.71 ;;
	top) echo 0.1944 ;;
	preview-live) echo 0.5 ;;
	preview | live) echo 0.4638 ;;
	resource) echo 0.8542 ;;
	*) echo 0.5 ;;
	esac
}

# ---- launch -------------------------------------------------------------------
GD="gd"
[ -x "$ROOT/bin/gd" ] && GD="$ROOT/bin/gd"
command -v "$GD" >/dev/null || fail "$GD not found"
log "launching: FREEWORSHIP_LIBRARY=$LIB $GD run"
(
	cd "$ROOT" || exit 1
	export FREEWORSHIP_LIBRARY="$LIB" FREEWORSHIP_CONTROL_KEY="$KEY"
	exec "$GD" run
) >"$LOG" 2>&1 &
APP_PID=$!

for _ in $(seq 1 600); do
	BASE="$(grep -m1 -o 'Control server: http://[0-9.:]*' "$LOG" | sed 's/^Control server: //')"
	[ -n "$BASE" ] && break
	app_running || fail "the app exited before starting the control server"
	sleep 0.5
done
[ -n "$BASE" ] || fail "no 'Control server:' line in the log"
log "control server at $BASE"

for _ in $(seq 1 240); do
	LAST="$(state)"
	python3 -c 'import json,sys; s=json.loads(sys.argv[1])["data"]; sys.exit(0 if not s["import"]["running"] else 1)' "$LAST" 2>/dev/null && break
	sleep 0.5
done
check "import finished" "$LAST" 'not s["import"]["running"]'
act "output on" output '{"on":true}' 's["outputOpen"]'
act "search amazing" search-songs '{"q":"amazing"}' 's["songs"]["results"] >= 1'
act "select Amazing Grace" select-song '{"title":"Amazing Grace"}' '"Amazing Grace" in s["preview"]["title"]'
act "add the song to the schedule" add-to-schedule '{"source":"song"}' 's["schedule"]["items"] == ["Amazing Grace"]'
act "reference jn 3:16-18" reference '{"text":"jn 3:16-18"}' 's["preview"]["count"] == 3'
act "add the passage to the schedule" add-to-schedule '{"source":"scripture"}' 'len(s["schedule"]["items"]) == 2'
act "select the song again" select-song '{"title":"Amazing Grace"}' '"Amazing Grace" in s["preview"]["title"]'
act "go live" go-live '' 's["live"]["title"] == s["preview"]["title"]'
act "app idle" wait-idle '' 's.get("idle")'

# ---- drags -------------------------------------------------------------------------
# For every splitter: real drags both ways, with and without bursts of motions
# per frame, then programmatic moves across the whole range (including the
# snap-close zones) and back to its default, so the next splitter is visible.
for name in $SPLITTERS; do
	for delta in -6 6 -2 2; do
		for burst in 1 "$BURST"; do
			desc="drag $name by $delta px/frame × $STEPS (burst $burst)"
			act "$desc" drag-split "{\"name\":\"$name\",\"delta\":$delta,\"steps\":$STEPS,\"burst\":$burst}" 'r.get("ok")'
			app_running || fail "the app died during: $desc"
		done
	done
	for pct in 0.05 0.95 0.3 0.7 "$(default_percent "$name")"; do
		act "split $name to $pct" split "{\"name\":\"$name\",\"percent\":$pct}" 'r.get("ok")'
	done
	act "app idle after $name" wait-idle '' 's.get("idle")'
	check "$name is open at its default" "$LAST" "abs(s['splitters']['$name']['percent'] - $(default_percent "$name")) < 1e-6 and s['splitters']['$name']['collapsed'] == 0"
done
LAST="$(state)"
check "still live after the drags" "$LAST" '"Amazing Grace" in s["live"]["title"]'

# ---- quit ------------------------------------------------------------------------------------
act "save the schedule (so closing does not ask)" save-schedule "{\"path\":\"$OUT/splitters.fws\"}" 'not s["schedule"]["dirty"]'
LAST="$(post close)"
log "closing the window"
for _ in $(seq 1 60); do
	app_running || break
	sleep 0.5
done
app_running && fail "the app did not exit after close"

BAD="$(grep -nE 'ERROR|WARNING|leaked|Leaked|panic:|SCRIPT ERROR|out of memory|SIGSEGV|crashed' "$LOG" || true)"
if [ -n "$BAD" ]; then
	printf '%s\n' "$BAD" >&2
	fail "the app log has errors or warnings"
fi
grep -q 'shutting down' "$LOG" || fail "no clean shutdown in the log"
log "PASS"
