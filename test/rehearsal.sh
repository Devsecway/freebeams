#!/usr/bin/env bash
# Sunday rehearsal: a whole service, run the way an operator runs it, for a
# while, on a fresh library (what a new install starts with).
#
#   test/rehearsal.sh                   # 3 passes through the service (~15 min)
#   REHEARSAL_PASSES=8 test/rehearsal.sh
#   SMOKE_APP=releases/darwin/universal/FreeBeams.app/Contents/MacOS/FreeBeams test/rehearsal.sh
#
# Saturday: build the schedule (songs, a passage, a motion background, a PDF
# deck), save it, quit. Sunday: relaunch, open it, turn the output on and run
# the service end to end, every slide of every item, with Black / Logo /
# Clear, a nursery code, a countdown and a ticker, and the video playing.
# Each pass is checked through /state; the app's memory is sampled after each
# pass, and the run fails on unbounded growth, on any ERROR / WARNING / panic
# in the log, or when the app does not quit cleanly. test/smoke.sh checks
# each feature; this checks that they hold up together, over time.
set -uo pipefail

# A Mac that idle-sleeps mid-run stalls every request for as long as it
# sleeps: keep it (and the display, the projector) awake for the run.
if [ "$(uname)" = "Darwin" ] && [ -z "${REHEARSAL_AWAKE:-}" ] && command -v caffeinate >/dev/null; then
	REHEARSAL_AWAKE=1 exec caffeinate -i -d "$0" "$@"
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/test/out/rehearsal"
rm -rf "$OUT"
mkdir -p "$OUT"
LOG="$OUT/app.log"
LIB="$(mktemp -t freebeams-rehearsal)"
SCHED="$OUT/sunday.fbs"
KEY="rehearsalkey"
PASSES="${REHEARSAL_PASSES:-3}"
# More than this many MB of growth from the end of the first pass to the end
# of the last is reported as a leak (the first pass warms the caches).
MAX_GROWTH_MB="${REHEARSAL_MAX_GROWTH_MB:-150}"
APP_PID=""
BASE=""

log() { printf '[rehearsal] %s\n' "$*"; }

descendants() {
	local p
	for p in "$@"; do
		echo "$p"
		# shellcheck disable=SC2046
		descendants $(pgrep -P "$p" 2>/dev/null)
	done
}
app_pids() { [ -n "$APP_PID" ] && descendants "$APP_PID"; }
app_running() {
	local p
	for p in $(app_pids); do kill -0 "$p" 2>/dev/null && return 0; done
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
	printf '\n[rehearsal] FAIL: %s\n' "$1" >&2
	[ -n "${2:-}" ] && printf '[rehearsal] last response: %s\n' "$2" | head -c 4000 >&2 && echo >&2
	printf '[rehearsal] app log: %s (last lines below)\n' "$LOG" >&2
	tail -n 25 "$LOG" >&2
	exit 1
}

post() {
	curl -sS --max-time 60 -X POST -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
		-d "${2:-}" "$BASE/action/$1" || echo '{"ok":false,"error":"curl failed"}'
}
state() { curl -sS --max-time 60 -H "Authorization: Bearer $KEY" "$BASE/state" || echo '{"ok":false}'; }

# holds RESPONSE PYTHON-EXPR: true when EXPR holds (s = the state, r = the
# response, d = its data).
holds() {
	python3 - "$1" "$2" <<'PY'
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
}
# check DESCRIPTION RESPONSE PYTHON-EXPR fails the run when EXPR does not hold.
check() {
	holds "$2" "$3" || fail "$1" "$2"
	[ -n "${QUIET:-}" ] || log "ok: $1"
}
act() {
	LAST="$(post "$2" "$3")"
	check "$1" "$LAST" "$4"
}
# field RESPONSE DOTTED.PATH prints a value of the state.
field() {
	python3 -c '
import json, sys
v = json.loads(sys.argv[1])["data"]
v = v.get("state", v)
for k in sys.argv[2].split("."):
    v = v[int(k)] if isinstance(v, list) else v[k]
print(v)' "$1" "$2"
}
wait_idle() { LAST="$(post wait-idle)"; QUIET=1 check "app idle" "$LAST" 'r.get("ok") and s.get("idle")'; }
# wait_for DESCRIPTION EXPR: polls /state for up to 30 s.
wait_for() {
	for _ in $(seq 1 60); do
		LAST="$(state)"
		holds "$LAST" "$2" 2>/dev/null && { log "ok: $1"; return; }
		sleep 0.5
	done
	check "$1" "$LAST" "$2"
}
shot() {
	curl -sS --max-time 60 -H "Authorization: Bearer $KEY" -o "$OUT/$1.png" "$BASE/screenshot?window=$2" ||
		fail "screenshot $1"
}

