# Phase 1 status

Status of every acceptance criterion in [PHASE1.md](PHASE1.md), and how each was verified. Recorded 2026-10-18 on
macOS (Apple M3 Max, one built-in Retina display, EasyWorship 8 data installed at `/Users/Shared/Softouch`).
Updated 2026-10-19 after the move to Godot 4.7.2 + graphics.gd 2026-10-14 (`bin/gd`), with a 1080p LG TV over HDMI as
the second display, the splitter-crash fix and the two formerly partial criteria (splitters, copyrighted Bibles) closed.

Legend: **pass**: verified. **partial**: works, but part of the criterion is not met or not verified.
**fail**: not met. **untested**: implemented but not verifiable on this machine.

How to reproduce:
```bash
gofmt -l . ; go build ./... && go vet ./... && go test -count=1 ./...
test/smoke.sh          # 302 checks (111 of them Phase 2: media, transitions and settings); screenshots and app.log in test/out/
test/splitters.sh      # 102 checks: every splitter dragged through Godot's input pipeline
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
| Schedule \| Preview \| Live on top, resources below, draggable splitters at the spec percentages | **pass** | The layout matches `ui-main-dark.png` and the mockup. Percentages: main 71%, schedule 19.44%, slide grids 46.38%, resources 85.42%; Preview \| Live defaults to 50% as in `docs/mockup/main-window.html` (EasyWorship's 71.19% is a design-time value; UI-SPEC §3.2 says so). Real drags go through Godot's input pipeline (`drag-split`): smoke `drag Preview \| Live left` / `back`, `Preview \| Live is back at 50%`, `drag the resource splitter left` / `back`; `test/splitters.sh` drags all six main splitters both ways, with bursts of motions per frame, and moves them through their snap zones (102 checks). Positions persist in the `ui.layout` setting and are restored at startup (`TestLayoutRoundTrip`; `/state.splitters`). See "Splitter crash" below. |
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
| EW Bibles imported; only public-domain ones automatically; copyrighted (HCSB) skipped unless the user opts in | **pass** | KJV and RVA are imported, and HCSB is skipped as copyrighted (smoke `KJV imported`; import summary in the log). The opt-in is the Scriptures gear menu's "Import Copyrighted Bibles from EasyWorship…" (enabled while EasyWorship has copyrighted Bibles the library lacks), which asks first (screenshot `test/out/import-bibles-confirm.png`) and then imports only Bibles (`easyworship.Options{BiblesOnly, IncludeCopyrightedBibles}`). Smoke: `HCSB is offered for import`, `import-bibles asks first`, `cancel the import`, `import the copyrighted Bibles` (HCSB listed, status "Imported HCSB"), `the imported Bible works`. Unit: `TestRealImportCopyrightedBiblesOnly`, `TestCopyrightedBiblesToImport`, `TestBibleImportSummary`. |
| References `jn 3:16`, `John 3:16-18`, `1 cor 13`, `ps 23:1-3` show one verse per slide with a reference footer | **pass** | Smoke: `reference jn 3:16` (footer `(KJV)`), `reference John 3:16-18` (3 slides), `reference 1 cor 13` (13), `reference ps 23:1-3` (3, footer `Psalms 23:1 (KJV)`), `reference in another Bible` (RVA). Errors are shown in the panel status: `missing verses report an error` ("No verses found"), and `unparseable reference reports an error`. Screenshots `app-main-scripture-live.png`, `app-output-scripture.png`. |

## Presenting

| Criterion | Status | Verification |
|---|---|---|
| Double-click a slide or press Enter / Go Live: the item goes live on the clicked slide | **pass** (see note) | Smoke: `go live (crossfade running)`, `Enter in the preview grid sends that slide live` (a native key event goes to the grid's activate callback, the same one double-click uses), `Enter (no focus) goes live with the preview`, `scripture live`, `select schedule item 0` → `schedule item live`. **Note:** a mouse double-click is not automated. |
| Output: full screen on the first non-primary display; a normal 16:9 window with one display | **pass** | Verified on a 1080p LG TV over HDMI (extended display), 2026-10-18. The output opens automatically at startup with `ExclusiveFullscreen` and covers the whole TV with no menu bar. The first approach (a borderless always-on-top window) left the macOS menu bar over the top of the output, so it was replaced (commit ba81789). Toggling Live off and on reopens it cleanly, and the operator display is unaffected (user-confirmed). One display: smoke `output on` (windowed 16:9). **2026-10-19:** `ExclusiveFullscreen` turned out to let the menu bar slide in at the top edge while the output has no focus, so the default is now `render.CoverScreen`: a borderless window raised above the menu bar level with Cocoa (`cover_darwin.go`; a failure to do so is logged, and the smoke log has no such line). The operator window drops vsync while the output is up, so a 120 Hz laptop does not pace the 60 Hz TV (`Output.syncToOutput`). The smoke and splitter suites ran with the TV as the output; a visual re-check of the menu bar after this change is still to be confirmed by eye. |
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
| Control server: `GET /state`, `GET /screenshot?window=main\|output`, POST actions | **pass** | Used by every smoke step. Actions: `search-songs`, `select-song`, `reference`, `select-preview-slide`, `go-live`, `next`, `prev`, `first`, `last`, `jump`, `black`, `clear`, `logo`, `output`, `go-back`, `add-to-schedule`, `select-schedule`, `remove-schedule`, `move-schedule`, `save-schedule`, `open-schedule`, `new-schedule`, `key`, `wait-idle`, `quit`. UI-driving hooks: `native-key` (`window: "output"` targets the output window), `focus`, `tab`, `editor`, `save-song`, `delete-song`, `dismiss` (topmost dialog), `accept` (topmost dialog's OK), `click-schedule`, `resource-go-live`, `close`, `split` and `drag-split` (move a named splitter programmatically or with a real mouse drag), `replay-input` (re-inject a `FREEWORSHIP_INPUT_LOG` recording), `import-bibles`. Screenshot windows: `main`, `output`, `editor`, `dialog`, `message`. `/state` also reports the open dialogs (`dialog`), the resource preview (`resource`), every splitter's position (`splitters`), the focused control (`focusOwner`) and the copyrighted Bibles available for import (`scripture.copyrightedAvailable`). |
| `test/smoke.sh` drives a song and a scripture to live, saves screenshots, asserts `/state` | **pass** | 302 checks (191 for Phase 1, plus the Phase 2 media, transitions and settings sections). Screenshots go to `test/out/` (git-ignored). The run ends through the real window-close path, and the script fails on any log error or warning. `test/splitters.sh` (102 checks) is the stress test for the layout crash below. |

## Review fixes

Fixes for the independent review's verified findings (2026-10-18). Every fix has a regression test: a unit test
(`go test ./...`) or a smoke check (`test/smoke.sh`, "smoke: X").

| # | Finding | Fix | Test |
|---|---|---|---|
| 1 | Backspace (a MacBook's "delete") changed the projector instead of removing an item | `core.Hotkeys` passes Backspace through to a focused list. The schedule removes the selected item and the song list asks to delete the song (like Delete). `App.UnhandledKeyInput` + `Hotkeys.Unhandled` turn a Backspace the list did not consume back into Prev (e.g. in the verse list). | `TestHotkeys` ("list keeps its navigation keys"), `TestHotkeysUnhandled`. Smoke: `Backspace in the schedule removes the item (not Prev)`, `Backspace in the song list asks to delete`, `Backspace in the verse list is Prev`, plus the existing `key backspace (prev)`. |
| 2 | Scripture Go Live sent the last clicked verse, not the highlighted one | One source of truth: `core.SelectedVerse` (Preview's selection while Preview holds the passage, else the last chosen row) drives the highlight, the resource preview (now derived, `scriptRes` removed) and Go Live (`goLiveVerse`). | `TestSelectedVerse`, `TestSameDeck`. Smoke: `resource Go Live sends the highlighted verse`, `Go Live again follows the preview` (and the `resource` caption checks). |
| 3 | Re-clicking the highlighted schedule row did nothing | Every Preview load goes through `App.setPreview`, so the schedule selection follows Preview (`core.ScheduleSelection`): library content clears it, and the next click on that row fires again. | `TestScheduleSelection`. Smoke (real mouse clicks via `click-schedule`): `click schedule row 0`, `preview library content`, `clicking row 0 again loads it again`. |
| 4 | Failed schedule save/open was silent | New `ui.MessageDialog` (OK only). `App.alert` queues the error and `render` projects the queue onto the dialog. Wired to schedule open/save, the native file dialogs, the EasyWorship import, and song load/save/delete. Dialogs opened over another dialog become its child (`MainWindow.dialogHost`), because Godot gives a window only one exclusive child. | Smoke: `opening a corrupt schedule fails`, `the error is shown in a dialog`, `error dialog has content` (screenshot `error-dialog.png`), `hotkeys are off while the error is shown`, `OK closes the error`. `TestBuildStateDialog`. |
| 5 | "Add to Schedule" after a failed lookup added the old passage | A failed or empty lookup (parse error, no Bible, DB error, no verses) calls `clearPassage`: passage, deck, rows and selection. Adding then does nothing and says so in the status. | Smoke: `the failed lookup cleared the passage`, `add-to-schedule after a failed lookup adds nothing`, `the schedule is unchanged and the status says why`. |
| 6 | Stale song loads still applied out of order | `core.Sequencer` applies song-load results in request order. A superseded preview is dropped; go live, add to schedule and edit still happen, in order. The control reply waits for its own result, and every ticket completes even if a load panics. | `TestSequencer`, `TestSequencerSupersededIntents`. |
| 7 | Save As kept the old title | `models.Schedule.SaveAs` names the schedule after the new file (`Save` is unchanged). The app uses it when `core.IsSaveAs` (another file than the current one), from the dialog or `save-schedule {path}`. | `TestScheduleSaveAs`, `TestIsSaveAs`. Smoke: `Save As renames the schedule`, `window title follows Save As`, `Save to the same file keeps the name`. |
| 8 | The import failure status was overwritten by the next search | The Songs footer comes from `core.SongsFooter`, which keeps "· EasyWorship import failed" next to the count. The details are shown in the error dialog (#4). | `TestSongsFooter`. |
| 9 | Quitting ignored the open song editor | `requestQuit` asks first when the editor is open. Cancel keeps it open. OK closes it and then asks about an unsaved schedule. | Smoke: `closing with the editor open asks first` (screenshot `quit-editor-confirm.png`), `Cancel keeps the editor open`, `OK discards the editor`, `then asks about the unsaved schedule`. |
| 10 | Hotkeys were dead while the output window had focus | `render.Output.OnInput` forwards the output window's `window_input` into the same hotkey path (`App.outputInput`). The main window's focus does not apply there, and an open dialog still disables hotkeys. | Smoke: `Left in the output window is Prev`, `Ctrl+B in the output window is Black`, `Cmd+B in the output window toggles back`, `Up in the output window is not the song list's Up`. |
| 11 | Fragile Tree signal guard in Songs/Scriptures render | `ui.quietly` blocks the Tree's signals around programmatic changes and flushes graphics.gd's call buffer before unblocking. It replaces the `rendering` flags. The other widgets were checked: they use `SetPressedNoSignal` or setters that do not emit. | Covered by the existing smoke selection checks (`select Amazing Grace`, `Down in the song list moves the list`, `select preview slide 2`). |
| 12 | Hand-made `.fws` files kept missing/duplicate item ids | `LoadSchedule` gives items without an id, or repeating an earlier id, a new unique id. | `TestLoadScheduleRepairsIDs`. |
| 13 | First grid tile drawn ~6px up-left after a deck change | Root cause: container sort handlers' buffered `FitChildInRect` calls were flushed inside a tile's own `Draw`. Godot moved the tile while it was drawing and dropped the redraw that refreshes its canvas transform. Sort handlers (`tileFlow`, `MonitorHost`, `Splitter`) now end with `flushCalls`. | Smoke pixel checks on `grid-deck-change.png`: `first preview tile is not drawn at the grid corner`, `first preview tile is at the grid padding`. (`pngcheck.py` gained `lacks`.) |
| 14 | Shutdown could stall on `Store.Close` | Async jobs are tracked. Shutdown cancels them and waits at most 3 s (`core.WaitTimeout`) before closing the library. `Store.Close` runs `PRAGMA optimize` on its own connection with a 500 ms busy timeout, because SQLite's busy handler ignores context cancellation. | `TestCloseIsBoundedWhileAnotherWriterHoldsTheLock` (10 s before, 0.5 s after), `TestWaitTimeout`. Smoke ends through the real close path. |

