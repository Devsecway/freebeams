# Phase 2 status

Status of Phase 2 (PLAN.md §4, **M4: Media & Themes**), recorded 2026-10-19 on macOS (Apple M3 Max, built-in
Retina display plus a 1080p TV over HDMI, EasyWorship 8 data at `/Users/Shared/Softouch`, Homebrew ffmpeg 8.1.1
**without** the `libtheora` encoder). This document covers the Media milestone; the rest of Phase 2 is listed at
the end.

Legend as in [PHASE1-STATUS.md](PHASE1-STATUS.md): **pass**, **partial**, **fail**, **untested**.

How to reproduce:
```bash
gofmt -l . ; go build ./... && go vet ./... && go test -count=1 ./...
test/smoke.sh          # 302 checks; screenshots and app.log in test/out/
```
"smoke: X" names a check that `test/smoke.sh` prints as `ok: X`.

## Media library

| Criterion | Status | Verification |
|---|---|---|
| `media` table (path, kind, duration, size, thumbnail, transcode, title, added) with CRUD and search | **pass** | Migration 2 (`internal/store/migrations.go`), `internal/store/media.go`. `TestMediaCRUD`, `TestSaveMediaItemsAtomic`, `TestSearchMedia` (LIKE wildcards are literal, kind filter, limit, counts). |
| Folder scan for png/jpg/jpeg/webp/bmp and mp4/mov/m4v/ogv/webm | **pass** | `media.Scan` (recursive, hidden entries skipped, single files accepted). `TestScan`, `TestMediaKindOf`. Smoke: `add a media folder` (a `.txt` in the folder is ignored), `adding the folder again adds nothing`. |
| `ffprobe` metadata, `ffmpeg` poster frames and MP4→OGV transcode behind interfaces; tests run without ffmpeg | **pass** | `media.Tools` implements `Prober`, `Thumbnailer`, `Transcoder`; `media.Processor` runs Describe / Thumbnail / Transcode. Unit tests use a fake (`TestProcessorVideoWithFakeTools`) and a zero `Tools` (`TestProcessorStillsWithoutTools`); `TestRealFFmpeg` generates a 2 s test clip with ffmpeg, probes it, takes a poster frame and transcodes it when libtheora is present (it skips the transcode, asserting `ErrNoTheora`, when it is not). |
| Stills work fully without ffmpeg | **pass** | PNG/JPEG dimensions and thumbnails are made in Go (`image.go`: box-filter downscale to 640 px, atomic JPEG write). WebP/BMP thumbnails need ffmpeg; Godot still displays the originals. |
| Graceful degradation without ffmpeg / without a Theora encoder | **pass** | `media.Detect` reports the tools (`/state` → `media.tools`, `media.ffmpeg`, `media.theora`). Videos are listed with their poster frame and the badge `Needs ffmpeg` / `Needs libtheora`; the footer says why they cannot play. Sending one live shows the poster. Smoke on this machine: `video live (playing, or its poster frame)`, `video playability matches the Theora encoder` (0 playable, no Theora). |
| EasyWorship stock media registered on first launch (`Media.db` + `Resources/{Images,Videos}`) | **pass** | `easyworship.ReadMedia` / `ImportMedia`; `ImportInto` covers media. Libraries that finished the Phase 1 import register media once (`import.easyworship.media.done`). `TestReadMediaSynthetic`, `TestImportMediaSynthetic`, `TestRealMedia` (23 items: 8 videos at 1280×720 with durations, 15 images at 1920×1080). Smoke: `EasyWorship media imported (8 videos, 15 images)`; log: `… 23 media items`. |
| Thumbnails and transcodes are made off the main thread | **pass** | `App.startMediaWork` runs one pass per request in a goroutine: metadata + thumbnails first, then transcodes one at a time; results are posted per item and applied in `Process`. ffmpeg is killed through the context at shutdown. Smoke: `media work finished`, `every listed item has a thumbnail (ffmpeg present)`. |