# rss_mb prints the resident memory of the app's Godot process in MB.
rss_mb() {
	local p best=0 kb
	for p in $(app_pids); do
		kb="$(ps -o rss= -p "$p" 2>/dev/null | tr -d ' ')"
		[ -n "$kb" ] && [ "$kb" -gt "$best" ] && best="$kb"
	done
	echo $((best / 1024))
}

launch() { # LOGFILE
	: >"$1"
	(
		cd "$ROOT" || exit 1
		export FREEBEAMS_LIBRARY="$LIB" FREEBEAMS_CONTROL_KEY="$KEY" FREEBEAMS_BIBLES_DIR="$OUT/bibles"
		[ -n "${SMOKE_APP:-}" ] && exec "$SMOKE_APP"
		exec "$ROOT/bin/gd" run
	) >"$1" 2>&1 &
	APP_PID=$!
	LOG="$1"
	BASE=""
	for _ in $(seq 1 600); do
		BASE="$(grep -m1 -o 'Control server: http://[0-9.:]*' "$1" | sed 's/^Control server: //')"
		[ -n "$BASE" ] && break
		app_running || fail "the app exited before starting the control server"
		sleep 0.5
	done
	[ -n "$BASE" ] || fail "no 'Control server:' line in $1"
	for _ in $(seq 1 480); do
		LAST="$(state)"
		python3 -c 'import json,sys; s=json.loads(sys.argv[1])["data"]; sys.exit(0 if not s["import"]["running"] and not s["bundled"]["running"] else 1)' "$LAST" 2>/dev/null && break
		sleep 0.5
	done
	wait_idle
}

quit_app() { # DESCRIPTION
	LAST="$(post close)"
	for _ in $(seq 1 60); do
		app_running || break
		sleep 0.5
	done
	app_running && fail "the app did not exit after quit ($1)"
	local bad
	bad="$(grep -nE 'ERROR|WARNING|leaked|Leaked|panic:|SCRIPT ERROR|fatal error' "$LOG" || true)"
	[ -z "$bad" ] || { printf '%s\n' "$bad" >&2; fail "the log has errors or warnings ($1)"; }
	grep -q 'shutting down' "$LOG" || fail "no clean shutdown in the log ($1)"
	log "ok: clean quit and a clean log ($1)"
}

# ---- Saturday: prepare the service ---------------------------------------------
mkdir -p "$OUT/bibles"
log "Saturday: preparing the service on a fresh library"
launch "$OUT/app-saturday.log"
check "a fresh install starts with the dark interface" "$LAST" 's["settings"]["interfaceColor"] == "dark"'
if [ "$(uname)" = "Darwin" ]; then
	check "the native chrome is dark too" "$LAST" '"DarkAqua" in s.get("nativeAppearance", "")'
fi
check "there are songs to sing" "$LAST" 's["songCount"] >= 3'
check "there is a Bible" "$LAST" '"KJV" in s["bibles"]'

SONGS=()
for q in amazing great holy; do
	act "search songs: $q" search-songs "{\"q\":\"$q\"}" 's["songs"]["results"] >= 1'
	t="$(field "$LAST" songs.first)"
	act "select \"$t\"" select-song "$(python3 -c 'import json,sys; print(json.dumps({"title": sys.argv[1]}))' "$t")" 's["preview"]["count"] >= 1'
	act "add \"$t\" to the schedule" add-to-schedule '{"source":"song"}' 's["schedule"]["items"][-1] == s["preview"]["title"]'
	SONGS+=("$t")