## Splitter crash (fixed 2026-10-19)

Dragging a splitter could take the app down with `Failed method: Container::_sort_children. Message queue out of
memory`, millions of `Object was deleted while awaiting a callback.` lines and a SIGSEGV. The crash reproduced under
automation with `drag-split` on the Preview | Live splitter (`test/splitters.sh`, which was written for it).

**Root cause:** a layout oscillation inside Godot, not a graphics.gd fault. The slide grid's content (`tileFlow`)
computed its minimum height from its own width. Inside a `ScrollContainer` that width depends on whether the vertical
scrollbar is showing, which depends on the content height. At widths where the tiles fit only without the scrollbar,
Godot toggled the scrollbar forever inside one message-queue flush (resize → re-sort → minimum size → resize …), each
round queuing more deferred calls, until the 32 MB queue overflowed. Counting Godot→Go callbacks per frame and dumping
the Go stack past 20,000 showed `tileFlow.GetMinimumSize` and the thumbnail `SlideView` resize handlers running
thousands of times in a single frame.

**Fix:** `SlideGrid.layoutWidth` lays the tiles out in the scroll area's width minus the vertical scrollbar, whether or
not it is showing, so the content height no longer depends on the scrollbar (`internal/ui/slidegrid.go`). The
`Callable.Defer`-based relayout tried in the meantime was not needed and was dropped.

**Also:** graphics.gd was moved from the June 2026 build to 2026-10-14 (Godot 4.7.2, via `bin/gd`): the older call ring
could corrupt a batch when a Go signal handler made engine calls during a flush (`crash in ring buffer flush at entry
N: FitChildInRect`), which is how the oscillation ended in a SIGSEGV on the old build. Both the smoke and the splitter
suites pass on the new build; the fix alone was not re-tested on the old one.

**Tests:** smoke `drag Preview | Live left` / `back` / `Preview | Live is back at 50%` / `drag the resource splitter
left` / `back`; `test/splitters.sh` (102 checks: six splitters × four deltas × two burst sizes, programmatic moves through
the snap zones, positions asserted via `/state.splitters`, clean shutdown, clean log; it samples the Godot process on a
hang).
