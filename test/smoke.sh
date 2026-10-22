#!/usr/bin/env bash
# End-to-end smoke test: launches FreeBeams with `gd run` on a fresh temp
# library, drives a song and a scripture to live through the control API,
# saves screenshots to test/out/ and asserts /state along the way.
#
#   test/smoke.sh            # needs gd, curl, python3
#
# Exit status is non-zero on the first failed check, with a clear message.
# The app log is kept in test/out/app.log.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/test/out"
mkdir -p "$OUT"
rm -f "$OUT"/*.png "$OUT"/*.fbs
LOG="$OUT/app.log"
: >"$LOG"

LIB="$(mktemp -t freebeams-smoke)"
SCHED="$OUT/smoke-schedule.fbs"
KEEPALIVE_TYPES="$OUT/keepalive-types.txt"
rm -f "$KEEPALIVE_TYPES"
KEY="testkey"
EW_SONGS="/Users/Shared/Softouch/Easyworship/Default/v6.1/Databases/Data/Songs.db"
APP_PID=""
BASE=""

log() { printf '[smoke] %s\n' "$*"; }

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
	printf '\n[smoke] FAIL: %s\n' "$1" >&2
	if [ -n "${2:-}" ]; then printf '[smoke] last response: %s\n' "$2" | head -c 6000 >&2; echo >&2; fi
	printf '[smoke] app log: %s (last lines below)\n' "$LOG" >&2
	tail -n 25 "$LOG" >&2
	exit 1
}

# post ACTION [JSON] prints the response body.
post() {
	curl -sS --max-time 60 -X POST -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
		-d "${2:-}" "$BASE/action/$1" || echo '{"ok":false,"error":"curl failed"}'
}
state() { curl -sS --max-time 60 -H "Authorization: Bearer $KEY" "$BASE/state" || echo '{"ok":false}'; }

# check DESCRIPTION RESPONSE PYTHON-EXPR: evaluates EXPR with s = the state in
# RESPONSE (r = the whole response) and fails the run when it is false.
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

# act DESCRIPTION ACTION JSON EXPR: post an action and check the result.
act() {
	local resp
	resp="$(post "$2" "$3")"
	check "$1" "$resp" "$4"
	LAST="$resp"
}

# pixel DESCRIPTION FILE ARGS...: a test/pngcheck.py assertion on a screenshot.
pixel() {
	local desc="$1"
	shift
	python3 "$ROOT/test/pngcheck.py" "$@" || fail "$desc"
	log "ok: $desc"
}

wait_idle() { LAST="$(post wait-idle)"; check "app idle" "$LAST" 'r.get("ok") and s.get("idle")'; }

shot() { # NAME WINDOW
	local f="$OUT/$1.png"
	curl -sS --max-time 60 -H "Authorization: Bearer $KEY" -o "$f" "$BASE/screenshot?window=$2" || fail "screenshot $1"
	python3 -c 'import sys; d=open(sys.argv[1],"rb").read(8); sys.exit(0 if d==b"\x89PNG\r\n\x1a\n" else 1)' "$f" ||
		fail "screenshot $1 is not a PNG: $(head -c 300 "$f")"
	log "saved $f"
}

# ---- launch -------------------------------------------------------------------
[ -x "$ROOT/bin/gd" ] || fail "bin/gd missing"
log "launching: FREEBEAMS_LIBRARY=$LIB bin/gd run"
(
	cd "$ROOT" || exit 1
	export FREEBEAMS_LIBRARY="$LIB" FREEBEAMS_CONTROL_KEY="$KEY"
	# The keepalive check (internal/app/keepalivecheck.go) logs every type
	# graphics.gd's per-frame walk reaches that goroutines mutate, and lists
	# the walked types in $KEEPALIVE_TYPES at shutdown (see "keepalive walk").
	export FREEBEAMS_KEEPALIVE_CHECK="$KEEPALIVE_TYPES"
	exec "$ROOT/bin/gd" run
) >"$LOG" 2>&1 &
APP_PID=$!

for _ in $(seq 1 600); do # up to 5 minutes (the first build compiles everything)
	BASE="$(grep -m1 -o 'Control server: http://[0-9.:]*' "$LOG" | sed 's/^Control server: //')"
	[ -n "$BASE" ] && break
	app_running || fail "the app exited before starting the control server"
	sleep 0.5
done
[ -n "$BASE" ] || fail "no 'Control server:' line in the log"
log "control server at $BASE"

# ---- startup: first-run import ------------------------------------------------------
for _ in $(seq 1 240); do
	LAST="$(state)"
	python3 -c 'import json,sys; s=json.loads(sys.argv[1])["data"]; sys.exit(0 if not s["import"]["running"] else 1)' "$LAST" 2>/dev/null && break
	sleep 0.5
done
check "import finished" "$LAST" 'not s["import"]["running"] and not s["import"].get("error")'
wait_idle
if [ -f "$EW_SONGS" ]; then
	check "EasyWorship songs imported (songCount >= 1)" "$LAST" 's["songCount"] >= 1 and s["songs"]["countText"].endswith("songs")'
	check "KJV imported" "$LAST" '"KJV" in s["bibles"]'
	log "library: $(python3 -c 'import json,sys; s=json.loads(sys.argv[1])["data"]; print(s["songCount"], "songs;", s["import"]["summary"])' "$LAST")"
else
	log "EasyWorship is not installed: skipping the import assertions"
fi
SONGS="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["data"]["songCount"])' "$LAST")"
check "window title shows the schedule" "$LAST" 's["windowTitle"].startswith("FreeBeams") and "Untitled" in s["windowTitle"]'
shot app-main-start main

act "output on" output '{"on":true}' 's["outputOpen"] and s["outputEnabled"]'

# A projector must stay up while the operator works in another app (browser,
# planning tools). Godot marks a borderless window that covers its screen
# hidesOnDeactivate, which emptied the TV until FreeBeams was active again;
# the cover-screen code turns that off. Checked through the window server (what
# a screenshot of the TV would show), only when a second display is present.
if python3 -c 'import json,sys; sys.exit(0 if json.loads(sys.argv[1])["data"]["screens"] > 1 else 1)' "$LAST" 2>/dev/null; then
	osascript -e 'tell application "Finder" to activate' >/dev/null 2>&1 || true
	sleep 1.5
	act "output stays on the TV while another app is active" output-native '' 'd["onScreen"] and d["shown"] and not d["hidesOnDeactivate"]'
	osascript -e 'tell application "Godot" to activate' >/dev/null 2>&1 || true
	sleep 1
fi

# ---- song ------------------------------------------------------------------------
act "search amazing" search-songs '{"q":"amazing"}' 's["songs"]["results"] >= 1 and s["songs"]["query"] == "amazing"'
act "select Amazing Grace" select-song '{"title":"Amazing Grace"}' '"Amazing Grace" in s["preview"]["title"] and s["preview"]["index"] == 0'
act "go live (crossfade running)" go-live '' 's["live"]["title"] == s["preview"]["title"] and "Amazing grace" in s["live"]["text"] and s["live"]["index"] == 0 and s["transitioning"]'
wait_idle
shot song-live-main main
shot song-live-output output
check "song row selected" "$LAST" 's["songs"]["current"] == "Amazing Grace" and s["songs"]["selected"] >= 0'
# The selected song row is drawn highlighted (list.sel.inactive #313131: the
# list is unfocused, so it is the subtle grey, not the blue).
pixel "selected song row is highlighted" "$OUT/song-live-main.png" has 313131 0.22 0.76 0.58 0.95 30000
pixel "song output has content" "$OUT/song-live-output.png" notblank

# Hotkeys with no control focused (a stray click on the window would otherwise
# hand Home/End to a list).
act "no control has focus" focus '{"target":"none"}' 's["focus"] == "none"'
act "next slide" next '' 's["live"]["index"] == 1'
act "key v (first verse)" key '{"name":"v"}' 'd["consumed"] and s["live"]["label"].startswith("Verse")'
act "key 2 (Verse 2)" key '{"name":"2"}' 's["live"]["label"] == "Verse 2"'
act "key End (last slide)" key '{"name":"End"}' 's["live"]["index"] == s["live"]["count"] - 1'
act "key Home (first slide)" key '{"name":"Home"}' 's["live"]["index"] == 0'
act "key space (next)" key '{"name":"space"}' 's["live"]["index"] == 1'
act "key backspace (prev)" key '{"name":"backspace"}' 's["live"]["index"] == 0'
act "jump chorus or verse 1" jump '{"kind":"verse","n":1}' 's["live"]["label"] == "Verse 1"'

# ---- real key events through Godot's input pipeline (keyName, focus rules) ----------
act "native Right (next)" native-key '{"name":"right"}' 's["live"]["index"] == 1'
act "native V" native-key '{"name":"v"}' 's["live"]["label"] == "Verse 1"'
act "native 3 within 1s (Verse 3)" native-key '{"name":"3"}' 's["live"]["label"] == "Verse 3"'
act "native Ctrl+B" native-key '{"name":"b","ctrl":true}' 's["mode"] == "black" and s["transitioning"]'
act "native Cmd+B" native-key '{"name":"b","meta":true}' 's["mode"] == "normal"'
act "focus the search field" focus '{"target":"song-search"}' 's["focus"] == "text"'
act "N types into the search field (no hotkey)" native-key '{"name":"n"}' 's["live"]["label"] == "Verse 3"'
act "Cmd+C in a text field is not Clear" native-key '{"name":"c","meta":true}' 's["mode"] == "normal"'
act "key Cmd+C with text focus is not consumed" key '{"name":"c","meta":true}' 'not d["consumed"] and s["mode"] == "normal"'
sleep 0.5
wait_idle
check "typed N reached the search (debounced)" "$LAST" 's["songs"]["query"] in ("amazingn", "namazing")'
act "Backspace edits the text (not Prev)" native-key '{"name":"backspace"}' 's["live"]["label"] == "Verse 3"'
sleep 0.5
wait_idle
check "search back to amazing" "$LAST" 's["songs"]["query"] == "amazing" and s["songs"]["results"] >= 1'
act "focus the preview grid" focus '{"target":"preview-grid"}' 's["focus"] == "grid" and s["preview"]["index"] == 0'
act "Right in the preview grid selects the next preview slide" native-key '{"name":"right"}' 's["preview"]["index"] == 1 and s["live"]["label"] == "Verse 3" and s["focus"] == "grid"'
act "Space in the preview grid is still Next" native-key '{"name":"space"}' 's["live"]["index"] == 3'
# Enter on a preview tile = the tile's activate (the same callback as double-click).
act "Enter in the preview grid sends that slide live" native-key '{"name":"enter"}' 's["live"]["index"] == 1 and s["preview"]["index"] == 1'
act "focus the song list" focus '{"target":"song-list"}' 's["focus"] == "list"'
act "select Amazing Grace again" select-song '{"title":"Amazing Grace"}' 's["songs"]["current"] == "Amazing Grace"'
SEL="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["data"]["songs"]["selected"])' "$LAST")"
act "Down in the song list moves the list (not the schedule)" native-key '{"name":"down"}' "s[\"songs\"][\"selected\"] == $SEL + 1"
wait_idle
check "the next song is in preview" "$LAST" 's["preview"]["title"] != "Amazing Grace" and s["live"]["title"] == "Amazing Grace"'
act "Up in the song list" native-key '{"name":"up"}' "s[\"songs\"][\"selected\"] == $SEL"
wait_idle
check "Amazing Grace is back in preview" "$LAST" 's["preview"]["title"] == "Amazing Grace" and s["songs"]["current"] == "Amazing Grace"'
# Space is a hotkey (Next). It must not also reach the focused list, whose
# ui_accept would activate the row and send the song live again from slide 1.
act "Space with the song list focused" native-key '{"name":"space"}' 's["live"]["index"] == 2'
wait_idle
check "Space did not also activate the song row" "$LAST" 's["live"]["index"] == 2 and s["focus"] == "list"'
# PgDn is a hotkey (go live with the next schedule item; the schedule is empty
# here). If it leaked to the focused list, the list would page its selection.
act "PgDn with the song list focused" native-key '{"name":"pagedown"}' "s[\"songs\"][\"selected\"] == $SEL"
wait_idle
check "PgDn did not also page the song list" "$LAST" "s[\"songs\"][\"selected\"] == $SEL and s[\"songs\"][\"current\"] == \"Amazing Grace\""
act "release focus" focus '{"target":"none"}' 's["focus"] == "none"'
act "Enter (no focus) goes live with the preview" native-key '{"name":"enter"}' 's["live"]["title"] == "Amazing Grace" and s["live"]["index"] == s["preview"]["index"]'

# Review #10: keys typed while the (windowed) output window has keyboard
# focus are hotkeys too. The main window's focused list must not matter.
act "go live with slide 3" go-live '{"slide":2}' 's["live"]["index"] == 2'
act "focus the song list (main window)" focus '{"target":"song-list"}' 's["focus"] == "list"'
SEL="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["data"]["songs"]["selected"])' "$LAST")"
act "Left in the output window is Prev" native-key '{"name":"left","window":"output"}' "s[\"live\"][\"index\"] == 1 and s[\"songs\"][\"selected\"] == $SEL"
act "Ctrl+B in the output window is Black" native-key '{"name":"b","ctrl":true,"window":"output"}' 's["mode"] == "black"'
act "Cmd+B in the output window toggles back" native-key '{"name":"b","meta":true,"window":"output"}' 's["mode"] == "normal"'
act "Up in the output window is not the song list's Up" native-key '{"name":"up","window":"output"}' "s[\"songs\"][\"selected\"] == $SEL"
act "release focus again" focus '{"target":"none"}' 's["focus"] == "none"'

# ---- scripture ---------------------------------------------------------------------
act "reference jn 3:16" reference '{"text":"jn 3:16"}' 's["preview"]["title"] == "John 3:16" and "For God so loved" in s["preview"]["text"]'
act "scripture live" go-live '' 's["live"]["title"] == "John 3:16" and "For God so loved" in s["live"]["text"] and "(KJV)" in s["live"]["footer"]'
wait_idle
shot scripture-live-main main
shot scripture-live-output output
act "Go Back returns the previous item to live" go-back '' 's["live"]["title"] == "Amazing Grace"'
act "Go Back again" go-back '' 's["live"]["title"] == "John 3:16"'
pixel "scripture output has content" "$OUT/scripture-live-output.png" notblank
# The verse list lists whole chapters with the referenced verses selected;
# Preview shows the selected verses.
act "reference John 3:16-18" reference '{"text":"John 3:16-18"}' 's["scripture"]["verses"] == 36 and s["scripture"]["listed"] == "John 3" and s["scripture"]["rows"] == [15, 16, 17] and s["preview"]["count"] == 3'
act "select preview slide 2" select-preview-slide '{"i":2}' 's["preview"]["index"] == 2 and "18" in s["preview"]["label"] and s["scripture"]["selected"] == 17'
act "reference 1 cor 13" reference '{"text":"1 cor 13"}' 's["preview"]["title"] == "1 Corinthians 13" and s["scripture"]["verses"] == 13 and s["preview"]["count"] == 13'
act "reference ps 23:1-3" reference '{"text":"ps 23:1-3"}' 's["preview"]["title"] == "Psalms 23:1-3" and s["preview"]["count"] == 3 and s["preview"]["footer"] == "Psalms 23:1 (KJV)"'
act "reference in another Bible" reference '{"text":"jn 3:16","bible":"RVA"}' 's["scripture"]["bible"] == "RVA" and "(RVA)" in s["preview"]["footer"]'
act "back to KJV" reference '{"text":"John 3:16-18","bible":"KJV"}' 's["scripture"]["bible"] == "KJV" and s["preview"]["count"] == 3'
if [ -f "$EW_SONGS" ]; then
	# The first-run import leaves copyrighted Bibles (HCSB here) out; the
	# Scriptures gear menu imports them on request, after a question.
	check "HCSB is offered for import" "$LAST" '"HCSB" in s["scripture"]["copyrightedAvailable"] and "HCSB" not in s["bibles"]'
	act "import-bibles asks first" import-bibles '{}' 's["dialog"]["confirm"] == "Import Copyrighted Bibles"'
	shot import-bibles-confirm dialog
	act "cancel the import" dismiss '' 's["focus"] != "dialog" and "HCSB" not in s["bibles"]'
	act "import the copyrighted Bibles" import-bibles '{"confirm":true}' '"HCSB" in s["bibles"] and s["scripture"]["copyrightedAvailable"] == [] and "HCSB" in s["scripture"]["status"]'
	act "the imported Bible works" reference '{"text":"jn 3:16","bible":"HCSB"}' 's["scripture"]["bible"] == "HCSB" and "(HCSB)" in s["preview"]["footer"]'
	act "back to KJV after the import" reference '{"text":"John 3:16-18","bible":"KJV"}' 's["scripture"]["bible"] == "KJV" and s["preview"]["count"] == 3'
fi
# Splitter drags with content in every grid: a width-dependent minimum size
# inside a ScrollContainer used to make Godot toggle the scrollbar forever
# (message queue out of memory) at some widths. Preview | Live and the
# resource splitter, there and back, with bursts of motions per frame.
act "drag Preview | Live left" drag-split '{"name":"preview-live","delta":-6,"steps":40,"burst":3}' 'r["ok"]'
act "drag Preview | Live back" drag-split '{"name":"preview-live","delta":6,"steps":40,"burst":3}' 'r["ok"]'
check "Preview | Live is back at 50%" "$LAST" 'abs(s["splitters"]["preview-live"]["percent"] - 0.5) < 0.001 and s["splitters"]["preview-live"]["collapsed"] == 0'
act "drag the resource splitter left" drag-split '{"name":"resource","delta":-6,"steps":40,"burst":3}' 'r["ok"]'
act "drag the resource splitter back" drag-split '{"name":"resource","delta":6,"steps":40,"burst":3}' 'r["ok"]'
wait_idle
# Review #13: after the deck changes above (13, 3, 1, then 3 slides) the
# first Preview tile (selected: its tile.frame is #233563) must sit at the
# grid padding, not at the grid's top-left corner.
wait_idle
shot grid-deck-change main
pixel "first preview tile is not drawn at the grid corner" "$OUT/grid-deck-change.png" lacks 233563 0.196 0.1008 0.31 0.1067
pixel "first preview tile is at the grid padding" "$OUT/grid-deck-change.png" has 233563 0.196 0.108 0.31 0.1114 500

# Review #2: the verse Go Live sends is the highlighted one, also after
# moving in the Preview grid (not the last clicked verse row).
act "the passage goes live at verse 16" go-live '{"slide":0}' 's["live"]["title"] == "John 3:16-18" and s["live"]["index"] == 0'
act "select preview slide 3 (John 3:18)" select-preview-slide '{"i":2}' 's["scripture"]["selected"] == 17 and s["resource"]["caption"] == "John 3:18 · 3 / 3"'
act "resource Go Live sends the highlighted verse" resource-go-live '' 's["live"]["title"] == "John 3:16-18" and s["live"]["index"] == 2 and s["live"]["label"] == "John 3:18" and s["scripture"]["selected"] == 17'
act "move the preview back to slide 2" select-preview-slide '{"i":1}' 's["scripture"]["selected"] == 16 and s["resource"]["caption"] == "John 3:17 · 2 / 3"'
act "Go Live again follows the preview" resource-go-live '' 's["live"]["index"] == 1 and s["live"]["label"] == "John 3:17"'
# Review #1: the verse list has no use for Backspace, so it is still Prev.
act "focus the verse list" focus '{"target":"verse-list"}' 's["focus"] == "list"'
act "Backspace in the verse list is Prev" native-key '{"name":"backspace"}' 's["live"]["index"] == 0 and s["schedule"]["items"] == []'
act "release focus (verses)" focus '{"target":"none"}' 's["focus"] == "none"'

# The locator: a partial book lists the books it may be (typed through
# Godot's input pipeline), a suggestion completes the field, "john 3" lists
# the chapter, and Shift / Cmd clicks select verses that go to Preview and
# Live together. A double-click on the Preview monitor goes live.
act "focus the reference field" focus '{"target":"reference"}' 's["focus"] == "text"'
act "Cmd+A selects the field's text" native-key '{"name":"a","meta":true}' 'r["ok"]'
act "type j" native-key '{"name":"j"}' 's["scripture"]["field"] == "j" and s["scripture"]["suggestionsShown"]'
act "type o" native-key '{"name":"o"}' 's["scripture"]["field"] == "jo"'
act "type h" native-key '{"name":"h"}' 's["scripture"]["field"] == "joh" and s["scripture"]["suggestions"] == ["John", "1 John", "2 John", "3 John"] and s["scripture"]["suggestionsShown"] and s["scripture"]["suggestion"] == 0'
sleep 0.5
wait_idle
check "joh is not an error (John 1 is listed)" "$LAST" 'not s["scripture"]["error"] and s["scripture"]["listed"] == "John 1" and s["scripture"]["suggestionsShown"]'
shot scripture-suggestions main
act "Down highlights 1 John" native-key '{"name":"down"}' 's["scripture"]["suggestion"] == 1 and s["scripture"]["field"] == "joh"'
act "Enter picks 1 John" native-key '{"name":"enter"}' 's["scripture"]["field"] == "1 John " and not s["scripture"]["suggestionsShown"]'
wait_idle
check "1 John 1 is listed" "$LAST" 's["scripture"]["listed"] == "1 John 1" and s["preview"]["title"] == "1 John 1" and not s["scripture"]["error"]'
act "Cmd+A again" native-key '{"name":"a","meta":true}' 'r["ok"]'
act "type g" native-key '{"name":"g"}' 's["scripture"]["suggestions"] == ["Genesis", "Galatians"] and s["scripture"]["suggestionsShown"]'
act "Esc closes the suggestions" native-key '{"name":"escape"}' 'not s["scripture"]["suggestionsShown"] and s["scripture"]["field"] == "g" and s["focus"] == "text"'
act "type e (the list opens again)" native-key '{"name":"e"}' 's["scripture"]["suggestions"] == ["Genesis"] and s["scripture"]["suggestionsShown"] and s["scripture"]["suggestion"] == 0'
act "click the Genesis suggestion" click '{"target":"suggestion","index":0}' 's["scripture"]["field"] == "Genesis " and not s["scripture"]["suggestionsShown"] and s["focus"] == "text"'
wait_idle
check "a book alone lists its first chapter" "$LAST" 's["scripture"]["listed"] == "Genesis 1" and s["scripture"]["verses"] == 31'
act "xyz 3 is an error" reference '{"text":"xyz 3"}' 'not r["ok"] and "unknown book" in r["error"]'
LAST="$(state)"
check "the unknown book is a red status, with no suggestions" "$LAST" 's["scripture"]["error"] and s["scripture"]["suggestions"] == []'
act "jo 3 is ambiguous, not an error" reference '{"text":"jo 3"}' 'r["ok"] and not s["scripture"]["error"] and s["scripture"]["status"].startswith("Choose a book: Joshua") and len(s["scripture"]["suggestions"]) == 8'
act "john 3 lists the chapter" reference '{"text":"john 3"}' 's["scripture"]["verses"] == 36 and s["scripture"]["rows"] == list(range(36)) and s["preview"]["title"] == "John 3" and s["preview"]["count"] == 36'
act "john 3:16 selects verse 16" reference '{"text":"john 3:16"}' 's["scripture"]["verses"] == 36 and s["scripture"]["rows"] == [15] and s["preview"]["count"] == 1 and s["preview"]["title"] == "John 3:16"'
act "click verse 16" click '{"target":"verse","index":15}' 's["scripture"]["rows"] == [15] and s["focus"] == "list"'
act "Shift-click verse 18" click '{"target":"verse","index":17,"shift":true}' 's["scripture"]["rows"] == [15, 16, 17] and s["preview"]["title"] == "John 3:16-18" and s["preview"]["count"] == 3 and s["scripture"]["field"] == "John 3:16-18"'
wait_idle
shot scripture-multi-preview main
act "Go Live sends the three verses" resource-go-live '' 's["live"]["title"] == "John 3:16-18" and s["live"]["count"] == 3 and s["live"]["index"] == 0 and "For God so loved" in s["live"]["text"]'
wait_idle
shot scripture-multi-output output
pixel "the passage is on the output" "$OUT/scripture-multi-output.png" notblank
act "Cmd-click verse 20 adds it" click '{"target":"verse","index":19,"meta":true}' 's["scripture"]["rows"] == [15, 16, 17, 19] and s["preview"]["title"] == "John 3:16-18, 20" and s["preview"]["count"] == 4'
act "Cmd-click verse 17 removes it" click '{"target":"verse","index":16,"meta":true}' 's["scripture"]["rows"] == [15, 17, 19] and s["scripture"]["passage"] == "John 3:16, 18, 20" and s["scripture"]["field"] == "John 3:16, 18, 20"'
act "Shift+Down extends from the anchor" native-key '{"name":"down","shift":true}' 's["scripture"]["rows"] == [16, 17] and s["preview"]["title"] == "John 3:17-18"'
act "a double-click in the selection sends it all live" click '{"target":"verse","index":17,"double":true}' 's["live"]["title"] == "John 3:17-18" and s["live"]["index"] == 1 and s["scripture"]["rows"] == [16, 17]'
act "a plain click in the selection" click '{"target":"verse","index":16}' 's["scripture"]["rows"] == [16, 17]'
sleep 0.6
LAST="$(state)"
check "narrows it once no double-click follows" "$LAST" 's["scripture"]["rows"] == [16] and s["preview"]["title"] == "John 3:17"'
act "add the passage to the schedule" add-to-schedule '{"source":"scripture"}' 's["schedule"]["items"] == ["John 3:17"]'
act "remove it again" remove-schedule '{"index":0}' 's["schedule"]["items"] == []'
act "click verse 21" click '{"target":"verse","index":20}' 's["preview"]["title"] == "John 3:21" and s["live"]["title"] == "John 3:17-18"'
act "a single click on the Preview monitor does nothing" click '{"target":"preview-monitor"}' 's["live"]["title"] == "John 3:17-18"'
act "a double-click on the Preview monitor goes live" click '{"target":"preview-monitor","double":true}' 's["live"]["title"] == "John 3:21" and s["live"]["index"] == s["preview"]["index"]'
act "release focus (locator)" focus '{"target":"none"}' 's["focus"] == "none"'

act "missing verses report an error" reference '{"text":"jn 3:99"}' 'not r["ok"] and "no verses" in r["error"].lower()'
LAST="$(state)"
check "scripture status shows No verses found" "$LAST" 's["scripture"]["error"] and "No verses found" in s["scripture"]["status"]'
# Review #5: a failed lookup clears the old passage; adding then does nothing.
check "the failed lookup cleared the passage" "$LAST" 's["scripture"]["verses"] == 0 and not s["scripture"].get("passage") and s["resource"]["title"] == ""'
act "add-to-schedule after a failed lookup adds nothing" add-to-schedule '{"source":"scripture"}' 'not r["ok"] and "nothing to add" in r["error"].lower()'
LAST="$(state)"
check "the schedule is unchanged and the status says why" "$LAST" 's["schedule"]["items"] == [] and "Nothing to add" in s["scripture"]["status"]'
act "unparseable reference reports an error" reference '{"text":"jn 99:1"}' 'not r["ok"] and "out of range" in r["error"]'
LAST="$(state)"
check "scripture status shows the parse error" "$LAST" 's["scripture"]["error"] and "out of range" in s["scripture"]["status"] and s["scripture"]["verses"] == 0'

# ---- black / clear / logo -----------------------------------------------------------------
act "black on" black '' 's["mode"] == "black"'
wait_idle
shot black-main main
shot black-output output
pixel "black output is black" "$OUT/black-output.png" black
pixel "live monitor is black" "$OUT/black-main.png" has 000000 0.66 0.40 0.93 0.63 290000
act "black off" black '' 's["mode"] == "normal"'
act "ctrl+b hotkey" key '{"name":"b","ctrl":true}' 's["mode"] == "black"'
act "cmd+b hotkey" key '{"name":"b","meta":true}' 's["mode"] == "normal"'
act "logo on" logo '' 's["mode"] == "logo"'
wait_idle
shot logo-output output
pixel "logo output has content" "$OUT/logo-output.png" notblank
# The default logo is the built-in FreeBeams screen (core.WriteDefaultLogo): the screen gradient is
# #221E69 along its anti-diagonal, and the wordmark's slide is pure white in the middle.
LAST="$(state)"
check "the default logo is the FreeBeams screen" "$LAST" 's["settings"]["logo"]["kind"] == "image" and s["settings"]["logo"]["path"].endswith("FreeBeams.png")'
pixel "logo output shows the FreeBeams screen gradient" "$OUT/logo-output.png" has 221e69 0.0 0.0 1.0 1.0 2000
pixel "logo output shows the FreeBeams wordmark" "$OUT/logo-output.png" has ffffff 0.3 0.35 0.7 0.65 300
act "ctrl+l hotkey (logo off)" key '{"name":"l","ctrl":true}' 's["mode"] == "normal"'
act "ctrl+c hotkey (clear on)" key '{"name":"c","ctrl":true}' 's["mode"] == "clear"'
act "clear off" clear '' 's["mode"] == "normal"'

# ---- schedule ------------------------------------------------------------------------
act "add song to schedule" add-to-schedule '{"source":"song"}' 's["schedule"]["items"] == ["Amazing Grace"] and s["schedule"]["dirty"]'
act "reference jn 3:16 again" reference '{"text":"jn 3:16"}' 's["scripture"]["passage"] == "John 3:16"'
act "add scripture to schedule" add-to-schedule '{"source":"scripture"}' 's["schedule"]["items"] == ["Amazing Grace", "John 3:16"]'
check "title has a dirty marker" "$LAST" 's["windowTitle"].endswith(" •")'
act "select schedule item 0" select-schedule '{"index":0}' 's["preview"]["title"] == "Amazing Grace" and s["schedule"]["selected"] == 0'
act "schedule item live" go-live '' 's["schedule"]["live"] == 0 and s["live"]["title"] == "Amazing Grace"'
act "PgDn: go live with the next item" key '{"name":"pagedown"}' 's["schedule"]["live"] == 1 and s["live"]["title"] == "John 3:16"'
act "PgUp: go live with the previous item" key '{"name":"pageup"}' 's["schedule"]["live"] == 0 and s["live"]["title"] == "Amazing Grace"'
act "Down: next item into preview" key '{"name":"down"}' 's["schedule"]["selected"] == 1 and s["preview"]["title"] == "John 3:16" and s["schedule"]["live"] == 0'
wait_idle
shot schedule-main main

# Review #3: the selected row follows Preview, so after Preview gets library
# content, clicking the previously selected row loads it again.
act "click schedule row 0" click-schedule '{"index":0}' 's["schedule"]["selected"] == 0 and s["preview"]["title"] == "Amazing Grace" and s["focus"] == "list"'
act "preview library content" reference '{"text":"jn 3:17"}' 's["preview"]["title"] == "John 3:17" and s["schedule"]["selected"] == -1'
act "clicking row 0 again loads it again" click-schedule '{"index":0}' 's["schedule"]["selected"] == 0 and s["preview"]["title"] == "Amazing Grace"'
# Review #1: Backspace (a MacBook's delete key) in the focused schedule
# removes the selected item; it is not Prev.
act "next slide on the live item" next '' 's["live"]["title"] == "Amazing Grace" and s["live"]["index"] == 1'
act "click schedule row 1" click-schedule '{"index":1}' 's["schedule"]["selected"] == 1 and s["preview"]["title"] == "John 3:16" and s["focus"] == "list"'
act "Backspace in the schedule removes the item (not Prev)" native-key '{"name":"backspace"}' 's["schedule"]["items"] == ["Amazing Grace"] and s["live"]["title"] == "Amazing Grace" and s["live"]["index"] == 1'
act "look up jn 3:16 again" reference '{"text":"jn 3:16"}' 's["scripture"]["passage"] == "John 3:16"'
act "add it back" add-to-schedule '{"source":"scripture"}' 's["schedule"]["items"] == ["Amazing Grace", "John 3:16"] and s["schedule"]["live"] == 0'
act "move item" move-schedule '{"from":1,"to":0}' 's["schedule"]["items"] == ["John 3:16", "Amazing Grace"] and s["schedule"]["live"] == 1'
act "move back" move-schedule '{"from":0,"to":1}' 's["schedule"]["items"] == ["Amazing Grace", "John 3:16"]'
act "save schedule" save-schedule "{\"path\":\"$SCHED\"}" 'not s["schedule"]["dirty"] and s["schedule"]["path"].endswith("smoke-schedule.fbs") and s["schedule"]["title"] == "smoke-schedule"'
[ -s "$SCHED" ] || fail "schedule file $SCHED was not written"
check "title without dirty marker" "$LAST" 's["windowTitle"] == "FreeBeams — smoke-schedule"'
# Review #7: saving to another file is a Save As: the schedule takes its name.
act "Save As renames the schedule" save-schedule "{\"path\":\"$OUT/smoke-renamed.fbs\"}" 's["schedule"]["title"] == "smoke-renamed" and s["schedule"]["path"].endswith("smoke-renamed.fbs") and not s["schedule"]["dirty"]'
check "window title follows Save As" "$LAST" 's["windowTitle"] == "FreeBeams — smoke-renamed"'
act "Save to the same file keeps the name" save-schedule '{}' 's["schedule"]["title"] == "smoke-renamed"'
act "new schedule" new-schedule '' 's["schedule"]["items"] == [] and s["schedule"]["title"] == "Untitled"'
act "open schedule" open-schedule "{\"path\":\"$SCHED\"}" 'len(s["schedule"]["items"]) == 2 and s["schedule"]["items"] == ["Amazing Grace", "John 3:16"] and not s["schedule"]["dirty"] and s["schedule"]["title"] == "smoke-schedule"'
# Review #4: a failed open is shown to the operator (an error dialog), and
# the current schedule stays.
printf 'not a schedule {' >"$OUT/corrupt.fbs"
act "opening a corrupt schedule fails" open-schedule "{\"path\":\"$OUT/corrupt.fbs\"}" 'not r["ok"]'
wait_idle
check "the error is shown in a dialog" "$LAST" 's["focus"] == "dialog" and s["dialog"]["message"]["title"] == "Open Schedule" and "corrupt.fbs" in s["dialog"]["message"]["message"] and s["schedule"]["title"] == "smoke-schedule" and len(s["schedule"]["items"]) == 2'
shot error-dialog message
pixel "error dialog has content" "$OUT/error-dialog.png" notblank
act "hotkeys are off while the error is shown" key '{"name":"n"}' 'not d["consumed"]'
act "OK closes the error" dismiss '' 'not s["dialog"].get("message") and s["dialog"]["alerts"] == 0 and s["focus"] != "dialog"'
act "remove schedule item" remove-schedule '{"index":1}' 's["schedule"]["items"] == ["Amazing Grace"] and s["schedule"]["dirty"]'

# ---- media -----------------------------------------------------------------------------
# field RESPONSE a.b.c prints a value of the state in RESPONSE.
field() {
	python3 -c 'import json,sys
v = json.loads(sys.argv[1])["data"]
for k in sys.argv[2].split("."):
    v = v[k]
print(v)' "$1" "$2"
}
act "Media tab" tab '{"tab":"media"}' 's["tab"] == 2'
# Thumbnails, poster frames and transcodes are made in the background; wait for the pass.
for _ in $(seq 1 240); do
	LAST="$(state)"
	python3 -c 'import json,sys; s=json.loads(sys.argv[1])["data"]; sys.exit(0 if not s["media"]["working"] else 1)' "$LAST" 2>/dev/null && break
	sleep 0.5
done
check "media work finished" "$LAST" 'not s["media"]["working"] and s["media"]["tools"] != ""'
log "media tools: $(field "$LAST" media.tools); $(field "$LAST" media.countText)"
if [ -f "$EW_SONGS" ]; then
	check "EasyWorship media imported (8 videos, 15 images)" "$LAST" 's["media"]["count"] >= 23 and s["media"]["videos"] >= 8 and s["media"]["images"] >= 15'
fi
FFMPEG="$(field "$LAST" media.ffmpeg)"
if [ "$FFMPEG" = "True" ]; then
	check "every listed item has a thumbnail (ffmpeg present)" "$LAST" 's["media"]["pending"] == 0'
else
	check "stills have thumbnails without ffmpeg" "$LAST" 's["media"]["pending"] <= s["media"]["videos"]'
fi
MEDIA_COUNT="$(field "$LAST" media.count)"
IMAGES="$(field "$LAST" media.images)"
VIDEOS="$(field "$LAST" media.videos)"
wait_idle
shot media-tab main
pixel "media tab has content" "$OUT/media-tab.png" notblank
act "search media" search-media '{"q":"still"}' 's["media"]["query"] == "still" and s["media"]["results"] <= s["media"]["count"]'
act "filter images" search-media '{"q":"","kind":"image"}' 's["media"]["kind"] == "image" and s["media"]["results"] == s["media"]["images"] and s["media"]["query"] == ""'
if [ "$IMAGES" -gt 0 ]; then
	act "select the first image" select-media '{"index":0}' 's["media"]["selected"] == 0 and s["preview"]["count"] == 1 and s["preview"]["title"] == s["media"]["current"] and s["resource"]["title"] == s["media"]["current"]'
	act "image live" go-live '' 's["live"]["title"] == s["media"]["current"] and s["live"]["count"] == 1 and s["transitioning"]'
	wait_idle
	shot media-image-output output
	pixel "image output has content" "$OUT/media-image-output.png" notblank
fi
# A synthetic solid #2a7f62 image gives an exact pixel check on the projector.
MEDIA_DIR="$OUT/media"
mkdir -p "$MEDIA_DIR"
python3 - "$MEDIA_DIR/smoke-green.png" <<'PY'
import struct, sys, zlib
w, h = 320, 180
raw = (b"\x00" + bytes([0x2A, 0x7F, 0x62]) * w) * h
def chunk(kind, data):
    return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF)
png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b"")
open(sys.argv[1], "wb").write(png)
PY
printf 'not media' >"$MEDIA_DIR/notes.txt"
act "add a media folder" add-media "{\"path\":\"$MEDIA_DIR\"}" "s[\"media\"][\"count\"] == $MEDIA_COUNT + 1 and s[\"media\"][\"images\"] == $IMAGES + 1"
act "adding the folder again adds nothing" add-media "{\"path\":\"$MEDIA_DIR\"}" "s[\"media\"][\"count\"] == $MEDIA_COUNT + 1"
act "select the synthetic image" select-media '{"title":"smoke-green"}' 's["media"]["current"] == "smoke-green" and s["preview"]["title"] == "smoke-green"'
act "focus the media grid" focus '{"target":"media-grid"}' 's["focus"] == "grid"'
SEL="$(field "$LAST" media.selected)"
if [ "$SEL" -gt 0 ]; then
	act "Left in the media grid moves the selection" native-key '{"name":"left"}' "s[\"media\"][\"selected\"] == $SEL - 1 and s[\"preview\"][\"title\"] != \"smoke-green\""
	act "Right moves it back" native-key '{"name":"right"}' "s[\"media\"][\"selected\"] == $SEL and s[\"preview\"][\"title\"] == \"smoke-green\""
fi
act "Enter in the media grid sends the item live" native-key '{"name":"enter"}' 's["live"]["title"] == "smoke-green"'
act "release focus (media)" focus '{"target":"none"}' 's["focus"] == "none"'
wait_idle
shot media-green-output output
shot media-green-main main
pixel "the output shows the image" "$OUT/media-green-output.png" has 2a7f62 0.25 0.25 0.75 0.75 10000
pixel "the live monitor shows the image" "$OUT/media-green-main.png" has 2a7f62 0.68 0.42 0.91 0.61 2000
act "add media to schedule" add-to-schedule '{"source":"media"}' 's["schedule"]["items"][-1] == "smoke-green" and s["schedule"]["dirty"]'
MEDIA_ROW=$(( $(python3 -c 'import json,sys; print(len(json.loads(sys.argv[1])["data"]["schedule"]["items"]))' "$LAST") - 1 ))
act "clear live with black" black '' 's["mode"] == "black"'
act "black off" black '' 's["mode"] == "normal"'
act "select the media schedule item" select-schedule "{\"index\":$MEDIA_ROW}" "s[\"preview\"][\"title\"] == \"smoke-green\" and s[\"schedule\"][\"selected\"] == $MEDIA_ROW"
act "media schedule item live" go-live '' "s[\"schedule\"][\"live\"] == $MEDIA_ROW and s[\"live\"][\"title\"] == \"smoke-green\""
act "remove asks for confirmation" remove-media '{"title":"smoke-green"}' 's["focus"] == "dialog" and s["dialog"]["confirm"] == "Remove Media"'
act "cancel the removal" dismiss '' 's["focus"] != "dialog"'
act "remove the synthetic image" remove-media '{"title":"smoke-green","confirm":true}' "s[\"media\"][\"count\"] == $MEDIA_COUNT and s[\"media\"].get(\"current\", \"\") == \"\""
check "the schedule still has the media item" "$LAST" "s[\"schedule\"][\"items\"][$MEDIA_ROW] == \"smoke-green\" and s[\"live\"][\"title\"] == \"smoke-green\""
act "filter videos" search-media '{"q":"","kind":"video"}' 's["media"]["kind"] == "video" and s["media"]["results"] == s["media"]["videos"]'
if [ "$VIDEOS" -gt 0 ]; then
	act "select the first video" select-media '{"index":0}' 's["media"]["selected"] == 0 and s["preview"]["count"] == 1 and s["preview"]["title"] == s["media"]["current"]'
	act "video live (playing, or its poster frame)" go-live '' 's["live"]["title"] == s["media"]["current"]'
	wait_idle
	shot media-video-output output
	shot media-video-main main
	if [ "$FFMPEG" = "True" ]; then
		pixel "video output shows the poster frame or the video" "$OUT/media-video-output.png" notblank
	fi
	check "video playability matches the player (native or the Theora encoder)" "$LAST" '((s["media"]["native"] or s["media"]["theora"]) and s["media"]["playable"] == s["media"]["videos"]) or (not s["media"]["native"] and not s["media"]["theora"] and s["media"]["playable"] == 0)'
fi
act "all media" search-media '{"q":""}' 's["media"]["kind"] == "" and s["media"]["results"] == s["media"]["count"]'
act "Songs tab" tab '{"tab":"songs"}' 's["tab"] == 0'

# ---- song editor / delete -------------------------------------------------------
act "select Amazing Grace for editing" select-song '{"title":"Amazing Grace"}' 's["songs"]["current"] == "Amazing Grace"'
act "open the song editor" editor '{"mode":"edit"}' 's["focus"] == "dialog"'
wait_idle
shot song-editor editor
pixel "song editor has content" "$OUT/song-editor.png" notblank
act "hotkeys are off while a dialog is open" key '{"name":"n"}' 'not d["consumed"]'
act "close the editor" editor '{"mode":"close"}' 's["focus"] != "dialog"'
act "save a new song (editor OK path)" save-song '{"title":"Smoke Test Song","author":"FreeBeams","lyrics":"[Verse 1]\nline one\nline two\n\n[Chorus]\nchorus line"}' 's["songs"]["current"] == "Smoke Test Song" and s["preview"]["title"] == "Smoke Test Song" and s["preview"]["count"] == 2'
wait_idle
act "the new song is searchable" search-songs '{"q":"smoke test"}' "s[\"songs\"][\"results\"] == 1 and s[\"songCount\"] == $SONGS + 1"
act "delete asks for confirmation" delete-song '{"index":0}' 's["focus"] == "dialog"'
wait_idle
shot delete-confirm dialog
act "dismiss the dialog" dismiss '' 's["focus"] != "dialog"'
# Review #1: Backspace (a MacBook's delete key) in the song list asks to
# delete the selected song, like Delete; it is not Prev.
act "focus the song list for delete" focus '{"target":"song-list"}' 's["focus"] == "list" and s["songs"]["selected"] == 0'
LIVEIDX="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["data"]["live"]["index"])' "$LAST")"
act "Backspace in the song list asks to delete" native-key '{"name":"backspace"}' "s[\"focus\"] == \"dialog\" and s[\"dialog\"][\"confirm\"] == \"Delete Song\" and s[\"live\"][\"index\"] == $LIVEIDX"
act "cancel the delete" dismiss '' 's["focus"] != "dialog" and s["songs"]["results"] == 1'
act "delete confirmed" delete-song '{"index":0,"confirm":true}' 'r["ok"]'
wait_idle
check "song deleted" "$LAST" "s[\"songs\"][\"results\"] == 0 and s[\"songCount\"] == $SONGS"

# ---- closing the window ------------------------------------------------------------
# Review #9: closing while the song editor is open asks first; Cancel keeps
# the editor (and the app) open.
act "open the song editor again" editor '{"mode":"new"}' 's["dialog"]["editor"]'
act "close the window with the editor open" close '' 'r["ok"]'
wait_idle
check "closing with the editor open asks first" "$LAST" 's["dialog"]["confirm"] == "Quit FreeBeams" and s["dialog"]["editor"]'
app_running || fail "the app quit with the song editor open"
shot quit-editor-confirm dialog
act "Cancel keeps the editor open" dismiss '' 's["dialog"]["editor"] and not s["dialog"].get("confirm")'
# OK discards the editor, then asks about the unsaved schedule (the question
# moves from over the editor to over the main window).
act "close the window again" close '' 'r["ok"]'
wait_idle
check "asks about the editor again" "$LAST" 's["dialog"]["confirm"] == "Quit FreeBeams" and s["dialog"]["editor"]'
act "OK discards the editor" accept '' 'r["ok"]'
wait_idle
check "then asks about the unsaved schedule" "$LAST" 's["dialog"]["confirm"] == "Quit FreeBeams" and not s["dialog"]["editor"] and s["schedule"]["dirty"]'
app_running || fail "the app quit without asking about the unsaved schedule"
act "cancel quitting (editor discarded)" dismiss '' 's["focus"] != "dialog" and not s["dialog"]["editor"]'
LAST="$(state)"
check "schedule has unsaved changes" "$LAST" 's["schedule"]["dirty"]'
act "close the window with unsaved changes" close '' 'r["ok"]'
wait_idle
check "closing with unsaved changes asks first" "$LAST" 's["focus"] == "dialog"'
app_running || fail "the app quit without asking about unsaved changes"
shot quit-confirm dialog
act "cancel quitting" dismiss '' 's["focus"] != "dialog"'
act "save before closing" save-schedule '{}' 'not s["schedule"]["dirty"]'

# ---- presentations -----------------------------------------------------------------------
# PDF / PowerPoint decks rendered to page images (internal/presentations). A
# generated two-page PDF with solid colours gives exact pixel checks on the
# projector; a generated .pptx exercises the embedded-image fallback (or
# LibreOffice when it is installed).
act "Presentations tab" tab '{"tab":"presentations"}' 's["tab"] == 3'
for _ in $(seq 1 120); do
	LAST="$(state)"
	python3 -c 'import json,sys; s=json.loads(sys.argv[1])["data"]; sys.exit(0 if not s["presentations"]["working"] else 1)' "$LAST" 2>/dev/null && break
	sleep 0.5
done
check "presentation renderers detected" "$LAST" 'not s["presentations"]["working"] and s["presentations"]["tools"] != ""'
log "presentation renderers: $(field "$LAST" presentations.tools)"
PRES_COUNT="$(field "$LAST" presentations.count)"
PRES_PDF="$(field "$LAST" presentations.pdf)"
PRES_OFFICE="$(field "$LAST" presentations.office)"
PRES_DIR="$OUT/presentations"
mkdir -p "$PRES_DIR"
# wait_presentations waits for the render job to finish with every listed item rendered.
wait_presentations() {
	for _ in $(seq 1 240); do
		LAST="$(state)"
		python3 -c 'import json,sys; s=json.loads(sys.argv[1])["data"]["presentations"]; sys.exit(0 if not s["working"] and s["pending"] == 0 else 1)' "$LAST" 2>/dev/null && break
		sleep 0.5
	done
}
# A two-page PDF: page 1 #2a5fbf (blue), page 2 #bf5f2a (orange), 16:9 pages that fill the canvas.
python3 - "$PRES_DIR/smoke-deck.pdf" <<'PY'
import sys
colors = [(0.165, 0.373, 0.749), (0.749, 0.373, 0.165)]
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
if [ "$PRES_PDF" = "True" ]; then
	act "add a PDF presentation" add-presentation "{\"path\":\"$PRES_DIR/smoke-deck.pdf\"}" "s[\"presentations\"][\"count\"] == $PRES_COUNT + 1"
	act "adding it again adds nothing" add-presentation "{\"path\":\"$PRES_DIR/smoke-deck.pdf\"}" "s[\"presentations\"][\"count\"] == $PRES_COUNT + 1"
	wait_presentations
	check "PDF pages rendered" "$LAST" 'not s["presentations"]["working"] and s["presentations"]["pending"] == 0 and s["presentations"]["failed"] == 0'
	act "select the PDF" select-presentation '{"title":"smoke-deck"}' 's["presentations"]["current"] == "smoke-deck" and s["presentations"]["currentPages"] == 2 and not s["presentations"]["currentPartial"] and s["preview"]["count"] == 2 and s["preview"]["title"] == "smoke-deck" and s["resource"]["title"] == "smoke-deck" and s["resource"]["caption"] == "Slide 1 · 1 / 2"'
	log "PDF renderer: $(field "$LAST" presentations.currentRenderer)"
	wait_idle
	shot presentations-tab main
	pixel "presentations tab has content" "$OUT/presentations-tab.png" notblank
	act "PDF live" go-live '' 's["live"]["title"] == "smoke-deck" and s["live"]["index"] == 0 and s["live"]["count"] == 2 and s["live"]["label"] == "Slide 1"'
	wait_idle
	shot presentation-page1-output output
	pixel "the output shows page 1" "$OUT/presentation-page1-output.png" has 2a5fbf 0.25 0.25 0.75 0.75 10000
	act "Next steps to page 2" next '' 's["live"]["index"] == 1 and s["live"]["label"] == "Slide 2"'
	wait_idle
	shot presentation-page2-output output
	shot presentation-page2-main main
	pixel "the output shows page 2" "$OUT/presentation-page2-output.png" has bf5f2a 0.25 0.25 0.75 0.75 10000
	pixel "page 1 is gone from the output" "$OUT/presentation-page2-output.png" lacks 2a5fbf 0.25 0.25 0.75 0.75 200
	pixel "the live monitor shows page 2" "$OUT/presentation-page2-main.png" has bf5f2a 0.68 0.42 0.91 0.61 2000
	act "Next at the last page stays" next '' 's["live"]["index"] == 1'
	act "Prev steps back to page 1" prev '' 's["live"]["index"] == 0 and s["live"]["label"] == "Slide 1"'
	act "focus the presentation grid" focus '{"target":"presentation-grid"}' 's["focus"] == "grid"'
	act "Enter in the presentation grid sends the deck live" native-key '{"name":"enter"}' 's["live"]["title"] == "smoke-deck" and s["live"]["index"] == 0'
	act "release focus (presentations)" focus '{"target":"none"}' 's["focus"] == "none"'
	act "add presentation to schedule" add-to-schedule '{"source":"presentation"}' 's["schedule"]["items"][-1] == "smoke-deck" and s["schedule"]["dirty"]'
	PRES_ROW=$(( $(python3 -c 'import json,sys; print(len(json.loads(sys.argv[1])["data"]["schedule"]["items"]))' "$LAST") - 1 ))
	act "clear live with black (presentation)" black '' 's["mode"] == "black"'
	act "black off (presentation)" black '' 's["mode"] == "normal"'
	act "select the presentation schedule item" select-schedule "{\"index\":$PRES_ROW}" "s[\"preview\"][\"title\"] == \"smoke-deck\" and s[\"preview\"][\"count\"] == 2 and s[\"schedule\"][\"selected\"] == $PRES_ROW"
	act "presentation schedule item live" go-live "{\"slide\":1}" "s[\"schedule\"][\"live\"] == $PRES_ROW and s[\"live\"][\"title\"] == \"smoke-deck\" and s[\"live\"][\"index\"] == 1"
	act "remove asks for confirmation (presentation)" remove-presentation '{"title":"smoke-deck"}' 's["focus"] == "dialog" and s["dialog"]["confirm"] == "Remove Presentation"'
	act "cancel the removal (presentation)" dismiss '' 's["focus"] != "dialog"'
	act "remove the PDF" remove-presentation '{"title":"smoke-deck","confirm":true}' "s[\"presentations\"][\"count\"] == $PRES_COUNT and s[\"presentations\"].get(\"current\", \"\") == \"\""
	check "the schedule still has the presentation" "$LAST" "s[\"schedule\"][\"items\"][$PRES_ROW] == \"smoke-deck\" and s[\"live\"][\"title\"] == \"smoke-deck\""
else
	log "no PDF renderer on this machine (no Core Graphics, no pdftoppm): skipping the PDF checks"
fi
# A minimal .pptx with one slide holding a solid #5f2abf 960x540 picture.
python3 - "$PRES_DIR/smoke-slides.pptx" <<'PY'
import struct, sys, zipfile, zlib
w, h = 960, 540
raw = (b"\x00" + bytes([0x5F, 0x2A, 0xBF]) * w) * h
def chunk(kind, data):
    return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF)
png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b"")
R = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
P = "http://schemas.openxmlformats.org/presentationml/2006/main"
A = "http://schemas.openxmlformats.org/drawingml/2006/main"
with zipfile.ZipFile(sys.argv[1], "w", zipfile.ZIP_DEFLATED) as z:
    z.writestr("[Content_Types].xml", '<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">'
        '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/>'
        '<Default Extension="png" ContentType="image/png"/>'
        '<Override PartName="/ppt/presentation.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml"/>'
        '<Override PartName="/ppt/slides/slide1.xml" ContentType="application/vnd.openxmlformats-officedocument.presentationml.slide+xml"/></Types>')
    z.writestr("_rels/.rels", '<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        '<Relationship Id="rId1" Type="%s/officeDocument" Target="ppt/presentation.xml"/></Relationships>' % R)
    z.writestr("ppt/presentation.xml", '<?xml version="1.0"?><p:presentation xmlns:p="%s" xmlns:r="%s"><p:sldIdLst><p:sldId id="256" r:id="rId1"/></p:sldIdLst>'
        '<p:sldSz cx="12192000" cy="6858000"/></p:presentation>' % (P, R))
    z.writestr("ppt/_rels/presentation.xml.rels", '<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        '<Relationship Id="rId1" Type="%s/slide" Target="slides/slide1.xml"/></Relationships>' % R)
    z.writestr("ppt/slides/slide1.xml", '<?xml version="1.0"?><p:sld xmlns:p="%s" xmlns:a="%s" xmlns:r="%s"><p:cSld><p:spTree>'
        '<p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/>'
        '<p:pic><p:nvPicPr><p:cNvPr id="2" name="Picture"/><p:cNvPicPr/><p:nvPr/></p:nvPicPr><p:blipFill><a:blip r:embed="rId1"/><a:stretch><a:fillRect/></a:stretch></p:blipFill>'
        '<p:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="12192000" cy="6858000"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic>'
        '</p:spTree></p:cSld></p:sld>' % (P, A, R))
    z.writestr("ppt/slides/_rels/slide1.xml.rels", '<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        '<Relationship Id="rId1" Type="%s/image" Target="../media/image1.png"/></Relationships>' % R)
    z.writestr("ppt/media/image1.png", png)
PY
act "add a PowerPoint deck" add-presentation "{\"path\":\"$PRES_DIR/smoke-slides.pptx\"}" "s[\"presentations\"][\"count\"] == $PRES_COUNT + 1"
wait_presentations
check "PowerPoint pages rendered" "$LAST" 'not s["presentations"]["working"] and s["presentations"]["pending"] == 0 and s["presentations"]["failed"] == 0'
if [ "$PRES_OFFICE" = "True" ]; then
	act "select the deck (LibreOffice)" select-presentation '{"title":"smoke-slides"}' 's["presentations"]["current"] == "smoke-slides" and s["presentations"]["currentPages"] == 1 and s["presentations"]["currentRenderer"] == "libreoffice" and not s["presentations"]["currentPartial"]'
else
	act "select the deck (embedded images)" select-presentation '{"title":"smoke-slides"}' 's["presentations"]["current"] == "smoke-slides" and s["presentations"]["currentPages"] == 1 and s["presentations"]["currentRenderer"] == "embedded" and s["presentations"]["currentPartial"] and s["preview"]["count"] == 1'
fi
act "deck live" go-live '' 's["live"]["title"] == "smoke-slides" and s["live"]["count"] == 1'
wait_idle
shot presentation-pptx-output output
pixel "the output shows the deck's slide" "$OUT/presentation-pptx-output.png" has 5f2abf 0.25 0.25 0.75 0.75 10000
act "remove the deck" remove-presentation '{"title":"smoke-slides","confirm":true}' "s[\"presentations\"][\"count\"] == $PRES_COUNT"
act "search presentations" search-presentations '{"q":"zzz-nothing"}' 's["presentations"]["query"] == "zzz-nothing" and s["presentations"]["results"] == 0'
act "all presentations" search-presentations '{"q":""}' "s[\"presentations\"][\"results\"] == $PRES_COUNT"
act "Songs tab (presentations)" tab '{"tab":"songs"}' 's["tab"] == 0'
act "save the schedule (presentations)" save-schedule '{}' 'not s["schedule"]["dirty"]'
# ---- themes --------------------------------------------------------------------------
# The Themes tab lists the seeded defaults. A theme with an unusual solid
# background reaches the output through the songs-default setting and through
# a per-item override, and editing a theme that is live re-renders the output.
act "Themes tab" tab '{"tab":"themes"}' 's["tab"] == 4'
act "themes listed (seeded defaults)" themes '' 's["themes"]["count"] == 2 and s["themes"]["songDefault"] == "Default Song" and s["themes"]["scriptureDefault"] == "Default Scripture"'
act "select the song default theme" select-theme '{"name":"Default Song"}' 's["themes"]["current"] == "Default Song" and s["resource"]["title"] == "Default Song" and s["resource"]["caption"] == "Verse 1 · 1 / 1"'
wait_idle
shot themes-tab main
pixel "themes tab has content" "$OUT/themes-tab.png" notblank
# A copy of the song default with a solid magenta background (#991a80).
THEME_JSON="$(post get-theme '{"name":"Default Song"}')"
check "get-theme returns the theme" "$THEME_JSON" 'd["theme"]["name"] == "Default Song" and d["theme"]["font"]["size"] > 0'
recolor() { # THEME-RESPONSE NAME RRGGBB [ID]: a save-theme body with that name and solid color
	python3 -c 'import json,sys
t = json.loads(sys.argv[1])["data"]["theme"]
t["name"] = sys.argv[2]
t["id"] = int(sys.argv[4]) if len(sys.argv) > 4 else t["id"]
c = sys.argv[3]
t["background"] = {"kind": "color", "color": {"R": int(c[0:2], 16) / 255, "G": int(c[2:4], 16) / 255, "B": int(c[4:6], 16) / 255, "A": 1}}
print(json.dumps({"theme": t}))' "$@"
}
act "save a new theme" save-theme "$(recolor "$THEME_JSON" "Smoke Magenta" 991a80 0)" 's["themes"]["count"] == 3 and "Smoke Magenta" in s["themes"]["names"] and s["themes"]["current"] == "Smoke Magenta"'
act "make it the songs default" set-default-theme '{"name":"Smoke Magenta","kind":"song"}' 's["themes"]["songDefault"] == "Smoke Magenta" and s["themes"]["scriptureDefault"] == "Default Scripture"'
act "search amazing (themes)" search-songs '{"q":"amazing"}' 's["songs"]["results"] >= 1'
act "a new song deck picks up the default" select-song '{"title":"Amazing Grace"}' 's["preview"]["title"] == "Amazing Grace"'
act "the themed song goes live" go-live '' 's["live"]["title"] == "Amazing Grace"'
wait_idle
shot theme-magenta-output output
shot theme-magenta-main main
pixel "the output shows the theme background" "$OUT/theme-magenta-output.png" has 991a80 0.0 0.0 0.2 0.08 5000
pixel "the live monitor shows the theme background" "$OUT/theme-magenta-main.png" has 991a80 0.68 0.42 0.91 0.61 2000
# Editing the live theme (same id, teal background) re-renders the output.
act "edit the live theme" save-theme "$(recolor "$(post get-theme '{"name":"Smoke Magenta"}')" "Smoke Magenta" 1a8099)" 's["themes"]["count"] == 3 and s["live"]["title"] == "Amazing Grace"'
wait_idle
shot theme-teal-output output
pixel "the output follows the edited theme" "$OUT/theme-teal-output.png" has 1a8099 0.0 0.0 0.2 0.08 5000
pixel "the old background is gone" "$OUT/theme-teal-output.png" lacks 991a80 0.0 0.0 0.2 0.08 50
# Per-item override from the schedule.
act "look up a scripture (themes)" reference '{"text":"jn 3:16"}' 's["scripture"]["passage"] == "John 3:16"'
act "add it to the schedule (themes)" add-to-schedule '{"source":"scripture"}' 's["schedule"]["items"][-1] == "John 3:16"'
THEME_ROW=$(( $(python3 -c 'import json,sys; print(len(json.loads(sys.argv[1])["data"]["schedule"]["items"]))' "$LAST") - 1 ))
act "select the scripture item" select-schedule "{\"index\":$THEME_ROW}" "s[\"preview\"][\"title\"] == \"John 3:16\" and s[\"schedule\"][\"selected\"] == $THEME_ROW"
act "scripture item live with the scripture default" go-live '' 's["live"]["title"] == "John 3:16"'
wait_idle
shot theme-scripture-output output
pixel "the scripture item does not use the song theme" "$OUT/theme-scripture-output.png" lacks 1a8099 0.0 0.0 0.2 0.08 50
act "apply the theme to the selected schedule item" apply-theme '{"name":"Smoke Magenta"}' 's["schedule"]["dirty"] and s["live"]["title"] == "John 3:16"'
wait_idle
shot theme-override-output output
pixel "the per-item override is live" "$OUT/theme-override-output.png" has 1a8099 0.0 0.0 0.2 0.08 5000
# Defaults cannot be deleted; copies and the editor.
act "deleting a default theme is refused" delete-theme '{"name":"Smoke Magenta","confirm":true}' 'not r["ok"] and "default theme" in r["error"]'
wait_idle
check "the refusal is shown to the operator" "$LAST" 's["dialog"]["message"]["title"] == "Delete Theme" and s["themes"]["count"] == 3'
act "dismiss the refusal" dismiss '' 'not s["dialog"].get("message")'
act "restore the songs default" set-default-theme '{"name":"Default Song","kind":"song"}' 's["themes"]["songDefault"] == "Default Song"'
act "duplicate a theme" duplicate-theme '{"name":"Smoke Magenta"}' 's["themes"]["count"] == 4 and s["themes"]["current"] == "Smoke Magenta Copy"'
act "open the theme editor on it" editor '{"mode":"theme"}' 's["dialog"]["themeEditor"] and s["themes"]["editing"] == "Smoke Magenta Copy" and s["focus"] == "dialog"'
wait_idle
shot theme-editor theme-editor
pixel "theme editor has content" "$OUT/theme-editor.png" notblank
act "hotkeys are off while the theme editor is open" key '{"name":"n"}' 'not d["consumed"]'
act "close the theme editor" editor '{"mode":"close"}' 'not s["dialog"].get("themeEditor") and s["focus"] != "dialog"'
act "delete asks first (theme)" delete-theme '{"name":"Smoke Magenta Copy"}' 's["dialog"]["confirm"] == "Delete Theme"'
act "cancel the theme delete" dismiss '' 's["focus"] != "dialog" and s["themes"]["count"] == 4'
act "delete the copy" delete-theme '{"name":"Smoke Magenta Copy","confirm":true}' 's["themes"]["count"] == 3'
act "delete the smoke theme" delete-theme '{"name":"Smoke Magenta","confirm":true}' 's["themes"]["count"] == 2 and s["live"]["title"] == "John 3:16"'
act "remove the scripture item (themes)" remove-schedule "{\"index\":$THEME_ROW}" "len(s[\"schedule\"][\"items\"]) == $THEME_ROW"
act "save the schedule (themes)" save-schedule '{}' 'not s["schedule"]["dirty"]'
act "Songs tab (themes)" tab '{"tab":"songs"}' 's["tab"] == 0'
# ---- transitions and settings (Phase 2) ----------------------------------------------------
# A second solid image (#1f3f9f) beside the green one makes the mid-transition
# geometry checkable per pixel on the projector: push/wipe/iris leave the old
# color on one side and the new one on the other. Durations are long (4 s) so
# a screenshot one second in lands mid-way whatever the machine's speed. The
# image lives in its own folder, recreated here, so it never changes what the
# media section's "add a media folder" finds in $MEDIA_DIR on a later run.
MEDIA_DIR2="$OUT/media-transitions"
rm -rf "$MEDIA_DIR2"
mkdir -p "$MEDIA_DIR2"
python3 - "$MEDIA_DIR2/smoke-blue.png" <<'PY'
import struct, sys, zlib
w, h = 320, 180
raw = (b"\x00" + bytes([0x1F, 0x3F, 0x9F]) * w) * h
def chunk(kind, data):
    return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF)
png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b"")
open(sys.argv[1], "wb").write(png)
PY
act "add the blue and green images" add-media "{\"paths\":[\"$MEDIA_DIR\",\"$MEDIA_DIR2\"]}" "s[\"media\"][\"count\"] == $MEDIA_COUNT + 2"
act "default transition is a 500 ms blend" settings '' 's["settings"]["transition"] == {"kind":"blend","duration":500} and s["settings"]["outputScreen"] == -1 and s["settings"]["interfaceColor"] == "dark"'
act "green live" select-media '{"title":"smoke-green"}' 's["media"]["current"] == "smoke-green"'
act "go live with green" go-live '' 's["live"]["title"] == "smoke-green"'
wait_idle
# Push left: the new slide enters from the right, so one second in the right
# edge is blue and the left edge still green.
act "transition: push-left, 4 s" transition '{"kind":"push-left","duration":4000}' 's["settings"]["transition"] == {"kind":"push-left","duration":4000}'
act "select blue" select-media '{"title":"smoke-blue"}' 's["preview"]["title"] == "smoke-blue"'
act "push running" go-live '' 's["live"]["title"] == "smoke-blue" and s["transitioning"]'
sleep 1
shot push-mid output
shot push-mid-main main
pixel "mid-push: the right edge shows the new slide" "$OUT/push-mid.png" has 1f3f9f 0.85 0.3 1.0 0.7 200
pixel "mid-push: the left edge still shows the old slide" "$OUT/push-mid.png" has 2a7f62 0.0 0.3 0.15 0.7 200
pixel "mid-push on the live monitor too" "$OUT/push-mid-main.png" has 1f3f9f 0.80 0.42 0.91 0.61 100
wait_idle
shot push-end output
pixel "the push ends on the new slide" "$OUT/push-end.png" has 1f3f9f 0.05 0.05 0.95 0.95 10000
# Wipe left: the new slide is revealed from the right edge leftwards.
act "transition: wipe-left" transition '{"kind":"wipe-left"}' 's["settings"]["transition"] == {"kind":"wipe-left","duration":4000}'
act "select green" select-media '{"title":"smoke-green"}' 's["preview"]["title"] == "smoke-green"'
act "wipe running" go-live '' 's["live"]["title"] == "smoke-green" and s["transitioning"]'
sleep 1
shot wipe-mid output
pixel "mid-wipe: the right edge shows the new slide" "$OUT/wipe-mid.png" has 2a7f62 0.85 0.3 1.0 0.7 200
pixel "mid-wipe: the left edge still shows the old slide" "$OUT/wipe-mid.png" has 1f3f9f 0.0 0.3 0.15 0.7 200
wait_idle
# Iris: a circle from the centre; the corners keep the old slide.
act "transition: iris" transition '{"kind":"Iris"}' 's["settings"]["transition"]["kind"] == "iris"'
act "select blue again" select-media '{"title":"smoke-blue"}' 's["preview"]["title"] == "smoke-blue"'
act "iris running" go-live '' 's["transitioning"]'
sleep 1
shot iris-mid output
pixel "mid-iris: the centre shows the new slide" "$OUT/iris-mid.png" has 1f3f9f 0.45 0.45 0.55 0.55 200
pixel "mid-iris: the corners still show the old slide" "$OUT/iris-mid.png" has 2a7f62 0.0 0.0 0.1 0.1 200
wait_idle
# Zoom in: the new slide grows from the centre; the corners keep the old one.
# The screenshots wait on the transition's progress (wait-transition), not on
# time, so they land mid-way however slow the machine is: at 45 % the eased
# scale is about 0.43, and the checks hold until it is about 0.8.
act "transition: zoom-in" transition '{"kind":"Zoom In"}' 's["settings"]["transition"] == {"kind":"zoom-in","duration":4000}'
act "select green (zoom)" select-media '{"title":"smoke-green"}' 's["preview"]["title"] == "smoke-green"'
act "zoom-in running" go-live '' 's["live"]["title"] == "smoke-green" and s["transitioning"]'
act "zoom-in reaches 45 %" wait-transition '{"percent":0.45}' '0.45 <= s["transitionProgress"] < 1'
shot zoomin-mid output
shot zoomin-mid-main main
pixel "mid-zoom-in: the centre shows the new slide" "$OUT/zoomin-mid.png" has 2a7f62 0.45 0.45 0.55 0.55 200
pixel "mid-zoom-in: the corners still show the old slide" "$OUT/zoomin-mid.png" has 1f3f9f 0.0 0.0 0.08 0.08 200
pixel "mid-zoom-in: the corners do not show the new slide" "$OUT/zoomin-mid.png" lacks 2a7f62 0.0 0.0 0.08 0.08 50
pixel "mid-zoom-in on the live monitor: new slide in the centre" "$OUT/zoomin-mid-main.png" has 2a7f62 0.78 0.49 0.82 0.53 100
pixel "mid-zoom-in on the live monitor: old slide in the corner" "$OUT/zoomin-mid-main.png" has 1f3f9f 0.65 0.375 0.67 0.40 100
wait_idle
shot zoomin-end output
pixel "the zoom-in ends on the new slide" "$OUT/zoomin-end.png" has 2a7f62 0.0 0.0 1.0 1.0 10000
pixel "the zoom-in leaves nothing of the old slide" "$OUT/zoomin-end.png" lacks 1f3f9f 0.0 0.0 1.0 1.0 50
# Zoom out: the old slide shrinks into the centre over the new one.
act "transition: zoom-out" transition '{"kind":"zoom-out"}' 's["settings"]["transition"]["kind"] == "zoom-out"'
act "select blue (zoom)" select-media '{"title":"smoke-blue"}' 's["preview"]["title"] == "smoke-blue"'
act "zoom-out running" go-live '' 's["live"]["title"] == "smoke-blue" and s["transitioning"]'
act "zoom-out reaches 45 %" wait-transition '{"percent":0.45}' '0.45 <= s["transitionProgress"] < 1'
shot zoomout-mid output
pixel "mid-zoom-out: the centre still shows the old slide" "$OUT/zoomout-mid.png" has 2a7f62 0.45 0.45 0.55 0.55 200
pixel "mid-zoom-out: the corners show the new slide" "$OUT/zoomout-mid.png" has 1f3f9f 0.0 0.0 0.08 0.08 200
wait_idle
act "wait-transition without a transition fails" wait-transition '{"percent":0.5}' 'not r["ok"] and "no transition" in r["error"]'
act "unknown transition is rejected" transition '{"kind":"cube"}' 'not r["ok"]'
# Settings round trip: set-setting persists at once; the stored JSON is the
# control API's form.
act "set-setting transition" set-setting '{"key":"transition","value":{"kind":"wipe-up","duration":700}}' 's["settings"]["transition"] == {"kind":"wipe-up","duration":700}'
wait_idle
python3 - "$LIB" <<'PY' || fail "the transition setting was not persisted"
import json, sqlite3, sys
db = sqlite3.connect("file:%s?mode=ro" % sys.argv[1], uri=True)
row = db.execute("select value from settings where key = 'settings.transition'").fetchone()
sys.exit(0 if row and json.loads(row[0]) == {"kind": "wipe-up", "duration": 700} else 1)
PY
log "ok: the transition setting is persisted"
act "back to the default blend" transition '{"kind":"blend","duration":500}' 's["settings"]["transition"] == {"kind":"blend","duration":500}'
act "set-setting output.screen automatic" set-setting '{"key":"output.screen","value":-1}' 's["settings"]["outputScreen"] == -1 and s["outputOpen"]'
act "an unplugged display is rejected" set-setting '{"key":"output.screen","value":99}' 'not r["ok"]'
act "an unknown setting is rejected" set-setting '{"key":"bogus","value":1}' 'not r["ok"]'
# Logo image.
act "logo image" set-setting "{\"key\":\"output.logo\",\"value\":\"$MEDIA_DIR2/smoke-blue.png\"}" 's["settings"]["logo"]["kind"] == "image" and s["settings"]["logo"]["path"].endswith("smoke-blue.png")'
act "logo on" logo '' 's["mode"] == "logo"'
wait_idle
shot logo-blue-output output
pixel "the output shows the logo image" "$OUT/logo-blue-output.png" has 1f3f9f 0.25 0.25 0.75 0.75 10000
act "logo off" logo '' 's["mode"] == "normal"'
act "default logo" set-setting '{"key":"output.logo","value":""}' 'not s["settings"]["logo"].get("path", "").endswith("smoke-blue.png")'
# Interface color (View ▸ Interface Color) rebuilds the theme at runtime.
act "light interface" set-setting '{"key":"ui.interfaceColor","value":"light"}' 's["settings"]["interfaceColor"] == "light"'
wait_idle
shot light-main main
pixel "light interface has content" "$OUT/light-main.png" notblank
act "dark interface via the menu" menu '{"item":"interface-dark"}' 's["settings"]["interfaceColor"] == "dark"'
wait_idle
# Options dialog (Edit ▸ Options…): a child of the main window; hotkeys off;
# Cancel restores the settings from when it opened.
act "Options… opens the dialog" menu '{"item":"options"}' 's["dialog"]["settings"] and s["dialog"]["host"] == "main" and s["focus"] == "dialog"'
wait_idle
shot settings-dialog settings
pixel "settings dialog has content" "$OUT/settings-dialog.png" notblank
act "hotkeys are off while Options is open" key '{"name":"n"}' 'not d["consumed"]'
act "a change while the dialog is open applies at once" transition '{"kind":"iris","duration":900}' 's["settings"]["transition"] == {"kind":"iris","duration":900}'
act "Cancel restores the settings" dismiss '' 'not s["dialog"]["settings"] and s["settings"]["transition"] == {"kind":"blend","duration":500} and s["focus"] != "dialog"'
act "options opens again on the Transition page" options '{"page":"transition"}' 's["dialog"]["settings"]'
wait_idle
shot settings-transition settings
pixel "transition page has content" "$OUT/settings-transition.png" notblank
act "the Hotkeys page" options '{"page":"Hotkeys"}' 's["dialog"]["settings"]'
wait_idle
shot settings-hotkeys settings
pixel "hotkeys page has content" "$OUT/settings-hotkeys.png" notblank
act "an unknown page is rejected" options '{"page":"bogus"}' 'not r["ok"]'
act "change again" transition '{"kind":"cover-down"}' 's["settings"]["transition"]["kind"] == "cover-down"'
act "OK keeps the change" options '{"on":false}' 'not s["dialog"]["settings"] and s["settings"]["transition"]["kind"] == "cover-down"'
act "blend again" transition '{"kind":"blend","duration":500}' 's["settings"]["transition"] == {"kind":"blend","duration":500}'
# Crash safety (M6): an error dialog opens over the main window and leaves
# the projector output untouched.
act "blue live for the crash-safety check" go-live '' 's["live"]["title"] == "smoke-blue"'
wait_idle
shot output-before-error output
act "opening a corrupt schedule fails again" open-schedule "{\"path\":\"$OUT/corrupt.fbs\"}" 'not r["ok"]'
wait_idle
check "the error dialog is on the main window" "$LAST" 's["dialog"]["message"]["title"] == "Open Schedule" and s["dialog"]["host"] == "main" and s["outputOpen"]'
shot output-during-error output
pixel "the output is unchanged by the error dialog" "$OUT/output-before-error.png" same "$OUT/output-during-error.png"
pixel "the output still shows the slide" "$OUT/output-during-error.png" has 1f3f9f 0.25 0.25 0.75 0.75 10000
act "dismiss the error" dismiss '' 'not s["dialog"].get("message") and s["dialog"].get("host", "") == ""'
# Menus through the control API.
act "Help ▸ About" menu '{"item":"about"}' 's["dialog"]["message"]["title"] == "About FreeBeams" and s["dialog"]["host"] == "main"'
act "close About" dismiss '' 'not s["dialog"].get("message")'
act "unknown menu item is rejected" menu '{"item":"bogus"}' 'not r["ok"]'
act "File ▸ New Schedule (clean schedule, no question)" menu '{"item":"new-schedule"}' 's["schedule"]["title"] == "Untitled" and not s["schedule"]["dirty"] and s["focus"] != "dialog"'
act "open the saved schedule again" open-schedule "{\"path\":\"$SCHED\"}" 'not s["schedule"]["dirty"] and s["schedule"]["title"] == "smoke-schedule"'
act "Songs tab again" tab '{"tab":"songs"}' 's["tab"] == 0'

# ---- hotkeys editing -------------------------------------------------------------------------
# Options ▸ Hotkeys: Add Key… captures the next key press in the dialog
# (native-key into the settings window), refuses reserved keys, asks before
# taking another action's key, and the new key works at once in the main
# window. set-hotkey / reset-hotkeys are the control API's way in.
act "hotkeys: defaults in /state" hotkeys '' 's["settings"]["hotkeys"]["next"] == ["right","space","n"] and s["settings"]["hotkeys"]["black"] == ["ctrl+b"] and s["options"]["hotkeyCapture"] == ""'
act "Add Key… for Next" hotkey-capture '{"action":"next"}' 's["dialog"]["settings"] and s["options"]["hotkeyCapture"] == "next" and s["focus"] == "dialog"'
wait_idle
shot settings-hotkeys-capture settings
pixel "the hotkey editor has content" "$OUT/settings-hotkeys-capture.png" notblank
act "F is captured for Next" native-key '{"name":"f","window":"settings"}' 's["options"]["hotkeyCapture"] == "" and s["settings"]["hotkeys"]["next"] == ["right","space","n","f"] and s["dialog"]["settings"]'
act "Add Key… again" hotkey-capture '{"action":"next"}' 's["options"]["hotkeyCapture"] == "next"'
act "Cmd+Q is refused as a reserved key" native-key '{"name":"q","meta":true,"window":"settings"}' 's["options"]["hotkeyRefused"] and "reserved" in s["options"]["hotkeyStatus"] and "ctrl+q" not in str(s["settings"]["hotkeys"]) and s["dialog"]["settings"]'
wait_idle
shot settings-hotkeys-refused settings
act "Add Key… then Esc cancels the capture, not the dialog" hotkey-capture '{"action":"next"}' 's["options"]["hotkeyCapture"] == "next"'
act "Esc during a capture" native-key '{"name":"escape","window":"settings"}' 's["options"]["hotkeyCapture"] == "" and s["dialog"]["settings"] and s["settings"]["hotkeys"]["next"] == ["right","space","n","f"]'
act "Add Key… for Next (conflict)" hotkey-capture '{"action":"next"}' 's["options"]["hotkeyCapture"] == "next"'
act "Ctrl+B belongs to Black: asks first" native-key '{"name":"b","ctrl":true,"window":"settings"}' 's["dialog"]["confirm"] == "Reassign Key" and s["settings"]["hotkeys"]["black"] == ["ctrl+b"]'
wait_idle
shot settings-hotkeys-conflict dialog
act "cancel keeps Ctrl+B on Black" dismiss '' 's["dialog"].get("confirm", "") == "" and s["dialog"]["settings"] and s["settings"]["hotkeys"]["black"] == ["ctrl+b"]'
act "Add Key… for Logo (conflict)" hotkey-capture '{"action":"logo"}' 's["options"]["hotkeyCapture"] == "logo"'
act "F belongs to Next: asks first" native-key '{"name":"f","window":"settings"}' 's["dialog"]["confirm"] == "Reassign Key"'
act "reassign moves F to Logo" accept '' 's["settings"]["hotkeys"]["logo"] == ["ctrl+l","f"] and s["settings"]["hotkeys"]["next"] == ["right","space","n"]'
act "set-hotkey: Next on F and Right only" set-hotkey '{"action":"next","keys":["F","Right"],"reassign":true}' 's["settings"]["hotkeys"]["next"] == ["f","right"] and s["settings"]["hotkeys"]["logo"] == ["ctrl+l"]'
act "OK closes Options" options '{"on":false}' 'not s["dialog"]["settings"] and s["settings"]["hotkeys"]["next"] == ["f","right"]'
wait_idle
python3 - "$LIB" <<'PY2' || fail "the hotkeys were not persisted"
import json, sqlite3, sys
db = sqlite3.connect("file:%s?mode=ro" % sys.argv[1], uri=True)
row = db.execute("select value from settings where key = 'settings.hotkeys'").fetchone()
sys.exit(0 if row and json.loads(row[0])["next"] == ["f", "right"] else 1)
PY2
log "ok: the hotkeys are persisted"
act "search amazing (hotkeys)" search-songs '{"q":"amazing"}' 's["songs"]["results"] >= 1'
act "select Amazing Grace (hotkeys)" select-song '{"title":"Amazing Grace"}' 's["preview"]["title"] == "Amazing Grace"'
act "Amazing Grace live (hotkeys)" go-live '' 's["live"]["title"] == "Amazing Grace" and s["live"]["index"] == 0'
act "no focus (hotkeys)" focus '{"target":"none"}' 's["focus"] == "none"'
act "the new key F is Next (native)" native-key '{"name":"f"}' 's["live"]["index"] == 1'
act "Space is no longer Next (native)" native-key '{"name":"space"}' 's["live"]["index"] == 1'
act "set-hotkey refuses Cmd+Q" set-hotkey '{"action":"next","keys":["cmd+q"]}' 'not r["ok"] and "reserved" in r["error"]'
act "set-hotkey refuses a digit" set-hotkey '{"action":"next","keys":["5"]}' 'not r["ok"] and "reserved" in r["error"]'
act "set-hotkey reports a conflict" set-hotkey '{"action":"next","keys":["ctrl+l"]}' 'not r["ok"] and "already assigned to Logo" in r["error"]'
act "set-hotkey rejects an unknown action" set-hotkey '{"action":"spin","keys":["k"]}' 'not r["ok"]'
# A custom section key still takes its number (the digit window).
act "Verse on Q" set-hotkey '{"action":"jump-verse","keys":["q"]}' 's["settings"]["hotkeys"]["jump-verse"] == ["q"]'
act "slide jump on G" set-hotkey '{"action":"jump-slide","keys":["g"]}' 's["settings"]["hotkeys"]["jump-slide"] == ["g"]'
act "native Q (Verse)" native-key '{"name":"q"}' 's["live"]["label"] == "Verse 1"'
act "native 2 within 1s (Verse 2)" native-key '{"name":"2"}' 's["live"]["label"] == "Verse 2"'
act "native G (slide number)" native-key '{"name":"g"}' 's["live"]["label"] == "Verse 2"'
act "native 1 within 1s (slide 1)" native-key '{"name":"1"}' 's["live"]["index"] == 0'
# Cancel in Options restores the hotkeys from when it opened.
act "Options again" options '{"page":"hotkeys"}' 's["dialog"]["settings"]'
act "a change while Options is open" set-hotkey '{"action":"black","keys":["f2"]}' 's["settings"]["hotkeys"]["black"] == ["f2"]'
act "Cancel restores the hotkeys" dismiss '' 'not s["dialog"]["settings"] and s["settings"]["hotkeys"]["black"] == ["ctrl+b"]'
act "reset-hotkeys" reset-hotkeys '' 's["settings"]["hotkeys"]["next"] == ["right","space","n"] and s["settings"]["hotkeys"]["jump-verse"] == ["v"] and s["settings"]["hotkeys"]["jump-slide"] == ["s"]'
act "no focus (reset)" focus '{"target":"none"}' 's["focus"] == "none"'
act "Right is Next again (native)" native-key '{"name":"right"}' 's["live"]["index"] == 1'
act "F is no hotkey again (native)" native-key '{"name":"f"}' 's["live"]["index"] == 1'

# ---- default themes --------------------------------------------------------------------------
# Options ▸ Themes: a default theme per item type, the same state as the
# Themes tab's Set as Default. A change re-themes Preview when it shows a
# library song / passage with the old default, and new schedule items; items
# already in the schedule keep their snapshot, and Live is left alone.
act "Amazing Grace in Preview (themes)" select-song '{"title":"Amazing Grace"}' 's["preview"]["title"] == "Amazing Grace" and s["preview"]["theme"] == "Default Song"'
act "a schedule item before the change" add-to-schedule '{"source":"song"}' 's["schedule"]["items"][-1] == "Amazing Grace"'
DT_ROW=$(( $(python3 -c 'import json,sys; print(len(json.loads(sys.argv[1])["data"]["schedule"]["items"]))' "$LAST") - 1 ))
act "save a second song theme" save-theme "$(recolor "$(post get-theme '{"name":"Default Song"}')" "Smoke Default" 991a80 0)" '"Smoke Default" in s["themes"]["names"]'
act "Options ▸ Themes" options '{"page":"themes"}' 's["dialog"]["settings"]'
wait_idle
shot settings-themes settings
pixel "the Themes page has content" "$OUT/settings-themes.png" notblank
act "an item type without a default is refused" set-default-theme '{"kind":"media","themeName":"Smoke Default"}' 'not r["ok"]'
act "songs default (themeName)" set-default-theme '{"kind":"songs","themeName":"Smoke Default"}' 's["themes"]["songDefault"] == "Smoke Default" and s["themes"]["scriptureDefault"] == "Default Scripture"'
check "Preview follows the new default" "$LAST" 's["preview"]["title"] == "Amazing Grace" and s["preview"]["theme"] == "Smoke Default"'
check "Live keeps its theme" "$LAST" 's["live"]["title"] == "Amazing Grace" and s["live"]["theme"] == "Default Song"'
act "close Options (themes)" options '{"on":false}' 'not s["dialog"]["settings"]'
wait_idle
shot default-theme-main main
pixel "the preview monitor shows the new default" "$OUT/default-theme-main.png" has 991a80 0.24 0.37 0.55 0.66 2000
act "a new schedule item uses the new default" add-to-schedule '{"source":"song"}' 's["schedule"]["items"][-1] == "Amazing Grace"'
act "select the new item" select-schedule "{\"index\":$((DT_ROW + 1))}" 's["preview"]["theme"] == "Smoke Default"'
act "the earlier item keeps its snapshot" select-schedule "{\"index\":$DT_ROW}" 's["preview"]["theme"] == "Default Song"'
act "a passage in Preview (themes)" reference '{"text":"jn 3:16"}' 's["scripture"]["passage"] == "John 3:16" and s["preview"]["theme"] == "Default Scripture"'
act "scripture default" set-default-theme '{"kind":"scripture","themeName":"Smoke Default"}' 's["themes"]["scriptureDefault"] == "Smoke Default" and s["preview"]["theme"] == "Smoke Default"'
act "restore the scripture default" set-default-theme '{"kind":"scripture","themeName":"Default Scripture"}' 's["themes"]["scriptureDefault"] == "Default Scripture" and s["preview"]["theme"] == "Default Scripture"'
act "restore the songs default" set-default-theme '{"kind":"song","name":"Default Song"}' 's["themes"]["songDefault"] == "Default Song"'
act "remove the new item (themes)" remove-schedule "{\"index\":$((DT_ROW + 1))}" "len(s[\"schedule\"][\"items\"]) == $DT_ROW + 1"
act "remove the earlier item (themes)" remove-schedule "{\"index\":$DT_ROW}" "len(s[\"schedule\"][\"items\"]) == $DT_ROW"
act "delete the smoke default theme" delete-theme '{"name":"Smoke Default","confirm":true}' '"Smoke Default" not in s["themes"]["names"]'
act "save the schedule (default themes)" save-schedule '{}' 'not s["schedule"]["dirty"]'
act "Songs tab (default themes)" tab '{"tab":"songs"}' 's["tab"] == 0'

# ---- bundled Bibles and Import Bible File… (Phase 2) -----------------------------------------
# The KJV and ASV ship inside the app (internal/bibles/bundled, docs/BIBLES.md)
# and are installed on first launch, so a library without EasyWorship has
# them too; the startup wait above covered the install (/state.import.running
# stays true while it runs).
LAST="$(state)"
check "bundled KJV and ASV are installed" "$LAST" '"KJV" in s["bibles"] and "ASV" in s["bibles"] and s["bundled"]["done"] and not s["bundled"]["running"] and not s["bundled"].get("error") and s["bundled"]["version"] >= 1'
check "the bundled install accounted for both" "$LAST" 'sorted(s["bundled"]["installed"] + [x.split(" ")[0] for x in s["bundled"]["skipped"]]) == ["ASV", "KJV"]'
act "ASV wording" reference '{"text":"jn 3:16","bible":"ASV"}' 's["scripture"]["bible"] == "ASV" and "believeth on him" in s["preview"]["text"] and "eternal life" in s["preview"]["text"] and s["preview"]["footer"] == "John 3:16 (ASV)"'
act "KJV wording differs" reference '{"text":"jn 3:16","bible":"KJV"}' 's["scripture"]["bible"] == "KJV" and "believeth in him" in s["preview"]["text"] and s["preview"]["footer"] == "John 3:16 (KJV)"'
act "ASV Psalm 36:1 carries no leaked heading" reference '{"text":"ps 36:1","bible":"ASV"}' 's["preview"]["text"].startswith("The transgression of the wicked")'
act "ASV omits Acts 8:37 by design" reference '{"text":"acts 8:36-38","bible":"ASV"}' 's["preview"]["count"] == 2'
act "KJV has Acts 8:37" reference '{"text":"acts 8:36-38","bible":"KJV"}' 's["preview"]["count"] == 3'
# Import Bible File…: a fixture translation in the bible-translations layout
# (<ABBREV>/{old,new}/<Book>/<chapter>.json), written here with made-up and
# KJV wording only. The psalm carries the source's quirks (footnote marker,
# leaked heading, no-break space) to show the cleaning.
BIBLE_DIR="$OUT/bible-fixture/SMK"
rm -rf "$OUT/bible-fixture"
mkdir -p "$BIBLE_DIR/new/John" "$BIBLE_DIR/old/Psalm"
printf '%s' '{"16": "For God so loved the world (smoke fixture, verse sixteen).", "17": "For God sent not his Son into the world to condemn the world (smoke fixture)."}' >"$BIBLE_DIR/new/John/3.json"
printf '%s' '{"1": "[a]23 The Lord is my shepherd; I shall not want."}' >"$BIBLE_DIR/old/Psalm/23.json"
act "import-bible-file (confirmed) imports the folder" import-bible-file "{\"path\":\"$BIBLE_DIR\",\"confirm\":true}" '"SMK" in s["bibles"] and s["scripture"]["status"] == "Imported SMK: 3 verses" and not s["scripture"]["error"]'
act "the imported translation works, with cleaned text" reference '{"text":"ps 23:1","bible":"SMK"}' 's["scripture"]["bible"] == "SMK" and s["preview"]["text"] == "The Lord is my shepherd; I shall not want." and s["preview"]["footer"] == "Psalms 23:1 (SMK)"'
act "a chapter file of the folder is the same translation: a duplicate" import-bible-file "{\"path\":\"$BIBLE_DIR/new/John/3.json\",\"confirm\":true}" 'not r["ok"] and "SMK is already installed" in r["error"]'
wait_idle
check "the duplicate is reported, not imported twice" "$LAST" 's["dialog"]["message"]["title"] == "Import Bible File" and "already installed" in s["dialog"]["message"]["message"] and s["bibles"].count("SMK") == 1 and s["scripture"]["error"]'
shot import-bible-file-duplicate message
act "dismiss the duplicate message" dismiss '' 'not s["dialog"].get("message")'
# A single-file Bible JSON goes through the question first.
BIBLE_JSON="$OUT/smoke-bible.json"
printf '%s' '{"abbrev":"SMK2","name":"Smoke Bible","language":"en","books":{"John":{"3":{"16":"Smoke fixture, single file, verse sixteen."}},"Jude":{"1":{"3":"Smoke fixture, Jude verse three."}}}}' >"$BIBLE_JSON"
act "import-bible-file asks first" import-bible-file "{\"path\":\"$BIBLE_JSON\"}" 's["dialog"]["confirm"] == "Import Bible File" and "SMK2" not in s["bibles"]'
shot import-bible-file-confirm dialog
act "cancel the question" dismiss '' 's["focus"] != "dialog" and "SMK2" not in s["bibles"]'
act "ask again" import-bible-file "{\"path\":\"$BIBLE_JSON\"}" 's["dialog"]["confirm"] == "Import Bible File"'
act "OK imports it" accept '' 'r["ok"]'
wait_idle
check "SMK2 is installed after OK" "$LAST" '"SMK2" in s["bibles"] and s["scripture"]["status"] == "Imported SMK2: 2 verses"'
act "a single-chapter book in the imported file" reference '{"text":"jude 3","bible":"SMK2"}' 's["scripture"]["bible"] == "SMK2" and s["preview"]["text"] == "Smoke fixture, Jude verse three." and s["preview"]["footer"] == "Jude 1:3 (SMK2)"'
# The array layout (github.com/thiagobodruk/bible: 66 books in order, verses
# positional, no header): the abbreviation comes from the file name and the
# scrape's artefacts are cleaned (cross-references, "( Elohim)", "Lordis").
BIBLE_ARRAY="$OUT/en_smk3.json"
python3 - "$BIBLE_ARRAY" <<'PY'
import json, sys
books = [{"abbrev": "xx", "name": "Livro", "chapters": []} for _ in range(66)]
books[0]["chapters"] = [["In the beginning God ( Elohim) created the heavens and the earth. [Heb 11:3]"]]
books[18]["chapters"] = [[] for _ in range(22)] + [["The Lordis my shepherd [to feed me], [Ezek 34:11-31] I shall not want."]]
json.dump(books, open(sys.argv[1], "w"))
PY
act "import-bible-file reads the array layout" import-bible-file "{\"path\":\"$BIBLE_ARRAY\",\"confirm\":true}" '"SMK3" in s["bibles"] and s["scripture"]["status"] == "Imported SMK3: 2 verses"'
act "books resolve by position, with the cleaned text" reference '{"text":"gen 1:1","bible":"SMK3"}' 's["preview"]["text"] == "In the beginning God (Elohim) created the heavens and the earth." and s["preview"]["footer"] == "Genesis 1:1 (SMK3)"'
act "amplification brackets stay, the cross-reference goes" reference '{"text":"ps 23:1","bible":"SMK3"}' 's["preview"]["text"] == "The Lord is my shepherd [to feed me], I shall not want."'
act "a missing file is an error" import-bible-file '{"path":"/nonexistent/bible.json","confirm":true}' 'not r["ok"]'
wait_idle
check "the read error is in the message dialog" "$LAST" 's["dialog"]["message"]["title"] == "Import Bible File" and s["scripture"]["error"]'
act "dismiss the read error" dismiss '' 'not s["dialog"].get("message")'
act "import-bible-file needs a path" import-bible-file '{}' 'not r["ok"]'
act "back to KJV (bibles)" reference '{"text":"jn 3:16","bible":"KJV"}' 's["scripture"]["bible"] == "KJV"'
act "Songs tab again (bibles)" tab '{"tab":"songs"}' 's["tab"] == 0'
# ---- search: ranking, lyrics hint, harmless punctuation, consistent libraries ------------
# In the real library "Alas And Did My Savior Bleed" has "amazing" in its lyrics and sorted
# above "Amazing Grace" while results were in title order.
act "search is case-insensitive and ranks the title match first" search-songs '{"q":"AMAZING"}' 's["songs"]["results"] >= 2 and s["songs"]["first"] == "Amazing Grace" and s["songs"]["lyricsMatches"] >= 1'
act "a prefix finds the same song first" search-songs '{"q":"amaz"}' 's["songs"]["first"] == "Amazing Grace" and s["songs"]["results"] >= 2'
act "title first, the lyric matches counted in the footer" search-songs '{"q":"Amazing Grace"}' 's["songs"]["first"] == "Amazing Grace" and s["songs"]["lyricsMatches"] == s["songs"]["results"] - 1'
act "a lyrics-only word finds the song and says so" search-songs '{"q":"wretch"}' 's["songs"]["results"] == 1 and s["songs"]["first"] == "Amazing Grace" and s["songs"]["lyricsMatches"] == 1 and "1 matched in lyrics" in s["songs"]["countText"]'
wait_idle
shot search-lyrics main
act "two terms across title and lyrics" search-songs '{"q":"grace sweet"}' 's["songs"]["results"] >= 2 and s["songs"]["first"] == "Amazing Grace" and s["songs"]["lyricsMatches"] >= 1'
act "a quoted phrase" search-songs '{"q":"\"how sweet the sound\""}' 's["songs"]["results"] >= 1 and s["songs"]["first"] == "Amazing Grace"'
act "two separate words (i, ll) for comparison" search-songs '{"q":"i ll"}' 's["songs"]["results"] >= 1'
LOOSE="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["data"]["songs"]["results"])' "$LAST")"
act "a contraction is one phrase (fewer songs than i + ll)" search-songs '{"q":"I'"'"'ll"}' "s[\"songs\"][\"results\"] >= 1 and s[\"songs\"][\"results\"] < $LOOSE"
act "punctuation and FTS syntax are harmless" search-songs '{"q":"Lord'"'"'s \"grace (sweet)* -NEAR: ^"}' 'r["ok"] and s["songs"]["query"].startswith("Lord")'
act "only punctuation lists the library" search-songs '{"q":"*\"(-:^"}' 's["songs"]["results"] == s["songs"]["count"] and s["songs"]["lyricsMatches"] == 0'
act "search cleared" search-songs '{"q":""}' 's["songs"]["results"] == s["songs"]["count"] and s["songs"]["lyricsMatches"] == 0 and s["songs"]["first"] != ""'
# The media and presentation searches fold case and diacritics the same way.
act "media search, lower case" search-media '{"q":"still"}' 's["media"]["query"] == "still"'
STILL="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["data"]["media"]["results"])' "$LAST")"
act "media search ignores case" search-media '{"q":"STILL"}' "s[\"media\"][\"results\"] == $STILL"
act "all media again" search-media '{"q":""}' 's["media"]["results"] == s["media"]["count"]'
act "presentation search ignores case" search-presentations '{"q":"ZZZ-NOTHING"}' 's["presentations"]["results"] == 0'
act "all presentations again" search-presentations '{"q":""}' "s[\"presentations\"][\"results\"] == $PRES_COUNT"

# ---- playback ----------------------------------------------------------------------------
# Videos and audio play through the media player (docs/MEDIA.md): AVFoundation
# on macOS, the ffmpeg player on Windows / Linux, and here too with
# FREEBEAMS_PLAYER=ffmpeg test/smoke.sh. A 4 s solid #2a7f62 clip with a
# 440 Hz tone and a 3 s WAV, driven through the transport actions and
# /state.playback. Skipped where there is no media player (the Theora
# fallback plays nothing without a transcode) or ffmpeg (which makes the
# clip) is absent.
# pb_wait DESCRIPTION EXPR: polls /state (up to 10 s) until EXPR holds, then checks it.
pb_wait() {
	for _ in $(seq 1 40); do
		LAST="$(state)"
		python3 -c 'import json,sys; s=json.loads(sys.argv[1])["data"]; sys.exit(0 if eval(sys.argv[2], {"s": s}) else 1)' "$LAST" "$2" 2>/dev/null && break
		sleep 0.25
	done
	check "$1" "$LAST" "$2"
}
LAST="$(state)"
check "playback state is present" "$LAST" '"live" in s["playback"] and "preview" in s["playback"] and not s["playback"]["live"]["media"]'
PB_NATIVE="$(field "$LAST" playback.live.native)" # before the refusal below leaves an error reply in LAST
PB_PLAYER="$(field "$LAST" playback.live.player)"
log "media player: $PB_PLAYER"
if [ -n "${FREEBEAMS_PLAYER:-}" ]; then
	check "FREEBEAMS_PLAYER=$FREEBEAMS_PLAYER selects that player" "$LAST" "s[\"playback\"][\"live\"][\"player\"] == \"$FREEBEAMS_PLAYER\" and s[\"media\"][\"player\"] == \"$FREEBEAMS_PLAYER\""
fi
act "a monitor without media refuses play" play '' 'not r["ok"]'
if [ "$FFMPEG" = "True" ] && [ "$PB_NATIVE" = "True" ]; then
	PB_DIR="$OUT/playback"
	mkdir -p "$PB_DIR"
	ffmpeg -v error -y -f lavfi -i "color=c=0x2a7f62:s=320x180:r=30:d=4" -f lavfi -i "sine=frequency=440:duration=4" \
		-vf "scale=out_color_matrix=bt709:out_range=tv,format=yuv420p" -colorspace bt709 -color_primaries bt709 -color_trc bt709 -color_range tv \
		-c:v libx264 -preset veryfast -c:a aac -shortest "$PB_DIR/smoke-clip.mp4" || fail "ffmpeg could not make the test clip"
	python3 - "$PB_DIR/smoke-tone.wav" <<'PY'
import math, struct, sys, wave
w = wave.open(sys.argv[1], "wb")
w.setnchannels(1)
w.setsampwidth(2)
w.setframerate(22050)
w.writeframes(b"".join(struct.pack("<h", int(8000 * math.sin(2 * math.pi * 440 * i / 22050))) for i in range(22050 * 3)))
w.close()
PY
	act "Media tab (playback)" tab '{"tab":"media"}' 's["tab"] == 2'
	PB_COUNT="$(field "$LAST" media.count)"
	PB_VIDEOS="$(field "$LAST" media.videos)"
	PB_AUDIO="$(field "$LAST" media.audio)"
	act "add the test clip and tone" add-media "{\"path\":\"$PB_DIR\"}" "s[\"media\"][\"count\"] == $PB_COUNT + 2 and s[\"media\"][\"videos\"] == $PB_VIDEOS + 1 and s[\"media\"][\"audio\"] == $PB_AUDIO + 1 and s[\"media\"][\"native\"] and s[\"media\"][\"playable\"] == s[\"media\"][\"videos\"]"
	check "the footer counts the audio file" "$LAST" '"1 audio" in s["media"]["countText"] or "2 audio" in s["media"]["countText"]'
	act "the audio collection" search-media '{"q":"","kind":"audio"}' 's["media"]["kind"] == "audio" and s["media"]["results"] == s["media"]["audio"] and s["media"]["results"] >= 1'
	act "all media (playback)" search-media '{"q":""}' 's["media"]["kind"] == "" and s["media"]["results"] == s["media"]["count"]'
	act "select the clip: the preview waits, paused at 0" select-media '{"title":"smoke-clip"}' 's["media"]["current"] == "smoke-clip" and s["playback"]["preview"]["media"] and not s["playback"]["preview"]["playing"] and s["playback"]["preview"]["position"] == 0 and s["playback"]["preview"]["title"] == "smoke-clip" and s["playback"]["preview"]["loop"] and not s["playback"]["live"]["media"]'
	pb_wait "the preview player opened the clip (video and audio)" 's["playback"]["preview"]["duration"] > 3.9 and s["playback"]["preview"]["hasVideo"] and s["playback"]["preview"]["hasAudio"] and not s["playback"]["preview"].get("failed")'
	check "the preview is still paused at 0" "$LAST" 'not s["playback"]["preview"]["playing"] and s["playback"]["preview"]["position"] == 0'
	act "the clip goes live and plays (looping, full volume)" go-live '' 's["live"]["title"] == "smoke-clip" and s["playback"]["live"]["media"] and s["playback"]["live"]["playing"] and s["playback"]["live"]["loop"] and s["playback"]["live"]["volume"] == 1 and s["playback"]["live"]["title"] == "smoke-clip"'
	pb_wait "the live player plays the clip" 's["playback"]["live"]["position"] > 0.2 and s["playback"]["live"]["duration"] > 3.9 and s["playback"]["live"]["hasVideo"] and s["playback"]["live"]["hasAudio"]'
	PB_POS="$(field "$LAST" playback.live.position)"
	sleep 0.5
	LAST="$(state)"
	check "the live position advances" "$LAST" "s[\"playback\"][\"live\"][\"position\"] > $PB_POS"
	check "the preview player is untouched by live" "$LAST" 'not s["playback"]["preview"]["playing"] and s["playback"]["preview"]["position"] == 0'
	wait_idle
	shot playback-output output
	shot playback-main main
	pixel "the output shows the clip" "$OUT/playback-output.png" has 2a7f62 0.25 0.25 0.75 0.75 10000
	pixel "the live monitor shows the clip" "$OUT/playback-main.png" has 2a7f62 0.71 0.40 0.89 0.55 2000
	pixel "the preview monitor shows the clip's first frame" "$OUT/playback-main.png" has 2a7f62 0.31 0.40 0.49 0.55 2000
	act "pause" pause '' 'not s["playback"]["live"]["playing"]'
	sleep 0.3
	LAST="$(state)"
	PB_POS="$(field "$LAST" playback.live.position)"
	sleep 0.4
	LAST="$(state)"
	check "the position holds while paused" "$LAST" "s[\"playback\"][\"live\"][\"position\"] == $PB_POS and not s[\"playback\"][\"live\"][\"playing\"]"
	act "seek to 1 s" seek '{"seconds":1.0}' 'r["ok"]'
	pb_wait "the position follows the seek" 'abs(s["playback"]["live"]["position"] - 1.0) < 0.15 and not s["playback"]["live"]["playing"]'
	act "loop off" loop '{"on":false}' 'not s["playback"]["live"]["loop"]'
	act "seek near the end" seek '{"seconds":3.5}' 'r["ok"]'
	act "play" play '' 's["playback"]["live"]["playing"]'
	pb_wait "the clip stops at its end with loop off" 'not s["playback"]["live"]["playing"] and s["playback"]["live"]["position"] > 3.8'
	act "toggle-play restarts a clip that ended" toggle-play '' 's["playback"]["live"]["playing"]'
	act "loop on again" loop '{"on":true}' 's["playback"]["live"]["loop"]'
	act "black stops the media" black '' 's["mode"] == "black" and not s["playback"]["live"]["media"]'
	wait_idle # the fade to black ends: the player closes
	act "black off brings the clip back" black '' 's["mode"] == "normal" and s["playback"]["live"]["media"] and s["playback"]["live"]["playing"] and s["playback"]["live"]["loop"]'
	pb_wait "the clip plays again from the start" 's["playback"]["live"]["position"] > 0.1 and s["playback"]["live"]["position"] < 2.5 and s["playback"]["live"]["hasVideo"]'
	act "a seek without seconds is refused" seek '{}' 'not r["ok"]'
	act "a bad target is refused" seek '{"seconds":1,"target":"output"}' 'not r["ok"]'
	act "the preview's volume is its own" volume '{"value":0.25,"target":"preview"}' 's["playback"]["preview"]["volume"] == 0.25 and s["playback"]["live"]["volume"] == 1'
	act "play on the preview target (a second decoder)" play '{"target":"preview"}' 's["playback"]["preview"]["playing"] and s["playback"]["live"]["playing"]'
	pb_wait "the preview plays while live plays" 's["playback"]["preview"]["position"] > 0.1 and s["playback"]["live"]["playing"]'
	# The tone: sound only, over a black slide; the live volume carries over.
	act "select the tone" select-media '{"title":"smoke-tone"}' 's["media"]["current"] == "smoke-tone" and s["playback"]["preview"]["title"] == "smoke-tone" and not s["playback"]["preview"]["playing"]'
	act "the tone goes live and plays once" go-live '' 's["playback"]["live"]["media"] and s["playback"]["live"]["playing"] and not s["playback"]["live"]["loop"] and s["playback"]["live"]["title"] == "smoke-tone"'
	pb_wait "the live player plays the tone (audio only)" 's["playback"]["live"]["position"] > 0.2 and s["playback"]["live"]["duration"] > 2.9 and s["playback"]["live"]["hasAudio"] and not s["playback"]["live"]["hasVideo"]'
	act "volume" volume '{"value":0.5}' 's["playback"]["live"]["volume"] == 0.5'
	act "a volume that is not a number is refused" volume '{"value":"loud"}' 'not r["ok"]'
	# The audio clock: it runs in real time, follows a seek while playing
	# (the ffmpeg player restarts its decoder and drops the queued sound) and
	# stops at the end. The tone lasts 3 s: the screenshots come after it, or
	# it has ended before the seek (which then leaves it paused at 1.5).
	LAST="$(state)"
	PB_POS="$(field "$LAST" playback.live.position)"
	sleep 0.5
	LAST="$(state)"
	check "the tone's position advances in real time" "$LAST" "s[\"playback\"][\"live\"][\"playing\"] and s[\"playback\"][\"live\"][\"position\"] > $PB_POS + 0.3 or s[\"playback\"][\"live\"][\"position\"] > 2.5"
	act "seek the tone while it plays" seek '{"seconds":1.5}' 'r["ok"]'
	pb_wait "the tone plays on from the seek" 's["playback"]["live"]["playing"] and 1.55 < s["playback"]["live"]["position"] < 2.8'
	pb_wait "the tone stops at its end" 'not s["playback"]["live"]["playing"] and s["playback"]["live"]["position"] > 2.9'
	wait_idle
	shot playback-audio-output output
	shot playback-audio-main main
	pixel "audio shows a black output" "$OUT/playback-audio-output.png" black
	if [ "$PB_PLAYER" = "ffmpeg" ]; then
		grep -q 'the sound cannot play' "$LOG" && fail "the ffmpeg player could not decode a sound track"
		log "ok: the ffmpeg player decoded every sound track"
	fi
	act "remove the clip" remove-media '{"title":"smoke-clip","confirm":true}' "s[\"media\"][\"count\"] == $PB_COUNT + 1"
	act "remove the tone" remove-media '{"title":"smoke-tone","confirm":true}' "s[\"media\"][\"count\"] == $PB_COUNT and s[\"media\"][\"audio\"] == $PB_AUDIO"
	check "the tone keeps playing from the live deck" "$LAST" 's["live"]["title"] == "smoke-tone" and s["playback"]["live"]["media"]'
	act "Songs tab (playback)" tab '{"tab":"songs"}' 's["tab"] == 0'
else
	log "playback: skipped (ffmpeg=$FFMPEG native=$PB_NATIVE player=$PB_PLAYER)"
fi

# ---- song themes -----------------------------------------------------------------------------
# A song's own theme (Song.ThemeID) wins over the songs default, from save-song (the editor's
# OK path), the editor's Theme picker and the Songs list's Theme ▸ submenu. The theme has a
# solid orange background (#c46a1f) the default song theme never shows. Schedule items keep
# their snapshot; deleting the theme sends the song back to the default.
act "Songs tab (song themes)" tab '{"tab":"songs"}' 's["tab"] == 0'
SONG_THEME_BASE="$(post get-theme '{"name":"Default Song"}')"
act "save a theme for a song" save-theme "$(recolor "$SONG_THEME_BASE" "Smoke Song Theme" c46a1f 0)" '"Smoke Song Theme" in s["themes"]["names"] and s["themes"]["songDefault"] == "Default Song"'
act "save-song with a theme (editor OK path)" save-song '{"title":"Smoke Theme Song","author":"FreeBeams","themeName":"Smoke Song Theme","lyrics":"[Verse 1]\nthemed line one\n\n[Chorus]\nthemed chorus"}' 's["songs"]["current"] == "Smoke Theme Song" and s["songs"]["theme"] == "Smoke Song Theme" and s["preview"]["title"] == "Smoke Theme Song" and s["preview"]["theme"] == "Smoke Song Theme"'
act "save-song refuses an unknown theme" save-song '{"title":"Smoke Theme Song","themeName":"No Such Theme"}' 'not r["ok"] and "no theme matches" in r["error"]'
wait_idle
act "find the themed song" search-songs '{"q":"smoke theme song"}' 's["songs"]["results"] == 1'
act "select the themed song" select-song '{"title":"Smoke Theme Song"}' 's["songs"]["theme"] == "Smoke Song Theme" and s["preview"]["theme"] == "Smoke Song Theme"'
act "the themed song goes live" go-live '' 's["live"]["title"] == "Smoke Theme Song" and s["live"]["theme"] == "Smoke Song Theme"'
wait_idle
shot song-theme-output output
pixel "the output shows the song's own theme" "$OUT/song-theme-output.png" has c46a1f 0.0 0.0 0.2 0.08 5000
# The editor shows the song's theme; a pick in its Theme picker is saved by OK.
act "open the song editor (song themes)" editor '{"mode":"edit"}' 's["dialog"]["editor"] and s["dialog"]["songEditorTheme"] == "Smoke Song Theme"'
act "pick (Default) in the editor's Theme picker" song-editor-theme '{"themeName":"(Default)"}' 's["dialog"]["songEditorTheme"] == "(Default)"'
act "pick the theme again" song-editor-theme '{"themeName":"Smoke Song Theme"}' 's["dialog"]["songEditorTheme"] == "Smoke Song Theme"'
wait_idle
shot song-editor-theme editor
pixel "the song editor with its Theme picker has content" "$OUT/song-editor-theme.png" notblank
act "pick (Default) and press OK" song-editor-theme '{"themeName":"(Default)"}' 's["dialog"]["songEditorTheme"] == "(Default)"'
act "OK saves the song" accept '' 'r["ok"]'
wait_idle
check "the song uses the default theme again" "$LAST" 'not s["dialog"]["editor"] and s["songs"]["current"] == "Smoke Theme Song" and s["songs"]["theme"] == "" and s["preview"]["theme"] == "Default Song"'
act "go live with the default theme" go-live '' 's["live"]["title"] == "Smoke Theme Song" and s["live"]["theme"] == "Default Song"'
wait_idle
shot song-default-output output
pixel "the output background changed back" "$OUT/song-default-output.png" lacks c46a1f 0.0 0.0 0.2 0.08 50
# The Songs list's Theme ▸ submenu: (Default) and every theme, the song's own checked.
act "find the themed song again" search-songs '{"q":"smoke theme song"}' 's["songs"]["results"] == 1'
act "select it in the list" select-song '{"title":"Smoke Theme Song"}' 's["songs"]["selected"] == 0'
wait_idle
act "the Theme submenu lists (Default) and every theme" song-menu '{"mode":"theme"}' 'd["entries"][0] == "(Default)" and "Smoke Song Theme" in d["entries"] and "Default Song" in d["entries"] and d["checked"] == 0'
wait_idle
shot song-menu song-menu
shot song-theme-menu song-theme-menu
pixel "the song context menu has content" "$OUT/song-menu.png" notblank
pixel "the Theme submenu has content" "$OUT/song-theme-menu.png" notblank
act "close the song menu" song-menu '{"mode":"close"}' 'r["ok"]'
act "Theme ▸ sets the song's theme from the list" set-song-theme '{"title":"Smoke Theme Song","themeName":"Smoke Song Theme"}' 's["songs"]["theme"] == "Smoke Song Theme" and s["preview"]["title"] == "Smoke Theme Song" and s["preview"]["theme"] == "Smoke Song Theme" and s["live"]["theme"] == "Default Song"'
act "set-song-theme refuses an unknown theme" set-song-theme '{"title":"Smoke Theme Song","themeName":"No Such Theme"}' 'not r["ok"]'
wait_idle
act "the submenu checks the song's theme" song-menu '{"mode":"open"}' 'd["entries"][d["checked"]] == "Smoke Song Theme"'
act "close the song menu again" song-menu '{"mode":"close"}' 'r["ok"]'
act "the re-themed song goes live" go-live '' 's["live"]["theme"] == "Smoke Song Theme"'
wait_idle
shot song-theme-output-2 output
pixel "the output follows the list's Theme ▸ choice" "$OUT/song-theme-output-2.png" has c46a1f 0.0 0.0 0.2 0.08 5000
# Schedule items keep their snapshot; adding the song again uses its new theme.
act "add the themed song to the schedule" add-to-schedule '{"source":"song"}' 's["schedule"]["items"][-1] == "Smoke Theme Song"'
SONG_THEME_ROW=$(( $(python3 -c 'import json,sys; print(len(json.loads(sys.argv[1])["data"]["schedule"]["items"]))' "$LAST") - 1 ))
act "back to (Default) from the list" set-song-theme '{"title":"Smoke Theme Song","themeName":"(Default)"}' 's["songs"]["theme"] == "" and s["preview"]["theme"] == "Default Song"'
act "the schedule item keeps its theme" select-schedule "{\"index\":$SONG_THEME_ROW}" 's["preview"]["title"] == "Smoke Theme Song" and s["preview"]["theme"] == "Smoke Song Theme"'
act "add the song again" add-to-schedule '{"source":"song"}' "len(s[\"schedule\"][\"items\"]) == $SONG_THEME_ROW + 2"
act "the new item uses the song's new theme" select-schedule "{\"index\":$((SONG_THEME_ROW + 1))}" 's["preview"]["title"] == "Smoke Theme Song" and s["preview"]["theme"] == "Default Song"'
act "remove the second item (song themes)" remove-schedule "{\"index\":$((SONG_THEME_ROW + 1))}" "len(s[\"schedule\"][\"items\"]) == $SONG_THEME_ROW + 1"
act "remove the first item (song themes)" remove-schedule "{\"index\":$SONG_THEME_ROW}" "len(s[\"schedule\"][\"items\"]) == $SONG_THEME_ROW"
act "save the schedule (song themes)" save-schedule '{}' 'not s["schedule"]["dirty"]'
# Deleting the theme sends the song back to the default: no dangling id.
act "theme the song once more" set-song-theme '{"title":"Smoke Theme Song","themeName":"Smoke Song Theme"}' 's["songs"]["theme"] == "Smoke Song Theme"'
act "select the themed song (before the delete)" select-song '{"title":"Smoke Theme Song"}' 's["preview"]["theme"] == "Smoke Song Theme"'
act "delete the song's theme" delete-theme '{"name":"Smoke Song Theme","confirm":true}' 's["themes"]["count"] == 2 and "Smoke Song Theme" not in s["themes"]["names"]'
check "the song falls back to the default" "$LAST" 's["songs"]["theme"] == ""'
act "a new deck of the song uses the default" select-song '{"title":"Smoke Theme Song"}' 's["preview"]["title"] == "Smoke Theme Song" and s["preview"]["theme"] == "Default Song"'
act "the editor shows (Default) after the delete" editor '{"mode":"edit"}' 's["dialog"]["songEditorTheme"] == "(Default)"'
act "close the editor (song themes)" editor '{"mode":"close"}' 'not s["dialog"]["editor"]'
act "the fallback goes live" go-live '' 's["live"]["title"] == "Smoke Theme Song" and s["live"]["theme"] == "Default Song"'
wait_idle
shot song-fallback-output output
pixel "the output no longer shows the deleted theme" "$OUT/song-fallback-output.png" lacks c46a1f 0.0 0.0 0.2 0.08 50
act "the submenu checks (Default) after the delete" song-menu '{"mode":"open"}' 'd["checked"] == 0'
act "close the song menu (delete)" song-menu '{"mode":"close"}' 'r["ok"]'
act "delete the themed song" delete-song '{"index":0,"confirm":true}' 'r["ok"]'
wait_idle
act "clear the song search (song themes)" search-songs '{"q":""}' 's["songs"]["results"] == s["songs"]["count"]'

# ---- stage display (v0.2) ----------------------------------------------------------------------
# The stage display (EasyWorship's Foldback output): the live slide's text in white, the next slide
# in amber (#ffd15c), item title, clock, service timer, the output-mode badge (BLACK #d32f2f) and the
# operator's stage message on a teal band (#00796b), white on black. On this machine the TV (when
# connected) carries the output, so Automatic puts the stage in a window on the primary display.
act "Songs tab (stage)" tab '{"tab":"songs"}' 's["tab"] == 0'
act "save a first stage song" save-song '{"title":"Smoke Stage One","author":"FreeBeams","lyrics":"[Verse 1]\nstage one verse\n\n[Chorus]\nstage one chorus"}' 's["songs"]["current"] == "Smoke Stage One"'
act "save a second stage song" save-song '{"title":"Smoke Stage Two","author":"FreeBeams","lyrics":"[Verse 1]\nstage two verse"}' 's["songs"]["current"] == "Smoke Stage Two"'
wait_idle
STAGE_ROW=$(python3 -c 'import json,sys; print(len(json.loads(sys.argv[1])["data"]["schedule"]["items"]))' "$(state)")
act "find the stage songs" search-songs '{"q":"smoke stage"}' 's["songs"]["results"] == 2'
act "select the first stage song" select-song '{"title":"Smoke Stage One"}' 's["preview"]["title"] == "Smoke Stage One"'
act "add it to the schedule (stage)" add-to-schedule '{"source":"song"}' 's["schedule"]["items"][-1] == "Smoke Stage One"'
act "select the second stage song" select-song '{"title":"Smoke Stage Two"}' 's["preview"]["title"] == "Smoke Stage Two"'
act "add it to the schedule too (stage)" add-to-schedule '{"source":"song"}' 's["schedule"]["items"][-1] == "Smoke Stage Two"'
LAST="$(state)"
check "the stage display is off by default" "$LAST" 'not s["stage"]["settings"]["enabled"] and not s["stage"]["open"] and s["stage"]["screen"] == -1 and s["stage"]["monitor"] == "live"'
act "a stage screenshot needs the stage display" stage '{"on":false}' 'not s["stage"]["open"]'
curl -sS --max-time 60 -H "Authorization: Bearer $KEY" "$BASE/screenshot?window=stage" | grep -q 'not open' || fail "a closed stage display's screenshot is not an error"
log "ok: a closed stage display's screenshot is an error"
act "the first stage item goes live" select-schedule "{\"index\":$STAGE_ROW}" 's["preview"]["title"] == "Smoke Stage One"'
act "go live (stage)" go-live '' 's["live"]["title"] == "Smoke Stage One" and s["live"]["label"] == "Verse 1"'
act "Live ▸ Stage Display turns it on" menu '{"item":"stage-display"}' 's["stage"]["settings"]["enabled"] and s["stage"]["open"] and s["stage"]["screen"] >= 0 and not s["stage"]["conflict"] and (s["screens"] > 2 or not s["stage"]["fullscreen"])'
check "the stage shows the live slide and the next one" "$LAST" 's["stage"]["title"] == "Smoke Stage One" and s["stage"]["label"] == "Verse 1" and s["stage"]["current"] == "stage one verse" and s["stage"]["next"] == "stage one chorus" and s["stage"]["nextHeader"] == "NEXT · Chorus" and s["stage"]["badge"] == ""'
check "the stage has a clock and a stopped timer" "$LAST" 's["stage"]["clock"].endswith(("AM","PM")) and s["stage"]["timer"] == "00:00" and not s["stage"]["timerRunning"]'
wait_idle
check "the windowed stage gives the keyboard back to the operator window" "$LAST" 'not s["stage"]["focused"]'
shot stage-display stage
pixel "the stage is black" "$OUT/stage-display.png" has 000000 0.0 0.0 1.0 1.0 200000
pixel "the stage shows the live text in white" "$OUT/stage-display.png" has ffffff 0.1 0.15 0.9 0.55 500
pixel "the stage shows the next slide in amber" "$OUT/stage-display.png" has ffd15c 0.1 0.6 0.9 1.0 200
# Across the item boundary the next slide is the next schedule item's first.
act "End on the stage item (last slide)" key '{"name":"End"}' 's["live"]["label"] == "Chorus"'
check "the stage announces the next item" "$LAST" 's["stage"]["nextTitle"] == "Smoke Stage Two" and s["stage"]["next"] == "stage two verse" and s["stage"]["nextHeader"].startswith("NEXT ITEM · Smoke Stage Two")'
# Keys typed while the windowed stage display has the focus are the hotkeys, as on the output.
act "Left in the stage window is Prev" native-key '{"name":"left","window":"stage"}' 's["live"]["label"] == "Verse 1" and s["stage"]["label"] == "Verse 1"'
act "Right in the stage window is Next" native-key '{"name":"right","window":"stage"}' 's["live"]["label"] == "Chorus"'
# Black: the room sees black, the musicians keep the text and see a badge.
act "black (stage)" black '' 's["mode"] == "black" and s["stage"]["badge"] == "BLACK" and s["stage"]["current"] == "stage one chorus"'
wait_idle
shot stage-black stage
pixel "the stage shows the BLACK badge" "$OUT/stage-black.png" has d32f2f 0.5 0.0 1.0 0.15 500
pixel "the stage keeps the text during Black" "$OUT/stage-black.png" has ffffff 0.1 0.15 0.9 0.55 300
act "Live ▸ Black Output turns it off" menu '{"item":"black"}' 's["mode"] == "normal" and s["stage"]["badge"] == ""'
act "logo (stage)" logo '' 's["stage"]["badge"] == "LOGO"'
act "logo off (stage)" logo '' 's["mode"] == "normal" and s["stage"]["badge"] == ""'
# The service timer.
act "start the service timer" stage-timer '{"action":"start"}' 's["stage"]["timerRunning"]'
sleep 2.2
LAST="$(state)"
check "the timer runs" "$LAST" 's["stage"]["timerSeconds"] >= 2 and s["stage"]["timer"] != "00:00" and s["stage"]["timerRunning"]'
act "stop the timer" stage-timer '{"action":"stop"}' 'not s["stage"]["timerRunning"] and s["stage"]["timerSeconds"] >= 2'
STAGE_T="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["data"]["stage"]["timerSeconds"])' "$LAST")"
sleep 1.2
LAST="$(state)"
check "a stopped timer holds" "$LAST" "s[\"stage\"][\"timerSeconds\"] == $STAGE_T"
act "reset the timer" stage-timer '{"action":"reset"}' 's["stage"]["timerSeconds"] == 0 and s["stage"]["timer"] == "00:00"'
act "toggle starts it" stage-timer '{"action":"toggle"}' 's["stage"]["timerRunning"]'
act "toggle stops it" stage-timer '{"action":"toggle"}' 'not s["stage"]["timerRunning"]'
act "an unknown timer action is refused" stage-timer '{"action":"rewind"}' 'not r["ok"]'
act "reset the timer again" stage-timer '{"action":"reset"}' 's["stage"]["timerSeconds"] == 0'
# The stage message: control API, then Live ▸ Stage Message… typed into with real keys.
act "a stage message" stage-note '{"text":"  Repeat the\nchorus  "}' 's["stage"]["note"] == "Repeat the chorus"'
wait_idle
shot stage-note stage
pixel "the stage shows the message band" "$OUT/stage-note.png" has 00796b 0.0 0.8 1.0 1.0 5000
pixel "the message is white on the band" "$OUT/stage-note.png" has ffffff 0.1 0.85 0.9 1.0 100
pixel "the live text stays above the band" "$OUT/stage-note.png" has ffffff 0.1 0.15 0.9 0.5 300
shot stage-note-output output
pixel "the message never reaches the output" "$OUT/stage-note-output.png" lacks 00796b 0.0 0.0 1.0 1.0 50
act "take the message down" stage-note '{"text":""}' 's["stage"]["note"] == ""'
wait_idle
shot stage-no-note stage
pixel "the band is gone" "$OUT/stage-no-note.png" lacks 00796b 0.0 0.0 1.0 1.0 50
act "Live ▸ Stage Message… opens the dialog" menu '{"item":"stage-message"}' 's["dialog"]["stageMessage"] and s["dialog"]["host"] == "main" and s["focus"] == "dialog"'
wait_idle
for k in o k; do LAST="$(post native-key "{\"name\":\"$k\",\"window\":\"stage-message\"}")"; done
check "typing in the dialog" "$LAST" 'r["ok"] and s["dialog"]["stageMessage"] and s["stage"]["note"] == ""'
shot stage-message stage-message
pixel "the stage message dialog has content" "$OUT/stage-message.png" notblank
act "Enter shows the typed message" native-key '{"name":"enter","window":"stage-message"}' 'not s["dialog"].get("stageMessage") and s["stage"]["note"] == "ok"'
act "the dialog opens with the current message" stage-note '{"mode":"open"}' 's["dialog"]["stageMessage"]'
act "Cancel changes nothing" dismiss '' 'not s["dialog"].get("stageMessage") and s["stage"]["note"] == "ok"'
act "open it with a new message" stage-note '{"mode":"open","text":"Five minutes"}' 's["dialog"]["stageMessage"]'
act "Show (OK) puts it up" accept '' 'not s["dialog"].get("stageMessage") and s["stage"]["note"] == "Five minutes"'
act "open it again" stage-note '{"mode":"open"}' 's["dialog"]["stageMessage"]'
act "Clear takes it down" stage-note '{"mode":"clear"}' 'not s["dialog"].get("stageMessage") and s["stage"]["note"] == ""'
act "an unknown stage-note mode is refused" stage-note '{"mode":"bogus"}' 'not r["ok"]'
# The Live Output monitor's output selector previews the Foldback output.
act "the Live Output monitor shows Foldback" stage-monitor '{"on":true}' 's["stage"]["monitor"] == "foldback"'
wait_idle
shot stage-monitor main
pixel "the live monitor shows the stage's amber next slide" "$OUT/stage-monitor.png" has ffd15c 0.6 0.35 1.0 0.7 30
act "back to the Live Output" stage-monitor '{"on":false}' 's["stage"]["monitor"] == "live"'
wait_idle
shot stage-monitor-live main
pixel "the live monitor no longer shows the stage" "$OUT/stage-monitor-live.png" lacks ffd15c 0.6 0.35 1.0 0.7 30
# Settings: set-setting persists; the Options section's Cancel restores the stage settings.
act "a 24-hour clock and bigger text" set-setting '{"key":"stage","value":{"clock24h":true,"fontScale":1.5}}' 's["stage"]["settings"]["clock24h"] and s["stage"]["settings"]["fontScale"] == 1.5 and ":" in s["stage"]["clock"] and not s["stage"]["clock"].endswith(("AM","PM"))'
wait_idle
python3 - "$LIB" <<'PY' || fail "the stage settings were not persisted"
import json, sqlite3, sys
db = sqlite3.connect("file:%s?mode=ro" % sys.argv[1], uri=True)
row = db.execute("select value from settings where key = 'stage.settings'").fetchone()
v = json.loads(row[0]) if row else {}
sys.exit(0 if v.get("enabled") and v.get("clock24h") and v.get("fontScale") == 1.5 and v.get("screen") == -1 else 1)
PY
log "ok: the stage settings are persisted"
shot stage-big stage
pixel "the bigger stage text still has its next slide" "$OUT/stage-big.png" has ffd15c 0.05 0.5 0.95 1.0 200
act "no next slide, no timer" set-setting '{"key":"stage","value":{"showNext":false,"showTimer":false,"fontScale":1}}' 's["stage"]["next"] == "" and s["stage"]["timer"] == "" and s["stage"]["clock"] != ""'
wait_idle
shot stage-plain stage
pixel "without the next slide there is no amber" "$OUT/stage-plain.png" lacks ffd15c 0.0 0.0 1.0 1.0 20
# The header shrinks back from 150 %: a label must not keep the bigger text's minimum size (StageView.replace).
pixel "the clock stays inside the right margin" "$OUT/stage-plain.png" lacks ffffff 0.985 0.0 1.0 0.15 1
act "an unconnected stage display is refused" stage '{"screen":99}' 'not r["ok"] and "not connected" in r["error"]'
check "the stage keeps its display" "$(state)" 's["stage"]["settings"]["screen"] == -1 and s["stage"]["open"]'
act "the Stage Display options page" options '{"page":"stage"}' 's["dialog"]["settings"]'
wait_idle
shot stage-options settings
pixel "the Stage Display page has content" "$OUT/stage-options.png" notblank
act "change the stage in the dialog" set-setting '{"key":"stage","value":{"showNext":true,"showTimer":true}}' 's["stage"]["settings"]["showNext"] and s["stage"]["next"] != ""'
act "Cancel restores the stage settings" dismiss '' 'not s["dialog"]["settings"] and not s["stage"]["settings"]["showNext"] and not s["stage"]["settings"]["showTimer"]'
act "the defaults back" set-setting '{"key":"stage","value":{"showNext":true,"showTimer":true,"clock24h":false,"fontScale":1}}' 's["stage"]["settings"]["showNext"] and s["stage"]["settings"]["showTimer"] and not s["stage"]["settings"]["clock24h"]'
# Off again (the Live menu toggles it), and the stage songs are removed.
act "stage {on:false} closes it" stage '{"on":false}' 'not s["stage"]["settings"]["enabled"] and not s["stage"]["open"] and s["stage"]["screen"] == -1'
act "stage toggles it on" stage '' 's["stage"]["open"]'
act "Live ▸ Stage Display turns it off" menu '{"item":"stage-display"}' 'not s["stage"]["open"]'
act "remove the second stage item" remove-schedule "{\"index\":$((STAGE_ROW + 1))}" "len(s[\"schedule\"][\"items\"]) == $STAGE_ROW + 1"
act "remove the first stage item" remove-schedule "{\"index\":$STAGE_ROW}" "len(s[\"schedule\"][\"items\"]) == $STAGE_ROW"
act "save the schedule (stage)" save-schedule '{}' 'not s["schedule"]["dirty"]'
act "find the stage songs again" search-songs '{"q":"smoke stage"}' 's["songs"]["results"] == 2'
act "delete a stage song" delete-song '{"index":0,"confirm":true}' 'r["ok"]'
wait_idle
act "find the last stage song" search-songs '{"q":"smoke stage"}' 's["songs"]["results"] == 1'
act "delete the other stage song" delete-song '{"index":0,"confirm":true}' 'r["ok"]'
wait_idle
act "clear the song search (stage)" search-songs '{"q":""}' 's["songs"]["results"] == s["songs"]["count"]'

# ---- remote control (v0.2) -------------------------------------------------------------------
# The LAN remote (internal/remote, docs/REMOTE.md) on a free port, driven by a fake phone
# (test/remoteclient: the phone app's own pairing calls and WebSocket): a wrong code is refused,
# the right one waits for the operator's approval, the approved phone gets a token, and its taps
# (add a song, go live with it, next / prev, black on / off) take the operator's event path:
# /state and the projector show them. Revoking the phone closes its socket at once.
RC="$OUT/remoteclient"
(cd "$ROOT" && go build -o "$RC" ./test/remoteclient) || fail "build test/remoteclient"
RPORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"
RBASE="http://127.0.0.1:$RPORT"
act "the remote is off by default" state '' 'not s["remote"]["enabled"] and not s["remote"]["running"] and s["remote"]["devices"] == []'
act "a port below 1024 is refused" remote '{"port":80}' 'not r["ok"] and "between 1024 and 65535" in r["error"]'
act "turn the remote on" remote "{\"on\":true,\"port\":$RPORT}" "s[\"remote\"][\"enabled\"] and s[\"remote\"][\"running\"] and s[\"remote\"][\"port\"] == $RPORT and s[\"remote\"][\"configuredPort\"] == $RPORT and s[\"remote\"][\"url\"].endswith(\":$RPORT/\") and len(s[\"remote\"][\"code\"]) == 6"
RCODE="$(field "$LAST" remote.code)"
curl -sS --max-time 10 "$RBASE/" | grep -q 'FreeBeams' || fail "the remote does not serve the phone app"
log "ok: the remote serves the phone app"
RRESP="$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' "$RBASE/api/state")"
[ "$RRESP" = "401" ] || fail "an unpaired /api/state answered $RRESP, not 401"
log "ok: an unpaired phone gets no state (401)"
act "Remote ▸ Show Pairing Code…" remote-dialog '' 's["remote"]["dialog"] == "pairing" and s["focus"] == "dialog"'
wait_idle
shot remote-dialog remote
pixel "the pairing dialog shows the QR code" "$OUT/remote-dialog.png" has ffffff 0.0 0.0 0.5 0.6 20000
pixel "the QR code has dark modules" "$OUT/remote-dialog.png" has 000000 0.0 0.0 0.5 0.6 5000
WRONG="$(python3 -c 'import sys; c=sys.argv[1]; print("%06d" % ((int(c) + 1) % 1000000))' "$RCODE")"
LAST="$("$RC" pair -base "$RBASE" -code "$WRONG" -name "Smoke Phone")"
check "a wrong pairing code is refused" "$LAST" 'r["httpStatus"] == 403 and not r["ok"] and r["remaining"] == 4'
LAST="$("$RC" pair -base "$RBASE" -code "$RCODE" -name "Smoke Phone")"
check "the right code asks the operator" "$LAST" 'r["ok"] and r["id"] and r["secret"]'
PAIR_ID="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["id"])' "$LAST")"
PAIR_SECRET="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["secret"])' "$LAST")"
act "the phone waits for approval" state '' 'len(s["remote"]["pending"]) == 1 and s["remote"]["pending"][0]["name"] == "Smoke Phone" and s["remote"]["pending"][0]["kind"] == "phone"'
wait_idle
shot remote-dialog-pending remote
act "approve the phone" remote-approve '{"name":"Smoke Phone"}' "s[\"remote\"][\"pending\"] == [] and len(s[\"remote\"][\"devices\"]) == 1 and s[\"remote\"][\"devices\"][0][\"name\"] == \"Smoke Phone\" and s[\"remote\"][\"code\"] != \"$RCODE\""
LAST="$("$RC" wait -base "$RBASE" -id "$PAIR_ID" -secret "$PAIR_SECRET")"
check "the approved phone collects its token" "$LAST" 'r["ok"] and r["status"] == "approved" and len(r["token"]) == 43 and r["device"]["name"] == "Smoke Phone"'
RTOKEN="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["token"])' "$LAST")"
act "close the pairing dialog" dismiss '' 's["remote"].get("dialog", "") == "" and s["focus"] != "dialog"'
LAST="$(curl -sS --max-time 10 -H "Authorization: Bearer $RTOKEN" "$RBASE/api/songs?q=amazing%20grace")"
check "the phone searches the song library" "$LAST" 'r["ok"] and any(x["title"] == "Amazing Grace" for x in r["songs"])'
RSONG="$(python3 -c 'import json,sys; print([x["id"] for x in json.loads(sys.argv[1])["songs"] if x["title"] == "Amazing Grace"][0])' "$LAST")"
LAST="$(state)"
RITEMS="$(python3 -c 'import json,sys; print(len(json.loads(sys.argv[1])["data"]["schedule"]["items"]))' "$LAST")"
LAST="$("$RC" ws -base "$RBASE" -token "$RTOKEN" -do "add-song {\"id\":$RSONG}" -do "live-item {\"index\":$RITEMS}")"
check "the phone adds a song and sends it live" "$LAST" "r[\"ok\"] and r[\"hello\"][\"device\"][\"name\"] == \"Smoke Phone\" and len(r[\"state\"][\"schedule\"]) == $RITEMS + 1 and r[\"state\"][\"liveItem\"] == $RITEMS and r[\"state\"][\"live\"][\"title\"] == \"Amazing Grace\" and r[\"state\"][\"live\"][\"index\"] == 0"
act "the song is live in the app" state '' "s[\"schedule\"][\"live\"] == $RITEMS and s[\"live\"][\"title\"] == \"Amazing Grace\" and s[\"live\"][\"index\"] == 0 and s[\"mode\"] == \"normal\""
LAST="$("$RC" ws -base "$RBASE" -token "$RTOKEN" -do next -do next -do prev)"
check "next, next, prev from the phone" "$LAST" 'r["ok"] and [x["action"] for x in r["replies"]] == ["next", "next", "prev"] and r["state"]["live"]["index"] == 1 and r["state"]["live"]["label"]'
act "the app follows the phone (slide 2)" state '' 's["live"]["index"] == 1'
wait_idle
shot remote-slide2-output output
pixel "the output shows the live slide" "$OUT/remote-slide2-output.png" notblank
LAST="$("$RC" ws -base "$RBASE" -token "$RTOKEN" -do 'black {"on":true}' -do 'black {"on":true}')"
check "black from the phone (set, not toggled)" "$LAST" 'r["ok"] and r["state"]["mode"] == "black"'
act "the app is black" state '' 's["mode"] == "black" and s["live"]["index"] == 1'
wait_idle
shot remote-black-output output
pixel "the output is black (phone)" "$OUT/remote-black-output.png" black
LAST="$("$RC" ws -base "$RBASE" -token "$RTOKEN" -do 'black {"on":false}' -do quit -do 'open-schedule {"path":"/etc/passwd"}')"
check "black off from the phone; control-only actions are unknown to it" "$LAST" 'not r["ok"] and r["state"]["mode"] == "normal" and [x["ok"] for x in r["replies"]] == [True, False, False] and "unknown action" in r["replies"][1]["error"]'
act "the app is back to normal and still running" state '' 's["mode"] == "normal" and s["remote"]["running"]'
wait_idle
shot remote-normal-output output
pixel "the output shows the slide again" "$OUT/remote-normal-output.png" notblank
act "the Options dialog has a Remote page" options '{"page":"remote"}' 's["dialog"]["settings"] and s["remote"]["enabled"]'
wait_idle
shot remote-options settings
pixel "the Remote page has content" "$OUT/remote-options.png" notblank
act "close the Options dialog (remote)" dismiss '' 'not s["dialog"]["settings"] and s["remote"]["running"]'
# Revoking closes the phone's socket at once and its token stops working.
"$RC" watch -base "$RBASE" -token "$RTOKEN" -for 15s >"$OUT/remote-watch.json" 2>"$OUT/remote-watch.err" &
WATCH_PID=$!
for _ in $(seq 1 40); do
	grep -q connected "$OUT/remote-watch.err" 2>/dev/null && break
	sleep 0.25
done
act "the phone is connected" state '' 's["remote"]["clients"] == 1 and s["remote"]["devices"][0]["online"]'
act "remove the phone" remote-revoke '{"name":"Smoke Phone"}' 's["remote"]["devices"] == []'
wait "$WATCH_PID"
check "the phone is told it was removed and disconnected" "$(cat "$OUT/remote-watch.json")" 'r["revoked"] and r["closed"] and not r["timedOut"]'
RRESP="$(curl -sS --max-time 10 -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $RTOKEN" "$RBASE/api/session")"
[ "$RRESP" = "401" ] || fail "a revoked token answered $RRESP, not 401"
log "ok: the revoked token no longer works (401)"
act "turn the remote off" remote '{"on":false}' 'not s["remote"]["enabled"] and not s["remote"]["running"]'
curl -sS --max-time 5 -o /dev/null "$RBASE/" 2>/dev/null && fail "the remote still answers after it was turned off"
log "ok: nothing listens on the remote port any more"

# ---- alerts: ticker, nursery, countdown, clock (Alerts ▾) ----------------------------------------
# Alerts are an overlay above the live slide on the Live monitor and the output (render/overlay.go),
# projected every frame from core.Alerts. The styles get opaque test colors so the pixels are exact:
# message band #2a7f62, nursery badge #bf5f2a, countdown #2a5fbf, clock #991a80, over Black.
LAST="$(state)"
ALERT_STYLES="$(python3 -c 'import json,sys; print(json.dumps(json.loads(sys.argv[1])["data"]["settings"]["alerts"]))' "$LAST")"
check "three example alerts are saved" "$LAST" '[a["name"] for a in s["alerts"]["saved"]] == ["Parent Needed", "Service Starting", "Vehicle"] and not s["alerts"]["showing"]'
check "the Alerts menu starts closed" "$LAST" 'not s["dialog"].get("alertsMenu")'
act "alert styles with opaque test colors" set-setting '{"key":"alerts","value":{
 "message":{"size":54,"bold":true,"color":{"R":1,"G":1,"B":1,"A":1},"background":{"R":0.1647,"G":0.498,"B":0.3843,"A":1}},
 "nursery":{"size":64,"bold":true,"color":{"R":1,"G":1,"B":1,"A":1},"background":{"R":0.749,"G":0.3725,"B":0.1647,"A":1}},
 "countdown":{"size":72,"bold":true,"color":{"R":1,"G":1,"B":1,"A":1},"background":{"R":0.1647,"G":0.3725,"B":0.749,"A":1}},
 "clock":{"size":44,"bold":true,"color":{"R":1,"G":1,"B":1,"A":1},"background":{"R":0.6,"G":0.102,"B":0.502,"A":1}}}}' \
	's["settings"]["alerts"]["nursery"]["background"]["A"] == 1 and s["settings"]["alerts"]["nurseryPosition"] == "top-right" and s["settings"]["alerts"]["countdownPosition"] == "center"'
act "black under the alerts" black '{"on":true}' 's["mode"] == "black"'
act "a ticker" alert '{"text":"The owner of the blue van, please move it.","display":"ticker","count":3}' 's["alerts"]["showing"] and s["alerts"]["moving"] and s["alerts"]["active"][0]["style"] == "ticker" and s["alerts"]["active"][0]["location"] == "bottom" and s["alerts"]["active"][0]["passes"] == 3 and s["alerts"]["message"].startswith("The owner")'
TICK_X="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["data"]["alerts"]["active"][0]["x"])' "$LAST")"
act "a nursery code" nursery '{"code":"A12"}' 's["alerts"]["nursery"] == ["A12"]'
act "the same code is refused" nursery '{"code":"a12"}' 'not r["ok"]'
act "a five-minute countdown" countdown '{"minutes":5,"caption":"Service starts in"}' 's["alerts"]["countdown"]["active"] and s["alerts"]["countdown"]["text"] in ("05:00", "04:59") and s["alerts"]["countdown"]["position"] == "center"'
act "the clock" clock '{"on":true}' 's["alerts"]["clock"]["on"] and s["alerts"]["clock"]["position"] == "top-left" and ":" in s["alerts"]["clock"]["text"]'
wait_idle
check "a scrolling ticker does not keep the app busy" "$LAST" 's["idle"] and s["alerts"]["moving"]'
check "the ticker moves left" "$LAST" "s[\"alerts\"][\"active\"][0][\"x\"] < $TICK_X"
shot alerts-output output
shot alerts-main main
pixel "the output shows the message band" "$OUT/alerts-output.png" has 2a7f62 0.0 0.93 1.0 1.0 5000
pixel "the output shows the nursery badge top right" "$OUT/alerts-output.png" has bf5f2a 0.8 0.0 1.0 0.15 2000
pixel "the output shows the countdown in the centre" "$OUT/alerts-output.png" has 2a5fbf 0.3 0.3 0.7 0.7 5000
pixel "the output shows the clock top left" "$OUT/alerts-output.png" has 991a80 0.0 0.0 0.25 0.15 2000
pixel "no badge where none belongs" "$OUT/alerts-output.png" lacks bf5f2a 0.0 0.0 0.5 0.5 50
pixel "the live monitor shows the band" "$OUT/alerts-main.png" has 2a7f62 0.6 0.3 1.0 0.75 300
pixel "the live monitor shows the nursery badge" "$OUT/alerts-main.png" has bf5f2a 0.6 0.3 1.0 0.75 100
pixel "the preview monitor has no alerts" "$OUT/alerts-main.png" lacks bf5f2a 0.15 0.3 0.6 0.75 20
act "Next ends the ticker" alert-next '' 's["alerts"]["active"] == [] and s["alerts"]["nursery"] == ["A12"]'
act "a banner at the top" alert '{"text":"Welcome","display":"static","location":"top","seconds":60}' 's["alerts"]["active"][0]["style"] == "static" and s["alerts"]["active"][0]["location"] == "top" and s["alerts"]["active"][0]["end"] == "duration" and not s["alerts"]["moving"]'
wait_idle
shot alerts-banner-output output
pixel "the banner band is at the top" "$OUT/alerts-banner-output.png" has 2a7f62 0.0 0.0 1.0 0.08 5000
pixel "no band at the bottom" "$OUT/alerts-banner-output.png" lacks 2a7f62 0.0 0.92 1.0 1.0 50
act "a second message waits its turn" alert '{"text":"Second","display":"static"}' 'len(s["alerts"]["active"]) == 2 and s["alerts"]["active"][0]["showing"] and not s["alerts"]["active"][1]["showing"]'
act "clear the messages" alert-clear '{"kind":"messages"}' 's["alerts"]["active"] == [] and s["alerts"]["nursery"] == ["A12"] and s["alerts"]["countdown"]["active"]'
# A saved alert with a token: refused without its value, filled from {values}, or asked for by the menu.
act "a token alert needs its value" alert '{"name":"Vehicle"}' 'not r["ok"] and "plate" in r["error"]'
act "a saved alert with its token filled" alert '{"name":"Vehicle","values":{"plate":"XYZ 123"}}' 's["alerts"]["active"][0]["text"] == "The owner of the vehicle with plate XYZ 123, please move it." and s["alerts"]["active"][0]["name"] == "Vehicle"'
act "a {countdown} token follows the countdown" alert '{"name":"Service Starting"}' 's["alerts"]["active"][1]["end"] == "token"'
act "clear the messages again" alert-clear '{"kind":"messages"}' 's["alerts"]["active"] == []'
act "the Alerts menu opens" alerts-menu '{"item":"open"}' 'd["open"] and d["entries"][:3] == ["Parent Needed", "Service Starting", "Vehicle"] and "Nursery…" in d["entries"] and d["clock"]'
wait_idle
shot alerts-menu alerts-menu
pixel "the Alerts menu has content" "$OUT/alerts-menu.png" notblank
act "close the Alerts menu" alerts-menu '{"item":"close"}' 'not d["open"]'
act "Alerts ▸ Vehicle asks for the plate" alerts-menu '{"item":"alert","name":"Vehicle"}' 'd["state"]["dialog"]["alertDialog"] == "alert-fill"'
act "USE with an empty plate is refused" accept '' 's["dialog"]["alertDialog"] == "alert-fill" and s["alerts"]["active"] == []'
act "type the plate" alert-fill '{"values":{"plate":"FB 2026"}}' 'd["tokens"] == ["plate"] and d["preview"] == "The owner of the vehicle with plate FB 2026, please move it." and d["error"] == ""'
wait_idle
shot alert-fill alert-fill
pixel "the token dialog has content" "$OUT/alert-fill.png" notblank
act "USE shows the alert" accept '' 'not s["dialog"].get("alertDialog") and s["alerts"]["active"][0]["text"].endswith("plate FB 2026, please move it.")'
act "clear the vehicle alert" alert-clear '{"kind":"messages"}' 's["alerts"]["active"] == []'
# The nursery and countdown dialogs.
act "Alerts ▸ Nursery…" alerts-menu '{"item":"nursery"}' 'd["state"]["dialog"]["alertDialog"] == "nursery"'
act "type a code and Add" nursery-type '{"code":"B7"}' 's["alerts"]["nursery"] == ["A12", "B7"]'
wait_idle
shot alerts-nursery nursery
pixel "the nursery dialog has content" "$OUT/alerts-nursery.png" notblank
act "close the nursery dialog" dismiss '' 'not s["dialog"].get("alertDialog")'
act "Alerts ▸ Countdown…" alerts-menu '{"item":"countdown"}' 'd["state"]["dialog"]["alertDialog"] == "countdown"'
act "type a bad time" countdown-form '{"time":"half past"}' 'd["error"] == ""'
act "Start with a bad time keeps the dialog" accept '' 's["dialog"]["alertDialog"] == "countdown"'
act "2:30 in the dialog" countdown-form '{"minutes":2,"seconds":30,"caption":"Back in","position":"bottom-left","end":"hold"}' 'r["ok"]'
act "Start replaces the countdown" accept '' 'not s["dialog"].get("alertDialog") and s["alerts"]["countdown"]["text"] in ("02:30", "02:29") and s["alerts"]["countdown"]["caption"] == "Back in" and s["alerts"]["countdown"]["position"] == "bottom-left" and s["alerts"]["countdown"]["end"] == "hold"'
act "Alerts ▸ Clock turns the clock off" alerts-menu '{"item":"clock"}' 'not d["state"]["alerts"]["clock"]["on"] and not d["clock"]'
wait_idle
shot alerts-output-2 output
pixel "the countdown moved to the bottom left" "$OUT/alerts-output-2.png" has 2a5fbf 0.0 0.8 0.3 1.0 2000
pixel "the clock is gone" "$OUT/alerts-output-2.png" lacks 991a80 0.0 0.0 1.0 1.0 50
pixel "both nursery codes show" "$OUT/alerts-output-2.png" has bf5f2a 0.7 0.0 1.0 0.15 3000
act "remove a code" nursery-remove '{"code":"A12"}' 's["alerts"]["nursery"] == ["B7"]'
# Options ▸ Alerts: hide on Black, then everything goes.
act "Options ▸ Alerts" options '{"page":"alerts"}' 's["dialog"]["settings"]'
wait_idle
shot options-alerts settings
pixel "the Alerts options page has content" "$OUT/options-alerts.png" notblank
act "close the Options dialog (alerts)" options '{"on":false}' 'not s["dialog"]["settings"]'
act "hide alerts on Black" set-setting '{"key":"alerts","value":{"hideOnBlack":true}}' 's["settings"]["alerts"]["hideOnBlack"] and s["alerts"]["hidden"] and not s["alerts"]["showing"] and s["alerts"]["nursery"] == ["B7"]'
wait_idle
shot alerts-hidden-output output
pixel "Black hides the alerts" "$OUT/alerts-hidden-output.png" black
act "show alerts on Black again" set-setting '{"key":"alerts","value":{"hideOnBlack":false}}' 's["alerts"]["showing"]'
# Saved alerts: the editor's OK path, then delete.
act "Alerts ▸ New Alert…" alerts-menu '{"item":"new"}' 'd["state"]["dialog"]["alertDialog"] == "alert-editor"'
act "type a message without a name" alert-editor '{"alert":{"message":"Parking lot full"}}' 'r["ok"]'
act "OK without a name is refused" accept '' 's["dialog"]["alertDialog"] == "alert-editor"'
act "name the alert" alert-editor '{"alert":{"name":"Smoke Parking","message":"Parking lot {lot} is full","display":"static","end":"manual"}}' 'd["error"] == ""'
act "OK saves it" accept '' 'r["ok"]'
wait_idle
check "the saved alert is listed" "$LAST" '"Smoke Parking" in [a["name"] for a in s["alerts"]["saved"]] and not s["dialog"].get("alertDialog")'
act "the menu lists it" alerts-menu '{"item":"close"}' '"Smoke Parking" in d["entries"]'
act "show it until removed" alert '{"name":"Smoke Parking","values":{"lot":"B"}}' 's["alerts"]["active"][0]["text"] == "Parking lot B is full" and s["alerts"]["active"][0]["remaining"] == -1'
act "Alerts ▸ Alert Manager…" alerts-menu '{"item":"manager"}' 'd["state"]["dialog"]["alertDialog"] == "alert-manager"'
SMOKE_ALERT="$(python3 -c 'import json,sys; print([a["name"] for a in json.loads(sys.argv[1])["data"]["state"]["alerts"]["saved"]].index("Smoke Parking"))' "$LAST")"
act "select it in the manager" alert-manager "{\"index\":$SMOKE_ALERT}" 'r["ok"]'
wait_idle
shot alert-manager alert-manager
pixel "the Alert Manager has content" "$OUT/alert-manager.png" notblank
act "Delete asks first" alert-manager "{\"index\":$SMOKE_ALERT,\"mode\":\"delete\"}" 'r["ok"]'
wait_idle
check "the delete question is up (alerts)" "$LAST" 's["dialog"]["confirm"] == "Delete Alert"'
act "OK deletes the alert" accept '' 'r["ok"]'
wait_idle
check "the alert is gone" "$LAST" '"Smoke Parking" not in [a["name"] for a in s["alerts"]["saved"]] and len(s["alerts"]["saved"]) == 3'
act "close the manager" dismiss '' 'not s["dialog"].get("alertDialog")'
act "Alerts ▸ Clear All Alerts" alerts-menu '{"item":"clear"}' 'not d["state"]["alerts"]["showing"] and d["state"]["alerts"]["active"] == [] and d["state"]["alerts"]["nursery"] == [] and not d["state"]["alerts"]["countdown"]["active"]'
wait_idle
shot alerts-cleared-output output
pixel "the output is black again" "$OUT/alerts-cleared-output.png" black
act "restore the alert styles" set-setting "{\"key\":\"alerts\",\"value\":$ALERT_STYLES}" 's["settings"]["alerts"]["nursery"]["background"]["A"] < 1'
act "black off (alerts)" black '{"on":false}' 's["mode"] == "normal"'

# ---- audio playlist (background audio) ---------------------------------------------------------
# The schedule's audio playlist plays independently of the Live slide (docs/MEDIA.md "Audio
# playlist"): four short tones (one deleted after it was added, so it is skipped), driven through
# the audio-* / playlist-* actions and /state.audio; a song going live and Black leave it playing;
# the phone remote toggles it and skips tracks; the playlist round-trips through the .fbs file.
# Audio has no pixels: /state.audio (what the native player reports) is the truth.
LAST="$(state)"
check "the audio state is present (empty, stopped)" "$LAST" 's["audio"]["tracks"] == [] and s["audio"]["status"] == "stopped" and s["audio"]["track"] == -1 and s["audio"]["repeat"] == "off" and not s["audio"]["shuffle"]'
AU_NATIVE="$(field "$LAST" audio.native)"
# playlist_ffmpeg counts the app's ffmpeg processes decoding a playlist track
# (the ffmpeg player, FREEBEAMS_PLAYER=ffmpeg; AVFoundation runs none).
playlist_ffmpeg() {
	local p n=0
	for p in $(app_pids); do
		ps -o command= -p "$p" 2>/dev/null | grep -q 'ffmpeg .*pl-[0-9]-' && n=$((n + 1))
	done
	echo "$n"
}
# no_playlist_ffmpeg DESCRIPTION: waits (up to 5 s) until none is left.
no_playlist_ffmpeg() {
	local n
	for _ in $(seq 1 20); do
		n="$(playlist_ffmpeg)"
		[ "$n" = 0 ] && break
		sleep 0.25
	done
	[ "$n" = 0 ] || fail "$1: $n ffmpeg process(es) still decode a playlist track"
	log "ok: $1"
}
act "an empty playlist cannot play" audio-play '' 'not r["ok"] and "no track" in r["error"]'
act "a seek needs a playing track" audio-seek '{"seconds":1}' 'not r["ok"]'
act "a bad repeat mode is refused" audio-repeat '{"mode":"twice"}' 'not r["ok"]'
act "a fade-out over 30 s is refused" audio-fade-out '{"seconds":31}' 'not r["ok"]'
if [ "$FFMPEG" = "True" ] && [ "$AU_NATIVE" = "True" ]; then
	AU_DIR="$OUT/audio-playlist"
	rm -rf "$AU_DIR"
	mkdir -p "$AU_DIR"
	ffmpeg -v error -y -f lavfi -i "sine=frequency=440:duration=3" "$AU_DIR/pl-1-prelude.wav" &&
		ffmpeg -v error -y -f lavfi -i "sine=frequency=660:duration=8" -c:a aac "$AU_DIR/pl-2-hymn.m4a" &&
		ffmpeg -v error -y -f lavfi -i "sine=frequency=880:duration=3" "$AU_DIR/pl-3-gone.wav" &&
		ffmpeg -v error -y -f lavfi -i "sine=frequency=550:duration=3" "$AU_DIR/pl-4-postlude.wav" || fail "ffmpeg could not make the playlist tones"
	LAST="$(state)"
	AU_AUDIO="$(field "$LAST" media.audio)"
	AU_SCHED="$(field "$LAST" schedule.path)"
	act "add a folder of tones to the playlist" playlist-add "{\"path\":\"$AU_DIR\"}" "[t[\"title\"] for t in s[\"audio\"][\"tracks\"]] == [\"pl-1-prelude\", \"pl-2-hymn\", \"pl-3-gone\", \"pl-4-postlude\"] and s[\"media\"][\"audio\"] == $AU_AUDIO + 4 and s[\"schedule\"][\"dirty\"] and s[\"audio\"][\"status\"] == \"stopped\" and s[\"audio\"][\"track\"] == 0"
	act "adding a library item again (Media tab menu)" media-menu '{"item":"add-to-playlist","title":"pl-1-prelude"}' 'len(s["audio"]["tracks"]) == 5 and s["audio"]["tracks"][4]["title"] == "pl-1-prelude"'
	act "remove the duplicate" playlist-remove '{"index":4}' 'len(s["audio"]["tracks"]) == 4'
	act "the video collection (audio)" search-media '{"q":"","kind":"video"}' 's["media"]["kind"] == "video"'
	if [ "$(field "$LAST" media.results)" != "0" ]; then
		act "a video cannot join the playlist" media-menu '{"item":"add-to-playlist","index":0}' 'not r["ok"] and "not an audio file" in r["error"]'
	fi
	act "all media (audio)" search-media '{"q":""}' 's["media"]["kind"] == "" and len(s["audio"]["tracks"]) == 4'
	rm "$AU_DIR/pl-3-gone.wav"
	act "a deleted file is marked missing" state '' '[t.get("missing", False) for t in s["audio"]["tracks"]] == [False, False, True, False]'
	act "volume 0.6" audio-volume '{"value":0.6}' 's["audio"]["volume"] == 0.6'
	act "a volume that is not a number is refused (audio)" audio-volume '{"value":"loud"}' 'not r["ok"]'
	act "play the playlist" audio-play '' 's["audio"]["status"] == "playing" and s["audio"]["track"] == 0 and s["audio"]["title"] == "pl-1-prelude" and s["audio"]["next"] == 1'
	pb_wait "the first track plays" 's["audio"]["loaded"] and s["audio"]["position"] > 0.2 and s["audio"]["duration"] > 2.9 and s["audio"]["playing"]'
	AU_POS="$(field "$LAST" audio.position)"
	sleep 0.4
	LAST="$(state)"
	check "the position advances" "$LAST" "s[\"audio\"][\"position\"] > $AU_POS and s[\"audio\"][\"track\"] == 0"
	wait_idle # a playing playlist does not keep the app busy
	shot audio-playlist-main main
	act "seek near the end of the first track" audio-seek '{"seconds":2.5}' 'r["ok"]'
	pb_wait "the first track ends and the second starts" 's["audio"]["track"] == 1 and s["audio"]["playing"] and s["audio"]["loaded"] and s["audio"]["position"] < 1.5 and s["audio"]["title"] == "pl-2-hymn"'
	act "select a song" select-song '{"title":"Amazing Grace"}' 's["preview"]["title"] == "Amazing Grace"'
	act "the song goes live; the music plays on" go-live '' 's["live"]["title"] == "Amazing Grace" and s["audio"]["playing"] and s["audio"]["track"] == 1'
	act "black; the music plays on" black '{"on":true}' 's["mode"] == "black" and s["audio"]["playing"] and s["audio"]["track"] == 1'
	wait_idle # past the fade to black
	AU_POS="$(field "$(state)" audio.position)"
	sleep 0.4
	act "the music advances under Black" state '' "s[\"audio\"][\"position\"] > $AU_POS and s[\"audio\"][\"playing\"] and s[\"mode\"] == \"black\""
	act "logo; the music plays on" logo '{"on":true}' 's["mode"] == "logo" and s["audio"]["playing"]'
	act "back to normal (audio)" logo '{"on":false}' 's["mode"] == "normal" and s["audio"]["playing"]'
	act "clear; the music plays on" clear '{"on":true}' 's["mode"] == "clear" and s["audio"]["playing"]'
	act "clear off (audio)" clear '{"on":false}' 's["mode"] == "normal" and s["audio"]["playing"] and s["audio"]["track"] == 1'
	act "next skips the missing file" audio-next '' 's["audio"]["track"] == 3 and s["audio"]["playing"] and s["audio"]["title"] == "pl-4-postlude"'
	pb_wait "the fourth track plays" 's["audio"]["track"] == 3 and s["audio"]["loaded"] and s["audio"]["position"] > 0.1'
	act "prev goes back over the missing file" audio-prev '' 's["audio"]["track"] == 1 and s["audio"]["playing"]'
	act "play a track by index" audio-play '{"index":3}' 's["audio"]["track"] == 3 and s["audio"]["playing"]'
	act "a missing track cannot be played" audio-play '{"index":2}' 'not r["ok"]'
	pb_wait "the fourth track is open" 's["audio"]["track"] == 3 and s["audio"]["loaded"]'
	act "seek near the end of the last track (repeat off)" audio-seek '{"seconds":2.5}' 'r["ok"]'
	pb_wait "the list ends: stopped, the first track cued" 's["audio"]["status"] == "stopped" and s["audio"]["track"] == 0 and s["audio"]["position"] == 0 and not s["audio"]["loaded"]'
	act "repeat all" audio-repeat '{"mode":"all"}' 's["audio"]["repeat"] == "all" and s["audio"]["status"] == "stopped"'
	act "play the last track" audio-play '{"index":3}' 's["audio"]["track"] == 3 and s["audio"]["next"] == 0'
	pb_wait "the last track is open" 's["audio"]["track"] == 3 and s["audio"]["loaded"] and s["audio"]["position"] > 0.1'
	act "seek near the end (repeat all)" audio-seek '{"seconds":2.5}' 'r["ok"]'
	pb_wait "repeat all wraps to the first track" 's["audio"]["track"] == 0 and s["audio"]["playing"] and s["audio"]["loaded"] and s["audio"]["position"] < 1.5'
	act "repeat one" audio-repeat '{"mode":"one"}' 's["audio"]["repeat"] == "one"'
	act "seek near the end (repeat one)" audio-seek '{"seconds":2.5}' 'r["ok"]'
	pb_wait "the seek lands" 's["audio"]["position"] > 2.4 and s["audio"]["track"] == 0'
	pb_wait "repeat one plays the track again" 's["audio"]["track"] == 0 and s["audio"]["playing"] and s["audio"]["position"] < 1.5'
	act "repeat cycles back to off" audio-repeat '' 's["audio"]["repeat"] == "off"'
	act "shuffle on" audio-shuffle '{"on":true}' 's["audio"]["shuffle"] and sorted(s["audio"]["order"]) == [0, 1, 2, 3] and s["audio"]["order"][0] == s["audio"]["track"] == 0'
	act "shuffle off" audio-shuffle '' 'not s["audio"]["shuffle"] and s["audio"]["order"] == [0, 1, 2, 3]'
	act "pause" audio-pause '' 's["audio"]["status"] == "paused" and not s["audio"]["playing"]'
	sleep 0.3
	AU_POS="$(field "$(state)" audio.position)"
	sleep 0.4
	act "the position holds while paused" state '' "s[\"audio\"][\"position\"] == $AU_POS and s[\"audio\"][\"status\"] == \"paused\""
	act "toggle resumes" audio-toggle '' 's["audio"]["playing"]'
	act "stop at once" audio-stop '{"fade":false}' 's["audio"]["status"] == "stopped" and not s["audio"]["loaded"] and s["audio"]["track"] == 0'
	act "a fade-out of 1.5 s" audio-fade-out '{"seconds":1.5}' 's["audio"]["fadeOut"] == 1.5'
	act "play for the fade" audio-play '' 's["audio"]["playing"]'
	pb_wait "playing before the fade" 's["audio"]["loaded"] and s["audio"]["position"] > 0.2'
	act "stop fades out" audio-stop '' 's["audio"]["fading"] and s["audio"]["status"] == "playing"'
	pb_wait "the volume ramps down" 's["audio"]["fading"] and 0 < s["audio"]["gain"] < 0.9'
	pb_wait "the fade ends stopped" 's["audio"]["status"] == "stopped" and not s["audio"]["fading"] and s["audio"]["gain"] == 1 and not s["audio"]["loaded"]'
	no_playlist_ffmpeg "the stopped playlist leaves no decoder running"
	# The phone remote: play / pause, next and previous track (internal/remote/actions.go).
	RPORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"
	RBASE="http://127.0.0.1:$RPORT"
	act "turn the remote on (audio)" remote "{\"on\":true,\"port\":$RPORT}" 's["remote"]["running"]'
	RCODE="$(field "$LAST" remote.code)"
	LAST="$("$RC" pair -base "$RBASE" -code "$RCODE" -name "Music Phone")"
	check "a phone asks to pair (audio)" "$LAST" 'r["ok"] and r["id"]'
	PAIR_ID="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["id"])' "$LAST")"
	PAIR_SECRET="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["secret"])' "$LAST")"
	act "approve the music phone" remote-approve '{"name":"Music Phone"}' 'len(s["remote"]["devices"]) == 1'
	RTOKEN="$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["token"])' "$("$RC" wait -base "$RBASE" -id "$PAIR_ID" -secret "$PAIR_SECRET")")"
	LAST="$("$RC" ws -base "$RBASE" -token "$RTOKEN" -do audio-toggle -do audio-next)"
	check "the phone plays the music and skips a track" "$LAST" 'r["ok"] and [x["action"] for x in r["replies"]] == ["audio-toggle", "audio-next"] and r["state"]["audio"] == {"title": "pl-2-hymn", "status": "playing", "tracks": 4}'
	act "the app plays the second track" state '' 's["audio"]["track"] == 1 and s["audio"]["playing"]'
	LAST="$("$RC" ws -base "$RBASE" -token "$RTOKEN" -do audio-prev -do audio-toggle)"
	check "the phone goes back a track and pauses" "$LAST" 'r["ok"] and r["state"]["audio"]["title"] == "pl-1-prelude" and r["state"]["audio"]["status"] == "paused"'
	act "remove the music phone" remote-revoke '{"name":"Music Phone"}' 's["remote"]["devices"] == []'
	act "turn the remote off (audio)" remote '{"on":false}' 'not s["remote"]["running"]'
	# Editing and the schedule file.
	act "move the last track to the top" playlist-move '{"from":3,"to":0}' '[t["title"] for t in s["audio"]["tracks"]] == ["pl-4-postlude", "pl-1-prelude", "pl-2-hymn", "pl-3-gone"] and s["audio"]["track"] == 1 and s["audio"]["status"] == "paused"'
	act "a move out of range is refused" playlist-move '{"from":0,"to":9}' 'not r["ok"]'
	act "remove the missing track" playlist-remove '{"index":3}' 'len(s["audio"]["tracks"]) == 3'
	act "repeat all and a 2 s fade-out for the file" audio-repeat '{"mode":"all"}' 's["audio"]["repeat"] == "all"'
	act "fade-out 2 s" audio-fade-out '{"seconds":2}' 's["audio"]["fadeOut"] == 2'
	act "play on (audio)" audio-toggle '' 's["audio"]["playing"]'
	wait_idle
	shot audio-playlist-playing main
	act "save the schedule with the playlist" save-schedule '{}' 'not s["schedule"]["dirty"]'
	python3 - "$AU_SCHED" <<'PY' || fail "the saved schedule has the playlist"
import json, sys
d = json.load(open(sys.argv[1]))
p = d["playlist"]
assert [t["title"] for t in p["tracks"]] == ["pl-4-postlude", "pl-1-prelude", "pl-2-hymn"], p
assert p["repeat"] == "all" and p["fadeOut"] == 2 and all(t["id"] and t["path"] and t["mediaId"] for t in p["tracks"]), p
PY
	log "ok: the saved schedule has the playlist"
	act "a new schedule has no playlist; the music stops" new-schedule '' 's["audio"]["tracks"] == [] and s["audio"]["status"] == "stopped"'
	pb_wait "the player closed" 'not s["audio"]["loaded"] and s["audio"]["status"] == "stopped"'
	act "open the schedule again" open-schedule "{\"path\":\"$AU_SCHED\"}" '[t["title"] for t in s["audio"]["tracks"]] == ["pl-4-postlude", "pl-1-prelude", "pl-2-hymn"] and s["audio"]["repeat"] == "all" and s["audio"]["fadeOut"] == 2 and s["audio"]["status"] == "stopped" and s["audio"]["track"] == 0 and not s["schedule"]["dirty"]'
	act "the reopened playlist plays" audio-play '' 's["audio"]["playing"] and s["audio"]["title"] == "pl-4-postlude"'
	pb_wait "the reopened playlist's first track plays" 's["audio"]["loaded"] and s["audio"]["position"] > 0.1'
	if [ "${PB_PLAYER:-}" = "ffmpeg" ]; then
		[ "$(playlist_ffmpeg)" -ge 1 ] || fail "the ffmpeg player runs no decoder for the playing track"
		log "ok: the ffmpeg player decodes the playing track"
	fi
	act "stop (audio)" audio-stop '{"fade":false}' 's["audio"]["status"] == "stopped"'
	no_playlist_ffmpeg "stop kills the track's decoder"
	act "Clear Playlist asks first" playlist-clear '' 'len(s["audio"]["tracks"]) == 3'
	wait_idle
	check "the clear question is up" "$LAST" 's["dialog"]["confirm"] == "Clear Playlist"'
	act "cancel the clear" dismiss '' 'len(s["audio"]["tracks"]) == 3 and s["focus"] != "dialog"'
	act "clear the playlist (confirmed)" playlist-clear '{"confirm":true}' 's["audio"]["tracks"] == [] and s["audio"]["track"] == -1 and s["schedule"]["dirty"]'
	for t in pl-1-prelude pl-2-hymn pl-3-gone pl-4-postlude; do
		act "remove $t from the library" remove-media "{\"title\":\"$t\",\"confirm\":true}" 'r["ok"]'
	done
	act "the library has its audio count back" state '' "s[\"media\"][\"audio\"] == $AU_AUDIO"
	act "save the schedule (audio)" save-schedule '{}' 'not s["schedule"]["dirty"]'
else
	log "audio playlist: playback skipped (ffmpeg=$FFMPEG native=$AU_NATIVE)"
fi

# ---- preview restyle (the Preview Output monitor's context menu) -----------------------------
# A right-click (Control-click on a Mac) on the Preview Output monitor restyles the item in
# Preview on its own (UI-SPEC §3.4): Change Theme ▸, Background ▸, Text ▸, Edit Theme for This
# Item…, Revert to Global Theme. The saved theme never changes; Live changes only when the item
# is sent live again. A #1f3f9f solid background, a #5a2a7f library image and #ffd400 text tell
# the looks apart. Regions: the Preview monitor's top band (x 0.26-0.53, y 0.375-0.41 of the
# main window) and the Live monitor's (x 0.66-0.94); the output's top-left corner.
PS_DIR="$OUT/preview-style"
rm -rf "$PS_DIR"
mkdir -p "$PS_DIR"
python3 - "$PS_DIR/smoke-style-bg.png" <<'PY'
import struct, sys, zlib
w, h = 320, 180
raw = b"".join(b"\x00" + bytes([0x5A, 0x2A, 0x7F]) * w for y in range(h))
def chunk(kind, data):
    return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF)
open(sys.argv[1], "wb").write(b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b""))
PY
act "Media tab (restyle)" tab '{"tab":"media"}' 's["tab"] == 2'
PS_MEDIA="$(field "$LAST" media.count)"
act "add the background image (restyle)" add-media "{\"path\":\"$PS_DIR/smoke-style-bg.png\"}" "s[\"media\"][\"count\"] == $PS_MEDIA + 1"
act "Songs tab (restyle)" tab '{"tab":"songs"}' 's["tab"] == 0'
act "search amazing (restyle)" search-songs '{"q":"amazing"}' 's["songs"]["results"] >= 1'
act "preview Amazing Grace from the library" select-song '{"title":"Amazing Grace"}' 's["preview"]["title"] == "Amazing Grace" and s["schedule"]["selected"] == -1 and not s["preview"]["style"]["custom"] and s["preview"]["style"]["background"] == "gradient" and s["preview"]["style"]["color"] == "#ffffff"'
PS_SIZE="$(field "$LAST" preview.style.size)"
PS_SAVED="$(post get-theme '{"name":"Default Song"}')"
check "the saved theme before" "$PS_SAVED" 'd["theme"]["background"]["kind"] == "gradient" and not d["theme"].get("custom")'
act "the live item before (restyle)" go-live '' 's["live"]["title"] == "Amazing Grace" and not s["live"]["style"]["custom"]'
# The menu, through Godot's input pipeline.
act "a right-click on the Preview monitor opens its menu" monitor-context-click '{"monitor":"preview"}' \
	's["dialog"]["previewMenu"] and [e["label"] for e in d["menu"]] == ["Change Theme", "Background", "Text", "Edit Theme for This Item…", "Revert to Global Theme"] and not d["menu"][0].get("disabled") and d["menu"][4].get("disabled")'
wait_idle
shot preview-menu preview-menu
pixel "the Preview menu has content" "$OUT/preview-menu.png" notblank
act "close the Preview menu" preview-menu '{"item":"close"}' 'not s["dialog"].get("previewMenu")'
if [ "$(uname)" = "Darwin" ]; then
	act "a Control-click opens it too" monitor-context-click '{"monitor":"preview","ctrl":true}' 's["dialog"]["previewMenu"]'
	act "close the Preview menu (Control-click)" preview-menu '{"item":"close"}' 'not s["dialog"].get("previewMenu")'
fi
act "a right-click on the Live monitor opens no menu" monitor-context-click '{"monitor":"live"}' 'not s["dialog"].get("previewMenu")'
act "Text ▸ shows the item's text style" preview-menu '{"item":"open","sub":"text"}' \
	'[e["label"] for e in d["text"]][:3] == ["Increase Font", "Decrease Font", "Font Color…"] and [e["label"] for e in d["text"] if e.get("checked")] == ["Bold", "Outline", "Shadow", "Align Center"] and d["text"][-1].get("disabled")'
wait_idle
shot preview-text-menu preview-text-menu
act "Change Theme ▸ checks the item's theme" preview-menu '{"item":"open","sub":"theme"}' '[e["label"] for e in d["theme"] if e.get("checked")] == ["Default Song"]'
wait_idle
shot preview-theme-menu preview-theme-menu
act "Background ▸" preview-menu '{"item":"open","sub":"background"}' '[e["label"] for e in d["background"]] == ["Select Media…", "Solid Color…", "Reset to Theme Background"]'
wait_idle
shot preview-background-menu preview-background-menu
act "close the Preview menu (submenus)" preview-menu '{"item":"close"}' 'not s["dialog"].get("previewMenu")'
# Background ▸ Solid Color… through the color dialog.
act "Solid Color… opens the color dialog" preview-menu '{"item":"background-color"}' 's["dialog"]["previewDialog"] == "color" and s["focus"] == "dialog"'
act "pick #1f3f9f" preview-color '{"color":"#1f3f9f"}' 's["dialog"]["previewDialog"] == "color"'
wait_idle
shot preview-color color
act "OK restyles the preview" accept '' 's["preview"]["style"]["custom"] and s["preview"]["style"]["background"] == "color" and s["preview"]["style"]["backgroundColor"] == "#1f3f9f" and not s["dialog"].get("previewDialog")'
check "Live keeps its look until sent live" "$LAST" 'not s["live"]["style"]["custom"] and s["live"]["style"]["background"] == "gradient"'
wait_idle
shot preview-solid main
pixel "the Preview monitor shows the solid background" "$OUT/preview-solid.png" has 1f3f9f 0.26 0.375 0.53 0.41 2000
pixel "the Live monitor does not" "$OUT/preview-solid.png" lacks 1f3f9f 0.66 0.375 0.94 0.41 50
# Text ▸ Font Color… and the size.
act "Font Color… #ffd400" preview-menu '{"item":"text-color","color":"#ffd400"}' 's["preview"]["style"]["color"] == "#ffd400" and s["preview"]["style"]["backgroundColor"] == "#1f3f9f"'
act "Increase Font" preview-menu '{"item":"font-bigger"}' "s[\"preview\"][\"style\"][\"size\"] == $PS_SIZE + 8"
act "Decrease Font" preview-menu '{"item":"font-smaller"}' "s[\"preview\"][\"style\"][\"size\"] == $PS_SIZE"
act "Increase Font again" preview-menu '{"item":"font-bigger"}' "s[\"preview\"][\"style\"][\"size\"] == $PS_SIZE + 8"
act "Shadow off" preview-menu '{"item":"shadow"}' 'not s["preview"]["style"]["shadow"] and s["preview"]["style"]["outline"]'
act "Shadow on" preview-menu '{"item":"shadow","on":true}' 's["preview"]["style"]["shadow"]'
act "the Reset entries are enabled now" preview-menu '{"item":"open","sub":"text"}' 'not d["text"][-1].get("disabled") and not d["background"][-1].get("disabled") and not d["menu"][4].get("disabled")'
act "close the Preview menu (reset entries)" preview-menu '{"item":"close"}' 'not s["dialog"].get("previewMenu")'
wait_idle
shot preview-text main
pixel "the Preview monitor shows the yellow text" "$OUT/preview-text.png" has ffd400 0.26 0.4 0.53 0.62 200
# Go Live carries the override.
act "the restyled library item goes live" go-live '' 's["live"]["style"] == s["preview"]["style"] and s["live"]["style"]["custom"] and s["live"]["title"] == "Amazing Grace"'
wait_idle
shot preview-style-output output
pixel "the output shows the solid background" "$OUT/preview-style-output.png" has 1f3f9f 0.0 0.0 0.2 0.08 5000
pixel "the output shows the yellow text" "$OUT/preview-style-output.png" has ffd400 0.1 0.25 0.9 0.75 1000
[ "$(post get-theme '{"name":"Default Song"}')" = "$PS_SAVED" ] || fail "the saved theme changed" "$(post get-theme '{"name":"Default Song"}')"
log "ok: the saved theme is unchanged"
act "another verse keeps the override (library deck)" select-song '{"title":"Amazing Grace"}' 's["preview"]["style"]["custom"] and s["preview"]["style"]["backgroundColor"] == "#1f3f9f"'
# Add to Schedule carries it; then the schedule item's own copy changes.
act "the restyled song goes into the schedule" add-to-schedule '{"source":"song"}' 's["schedule"]["items"][-1] == "Amazing Grace" and s["schedule"]["dirty"]'
PS_ROW=$(( $(python3 -c 'import json,sys; print(len(json.loads(sys.argv[1])["data"]["schedule"]["items"]))' "$LAST") - 1 ))
act "select the restyled schedule item" select-schedule "{\"index\":$PS_ROW}" "s[\"schedule\"][\"selected\"] == $PS_ROW and s[\"preview\"][\"style\"][\"custom\"] and s[\"preview\"][\"style\"][\"color\"] == \"#ffd400\""
act "send the schedule item live" go-live '' "s[\"schedule\"][\"live\"] == $PS_ROW"
act "save the schedule (restyle)" save-schedule '{}' 'not s["schedule"]["dirty"]'
PS_SCHED="$(field "$LAST" schedule.path)"
act "Select Media… opens the media picker" preview-menu '{"item":"background-media"}' 's["dialog"]["previewDialog"] == "media"'
act "choose the image in the picker" preview-media '{"title":"smoke-style-bg"}' 's["dialog"]["previewDialog"] == "media"'
wait_idle
shot media-picker media-picker
pixel "the media picker has content" "$OUT/media-picker.png" notblank
act "OK sets the item's background" accept '' 's["preview"]["style"]["background"] == "image" and s["preview"]["style"]["backgroundPath"].endswith("smoke-style-bg.png") and s["schedule"]["dirty"] and not s["dialog"].get("previewDialog")'
check "Live keeps the solid background" "$LAST" 's["live"]["style"]["backgroundColor"] == "#1f3f9f" and s["live"]["style"]["background"] == "color"'
wait_idle
shot preview-image main
shot preview-image-output output
pixel "the Preview monitor shows the library image" "$OUT/preview-image.png" has 5a2a7f 0.26 0.375 0.53 0.41 2000
pixel "the schedule thumbnail shows it too" "$OUT/preview-image.png" has 5a2a7f 0.0 0.1 0.2 0.45 50
pixel "the output keeps the solid background until sent live" "$OUT/preview-image-output.png" lacks 5a2a7f 0.0 0.0 0.2 0.08 50
act "send it live again" go-live '' 's["live"]["style"]["background"] == "image"'
wait_idle
shot preview-image-live output
pixel "now the output shows the image" "$OUT/preview-image-live.png" has 5a2a7f 0.0 0.0 0.2 0.08 5000
# The .fbs keeps the override.
act "save the restyled schedule" save-schedule '{}' 'not s["schedule"]["dirty"]'
python3 - "$PS_SCHED" "$PS_ROW" <<'PY' || fail "the schedule file does not keep the override"
import json, sys
t = json.load(open(sys.argv[1]))["items"][int(sys.argv[2])]["theme"]
assert t["custom"] and t["name"] == "Default Song", t
assert t["background"]["kind"] == "image" and t["background"]["path"].endswith("smoke-style-bg.png"), t["background"]
assert abs(t["font"]["color"]["G"] - 0xd4 / 255) < 0.01 and t["font"]["color"]["B"] == 0, t["font"]
PY
log "ok: the schedule file keeps the override"
act "reopen the schedule (restyle)" open-schedule "{\"path\":\"$PS_SCHED\"}" 'not s["schedule"]["dirty"]'
act "the reopened item keeps its look" select-schedule "{\"index\":$PS_ROW}" 's["preview"]["style"]["custom"] and s["preview"]["style"]["background"] == "image" and s["preview"]["style"]["color"] == "#ffd400"'
# Edit Theme for This Item…: the editor on the item's copy; OK writes no theme.
PS_THEMES="$(field "$LAST" themes.count)"
act "Edit Theme for This Item… opens the editor" preview-menu '{"item":"edit-theme"}' 's["dialog"]["themeEditor"] and s["dialog"]["previewDialog"] == "item-theme" and s["themes"]["editing"] == "Default Song"'
wait_idle
shot preview-item-editor theme-editor
pixel "the item theme editor has content" "$OUT/preview-item-editor.png" notblank
pixel "its live preview shows the item's image" "$OUT/preview-item-editor.png" has 5a2a7f 0.6 0.1 0.95 0.4 2000
act "OK keeps the item's look and saves no theme" accept '' "not s[\"dialog\"].get(\"themeEditor\") and s[\"themes\"][\"count\"] == $PS_THEMES and s[\"preview\"][\"style\"][\"background\"] == \"image\""
# preview-theme and the per-part resets.
act "preview-theme sets a size and the alignment" preview-theme '{"size":120,"align":"left"}' 's["preview"]["style"]["size"] == 120 and s["preview"]["style"]["align"] == "left"'
act "Reset to Theme Text keeps the background" preview-menu '{"item":"text-reset"}' "s[\"preview\"][\"style\"][\"size\"] == $PS_SIZE and s[\"preview\"][\"style\"][\"color\"] == \"#ffffff\" and s[\"preview\"][\"style\"][\"background\"] == \"image\" and s[\"preview\"][\"style\"][\"custom\"]"
act "Change Theme ▸ Default Scripture" preview-menu '{"item":"theme","name":"Default Scripture"}' 's["preview"]["theme"] == "Default Scripture" and not s["preview"]["style"]["custom"] and s["preview"]["style"]["align"] == "left"'
act "Change Theme ▸ Default Song" preview-menu '{"item":"theme","name":"Default Song"}' 's["preview"]["theme"] == "Default Song" and not s["preview"]["style"]["custom"]'
act "restyle once more" preview-theme '{"backgroundColor":"#1f3f9f"}' 's["preview"]["style"]["custom"]'
act "Revert to Global Theme" preview-menu '{"item":"revert"}' 'not s["preview"]["style"]["custom"] and s["preview"]["style"]["background"] == "gradient" and s["preview"]["style"]["color"] == "#ffffff" and s["schedule"]["dirty"]'
wait_idle
shot preview-reverted main
pixel "the reverted Preview has no solid background" "$OUT/preview-reverted.png" lacks 1f3f9f 0.26 0.375 0.53 0.41 50
pixel "nor the image" "$OUT/preview-reverted.png" lacks 5a2a7f 0.26 0.375 0.53 0.41 50
[ "$(post get-theme '{"name":"Default Song"}')" = "$PS_SAVED" ] || fail "the saved theme changed (end)"
log "ok: the saved theme is still unchanged"
# Media items bring their own look: the menu is disabled.
act "preview the image item" select-media '{"title":"smoke-style-bg"}' 's["preview"]["title"] == "smoke-style-bg"'
act "the menu is disabled for media" preview-menu '{"item":"open"}' 'all(e.get("disabled") for e in d["menu"])'
act "restyling a media item is refused" preview-menu '{"item":"font-bigger"}' 'not r["ok"]'
act "close the Preview menu (media)" preview-menu '{"item":"close"}' 'not s["dialog"].get("previewMenu")'
act "Songs tab (restyle end)" tab '{"tab":"songs"}' 's["tab"] == 0'
act "remove the restyled item" remove-schedule "{\"index\":$PS_ROW}" "len(s[\"schedule\"][\"items\"]) == $PS_ROW"
act "remove the background image" remove-media '{"title":"smoke-style-bg","confirm":true}' "s[\"media\"][\"count\"] == $PS_MEDIA"
act "save the schedule (restyle end)" save-schedule '{}' 'not s["schedule"]["dirty"]'

# ---- item menus (media and presentation tile menus) --------------------------------------------
# The right-click / gear menus of the media and presentation tiles (UI-SPEC §3.6), driven
# through `media-menu {item}` / `presentation-menu {item}`. A square image whose top quarter
# is #2a7f62 (green) over #7f2a62 (magenta) tells the aspect ratios apart on the projector:
# Fill crops the band to the top 5% with no bars, Fit letterboxes the square between black
# bars with the band a quarter high, Stretch keeps the band a quarter high with no bars. The
# counts are taken here: the themes block added images of its own since the media block.
act "Media tab (menus)" tab '{"tab":"media"}' 's["tab"] == 2'
MENU_MEDIA="$(field "$LAST" media.count)"
MENU_PRES="$(field "$LAST" presentations.count)"
# The fixtures live in their own folder, recreated here, so a file left behind by an aborted run
# never changes what the media block's folder scan finds.
MENU_DIR="$OUT/media-menus"
rm -rf "$MENU_DIR"
mkdir -p "$MENU_DIR"
python3 - "$MENU_DIR/smoke-square.png" <<'PY'
import struct, sys, zlib
w, h = 200, 200
green, magenta = bytes([0x2A, 0x7F, 0x62]), bytes([0x7F, 0x2A, 0x62])
raw = b"".join(b"\x00" + (green if y < h // 4 else magenta) * w for y in range(h))
def chunk(kind, data):
    return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF)
png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b"")
open(sys.argv[1], "wb").write(png)
PY
act "add the square image" add-media "{\"path\":\"$MENU_DIR/smoke-square.png\"}" "s[\"media\"][\"count\"] == $MENU_MEDIA + 1"
act "select the square: Fill and Loop by default" select-media '{"title":"smoke-square"}' 's["media"]["current"] == "smoke-square" and s["media"]["aspect"] == "fill" and s["media"]["repeat"] == "loop" and s["media"]["view"] == "medium" and s["media"]["sort"] == "name"'
act "square live (Fill)" go-live '' 's["live"]["title"] == "smoke-square"'
wait_idle
shot menu-fill-output output
pixel "Fill covers the canvas: no bars" "$OUT/menu-fill-output.png" has 7f2a62 0.05 0.35 0.18 0.65 2000
pixel "Fill crops the band to the top edge" "$OUT/menu-fill-output.png" has 2a7f62 0.4 0.0 0.6 0.045 200
pixel "Fill: no band below the top edge" "$OUT/menu-fill-output.png" lacks 2a7f62 0.4 0.12 0.6 0.22 50
act "Aspect Ratio ▸ Fit" media-menu '{"item":"aspect-fit"}' 's["media"]["aspect"] == "fit" and s["media"]["current"] == "smoke-square"'
act "Fit live" go-live '' 's["live"]["title"] == "smoke-square"'
wait_idle
shot menu-fit-output output
pixel "Fit letterboxes: black bars beside the square" "$OUT/menu-fit-output.png" has 000000 0.05 0.3 0.18 0.7 2000
pixel "Fit shows the whole image: the band is a quarter high" "$OUT/menu-fit-output.png" has 2a7f62 0.45 0.1 0.55 0.2 200
pixel "Fit centre" "$OUT/menu-fit-output.png" has 7f2a62 0.4 0.4 0.6 0.6 2000
act "Aspect Ratio ▸ Stretch" media-menu '{"item":"aspect-stretch"}' 's["media"]["aspect"] == "stretch"'
act "Stretch live" go-live '' 's["live"]["title"] == "smoke-square"'
wait_idle
shot menu-stretch-output output
pixel "Stretch fills the canvas: no bars" "$OUT/menu-stretch-output.png" has 7f2a62 0.05 0.35 0.18 0.65 2000
pixel "Stretch keeps the band a quarter high" "$OUT/menu-stretch-output.png" has 2a7f62 0.45 0.1 0.55 0.2 200
act "Aspect Ratio ▸ Fit by title" media-menu '{"item":"aspect-fit","title":"smoke-square"}' 's["media"]["aspect"] == "fit"'
act "Repeating ▸ Play Once" media-menu '{"item":"repeat-once"}' 's["media"]["repeat"] == "once"'
act "Repeating ▸ Loop" media-menu '{"item":"repeat-loop"}' 's["media"]["repeat"] == "loop"'
act "an unknown menu item is refused" media-menu '{"item":"aspect-zoom"}' 'not r["ok"]'
act "an unknown tile is refused" media-menu '{"item":"reveal","title":"zzz-nothing"}' 'not r["ok"]'
act "reload the list" search-media '{"q":""}' 's["media"]["current"] == "smoke-square"'
act "the aspect ratio is read back from the library" select-media '{"title":"smoke-square"}' 's["media"]["aspect"] == "fit" and s["media"]["repeat"] == "loop"'
# Use As Logo Background: the item (with its aspect ratio) becomes the logo screen.
act "Use As Logo Background (media)" media-menu '{"item":"logo-background"}' 's["settings"]["logo"]["kind"] == "image" and s["settings"]["logo"]["path"].endswith("smoke-square.png") and s["settings"]["logo"]["fit"] == "fit"'
act "logo mode shows it" logo '' 's["mode"] == "logo"'
wait_idle
shot menu-logo-output output
pixel "the logo screen shows the square" "$OUT/menu-logo-output.png" has 7f2a62 0.4 0.4 0.6 0.6 2000
pixel "the logo screen letterboxes like the slide, over the logo colour" "$OUT/menu-logo-output.png" has 1b2735 0.05 0.3 0.18 0.7 2000
act "logo off (menus)" logo '' 's["mode"] == "normal"'
# View ▸ and Sort by ▸ are state, persisted per tab, and survive a reload of the list.
act "View ▸ Small Thumbnails" media-menu '{"item":"view-small"}' 's["media"]["view"] == "small"'
act "View ▸ Details" media-menu '{"item":"view-details"}' 's["media"]["view"] == "details"'
wait_idle
shot media-details main
pixel "the media Details list has content" "$OUT/media-details.png" notblank
act "View ▸ Large Thumbnails" media-menu '{"item":"view-large"}' 's["media"]["view"] == "large"'
act "Sort by ▸ Date Added puts the newest item first" media-menu '{"item":"sort-date"}' 's["media"]["sort"] == "date" and s["media"]["selected"] == 0 and s["media"]["current"] == "smoke-square"'
act "Sort by ▸ Kind" media-menu '{"item":"sort-kind"}' 's["media"]["sort"] == "kind" and s["media"]["current"] == "smoke-square"'
act "Sort by ▸ Name" media-menu '{"item":"sort-name"}' 's["media"]["sort"] == "name"'
act "view and sort survive a reload of the list" search-media '{"q":""}' 's["media"]["view"] == "large" and s["media"]["sort"] == "name" and s["media"]["current"] == "smoke-square"'
act "View ▸ Medium Thumbnails" media-menu '{"item":"view-medium"}' 's["media"]["view"] == "medium"'
# Update items in Schedule: the schedule keeps its snapshot until asked.
act "add the square to the schedule" add-to-schedule '{"source":"media"}' 's["schedule"]["items"][-1] == "smoke-square"'
SQ_ROW=$(( $(python3 -c 'import json,sys; print(len(json.loads(sys.argv[1])["data"]["schedule"]["items"]))' "$LAST") - 1 ))
act "Aspect Ratio ▸ Fill (the schedule keeps its Fit snapshot)" media-menu '{"item":"aspect-fill"}' 's["media"]["aspect"] == "fill"'
act "select the square's schedule item" select-schedule "{\"index\":$SQ_ROW}" "s[\"preview\"][\"title\"] == \"smoke-square\" and s[\"schedule\"][\"selected\"] == $SQ_ROW"
act "the snapshot live" go-live '' "s[\"schedule\"][\"live\"] == $SQ_ROW"
wait_idle
shot menu-snapshot-output output
pixel "the snapshot still letterboxes (Fit)" "$OUT/menu-snapshot-output.png" has 000000 0.05 0.3 0.18 0.7 2000
act "Update items in Schedule" media-menu '{"item":"update-schedule","title":"smoke-square"}' "s[\"schedule\"][\"dirty\"] and s[\"preview\"][\"title\"] == \"smoke-square\" and s[\"schedule\"][\"selected\"] == $SQ_ROW"
act "the updated item live" go-live '' "s[\"schedule\"][\"live\"] == $SQ_ROW"
wait_idle
shot menu-updated-output output
pixel "the updated snapshot fills the canvas (Fill)" "$OUT/menu-updated-output.png" has 7f2a62 0.05 0.35 0.18 0.65 2000
act "black (menus)" black '' 's["mode"] == "black"'
act "black off (menus)" black '' 's["mode"] == "normal"'
# Refresh: a file that went missing is dropped after a question.
cp "$MEDIA_DIR/smoke-green.png" "$MENU_DIR/smoke-gone.png"
act "add a file that will go missing" add-media "{\"path\":\"$MENU_DIR/smoke-gone.png\"}" "s[\"media\"][\"count\"] == $MENU_MEDIA + 2"
rm "$MENU_DIR/smoke-gone.png"
act "Refresh asks about the missing file" media-menu '{"item":"refresh"}' 'r["ok"]'
wait_idle
check "the refresh question is up" "$LAST" 's["dialog"]["confirm"] == "Refresh Media"'
shot refresh-confirm dialog
act "cancel the refresh" dismiss '' "s[\"focus\"] != \"dialog\" and s[\"media\"][\"count\"] == $MENU_MEDIA + 2"
act "Refresh (confirmed) drops the missing file" media-menu '{"item":"refresh","confirm":true}' "s[\"media\"][\"count\"] == $MENU_MEDIA + 1 and s[\"media\"][\"current\"] == \"smoke-square\""
act "Refresh with nothing missing asks nothing" media-menu '{"item":"refresh"}' "s[\"media\"][\"count\"] == $MENU_MEDIA + 1 and not s[\"dialog\"].get(\"confirm\")"
# Delete File…: the file goes to the Trash (macOS; elsewhere the item is left out of the menu).
if [ "$(uname)" = "Darwin" ]; then
	cp "$MEDIA_DIR/smoke-green.png" "$MENU_DIR/smoke-trash.png"
	act "add a file to delete" add-media "{\"path\":\"$MENU_DIR/smoke-trash.png\"}" "s[\"media\"][\"count\"] == $MENU_MEDIA + 2"
	act "Delete File… asks first" media-menu '{"item":"delete-file","title":"smoke-trash"}' 'r["ok"]'
	wait_idle
	check "the delete question is up" "$LAST" 's["dialog"]["confirm"] == "Delete File"'
	shot delete-file-confirm dialog
	act "cancel the delete" dismiss '' 's["focus"] != "dialog"'
	[ -f "$MENU_DIR/smoke-trash.png" ] || fail "Delete File… removed the file before OK"
	act "Delete File… (confirmed) trashes the file and drops the item" media-menu '{"item":"delete-file","title":"smoke-trash","confirm":true}' "s[\"media\"][\"count\"] == $MENU_MEDIA + 1"
	[ ! -e "$MENU_DIR/smoke-trash.png" ] || fail "Delete File… left the file in place"
	log "smoke-trash.png is in the Trash"
	# Keep the user's Trash free of fixtures from repeated runs (only our own file name).
	rm -f "$HOME/.Trash/smoke-trash"*.png 2>/dev/null || true
else
	act "Delete File… is unavailable here" media-menu '{"item":"delete-file","title":"smoke-square","confirm":true}' 'not r["ok"]'
fi
# The presentation tile menu.
act "Presentations tab (menus)" tab '{"tab":"presentations"}' 's["tab"] == 3'
if [ "$PRES_PDF" = "True" ]; then
	act "add the PDF again" add-presentation "{\"path\":\"$PRES_DIR/smoke-deck.pdf\"}" "s[\"presentations\"][\"count\"] == $MENU_PRES + 1"
	wait_presentations
	act "select the PDF (menus)" select-presentation '{"title":"smoke-deck"}' 's["presentations"]["current"] == "smoke-deck" and s["presentations"]["currentPages"] == 2 and s["presentations"]["view"] == "medium" and s["presentations"]["sort"] == "name"'
	act "Use As Logo Background (presentation): page 1" presentation-menu '{"item":"logo-background"}' 's["settings"]["logo"]["kind"] == "image" and s["settings"]["logo"]["path"].endswith(".png")'
	act "logo mode shows page 1" logo '' 's["mode"] == "logo"'
	wait_idle
	shot menu-logo-pdf-output output
	pixel "the logo screen shows the PDF's first page" "$OUT/menu-logo-pdf-output.png" has 2a5fbf 0.25 0.25 0.75 0.75 10000
	act "logo off (presentation menus)" logo '' 's["mode"] == "normal"'
	act "View ▸ Details (presentations)" presentation-menu '{"item":"view-details"}' 's["presentations"]["view"] == "details"'
	wait_idle
	shot presentations-details main
	pixel "the presentations Details list has content" "$OUT/presentations-details.png" notblank
	act "View ▸ Small Thumbnails (presentations)" presentation-menu '{"item":"view-small"}' 's["presentations"]["view"] == "small"'
	act "Sort by ▸ Date Added puts the newest deck first" presentation-menu '{"item":"sort-date"}' 's["presentations"]["sort"] == "date" and s["presentations"]["selected"] == 0'
	act "Sort by ▸ Kind (presentations)" presentation-menu '{"item":"sort-kind"}' 's["presentations"]["sort"] == "kind" and s["presentations"]["current"] == "smoke-deck"'
	act "the presentation view and sort survive a reload" search-presentations '{"q":""}' 's["presentations"]["view"] == "small" and s["presentations"]["sort"] == "kind"'
	act "Sort by ▸ Name (presentations)" presentation-menu '{"item":"sort-name"}' 's["presentations"]["sort"] == "name"'
	act "View ▸ Medium Thumbnails (presentations)" presentation-menu '{"item":"view-medium"}' 's["presentations"]["view"] == "medium"'
	act "Remove from Library (menu) asks first" presentation-menu '{"item":"remove","title":"smoke-deck"}' 'r["ok"]'
	wait_idle
	check "the remove question is up (menu)" "$LAST" 's["dialog"]["confirm"] == "Remove Presentation"'
	act "cancel the removal (menu)" dismiss '' 's["focus"] != "dialog"'
	act "Remove from Library (menu, confirmed)" presentation-menu '{"item":"remove","title":"smoke-deck","confirm":true}' "s[\"presentations\"][\"count\"] == $MENU_PRES"
else
	log "no PDF renderer: skipping the presentation logo check"
fi
cp "$PRES_DIR/smoke-slides.pptx" "$MENU_DIR/smoke-gone.pptx"
act "add a deck that will go missing" add-presentation "{\"path\":\"$MENU_DIR/smoke-gone.pptx\"}" "s[\"presentations\"][\"count\"] == $MENU_PRES + 1"
wait_presentations
rm "$MENU_DIR/smoke-gone.pptx"
act "Refresh asks about the missing deck" presentation-menu '{"item":"refresh"}' 'r["ok"]'
wait_idle
check "the presentation refresh question is up" "$LAST" 's["dialog"]["confirm"] == "Refresh Presentations"'
act "cancel the presentation refresh" dismiss '' "s[\"focus\"] != \"dialog\" and s[\"presentations\"][\"count\"] == $MENU_PRES + 1"
act "Refresh (confirmed) drops the missing deck" presentation-menu '{"item":"refresh","confirm":true}' "s[\"presentations\"][\"count\"] == $MENU_PRES"
act "a presentation menu item needs a tile" presentation-menu '{"item":"reveal"}' 'not r["ok"]'
act "default logo again" set-setting '{"key":"logo","value":""}' 'not s["settings"]["logo"].get("path", "").endswith("smoke-square.png") and s["settings"]["logo"].get("path", "").endswith("FreeBeams.png")'
act "Songs tab (menus)" tab '{"tab":"songs"}' 's["tab"] == 0'
act "save the schedule (menus)" save-schedule '{}' 'not s["schedule"]["dirty"]'
# Open File Location last: the Finder window it opens takes the focus.
act "Open File Location shows the square in the Finder" media-menu '{"item":"reveal","title":"smoke-square"}' 'r["ok"] and "file location" not in s["lastError"]'
# The Finder window is now frontmost; an occluded Godot window stops redrawing (screenshots freeze), so come back.
sleep 1
osascript -e 'tell application "Godot" to activate' >/dev/null 2>&1 || true
sleep 1

# ---- quit ------------------------------------------------------------------------------------
LAST="$(post close)"
log "closing the window (clean schedule)"
for _ in $(seq 1 60); do
	app_running || break
	sleep 0.5
done
app_running && fail "the app did not exit after quit"

# ---- keepalive walk ---------------------------------------------------------------------
# graphics.gd walks every extension instance's fields each frame, on the main
# thread and without a lock, following pointers, maps, slices and interfaces.
# A map a goroutine writes meanwhile kills the app ("concurrent map iteration
# and map write"), so services must sit behind a keepalive.Hidden. The check
# reports what the walk actually compiled during this run, including the
# dynamic types it found behind interfaces.
KA="$(grep -E '\[keepalive-check\]' "$LOG" || true)"
if [ -n "$KA" ]; then
	printf '%s\n' "$KA" >&2
	fail "graphics.gd's keepalive walk reaches state that goroutines mutate (hide it with keepalive.Hidden; see internal/keepalive)"
fi
[ -s "$KEEPALIVE_TYPES" ] || fail "the keepalive check wrote no type list to $KEEPALIVE_TYPES"
log "ok: the keepalive walk reaches nothing goroutines mutate ($(wc -l <"$KEEPALIVE_TYPES" | tr -d ' ') walked types in $KEEPALIVE_TYPES)"

# ---- log hygiene ------------------------------------------------------------------------
BAD="$(grep -nE 'ERROR|WARNING|leaked|Leaked|panic:|SCRIPT ERROR' "$LOG" || true)"
if [ -n "$BAD" ]; then
	printf '%s\n' "$BAD" >&2
	fail "the app log has errors or warnings"
fi
grep -q 'shutting down' "$LOG" || fail "no clean shutdown in the log"
log "PASS (screenshots in $OUT)"