done
act "the reading: John 3:16-18" reference '{"text":"John 3:16-18"}' 's["preview"]["count"] == 3'
act "add the reading" add-to-schedule '{"source":"scripture"}' 's["schedule"]["items"][-1] == "John 3:16-18"'

act "videos" search-media '{"q":"","kind":"video"}' 's["media"]["results"] >= 0'
HAVE_VIDEO="$(python3 -c 'import json,sys; print(1 if json.loads(sys.argv[1])["data"]["media"]["results"] > 0 else 0)' "$LAST")"
if [ "$HAVE_VIDEO" = 1 ]; then
	act "pick a motion background" select-media '{"index":0}' 's["media"]["selected"] == 0'
	VIDEO="$(field "$LAST" media.current)"
	act "add the welcome loop ($VIDEO)" add-to-schedule '{"source":"media"}' 's["schedule"]["items"][-1] == s["media"]["current"]'
else
	log "no video in the library (EasyWorship not installed?): the service runs without one"
fi

# The announcements: a three-page PDF (blue, orange, green pages).
python3 - "$OUT/announcements.pdf" <<'PY'
import sys
colors = [(0.165, 0.373, 0.749), (0.749, 0.373, 0.165), (0.165, 0.6, 0.3)]
objs = ["<< /Type /Catalog /Pages 2 0 R >>",
        "<< /Type /Pages /Kids [%s] /Count %d >>" % (" ".join("%d 0 R" % (3 + 2 * i) for i in range(len(colors))), len(colors))]
for i, (r, g, b) in enumerate(colors):
    content = "%.3f %.3f %.3f rg 0 0 1920 1080 re f" % (r, g, b)
    objs.append("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 1920 1080] /Contents %d 0 R >>" % (4 + 2 * i))
    objs.append("<< /Length %d >>\nstream\n%s\nendstream" % (len(content), content))
out = b"%PDF-1.4\n"
offsets = []
for i, o in enumerate(objs):
    offsets.append(len(out))
    out += ("%d 0 obj\n%s\nendobj\n" % (i + 1, o)).encode()
xref = len(out)
out += ("xref\n0 %d\n0000000000 65535 f \n" % (len(objs) + 1)).encode()
for off in offsets:
    out += ("%010d 00000 n \n" % off).encode()
out += ("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n" % (len(objs) + 1, xref)).encode()
open(sys.argv[1], "wb").write(out)
PY
act "add the announcements deck" add-presentation "{\"path\":\"$OUT/announcements.pdf\"}" 'r["ok"]'
wait_for "the deck is rendered" 'not s["presentations"]["working"] and s["presentations"]["pending"] == 0 and s["presentations"]["failed"] == 0'
act "select the deck" select-presentation '{"title":"announcements"}' 's["presentations"]["currentPages"] == 3'
act "add the deck" add-to-schedule '{"source":"presentation"}' 's["schedule"]["items"][-1] == "announcements"'

act "save the schedule" save-schedule "{\"path\":\"$SCHED\"}" 'not s["schedule"]["dirty"]'
ITEMS="$(python3 -c 'import json,sys; print(json.dumps(json.loads(sys.argv[1])["data"]["schedule"]["items"]))' "$LAST")"
NITEMS="$(python3 -c 'import json,sys; print(len(json.loads(sys.argv[1])))' "$ITEMS")"
log "the service: $ITEMS"
quit_app "Saturday"

# ---- Sunday: run it -------------------------------------------------------------------
log "Sunday: relaunching and running the service $PASSES times"
launch "$OUT/app-sunday.log"
act "open Saturday's schedule" open-schedule "{\"path\":\"$SCHED\"}" "s[\"schedule\"][\"items\"] == $ITEMS and not s[\"schedule\"][\"dirty\"]"
act "output on" output '{"on":true}' 's["outputOpen"] and s["outputEnabled"]'
act "logo before the service" logo '' 's["mode"] == "logo"'
act "the countdown" countdown '{"minutes":5,"caption":"Service starts in"}' 's["alerts"]["countdown"]["active"]'
wait_idle
shot sunday-start main

