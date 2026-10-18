# Phase 1 status

Status of every acceptance criterion in [PHASE1.md](PHASE1.md), and how each was verified. Recorded 2026-10-18 on
macOS (Apple M3 Max, one built-in Retina display, EasyWorship 8 data installed at `/Users/Shared/Softouch`).

Legend: **pass**: verified. **partial**: works, but part of the criterion is not met or not verified.
**fail**: not met. **untested**: implemented but not verifiable on this machine.

How to reproduce:
```bash
gofmt -l . ; go build ./... && go vet ./... && go test -count=1 ./...
test/smoke.sh          # 125 checks; screenshots and app.log in test/out/
```
In this document, "smoke: X" names a check that `test/smoke.sh` prints as `ok: X`. Screenshots that `test/smoke.sh`
captured are copied to `docs/screens/app-*.png`.

## Build & quality

| Criterion | Status | Verification |
|---|---|---|
| `go build`, `go vet`, `go test ./...` pass, no skipped core tests | **pass** | All packages pass. `go test -count=1 -v ./... \| grep -c -- '--- SKIP'` = 0 (the EasyWorship data tests run, because the data is installed). `gofmt -l .` is empty. |
| `gd run` launches with no Go panics or Godot errors | **pass** | `test/smoke.sh` runs the app with `gd run` and fails if `test/out/app.log` contains `ERROR`, `WARNING`, `leaked` or `panic:`. The final log has only the Godot banner and app lines, with no boilerplate warnings at all. The shutdown path (output closed, `render.ReleaseCaches()`, store closed) leaves no leak messages. |
| `models`, `store`, `importers` never import graphics.gd | **pass** | `go list -deps ./internal/models/... ./internal/store/... ./internal/importers/... \| grep -c ^graphics.gd` = 0. The new `internal/app/core` package also has 0. |
| Table-driven unit tests for the core logic | **pass** | Existing: `models` (lyrics, references, presenter, schedule round-trip), `store` (CRUD/search), `importers/easyworship` (`TestRealSongs`, `TestRealKJV`, `TestRealImportInto`, … skip cleanly without data). New in `internal/app/core`: `TestHotkeys` (24 table cases incl. digit timeout with an injected clock), `TestItemDeck`, `TestBuildState`, `TestGeneration`, `TestStepIndex`, `TestLiveItemIndex`, and others. |

## Operator window

| Criterion | Status | Verification |
|---|---|---|
| 58px toolbar: New, Open, Save \| Go Live \| Logo, Black, Clear \| Live, own icons | **pass** | Screenshots `app-main-*.png`. Toggles reflect state: Black is highlighted in `app-main-black.png`, and Live is highlighted while the output is open. Web, Remote and Alerts are shown disabled (Phase 2). |
| Schedule \| Preview \| Live on top, resources below, draggable splitters at the spec percentages | **partial** | The layout matches `ui-main-dark.png` (screenshots). The splitter percentages are the `ui` package's: main 71%, schedule 19.44%, slide grid 46.38%. **Deviation:** Preview/Live is 50%, not EasyWorship's 71.19% (a deliberate `ui` choice, documented in `mainwindow.go`). The app does not drive splitter dragging; the drag and snap logic is covered by `uicore` tests only. |
| 28px panel headers titled Schedule / Preview / Live | **pass** | Screenshots. |
| Resource tab bar: Songs, Scriptures, Media/Presentations/Themes ("Coming in Phase 2") | **pass** | Smoke: `Media tab`, then the screenshot `test/out/media-tab.png` shows "Coming in Phase 2". Songs and Scriptures appear in every main screenshot. |

## Songs

| Criterion | Status | Verification |
|---|---|---|
| First launch imports the EasyWorship library (223 songs); empty when EW is absent | **pass** | Smoke on a fresh temp library (`FREEWORSHIP_LIBRARY=$(mktemp)`): `import finished`, `EasyWorship songs imported (songCount >= 1)`. Log: `223 songs imported; Bibles: KJV, RVA; skipped HCSB (copyrighted)`. The summary is stored in the setting `import.easyworship.done`, so later launches skip the import. When EW is absent, the app logs "EasyWorship not installed; the library starts empty" (code path `startImport`; not run here, since EW is installed). |
| Songs tab: FTS search box, Title/Author/Copyright list, selecting shows the slides in Preview | **pass** | Smoke: `search amazing`, `select Amazing Grace`, `song row selected`, `selected song row is highlighted` (pixel check), and native typing into the search field (`typed N reached the search`). Screenshot `app-main-song-live.png`. Searches run in a goroutine; stale results are dropped (`TestGeneration`). |
| New/Edit dialog (title, author, copyright, CCLI, tagged lyrics); saving persists | **pass** (see note) | Smoke: `open the song editor` with `FormatLyrics` text (screenshot `app-song-editor.png`), then `save a new song (editor OK path)`, `the new song is searchable` (songCount + 1), `delete asks for confirmation` (screenshot `delete-confirm.png`), and `song deleted`. **Note:** `save-song` calls the same handler as the editor's OK button (`SongEditor.OnSave` → `saveSong`). The OK click itself is not automated. |

## Scriptures

