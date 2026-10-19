#!/usr/bin/env bash
# End-to-end smoke test: launches FreeWorship with `gd run` on a fresh temp
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
rm -f "$OUT"/*.png "$OUT"/*.fws
LOG="$OUT/app.log"
: >"$LOG"

LIB="$(mktemp -t freeworship-smoke)"
SCHED="$OUT/smoke-schedule.fws"
KEY="testkey"
EW_SONGS="/Users/Shared/Softouch/Easyworship/Default/v6.1/Databases/Data/Songs.db"
APP_PID=""
BASE=""

log() { printf '[smoke] %s\n' "$*"; }

app_pids() {
	# gd run builds the library and then runs Godot (as itself or a child).
	[ -n "$APP_PID" ] || return 0
	echo "$APP_PID"
	pgrep -P "$APP_PID" 2>/dev/null || true
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
	if [ -n "${2:-}" ]; then printf '[smoke] last response: %s\n' "$2" | head -c 2000 >&2; echo >&2; fi
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
log "launching: FREEWORSHIP_LIBRARY=$LIB bin/gd run"
(
	cd "$ROOT" || exit 1
	export FREEWORSHIP_LIBRARY="$LIB" FREEWORSHIP_CONTROL_KEY="$KEY"
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
check "window title shows the schedule" "$LAST" 's["windowTitle"].startswith("FreeWorship") and "Untitled" in s["windowTitle"]'
shot app-main-start main

act "output on" output '{"on":true}' 's["outputOpen"] and s["outputEnabled"]'

# ---- song ------------------------------------------------------------------------
act "search amazing" search-songs '{"q":"amazing"}' 's["songs"]["results"] >= 1 and s["songs"]["query"] == "amazing"'
act "select Amazing Grace" select-song '{"title":"Amazing Grace"}' '"Amazing Grace" in s["preview"]["title"] and s["preview"]["index"] == 0'
act "go live (crossfade running)" go-live '' 's["live"]["title"] == s["preview"]["title"] and "Amazing grace" in s["live"]["text"] and s["live"]["index"] == 0 and s["transitioning"]'
wait_idle
shot song-live-main main
shot song-live-output output
check "song row selected" "$LAST" 's["songs"]["current"] == "Amazing Grace" and s["songs"]["selected"] >= 0'
# The selected song row is drawn highlighted (highlight.2 #395792, unfocused list).
pixel "selected song row is highlighted" "$OUT/song-live-main.png" has 395792 0.22 0.76 0.58 0.95 30000
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
act "reference John 3:16-18" reference '{"text":"John 3:16-18"}' 's["scripture"]["verses"] == 3 and s["preview"]["count"] == 3'
act "select preview slide 2" select-preview-slide '{"i":2}' 's["preview"]["index"] == 2 and "18" in s["preview"]["label"] and s["scripture"]["selected"] == 2'
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
# first Preview tile (selected, #3a6dce) must sit at the grid padding, not
# at the grid's top-left corner.
wait_idle
shot grid-deck-change main
pixel "first preview tile is not drawn at the grid corner" "$OUT/grid-deck-change.png" lacks 3a6dce 0.196 0.1008 0.31 0.1067
pixel "first preview tile is at the grid padding" "$OUT/grid-deck-change.png" has 3a6dce 0.196 0.108 0.31 0.1114 500

# Review #2: the verse Go Live sends is the highlighted one, also after
# moving in the Preview grid (not the last clicked verse row).
act "the passage goes live at verse 16" go-live '{"slide":0}' 's["live"]["title"] == "John 3:16-18" and s["live"]["index"] == 0'
act "select preview slide 3 (John 3:18)" select-preview-slide '{"i":2}' 's["scripture"]["selected"] == 2 and s["resource"]["caption"] == "John 3:18 · 3 / 3"'
act "resource Go Live sends the highlighted verse" resource-go-live '' 's["live"]["title"] == "John 3:16-18" and s["live"]["index"] == 2 and s["live"]["label"] == "John 3:18" and s["scripture"]["selected"] == 2'
act "move the preview back to slide 2" select-preview-slide '{"i":1}' 's["scripture"]["selected"] == 1 and s["resource"]["caption"] == "John 3:17 · 2 / 3"'
act "Go Live again follows the preview" resource-go-live '' 's["live"]["index"] == 1 and s["live"]["label"] == "John 3:17"'
# Review #1: the verse list has no use for Backspace, so it is still Prev.
act "focus the verse list" focus '{"target":"verse-list"}' 's["focus"] == "list"'
act "Backspace in the verse list is Prev" native-key '{"name":"backspace"}' 's["live"]["index"] == 0 and s["schedule"]["items"] == []'
act "release focus (verses)" focus '{"target":"none"}' 's["focus"] == "none"'

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
act "save schedule" save-schedule "{\"path\":\"$SCHED\"}" 'not s["schedule"]["dirty"] and s["schedule"]["path"].endswith("smoke-schedule.fws") and s["schedule"]["title"] == "smoke-schedule"'
[ -s "$SCHED" ] || fail "schedule file $SCHED was not written"
check "title without dirty marker" "$LAST" 's["windowTitle"] == "FreeWorship — smoke-schedule"'
# Review #7: saving to another file is a Save As: the schedule takes its name.
act "Save As renames the schedule" save-schedule "{\"path\":\"$OUT/smoke-renamed.fws\"}" 's["schedule"]["title"] == "smoke-renamed" and s["schedule"]["path"].endswith("smoke-renamed.fws") and not s["schedule"]["dirty"]'
check "window title follows Save As" "$LAST" 's["windowTitle"] == "FreeWorship — smoke-renamed"'
act "Save to the same file keeps the name" save-schedule '{}' 's["schedule"]["title"] == "smoke-renamed"'
act "new schedule" new-schedule '' 's["schedule"]["items"] == [] and s["schedule"]["title"] == "Untitled"'
act "open schedule" open-schedule "{\"path\":\"$SCHED\"}" 'len(s["schedule"]["items"]) == 2 and s["schedule"]["items"] == ["Amazing Grace", "John 3:16"] and not s["schedule"]["dirty"] and s["schedule"]["title"] == "smoke-schedule"'
# Review #4: a failed open is shown to the operator (an error dialog), and
# the current schedule stays.
printf 'not a schedule {' >"$OUT/corrupt.fws"
act "opening a corrupt schedule fails" open-schedule "{\"path\":\"$OUT/corrupt.fws\"}" 'not r["ok"]'
wait_idle
check "the error is shown in a dialog" "$LAST" 's["focus"] == "dialog" and s["dialog"]["message"]["title"] == "Open Schedule" and "corrupt.fws" in s["dialog"]["message"]["message"] and s["schedule"]["title"] == "smoke-schedule" and len(s["schedule"]["items"]) == 2'
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
	check "video playability matches the Theora encoder" "$LAST" '(s["media"]["theora"] and s["media"]["playable"] == s["media"]["videos"]) or (not s["media"]["theora"] and s["media"]["playable"] == 0)'
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
act "save a new song (editor OK path)" save-song '{"title":"Smoke Test Song","author":"FreeWorship","lyrics":"[Verse 1]\nline one\nline two\n\n[Chorus]\nchorus line"}' 's["songs"]["current"] == "Smoke Test Song" and s["preview"]["title"] == "Smoke Test Song" and s["preview"]["count"] == 2'
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
check "closing with the editor open asks first" "$LAST" 's["dialog"]["confirm"] == "Quit FreeWorship" and s["dialog"]["editor"]'
app_running || fail "the app quit with the song editor open"
shot quit-editor-confirm dialog
act "Cancel keeps the editor open" dismiss '' 's["dialog"]["editor"] and not s["dialog"].get("confirm")'
# OK discards the editor, then asks about the unsaved schedule (the question
# moves from over the editor to over the main window).
act "close the window again" close '' 'r["ok"]'
wait_idle
check "asks about the editor again" "$LAST" 's["dialog"]["confirm"] == "Quit FreeWorship" and s["dialog"]["editor"]'
act "OK discards the editor" accept '' 'r["ok"]'
wait_idle
check "then asks about the unsaved schedule" "$LAST" 's["dialog"]["confirm"] == "Quit FreeWorship" and not s["dialog"]["editor"] and s["schedule"]["dirty"]'
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
act "opening a corrupt schedule fails again" open-schedule "{\"path\":\"$OUT/corrupt.fws\"}" 'not r["ok"]'
wait_idle
check "the error dialog is on the main window" "$LAST" 's["dialog"]["message"]["title"] == "Open Schedule" and s["dialog"]["host"] == "main" and s["outputOpen"]'
shot output-during-error output
pixel "the output is unchanged by the error dialog" "$OUT/output-before-error.png" same "$OUT/output-during-error.png"
pixel "the output still shows the slide" "$OUT/output-during-error.png" has 1f3f9f 0.25 0.25 0.75 0.75 10000
act "dismiss the error" dismiss '' 'not s["dialog"].get("message") and s["dialog"].get("host", "") == ""'
# Menus through the control API.
act "Help ▸ About" menu '{"item":"about"}' 's["dialog"]["message"]["title"] == "About FreeWorship" and s["dialog"]["host"] == "main"'
act "close About" dismiss '' 'not s["dialog"].get("message")'
act "unknown menu item is rejected" menu '{"item":"bogus"}' 'not r["ok"]'
act "File ▸ New Schedule (clean schedule, no question)" menu '{"item":"new-schedule"}' 's["schedule"]["title"] == "Untitled" and not s["schedule"]["dirty"] and s["focus"] != "dialog"'
act "open the saved schedule again" open-schedule "{\"path\":\"$SCHED\"}" 'not s["schedule"]["dirty"] and s["schedule"]["title"] == "smoke-schedule"'
act "Songs tab again" tab '{"tab":"songs"}' 's["tab"] == 0'

# ---- quit ------------------------------------------------------------------------------------
LAST="$(post close)"
log "closing the window (clean schedule)"
for _ in $(seq 1 60); do
	app_running || break
	sleep 0.5
done
app_running && fail "the app did not exit after quit"

# ---- log hygiene ------------------------------------------------------------------------
BAD="$(grep -nE 'ERROR|WARNING|leaked|Leaked|panic:|SCRIPT ERROR' "$LOG" || true)"
if [ -n "$BAD" ]; then
	printf '%s\n' "$BAD" >&2
	fail "the app log has errors or warnings"
fi
grep -q 'shutting down' "$LOG" || fail "no clean shutdown in the log"
log "PASS (screenshots in $OUT)"