RSS=()
START="$(date +%s)"
for pass in $(seq 1 "$PASSES"); do
	log "---- pass $pass of $PASSES"
	act "countdown off" alert-clear '{"kind":"countdown"}' 'not s["alerts"]["countdown"]["active"]'
	for i in $(seq 0 $((NITEMS - 1))); do
		act "select item $i" select-schedule "{\"index\":$i}" "s[\"schedule\"][\"selected\"] == $i"
		count="$(field "$LAST" preview.count)"
		title="$(field "$LAST" preview.title)"
		act "\"$title\" live" go-live '' "s[\"live\"][\"title\"] == s[\"preview\"][\"title\"] and s[\"live\"][\"index\"] == 0 and s[\"schedule\"][\"live\"] == $i"
		# Going live keeps a Logo (or Black) up, as in EasyWorship: the
		# operator takes it down once the first slide is ready.
		case "$(field "$LAST" mode)" in
		logo) act "logo off" logo '' 's["mode"] == "normal"' ;;
		black) act "black off" black '' 's["mode"] == "normal"' ;;
		esac
		wait_idle
		if [ "$HAVE_VIDEO" = 1 ] && [ "$title" = "$VIDEO" ]; then
			wait_for "the welcome loop plays" 's["playback"]["live"]["playing"] and s["playback"]["live"]["position"] > 0.5'
			act "black over the video" black '' 's["mode"] == "black"'
			wait_idle
			act "black off" black '' 's["mode"] == "normal"'
			wait_for "the video is still playing after Black" 's["playback"]["live"]["playing"]'
			continue
		fi
		for slide in $(seq 1 $((count - 1))); do
			QUIET=1 act "next ($slide)" next '' "s[\"live\"][\"index\"] == $slide"
			wait_idle
			if [ "$slide" = 1 ] && [ "$i" = 0 ]; then
				act "a nursery code" nursery '{"code":"A12"}' 's["alerts"]["nursery"] == ["A12"]'
				act "a ticker" alert '{"text":"The owner of the blue van, please move it.","display":"ticker","count":1}' 's["alerts"]["showing"]'
			fi
		done
		log "ok: all $count slides of \"$title\""
		case "$i" in
		1)
			act "black" black '' 's["mode"] == "black"'
			wait_idle
			act "black off" black '' 's["mode"] == "normal"'
			;;
		2)
			act "clear" clear '' 's["mode"] == "clear"'
			wait_idle
			act "clear off" clear '' 's["mode"] == "normal"'
			;;
		esac
		wait_idle
	done
	act "the nursery code is done" alert-clear '{"kind":"nursery"}' 's["alerts"]["nursery"] == []'
	act "logo after the service" logo '' 's["mode"] == "logo"'
	wait_idle
	shot "pass-$pass-main" main
	shot "pass-$pass-output" output
	RSS+=("$(rss_mb)")
	log "pass $pass done after $(($(date +%s) - START)) s; memory ${RSS[${#RSS[@]} - 1]} MB"
	bad="$(grep -nE 'ERROR|WARNING|panic:|fatal error' "$LOG" || true)"
	[ -z "$bad" ] || { printf '%s\n' "$bad" >&2; fail "the log has errors after pass $pass"; }
done

log "memory after each pass (MB): ${RSS[*]}"
if [ "${#RSS[@]}" -ge 2 ]; then
	growth=$((RSS[${#RSS[@]} - 1] - RSS[0]))
	[ "$growth" -le "$MAX_GROWTH_MB" ] || fail "memory grew by $growth MB from pass 1 to pass $PASSES (limit $MAX_GROWTH_MB MB)"
	log "ok: memory grew by $growth MB from pass 1 to pass $PASSES (limit $MAX_GROWTH_MB MB)"
fi

LAST="$(state)"
check "running the service left the schedule unchanged" "$LAST" 'not s["schedule"]["dirty"]'
quit_app "Sunday"
log "PASS ($PASSES passes of $NITEMS items in $(($(date +%s) - START)) s; screenshots in $OUT)"