## Media tab (UI)

| Criterion | Status | Verification |
|---|---|---|
| `Media` tab replaces the placeholder: search box, Videos / Images collections, thumbnail grid, Add to Schedule | **pass** | `ui.MediaPanel` (dumb view: setters + `On…` callbacks) with a `SlideGrid` of 16:9 tiles (title + badge), footer `+▾` (Add Media Files… / Folder… through the native dialogs), `⚙▾` and context menu (Add to Schedule, Go Live, Remove from Library), view sizer. Screenshots `test/out/media-tab.png`, `media-video-main.png`, `media-green-main.png`. Smoke: `Media tab`, `media tab has content`, `search media`, `filter images`, `filter videos`, `all media`. |
| Selecting an item shows it in the resource preview | **pass** | `App.resourcePreview` (`TabMedia`) → `MonitorHost` slot. Smoke: `select the first image` (`resource.title == media.current`), `select the synthetic image`. |
| Keyboard: the grid takes arrows / Enter like the Preview grid | **pass** | `focus media-grid` → `FocusGrid`. Smoke: `Left in the media grid moves the selection`, `Right moves it back`, `Enter in the media grid sends the item live`. |
| Thumbnails: Godot `Image` for stills, ffmpeg poster frames for videos, generated off the main thread | **pass** | The pipeline writes 640 px JPEGs to the cache dir (`~/Library/Caches/FreeWorship/media`, `FREEWORSHIP_CACHE` overrides); the grid's thumbnails are `render.SlideView`s showing the thumbnail file (`core.ThumbBackground`). New posters rebuild the tiles (the thumbnail path is part of `ui.MediaRow`). |
| Remove from library asks first; the file is not deleted | **pass** | Smoke: `remove asks for confirmation`, `cancel the removal`, `remove the synthetic image`, `the schedule still has the media item`. |

## Presenting media

| Criterion | Status | Verification |
|---|---|---|
| An image item live = full-screen image on the output | **pass** | Smoke: `image live`, `image output has content`; a synthetic solid `#2a7f62` image: `the output shows the image` (exact pixel check on the projector screenshot), `the live monitor shows the image`. Screenshots `media-green-output.png`, `media-green-main.png`. |
| Media items in the schedule (snapshot, portable) | **pass** | `core.MediaItem` stores the source path and poster; `core.ResolveDeck` substitutes the playable file when the item is selected. Smoke: `add media to schedule`, `select the media schedule item`, `media schedule item live`. `TestMediaDeckAndItem`, `TestResolveDeck`, `TestMediaScheduleItemRoundTrip` (.fws JSON keeps `mediaId` and `poster`). |
| Looping motion background (`models.BackgroundVideo`) behind the text, muted, low-processor mode off while playing | **untested** | Implemented in `internal/render/video.go`: a `VideoStreamPlayer` per layer between the background texture and the text, `loop`, `-80 dB`, `AcquireAnimating` on play and `ReleaseAnimating` when the layer changes background, is hidden after a crossfade (`visibility_changed`) or leaves the tree (`tree_exiting`). **Not exercised**: this machine's ffmpeg has no `libtheora` encoder and no `.ogv` exists on it, so no Theora file could be produced. Everything up to playback (the poster fallback, the badge, the footer) is covered by the smoke. To verify: install an ffmpeg with libtheora (`brew install ffmpeg` builds include it when `theora` is a dependency) or drop any `.ogv` into a media folder; the next pipeline pass transcodes the EW videos and `media.playable` becomes 8. |
| Thumbnails never play video | **pass** | `App.thumbnail` turns a video background into its poster (`core.StillBackground`), so Preview/Live/schedule thumbnails are stills. `TestStillAndThumbBackground`. |
| Preview / Live monitors, resource preview and output all play | **untested** | Same as above (needs a Theora file). All four are `SlideView`s and go through the same `layer.apply`. Four simultaneous software Theora decodes of 720p are expected to be fine on this hardware but have not been measured. |