| Criterion | Status | Verification |
|---|---|---|
| EW Bibles imported; only public-domain ones automatically; copyrighted (HCSB) skipped unless the user opts in | **partial** | KJV and RVA are imported, and HCSB is skipped as copyrighted (smoke `KJV imported`; import summary in the log). **Gap:** the opt-in exists only in the importer API (`easyworship.Options.IncludeCopyrightedBibles`). The app has no UI or setting for it yet (settings UI is Phase 2). |
| References `jn 3:16`, `John 3:16-18`, `1 cor 13`, `ps 23:1-3` show one verse per slide with a reference footer | **pass** | Smoke: `reference jn 3:16` (footer `(KJV)`), `reference John 3:16-18` (3 slides), `reference 1 cor 13` (13), `reference ps 23:1-3` (3, footer `Psalms 23:1 (KJV)`), `reference in another Bible` (RVA). Errors are shown in the panel status: `missing verses report an error` ("No verses found"), and `unparseable reference reports an error`. Screenshots `app-main-scripture-live.png`, `app-output-scripture.png`. |

## Presenting

| Criterion | Status | Verification |
|---|---|---|
| Double-click a slide or press Enter / Go Live: the item goes live on the clicked slide | **pass** (see note) | Smoke: `go live (crossfade running)`, `Enter in the preview grid sends that slide live` (a native key event goes to the grid's activate callback, the same one double-click uses), `Enter (no focus) goes live with the preview`, `scripture live`, `select schedule item 0` → `schedule item live`. **Note:** a mouse double-click is not automated. |
| Output: borderless full screen on the first non-primary display; a normal 16:9 window with one display | **partial: implemented, untested (no 2nd display)** | One display: smoke `output on` (outputOpen, not fullscreen); the output screenshots are 1920×1080 px (960×540 pt at 2×). The second-display path (`render.Output.Open(DefaultOutputScreen())`, borderless and covering the screen) opens automatically at startup when `len(Screens()) > 1`, but this machine has a single display. |
| Auto-fit text with outline and shadow; color, gradient or image backgrounds | **pass** | `app-output-song.png` and `app-output-scripture.png`: crisp 1080p text with outline and shadow on gradient themes. The logo shows an image background (`test/out/logo-output.png`, the first EW stock image). Auto-fit is unit-tested in `render/layout`. Smoke pixel checks: `song output has content`, `scripture output has content`, `logo output has content`. |
| 0.5s crossfade between live slides and into/out of Black/Clear/Logo | **pass** | Smoke: the go-live reply has `transitioning: true` (`go live (crossfade running)`, `native Ctrl+B` into black). The duration is `render.DefaultTransition` (500 ms) on the live monitor and the output. The render demo's mid-fade capture is `docs/screens/render-output-mid.png`. |
| Hotkeys (UI-SPEC §3.7): next/prev, first/last, schedule up/down, Ctrl+B/C/L, section jumps V/C/B/R/T/I/E | **pass** | `TestHotkeys` covers every key, ⌘ as well as Ctrl, the 1 s digit timeout (V then 2 = Verse 2, S 4 2 = slide 42), key repeat for arrows only, and focus rules. Smoke drives the control `key` path and real Godot key events (`native-key`): Right/Space/N, Backspace, Home/End, V+2, V+3, PgDn/PgUp, Down, Ctrl/⌘+B, Ctrl+L, Ctrl+C. Hotkeys are off in text fields (`N types into the search field`, `Cmd+C in a text field is not Clear`) and while a dialog is open (`hotkeys are off while a dialog is open`). |
| Live pane highlights the live slide; the schedule marks the live item with a red outline | **pass** | Screenshots `app-main-song-live.png` (live tile selected with a red outline) and `app-main-schedule.png` (red outline on the live schedule row). Smoke: `schedule item live` (`schedule.live == 0`), `PgDn: go live with the next item`. |

## Schedule

| Criterion | Status | Verification |
|---|---|---|
| Add to schedule from Songs/Scriptures, remove, move up/down, select an item into Preview | **pass** | Smoke: `add song to schedule`, `add scripture to schedule`, `select schedule item 0`, `move item`, `move back`, `remove schedule item`, `Down: next item into preview`. Items are snapshots (`TestSongDeckAndItem`). |
| Save/Open `.fws` (JSON) through native file dialogs; title shows the name and a dirty marker | **partial** | Save and open are tested end to end through the control API (`save schedule`, `new schedule`, `open schedule` → 2 items), and the title is checked (`title has a dirty marker`, `title without dirty marker` = "FreeWorship — smoke-schedule"). The toolbar's Open/Save use `DisplayServer.FileDialogShow` (`*.fws` filter; Save As when there is no path), but **the native modal dialogs are not exercised by automation**. Closing the window with unsaved changes asks first (smoke `closing with unsaved changes asks first`, screenshot `quit-confirm.png`). |

## Testability

| Criterion | Status | Verification |
|---|---|---|
| Control server: `GET /state`, `GET /screenshot?window=main\|output`, POST actions | **pass** | Used by every smoke step. Actions: `search-songs`, `select-song`, `reference`, `select-preview-slide`, `go-live`, `next`, `prev`, `first`, `last`, `jump`, `black`, `clear`, `logo`, `output`, `go-back`, `add-to-schedule`, `select-schedule`, `remove-schedule`, `move-schedule`, `save-schedule`, `open-schedule`, `new-schedule`, `key`, `wait-idle`, `quit`. UI-driving hooks: `native-key`, `focus`, `tab`, `editor`, `save-song`, `delete-song`, `dismiss`, `close`. Screenshot windows: `main`, `output`, `editor`, `dialog`. |
| `test/smoke.sh` drives a song and a scripture to live, saves screenshots, asserts `/state` | **pass** | 125 checks. Screenshots go to `test/out/` (git-ignored). The run ends through the real window-close path, and the script fails on any log error or warning. |