## Testability

| Criterion | Status | Verification |
|---|---|---|
| Control actions and `/state` | **pass** | Actions: `search-media {q, kind}`, `select-media {index\|title}`, `add-media {path\|paths}`, `remove-media {index\|title, confirm}`, `add-to-schedule {source: "media"}`; `resource-go-live` and `focus {target: "media-grid"}` cover the tab. `/state.media`: `count, videos, images, query, kind, results, selected, current, countText, working, pending, playable, ffmpeg, theora, tools`. |
| Smoke checks | **pass** | `test/smoke.sh` runs 302 checks (Phase 1 alone: 191; the media section 37; the transitions and settings section 74); the media section is between the schedule and song-editor sections, the transitions and settings section right before the quit section. The launch section is unchanged. Both full runs on 2026-10-19 passed first time, with the TV output open fullscreen. |

## Shared files touched (for the concurrent splitter / graphics.gd work on `main`)

- `internal/render/slideview.go`: +10 lines: the `video` field on `layer`, one line in `newLayer`, the
  `BackgroundVideo` case in `apply` (plus `l.video.hide()` for other kinds) and the video case in
  `normalizeBackground`. `build`/`relayout`/`show` are untouched. `output.go`, `splitter.go`, `monitor.go`,
  `events.go` (ui) and `go.mod` are untouched.
- `internal/app/app.go` (+8: `md mediaState` field, `initMedia()` in Ready, media import flag), `view.go` (+39:
  `wireMedia()` call, media render block, `resourcePreview` case, poster stills in `thumbnail`), `events.go` (+13:
  Media cases in `addCurrentToSchedule` / `resourceGoLive`, `ResolveDeck` in `selectScheduleItem`), `control.go`
  (media actions and `/state.media`), `hooks.go` (+2), `input.go` (1 line).
- `internal/ui/slidegrid.go`: `SlideGridItem.Badge`, `SlideGrid.OnContextMenu`, right-click in `slideTile.GuiInput`;
  `mainwindow.go`: the `Media` field and page; `icons.go`: two 16 px icons.

## Transitions

Recorded 2026-10-19 on the same machine, with the TV as the projector output. PLAN.md §1/§3 and M6 ("more
transitions").

| Criterion | Status | Verification |
|---|---|---|
| `models.Transition{Kind, Duration}`: Blend, Cut, Push/Wipe/Cover/Reveal (left, right, up, down), Iris; JSON in milliseconds; lenient name parser | **pass** | `internal/models/transition.go`. `TestTransitionKinds`, `TestParseTransitionKind` ("Push Left", `push_left`, `crossfade`, family names alone = the leftward variant), `TestTransitionNormalized` (unknown kind → blend, cut → 0 ms, clamp to 10 s), `TestTransitionJSON` (`{"kind":"push-left","duration":750}`, nested in a settings record). Directional kinds name the **direction of motion**: Push Left pushes the old slide off to the left; Wipe Left reveals the new one from the right edge leftwards; Cover Left slides the new slide in over the old, moving left; Reveal Left slides the old slide away to the left. |
| Geometry and easing are pure and tested | **pass** | `layout.Transition(kind, t, w, h)` returns both layers' offsets, the incoming alpha, the z-order and a rect/circle mask (`internal/render/layout/transition.go`). `TestTransitionEndpoints` (every kind hides the incoming layer at t = 0 and is `Rest` at t = 1, progress clamped), `TestTransitionMidway` (every kind at t = 0.5), `TestTransitionMonotonic`. |
| Rendering: the same two-layer SlideView, no third state | **pass** | `internal/render/transitions.go` projects a `layout.TransitionFrame` onto the layers each frame: each layer root (at the letterbox rect, clipping) holds an `inner` content node that push/cover/reveal move, so nothing slides over the letterbox bars; blend is the root's modulate; wipe and iris are drawn by the root itself and clip its children (`clip_children = only`, antialiased circle). `show` still jumps a running transition to its end before starting the next one, `SetFrame`'s identity rules are untouched, and every transition holds exactly one `AcquireAnimating`. Verified live on the TV: `test/smoke.sh` screenshots `push-mid.png`, `wipe-mid.png`, `iris-mid.png` (old color on one side, new on the other, mid-way) and `push-end.png`; a scratch run also checked cover-up and reveal-right. |
| The live monitor and the output use the configured transition; thumbnails cut | **pass** | `App.render` → `SlideView.SetTransition` on the live monitor and the output's view (idempotent; the preview monitor keeps the default crossfade). Smoke: `mid-push on the live monitor too`. Thumbnails and the resource preview keep a 0 ms duration. |
| Control | **pass** | `transition {kind, duration}` (either part optional), `/state.transitioning` as before. Smoke: `push running`, `wipe running`, `iris running`, `unknown transition is rejected`. |

## Settings (Options dialog) and menus

| Criterion | Status | Verification |
|---|---|---|
| `core.Settings` is authoritative state, persisted one key each, applied by one idempotent render | **pass** | `internal/app/settings.go`: `settings.transition`, `output.screen` (−1 = first non-primary display), `output.logo` (the existing key), `ui.interfaceColor`. `loadSettings` reads them at startup (`TestSettingsDefaultsAndNormalized`, `TestOutputScreenFor`); `renderSettings` projects them onto the live views, the output display (the output is moved when the configured display changes or is unplugged), the logo, the theme, the menus and the dialog. |
| Options dialog: Output (display picker), Transition (kind + duration), Logo (image file), Library (copyrighted Bibles import), Hotkeys (read-only table) | **pass** | `ui.SettingsDialog` (`internal/ui/settings.go`, 712 × 532 with a 188 px section menu, UI-SPEC §5): a dumb view with `SetValues` / `SetScreens` / `SetHotkeys` and `On…` callbacks. Changes apply at once; Cancel restores the settings from when the dialog opened; OK keeps them. The logo preview is a `render.SlideView` the app hosts in the dialog's `MonitorHost`. "Choose Image…" uses `DisplayServer.FileDialogShow`. The hotkey table comes from `core.Bindings()`, derived from the hotkey tables so it cannot drift (`TestBindings`). Smoke: `Options… opens the dialog` (`dialog.settings`, `dialog.host == "main"`), screenshot `settings-dialog.png`, `hotkeys are off while Options is open`, `Cancel restores the settings`, `OK keeps the change`. |
| Values persist and round-trip | **pass** | Smoke: `set-setting transition` then a read of the `settings` table in the smoke library (`the transition setting is persisted`); `set-setting output.screen automatic`, `an unplugged display is rejected`, `an unknown setting is rejected`; logo: `logo image` → `the output shows the logo image` (exact pixel check on the projector) → `default logo`; `light interface` → `dark interface via the menu` (the theme is rebuilt at runtime, screenshot `light-main.png`). |
| Menu bar (File: New Schedule/Song, Open…, Open Recent, Save, Save As…, Quit; Edit: Options…; View: Interface Color; Help: About) | **pass** | `ui.Menus` (`internal/ui/menus.go`): Godot `MenuBar` + `PopupMenu`s with `prefer_global_menu`, so on macOS they live in the system menu bar and take no window space; a row above the toolbar elsewhere. Wired through `OnAction` / `OnOpenRecent`; Open Recent is projected from the `schedule.recent` setting. The control action `menu {item}` runs an item by name. Smoke: `Help ▸ About`, `File ▸ New Schedule`, `dark interface via the menu`, `unknown menu item is rejected`. The native menus themselves are not clickable from automation. |
| Crash safety (M6): no dialog or error can appear on the projector output | **pass** | Audit: every dialog (`SongEditor`, `SettingsDialog`, `ConfirmDialog`, `MessageDialog`) is a child of `FwMainWindow` or of the dialog above it (`MainWindow.dialogHost`); `App.alert` only ever queues into the message dialog; the native file dialogs attach to the main window; the output window hosts one `SlideView` and nothing else. `/state.dialog.host` reports the topmost dialog's parent (`TopDialogHost`). Smoke: `the error dialog is on the main window`, `the output is unchanged by the error dialog` (`pngcheck.py … same` on screenshots taken before and during the error). |
| Control | **pass** | `settings`, `set-setting {key, value}`, `transition {kind, duration}`, `options {on, page}`, `menu {item}`; `/state.settings`, `/state.dialog.settings`, `/state.dialog.host`; `/screenshot?window=settings`. |
| Smoke | **pass** | The "transitions and settings" section adds 74 checks (302 in all), with the screenshots `push-mid.png`, `wipe-mid.png`, `iris-mid.png`, `logo-blue-output.png`, `light-main.png`, `settings-dialog.png`, `settings-transition.png`, `settings-hotkeys.png`, `output-before-error.png` / `output-during-error.png`. The full run on 2026-10-19 passed with one display connected (windowed output); the transitions were also verified on the TV in a scratch run (`cover-up`, `reveal-right` included). |

### Shared files touched (transitions and settings)

- `internal/render/slideview.go` (+~20 lines): `kind` on `SlideView`; `inner`, `offset`, `mask` on `layer`; `newLayer`
  puts the content under `inner` and connects the root's draw signal; `apply` calls `placeAt`; `show`, `Process`,
  `finishTransition` and `relayout` call `beginTransition` / `applyTransition` / `endTransition`. `output.go`,
  `cover_*.go`, `video.go` are untouched.
- `internal/app/app.go` (settings fields, `loadSettings()` replaces the logo block in Ready), `view.go`
  (`wireSettings()`, `renderSettings(v)`, the output's screen and `SetTransition` in render), `control.go`
  (`actionArgs` keys, `handleSettingsCommand`, `/state.settings` and dialog fields, `screenshot?window=settings`),
  `events.go` (`rememberRecent` keeps `a.recent`), `core/state.go` (`Settings`, `DialogState.Settings/Host`).
- `internal/ui/mainwindow.go`: `Menus` and `Settings` fields, the menu bar above the toolbar, `Settings` in
  `dialogs()`, `TopDialogHost()`. `toolbar.go`, `splitter.go`, `monitor.go`, `events.go` are untouched.
- `test/pngcheck.py`: a `same FILE2` mode. `test/smoke.sh`: one block before `# ---- quit`.

## Remaining for Phase 2

- **Video playback verification** on a machine with a Theora-capable ffmpeg (see above), then: a progress
  indicator in the resource preview's mini toolbar (the state has `media.working` and per-item `Converting NN%`
  badges already), and hot-swapping a live poster for the finished transcode.
- **Video option B** (PLAN §3: ffmpeg pipe → YUV textures) for machines without libtheora, or bundling an ffmpeg
  with libtheora in the release.
- **Theme editor** (background color / gradient / image / video, font, outline, shadow, alignment, margins, live
  preview; default theme per item type) and the **Themes** tab. The media library's `Media.Background()` is the
  hook for "use as background".
- **Settings**: default themes per item type (the Options dialog has Output, Transition, Logo, Library and a
  read-only Hotkeys table; editable hotkeys and a Zoom transition are not done). The native menus cannot be
  driven by automation (the `menu {item}` action covers the handlers).
- **Packaging**: signed DMG, Windows, Linux (PLAN §4 M6).
- Media library polish: audio (EasyWorship's `Audio` folder), drag-and-drop into the schedule, per-item titles and
  tags, removing items whose files have gone missing.
