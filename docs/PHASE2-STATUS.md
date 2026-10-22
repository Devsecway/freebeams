# Phase 2 status

Status of Phase 2 (PLAN.md §4, **M4: Media & Themes**), recorded 2026-10-19 on macOS (Apple M3 Max, built-in
Retina display plus a 1080p TV over HDMI, EasyWorship 8 data at `/Users/Shared/Softouch`, Homebrew ffmpeg 8.1.1
**without** the `libtheora` encoder). This document covers the Media milestone and the Themes milestone; the rest
of Phase 2 is listed at the end.

Legend as in [PHASE1-STATUS.md](PHASE1-STATUS.md): **pass**, **partial**, **fail**, **untested**.

How to reproduce:
```bash
gofmt -l . ; go build ./... && go vet ./... && go test -count=1 ./...
test/smoke.sh          # 1065 checks (Phase 1: 191; media, presentations, themes, transitions/settings, Bibles, search, playback, song-theme, stage-display, remote-control, alerts, audio-playlist, preview-restyle (89) and tile-menu blocks)
FREEBEAMS_PLAYER=ffmpeg test/smoke.sh   # 979 before the preview-restyle block (not re-run since): the same with the ffmpeg player (Windows / Linux) on a Mac
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
| Thumbnails: Godot `Image` for stills, ffmpeg poster frames for videos, generated off the main thread | **pass** | The pipeline writes 640 px JPEGs to the cache dir (`~/Library/Caches/FreeBeams/media`, `FREEBEAMS_CACHE` overrides); the grid's thumbnails are `render.SlideView`s showing the thumbnail file (`core.ThumbBackground`). New posters rebuild the tiles (the thumbnail path is part of `ui.MediaRow`). |
| Remove from library asks first; the file is not deleted | **pass** | Smoke: `remove asks for confirmation`, `cancel the removal`, `remove the synthetic image`, `the schedule still has the media item`. |

## Presenting media

| Criterion | Status | Verification |
|---|---|---|
| An image item live = full-screen image on the output | **pass** | Smoke: `image live`, `image output has content`; a synthetic solid `#2a7f62` image: `the output shows the image` (exact pixel check on the projector screenshot), `the live monitor shows the image`. Screenshots `media-green-output.png`, `media-green-main.png`. |
| Media items in the schedule (snapshot, portable) | **pass** | `core.MediaItem` stores the source path and poster; `core.ResolveDeck` substitutes the playable file when the item is selected. Smoke: `add media to schedule`, `select the media schedule item`, `media schedule item live`. `TestMediaDeckAndItem`, `TestResolveDeck`, `TestMediaScheduleItemRoundTrip` (.fbs JSON keeps `mediaId` and `poster`). |
| Looping motion background (`models.BackgroundVideo`) behind the text, low-processor mode off while playing | **pass** (macOS, native) / **untested** (Theora path) | macOS plays every video and audio file through AVFoundation (`internal/render/avplayer_darwin.go`, `videosource.go`; [MEDIA.md](MEDIA.md)): no transcode, no `Needs libtheora`. Videos loop by default (`Background.Loop`), gapless (`actionAtItemEnd = None` + seek to 0 on the end notification); `AcquireAnimating` is held while a source plays. Smoke (playback block): `the clip goes live and plays (looping, full volume)`, `the live position advances`, `the clip stops at its end with loop off`, `black off brings the clip back`, `the clip plays again from the start`. The Theora path (`video.go`) is unchanged and still unexercised here (no libtheora, no `.ogv`); it is what Windows / Linux use when no ffmpeg is found (with ffmpeg they use the ffmpeg player, see "Video and audio off macOS"). |
| Thumbnails never play video | **pass** | `App.thumbnail` turns a video or audio background into its poster / black (`core.StillBackground`), so Preview/Live/schedule thumbnails are stills; `RoleNone` views open no player. `TestStillAndThumbBackground`. |
| Preview / Live monitors and output all play | **pass** (macOS) | One decoder per role: `RoleLive` is shared by the Live Output monitor and the projector window (sound plays once), `RolePreview` has its own, paused at 0 (its first frame shows). Smoke: `the output shows the clip` (exact `#2a7f62` pixel check on the projector screenshot), `the live monitor shows the clip`, `the preview monitor shows the clip's first frame`, `play on the preview target (a second decoder)`, `the preview plays while live plays`. A 720p H.264 EasyWorship loop playing to the live monitor and the output window costs ~44 % of one core (Godot process, `ps`), the operator UI redrawing at full rate included. The resource preview is `RoleNone` (poster still). |
| Audio items (`models.MediaAudio`: mp3, m4a, aac, wav, aiff, aif, flac, ogg) | **pass** (macOS) | An audio item is a one-slide deck with a `BackgroundAudio` (black, sound only, plays once by default); the library lists an Audio collection (`audio` icon), the footer counts `N audio`, `search-media {kind: "audio"}`, `/state.media.audio`, the EasyWorship `Resources/Audio` folder (`<audio>` rows) is imported. Smoke: `add the test clip and tone`, `the audio collection`, `the tone goes live and plays once`, `the live player plays the tone (audio only)` (`hasAudio` and not `hasVideo`), `audio shows a black output`. Audio goes to the system's default output device. |
| Transport bar under the Preview Output and Live Output monitors | **pass** | `ui.Transport` (`internal/ui/transport.go`): seek slider + `0:00 / 0:30`, round play / pause, skip to start / end, loop toggle, volume; shown only while the monitor's slide is a video or audio file. The state is `core.Playback` per monitor on the App (`internal/app/playback.go`), projected every frame while a file is open; `/state.playback.{live,preview}`. Actions `play`, `pause`, `toggle-play`, `seek {seconds}`, `volume {value}`, `loop {on}` with `target: live\|preview`. Smoke: `pause`, `the position holds while paused`, `the position follows the seek`, `loop off`, `volume`, `the preview's volume is its own`, `a bad target is refused`, `a seek without seconds is refused`, `a volume that is not a number is refused`, `a monitor without media refuses play`. Screenshots `playback-main.png`, `playback-audio-main.png`. |

## Testability

| Criterion | Status | Verification |
|---|---|---|
| Control actions and `/state` | **pass** | Actions: `search-media {q, kind}` (kind `video`, `image`, `audio`), `select-media {index\|title}`, `add-media {path\|paths}`, `remove-media {index\|title, confirm}`, `add-to-schedule {source: "media"}`; `resource-go-live` and `focus {target: "media-grid"}` cover the tab; `play`, `pause`, `toggle-play`, `seek`, `volume`, `loop` the transport. `/state.media`: `count, videos, images, audio, query, kind, results, selected, current, countText, working, pending, playable, ffmpeg, theora, native, tools`; `/state.playback.{live,preview}`: `playing, position, duration, loop, volume, hasVideo, hasAudio, title, native, media, failed`. |
| Smoke checks | **pass** | `test/smoke.sh` runs 302 checks (Phase 1 alone: 191; the media section 37; the transitions and settings section 74); the media section is between the schedule and song-editor sections, the transitions and settings section right before the quit section. The launch section is unchanged. Both full runs on 2026-10-19 passed first time, with the TV output open fullscreen. |

## Presentations (PDF / PowerPoint / Keynote → image slides)

Recorded 2026-10-19 on the same machine. Renderers found here: **Core Graphics** (in-process, macOS) for PDF;
**no `pdftoppm`** (poppler) and **no LibreOffice** (`soffice`), so PowerPoint / Keynote decks fall back to their
embedded images. Nothing was installed. EasyWorship's `Presentations.db` holds one built-in, non-file-based
"Introduction" row and `Resources/Presentations` is empty, so there was nothing to register from it.

| Criterion | Status | Verification |
|---|---|---|
| `internal/presentations` (pure Go): `Renderer` interface, page PNGs (1920×1080, letterboxed on black) + a 640px JPEG thumbnail in `~/Library/Caches/FreeBeams/presentations/<key>/` (`FREEBEAMS_CACHE/presentations` overrides); the key hashes path, size and mtime so an edited document renders again; `.part` folder renamed into place | **pass** | `TestKeyAndPaths`, `TestDefaultCacheDir`, `TestLetterbox` (16:9 fills, 4:3 / portrait letterboxed, small images scaled at most 2×), `TestScan`, `TestNumberedFiles`, `TestCanRender` (backend choice per type and tool set). |
| PDF through Core Graphics (cgo, `pdf_darwin.go`; `CGPDFDocument` → `CGBitmapContext` → Go `image/png`); `pdftoppm` on PATH elsewhere | **pass** (CG) / **untested** (pdftoppm) | `TestRenderPDF`: a hand-written two-page PDF renders to two pages with the exact fill colours, a 640×360 thumbnail, no `.part` left behind, the second render reuses the folder, cancellation leaves nothing, garbage is rejected. `TestRenderPDFWithoutRenderer` → `ErrUnavailable`. pdftoppm is not on this machine: the code path (`-scale-to 2160`, numeric output ordering, letterbox) is covered only by the pure helpers. |
| PPTX / PPT / Keynote / ODP through LibreOffice headless → PDF → pages, per-conversion user profile | **untested** | `soffice` is not installed; `TestRealLibreOffice` skips. `sofficeConverter` / `Service.renderOffice` are exercised only by `TestCanRender`. |
| Fallback without LibreOffice: a `.pptx` is read as a zip (`presentation.xml` slide order → slide rels → the slide's largest picture), slides without pictures skipped, `docProps/thumbnail.jpeg` when no slide has one; a `.key` shows its `preview.jpeg`; the deck is marked **Partial** ("embedded images") | **pass** | `TestRenderPPTXEmbedded` (3 slides → 2 pages: red PNG scaled 2×, green 4:3 JPEG letterboxed; thumbnail-only deck; deck without any image fails; `.ppt` → `ErrUnavailable`; garbage fails), `TestRenderKeynoteEmbedded`. Smoke: `PowerPoint pages rendered`, `select the deck (embedded images)` (`currentRenderer == "embedded"`, `currentPartial`), `the output shows the deck's slide` (exact pixel check). |
| `presentations` table (path, title, pages, cache_key, renderer, partial, added) with CRUD and search | **pass** | Migration 3; `internal/store/presentations.go`. `TestPresentationCRUD`, `TestSavePresentationsAtomic`, `TestSearchPresentations`. |
| Presentations tab (`FwPresentationPanel`, dumb view): search, grid of first-page thumbnails with badges (`12 slides`, `Rendering 3/12`, `5 slides · embedded`, `Needs LibreOffice`, `Needs poppler`), Add Presentation… (native dialog, pdf/pptx/ppt/key/odp), context menu / gear menu (Add to Schedule, Go Live, Remove from Library), footer status, view sizer | **pass** | `internal/ui/presentations.go`; `TestPresentationBadge`, `TestPresentationsFooter`. Smoke: `Presentations tab`, `presentations tab has content` (`test/out/presentations-tab.png`), `search presentations`, `all presentations`. |
| Selecting shows page 1 in the resource preview; the pages are a deck of image slides that goes live and steps with the normal hotkeys | **pass** | `core.PresentationDeck` (one `SlidePresentation` per page, `Slide N` labels; a placeholder slide while rendering). Smoke: `select the PDF` (`resource.caption == "Slide 1 · 1 / 2"`), `PDF live`, `the output shows page 1` (#2a5fbf), `Next steps to page 2`, `the output shows page 2` (#bf5f2a) / `page 1 is gone from the output` / `the live monitor shows page 2`, `Next at the last page stays`, `Prev steps back to page 1`, `Enter in the presentation grid sends the deck live`. |
| Rendering runs off the main thread in a tracked job; progress in the badges and `/state.presentations.working`; failures are logged once and not retried this session | **pass** | `App.startPresentationWork` / `runPresentationWork` (one pass over the library, one document at a time, progress posts throttled to 250 ms). Smoke: `PDF pages rendered` (`pending == 0`, `failed == 0`). |
| Schedule items snapshot the page slides with the document path and cache key; a stale or missing snapshot is rebuilt from the library's current render when the row is selected | **pass** | `core.PresentationItem`, `core.ResolvePresentationItem` (`TestPresentationDeckAndItem`, `TestResolvePresentationItem`, `TestPresentationScheduleItemRoundTrip`: `.fbs` keeps `presentationId`, `source`, `cacheKey`). Smoke: `add presentation to schedule`, `select the presentation schedule item`, `presentation schedule item live` (slide 2), `remove asks for confirmation (presentation)`, `cancel the removal (presentation)`, `remove the PDF`, `the schedule still has the presentation`. An unrendered deck is refused ("wait for the slides to finish rendering"). |
| EasyWorship `Resources/Presentations` files registered once on first launch | **pass** (no data here) | `easyworship.FindPresentations` / `ImportPresentations` (`TestFindAndImportPresentationsSynthetic`; `TestRealPresentations` logs 0 files on this machine); `App.startPresentationImport` under `import.easyworship.presentations.done`. |
| Control actions and `/state` | **pass** | `search-presentations {q}`, `select-presentation {index\|title}`, `add-presentation {path\|paths}`, `remove-presentation {index\|title, confirm}`, `add-to-schedule {source: "presentation"}`, `focus {target: "presentation-grid"}`, `tab {tab: "presentations"}`. `/state.presentations`: `count, query, results, selected, current, currentPages, currentPartial, currentRenderer, countText, working, pending, failed, pdf, office, tools`. |

Known limits: the Preview / Live grids load the full 1920×1080 page PNGs for their thumbnails (only the library
tile uses `thumb.jpg`), so a long deck costs a few hundred milliseconds on the main thread when it is first
selected; per-page thumbnails would fix that. A presentation whose document is removed from the library keeps
working from the schedule snapshot while the cache folder exists, but nothing re-renders it once the cache is
cleared. Password-protected PDFs are reported as unavailable. Binary `.ppt` needs LibreOffice.

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
- Presentations milestone (additive only): `internal/app/app.go` (+3: `pr presentationState`, `initPresentations()`),
  `view.go` (+30: view-cache fields, `wirePresentations()` call, the render block, the `TabPresentations` resource
  preview case), `events.go` (+9: `ResolvePresentationItem` in `selectScheduleItem`, the tab cases in
  `addCurrentToSchedule` / `resourceGoLive`), `control.go` (the presentation actions, the `presentation` schedule
  source, `/state.presentations`), `hooks.go` (+2: `presentation-grid` focus target), `input.go` (1 line: the grid
  is a `FocusGrid`), `layout.go` (the `presentations` splitter), `core/state.go` (`PresentationsState` and its
  `State` / `StateInput` field in a separate block), `ui/mainwindow.go` (+4: the `Presentations` field in its own
  block, the page), `ui/register.go` (+1), `ui/icons.go` (+1), `models/slide.go` (+3), `models/schedule.go` (+9),
  `store/migrations.go` (migration 3 appended), `importers/easyworship/easyworship.go` (+4 `Report` fields),
  `test/smoke.sh` (the `# ---- presentations` block before `# ---- quit`). `render/*`, `ui/splitter.go`,
  `ui/monitor.go`, `ui/events.go`, `go.mod` and the smoke launch section are untouched.
- Native playback milestone (additive): `internal/app/app.go` (+5: `pb playbackState` block, `initPlayback()`,
  `playbackEvents()` / `renderPlayback()` in `Process`), `view.go` (+5: `seekApplied` block in the view cache,
  `wirePlayback()` call, `renderPlayback()` in `render`), `control.go` (+6: `Seconds` arg block, `audio` in
  `search-media`, `handlePlaybackCommand` in the dispatcher chain, `Playback` in `state()`), `media.go` (audio
  counts, `Native` caps, no transcode when native), `core/state.go` (`MediaState.Audio/Native` block, `Playback`
  blocks), `core/media.go` (`MediaCaps.Native`, `MediaCounts`, audio badges / footer / item), `render/slideview.go`
  (`role`, `native` layer, `EnterTree`, `SetVideoRole`, audio case), `render/video.go` (`applyVideo` chooses the
  player), `render/output.go` (+1), `render/textures.go` (+1), `ui/mainwindow.go` (`PreviewTransport` /
  `LiveTransport` block, variadic `columnFooter`), `ui/theme.go` (+1), `ui/register.go` (+1), `ui/media.go` (+1 Audio
  row), `ui/uicore/icons.go` (a `transport & audio` block at the end), `models/media.go` (`MediaAudio`, `Plays()`,
  `Background.Loop` default), `models/slide.go` (`BackgroundAudio`, `Loop`, `Plays()`), `store/media.go` (+1 kind),
  `importers/easyworship/media.go` (`<audio>`), `media/*` (audio probing, `Processor.Native`), `test/smoke.sh` (the
  `# ---- playback` block before `# ---- quit`), `CLAUDE.md` (gotchas appended). New: `render/avplayer_darwin.go`,
  `render/avplayer_other.go`, `render/videosource.go`, `ui/transport.go`, `app/playback.go`, `core/playback.go`,
  `docs/MEDIA.md`. No store migration.

## Themes (PLAN §2 item 6, §4 M4 "Theme editor")

Recorded 2026-10-19 on the same machine. "smoke: X" names a check in the `# ---- themes` section of
`test/smoke.sh` (45 checks, between the window-closing and quit sections).

### Theme library

| Criterion | Status | Verification |
|---|---|---|
| `themes` table (id, name, json of `models.Theme`, created, updated) with CRUD and list | **pass** | Migration 3 (`internal/store/migrations.go`), `internal/store/themes.go`: `SaveTheme` (insert / update, name required, `ErrNotFound` for a missing id), `GetTheme`, `ListThemes` (by name, case-insensitively), `DeleteTheme`, `CountThemes`. `TestThemeCRUD`, `TestListThemesOrder`. |
| Default theme per item type: settings `theme.default.song` / `theme.default.scripture` | **pass** | `DefaultThemeID`, `SetDefaultTheme` (the theme must exist; only song / scripture), `GetDefaultTheme` (falls back to the built-in `models.Default*Theme()` with ID 0). Deleting a default theme clears its setting. `TestDefaultThemeSettings`. |
| Seeded with the built-in defaults on first launch | **pass** | `EnsureDefaultThemes` inserts "Default Song" and "Default Scripture" into an empty library and points the two settings at them; a later launch writes nothing; a cleared setting is re-pointed at the theme of that name (re-inserted when missing). `TestEnsureDefaultThemes`. Smoke: `themes listed (seeded defaults)`; log: `seeded the default themes`. |

### Themes tab (UI)

| Criterion | Status | Verification |
|---|---|---|
| `Themes` tab replaces the placeholder: library pane, thumbnail grid of themes, footer with New / gear / view sizer | **pass** | `ui.ThemePanel` (`FwThemePanel`, dumb view: `SetItems`, `SetSelected`, `SetCountText`, `SetApplyTarget`, `On…` callbacks). Each tile is a `render.SlideView` of `core.SampleSlide()` (four lines of Amazing Grace plus a footer) drawn with the theme, through the injected factory (`App.wireThemes` → `thumbnail(core.ThemeDeck(t), 0)`). Screenshot `test/out/themes-tab.png`. Smoke: `Themes tab`, `themes tab has content`. |
| New / Edit / Duplicate / Delete (asks first), Set as Songs / Scripture default, Apply to schedule item | **pass** | Tile context menu and footer gear (`themeMenu*`). Default themes carry the badge `Songs`, `Scripture` or `Songs · Scripture` (`core.ThemeBadge`). "Apply to “<item>”" names the selected schedule row and is disabled without one. Smoke: `duplicate a theme` ("Smoke Magenta Copy", `core.CopyName`), `delete asks first (theme)`, `cancel the theme delete`, `delete the copy`, `make it the songs default`, `restore the songs default`, `apply the theme to the selected schedule item`. |
| A default theme cannot be deleted | **pass** | `App.themeDeletable`: the operator is told which default it is ("Choose another default theme first."). Smoke: `deleting a default theme is refused`, `the refusal is shown to the operator`. |
| Selecting a theme shows its sample in the resource preview | **pass** | `App.resourcePreview` (`TabThemes`) → `a.th.deck`. Smoke: `select the song default theme` (`resource.title == "Default Song"`, caption `Verse 1 · 1 / 1`). |
| Keyboard: the grid takes arrows / Enter (Enter opens the editor) | **pass** | `focus {target: "theme-grid"}` → `FocusGrid` (`App.focus`); `OnActivate` → `editTheme`. Not in the smoke (the Media grid covers the shared `SlideGrid` keys). |

### Theme editor

| Criterion | Status | Verification |
|---|---|---|
| Dialog with background (color / gradient / image via the native file dialog), font family (installed system fonts), size, shrink-to minimum, bold / italic, color, line spacing, outline (on / size / color), shadow (on / offset / blur / color), horizontal / vertical alignment, margins, footer size / color | **pass** | `ui.ThemeEditor` (`internal/ui/themeeditor.go`), a 1020 × 620 `dialogWindow` like the song editor. The font list is `OS.GetSystemFonts()` (hidden "." families dropped, the theme's own family appended when not installed); `Browse…` goes through `OnBrowseImage` → `App.browseThemeImage` (`DisplayServer.FileDialogShow`) → `SetBackgroundImage`. Screenshot `test/out/theme-editor.png`. Smoke: `open the theme editor on it`, `theme editor has content`, `hotkeys are off while the theme editor is open`, `close the theme editor`. Video backgrounds are not offered in the editor (see Remaining). |
| Field values live in plain Go; the dialog is a projection | **pass** | `ThemeEditor.theme` is the state; every widget signal mutates it through `change()` and one `render()` projects it back with no-signal setters (`set_value_no_signal`, `set_pressed_no_signal`, `OptionButton.select`, `LineEdit.set_text`, `ColorPickerButton.set_pick_color`), ending with a getter so graphics.gd's buffered calls apply at once. |
| Live preview inside the dialog: a sample lyric with a footer | **pass** | `ThemeEditor.Preview` is a `MonitorHost` slot; the app puts a cut (0 ms) `render.SlideView` in it and `render()` feeds it `editorFrame(a.th.editing)` (Seq 0: compared by content) on every edit (`OnChange`). Visible in `theme-editor.png`. |
| OK / Cancel / Apply | **pass** | OK validates (name required), hides and calls `OnSave`; Apply calls `OnApply` and stays open (a new theme learns its id from the save, so OK then updates instead of inserting again); Cancel / Esc / the close button call `OnCancel`. `editor {mode: "theme" \| "new-theme" \| "close"}` drive the same paths. |

### Applying themes

| Criterion | Status | Verification |
|---|---|---|
| New song / scripture decks use the configured default theme | **pass** | `App.newSongDeck` / `newScriptureDeck` (`core.SongDeckWith` / `ScriptureDeckWith` with `defaultTheme(kind)`; the built-in until the library has answered). Smoke: `a new song deck picks up the default` → `the themed song goes live` → `the output shows the theme background` (exact pixel check of `#991a80` in the output's top-left corner), `the live monitor shows the theme background`. Screenshots `theme-magenta-output.png`, `theme-magenta-main.png`. |
| Changing a theme re-renders the decks that use it (Preview, Live, thumbnails, output) without re-transitioning more than necessary | **pass** | `App.applyThemeChange`: `core.RestyleSchedule` (items matched by `Theme.ID`, schedule dirty), `Presenter.RestyleDecks(core.RestyleByThemeID)` (themes replaced in place, slides and ids untouched, one Seq bump) and the library decks behind the resource previews. `SlideView.SetFrame` compares content, so only views whose look changed crossfade; the grids rebuild their tiles from the restyled decks. `TestPresenterRestyleDecks`, `TestRestyleScheduleAndApply`. Smoke: `edit the live theme` → `the output follows the edited theme` (`#1a8099`), `the old background is gone`. Screenshot `theme-teal-output.png`. |
| Per-item override from the schedule row's context menu ("Theme ▸") | **pass** | `ScheduleList` context menu gains a `Theme` submenu with one radio entry per theme (the item's own checked, `ScheduleRow.Theme`); `OnSetTheme` → `App.applyThemeToItem` (`core.ApplyItemTheme`, then `RestyleDecks(RestyleByItemID)` when the item is in Preview / Live). Also the Themes tab's "Apply to “<item>”" and the `apply-theme` action. Smoke: `scripture item live with the scripture default` (`lacks` the song color), `apply the theme to the selected schedule item`, `the per-item override is live`. Screenshots `theme-scripture-output.png`, `theme-override-output.png`. |
| Schedule items keep their theme snapshot when the theme is deleted | **pass** | The item's `Theme` is a copy; `DeleteTheme` leaves it alone. Smoke: `delete the smoke theme` (the overridden scripture item stays live). |

### Testability

| Criterion | Status | Verification |
|---|---|---|
| Control actions and `/state` | **pass** | Actions (`App.handleThemeCommand`): `themes`, `select-theme {index\|name}`, `get-theme {index\|name}` (the full theme JSON, for `save-theme` round trips), `save-theme {theme}`, `duplicate-theme {index\|name}`, `delete-theme {index\|name, confirm}`, `set-default-theme {index\|name, kind: "song"\|"scripture"}`, `apply-theme {index\|name, row}` (default: the selected schedule row), `editor {mode: "theme"\|"new-theme"\|"close"}`, `focus {target: "theme-grid"}`, `screenshot?window=theme-editor`. `/state.themes`: `count, names, selected, current, songDefault, scriptureDefault, countText, editor, editing`; `/state.dialog.themeEditor`. |
| Smoke checks | **pass** | 45 checks in the `# ---- themes` section (the full run has 273); the launch section is unchanged. Passed on 2026-10-19 (the first full run failed only because the block reused the song list left filtered to the deleted test song; it now searches first). |

### Shared files touched (for the concurrent Phase 2 work)

- `internal/app/app.go` (+3: the `th themeState` field, `initThemes()` in Ready), `view.go` (+32: `wireThemes()` call,
  the Themes render block, `TabThemes` in `resourcePreview`, `Theme` in the schedule rows, four `viewCache` fields),
  `events.go` (`a.newSongDeck` / `a.newScriptureDeck` in four places, a `TabThemes` case in `addCurrentToSchedule`),
  `control.go` (`Themes` and `ThemeEditor` in `state()`, two `actionArgs` fields, `handleThemeCommand` in the
  `default` case, the `theme-editor` screenshot window), `hooks.go` (`editor` modes `theme` / `new-theme`, `close`
  closes both editors, `focus` target `theme-grid`), `input.go` (the theme grid is a `FocusGrid`).
- `internal/ui/mainwindow.go` (`Themes` page, `ThemeEditor` dialog), `schedule.go` (`ScheduleRow.Theme`,
  `SetThemeNames` / `OnSetTheme`, the Theme submenu), `register.go` (`ThemePanel`).
- `internal/models/presenter_impl.go`: `Presenter.RestyleDecks`. `internal/app/core/state.go`: `ThemesState`,
  `DialogState.ThemeEditor`. `render/*`, `ui/splitter.go`, `ui/monitor.go`, `ui/events.go`, `go.mod` and the smoke
  launch section are untouched.

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

## Icons (operator UI icon set)

| Criterion | Status | Verification |
|---|---|---|
| One icon language (32 px toolbar / 16 px mini grids, 2 / 1.5 px rounded strokes on the pixel grid), own artwork, crisp at 1× and 2× | **pass** | `internal/ui/uicore/icons.go` (artwork, pure Go) + `internal/ui/icons.go` (DPITextures). `TestIconSVGWellFormed`, `TestIconGrid`, `TestIconMulti`, `TestIconColors`. Before/after at native Retina pixels: `docs/screens/icons-{before,after}-{toolbar,panels}-{dark,light}.png`; design notes and the audit in [ICONS.md](ICONS.md). The Media / Themes / Presentations library lists no longer show Godot's red fallback icon (`onThemeReady`; `docs/screens/icons-after-trees.png`). |
| State colors from palette tokens: Go Live accent, Live red only while on, Logo/Black/Clear brightened when active, hover, disabled; light palette | **pass** | Single-color icons are white masks tinted at draw time (`drawGlyph`, `glyphTint`); `live_on` / `golive` carry their own colors. Theme colors `glyph`, `glyph_hot`, `glyph_on`, `accent`, `toolbutton_text_sel` in `theme.go`; no hex in widgets. Smoke screenshots `black-main.png` (Black active), `light-main.png`. |
| Smoke | **pass** | 388/388 on 2026-10-19 after the redesign; no pixel check changed (none asserts an icon color). A first run failed once at `the output shows the image` (the media monitors still showed the previous image although `/state` had `live.title == smoke-green` and was idle; intermittent, not icon-related, passed on the rerun). |

## EasyWorship Mac look (toolbar icons and dark palette)

| Criterion | Status | Verification |
|---|---|---|
| Output icons read like EasyWorship's Mac toolbar: filled light screens whose content tells the function (Go Live green screen + arrow, Alerts blue bubble + signal, Logo picture, Black diagonal, Clear peel, Live colour bars grey/SMPTE by output state); New/Open/Save/Web/Remote in the same filled family; `golive16` matches | **pass** | `internal/ui/uicore/icons.go` (`rim32`/`screen32`/`bars32`, evenodd holes), placeholders `{g}` `{b}` `{w}` `{s1..s7}` resolved by `IconSVG` from `IconColors`/`PaletteIconColors`; `TestIconGrid`, `TestIconMulti`, `TestLiveBars`. `docs/screens/icons-ew-after-toolbar-{dark,light}.png` (Live off / Live + Black on). Design notes in [ICONS.md](ICONS.md). |
| Dark interface colours follow the Mac screenshots (grid `#2B2B2C`, output surround and resource area `#161616`, library `#0F1010`, flat tab bar `#2B2B2B` with a `#3F3F3F` tab and muted-blue underline, flat `#161616` footers and search strip, `#313131` unfocused list selection, `#626161` schedule cards with italic subtitles, `#233563` tile frame with a `#395792` caption bar); light interface unchanged | **pass** | New `uicore.Palette` tokens (`GridBk`, `OutputSurround`, `ResourceBk`, `CardBk`, `TabSel`, `TabDivider`, `TileFrame`, `TileCaption`, `ListSelInactive`, `IconGreen`, `IconBlue`, `ResourceTabBar`, `Footer`) wired through `FwGridBk`/`FwOutput`/`FwResource` and `Fw` theme colours; `TestPaletteMatchesSpec`, `TestGradientsMatchSpec`. Provenance in [UI-SPEC.md §1](UI-SPEC.md) "Mac measurements". `docs/screens/look-{before,after}-{songs,media}-dark.png` against the user's EW screenshots. Layout, sizes and behaviour untouched. |
| Smoke | **pass** | 432/432 checks, PASS on the first run, 2026-10-19 (`test/smoke.sh`, log hygiene clean). Two pixel checks moved with the palette: `selected song row is highlighted` asserts `313131` (list.sel.inactive; the list is unfocused) and `first preview tile …` asserts `233563` (tile.frame) instead of `3a6dce`. |

## Bundled Bibles and Import Bible File… (PLAN §1 "Bibles")

Recorded 2026-10-19 on the same machine. Policy and formats: [BIBLES.md](BIBLES.md).

| Criterion | Status | Verification |
|---|---|---|
| Only public-domain translations are bundled: KJV and ASV, nothing else | **pass** | `internal/bibles/bundled`: manifest `Translations` (abbrev, name, language, licence, attribution, source URL `github.com/hargarpay/bible-translations` + commit `98b32df`, bundle `Version` 1). `TestBundledTranslations` fails if `Abbrevs()` is anything but `[KJV ASV]` or a manifest entry is not `models.IsPublicDomainBible`. Repository fixtures use KJV or made-up verses only. |
| Embedded data, correct and clean | **pass** | gzip single-file Bible JSON, `go:embed`ded (in the Go dylib on every platform): KJV 1,259,917 B + ASV 1,259,101 B = **2,519,018 B** (2.4 MiB). KJV 31,102 verses / 66 books; ASV 31,086 / 66 (it omits 16 NT verses by design; the test checks each of the 16 is absent from the ASV and present in the KJV). Text checks: John 3:16 in both, ASV Psalm 36:1 (leaked heading removed), 1 Samuel 13:1 (`[[a]forty]` → "forty"), Matthew 18:24 (the one source patch, `TestPatchesApplied`), no brackets / no-break spaces anywhere. `gen/main.go` regenerates the data reproducibly (same hashes) and refuses to drift from the manifest. |
| `bibletext` reader: bible-translations folders, chapter files, single-file JSON; book-name mapping, cleaning, warnings not panics | **pass** | `internal/importers/bibletext`, pure Go. `TestCleanText` (12 cases), `TestDecode` (unknown book, bad chapter/verse keys, non-string, empty, duplicates, combined "16-17" keys → 10 warnings, verses in canonical order, document order decides duplicates), `TestReadDir` (folder, chapter file inside it and the folder itself give the same result; stray files warn), `TestReadErrors`, `TestEncodeDecodeRoundTrip`, `TestWarningsAreBounded` (50 + "… and N more"). `TestRealTranslations` decodes the raw repository when `FREEBEAMS_BIBLE_TRANSLATIONS` points at a clone (31,102 / 31,086, no warnings; skips otherwise). |
| First launch installs the bundled translations off the main thread, idempotently, tracked by a bundle version | **pass** | `App.startBundledInstall` → `bundled.Install` (one transaction per translation; abbreviation match is case-insensitive; `bibles.bundled.version` setting). `TestInstall` (fresh library → KJV, ASV; second run adds nothing; lookups and FTS work), `TestInstallSkipsExistingAbbrev` (a lower-case `kjv` from EasyWorship is kept), `TestInstallErrors` (cancelled context leaves no Bible and no version). Smoke: `bundled KJV and ASV are installed`, `the bundled install accounted for both`, `ASV wording` (`believeth on him`, footer `John 3:16 (ASV)`), `KJV wording differs`, `ASV Psalm 36:1 carries no leaked heading`, `ASV omits Acts 8:37 by design` / `KJV has Acts 8:37`. Footer: "Installing bundled Bibles…" → "Installed KJV, ASV". |
| The EasyWorship import does not duplicate the KJV; the default translation stays KJV | **pass** | `easyworship.Options.BundledBibles` reports the EasyWorship KJV as `KJV (bundled)` (`TestImportIntoSkipsBundledBibles`), on top of the existing abbreviation check. Log on this machine: `223 songs imported; Bibles: RVA; skipped HCSB (copyrighted), KJV (bundled); 23 media items`. `core.DefaultBible` is unchanged (smoke: the `(KJV)` footers). |
| Import Bible File… (gear menu): native dialog, parse off-thread, confirmation with the licence statement, import, refresh | **pass** | `internal/app/biblefile.go`, `ui.ScripturePanel.OnImportFile`. `DisplayServer.FileDialogShow` in `FileDialogModeOpenAny` (a file or a translation folder; filters `*.json`, `*.ewb`). `core.BibleFileQuestion` names the translation, its verse count and copyright notice (or "public domain") and states the user's licence responsibility (`TestBibleFileQuestion`). Smoke: `import-bible-file (confirmed) imports the folder` (fixture tree `SMK` in the repository layout, with the source's quirks), `the imported translation works, with cleaned text` (`Psalms 23:1 (SMK)`), `import-bible-file asks first` (screenshot `import-bible-file-confirm.png`), `cancel the question`, `OK imports it` / `SMK2 is installed after OK` (single-file JSON), `a single-chapter book in the imported file` (`Jude 1:3 (SMK2)`). |
| Duplicates reported, errors to the message dialog | **pass** | Smoke: `a chapter file of the folder is the same translation: a duplicate` (`SMK is already installed`), `the duplicate is reported, not imported twice` (message dialog, screenshot `import-bible-file-duplicate.png`, one `SMK` in `/state.bibles`), `a missing file is an error` / `the read error is in the message dialog`, `import-bible-file needs a path`. |
| Control and state | **pass** | `import-bible-file {path, confirm}`; `/state.bundled {running, done, installed, skipped, version, error}`; `/state.import.running` is also true while the bundled install runs, so the existing first-launch wait covers both imports; `/state.bibles` lists imported translations. |
| Smoke | **pass** | The "bundled Bibles and Import Bible File…" block adds 27 checks (415 in all, three of them `app idle` waits), one block before `# ---- quit`. 2026-10-19: three more for the array layout of thiagobodruk/bible (`import-bible-file reads the array layout`, `books resolve by position, with the cleaned text`, `amplification brackets stay, the cross-reference goes`; fixture `en_smk3.json`, 583 in all). |

### Shared files touched (bundled Bibles and Import Bible File…)

- `internal/app/app.go`: two fields in their own block (`bundled`, `bibleFile`), `a.startBundledInstall()` before
  `a.startImport()` in `Ready`, and `startImport` passes `a.ewImportOptions()` (3 lines). `control.go`: `Import:
  a.importDoc()`, `Bundled: a.bundled`, the `import-bible-file` case. `view.go`: `sc.OnImportFile` (1 line).
  `core/state.go`: `State.Bundled` / `StateInput.Bundled` in their own blocks, two nil-to-empty lines in `BuildState`.
- `internal/ui/scripture.go`: `scriptMenuImportFile`, `OnImportFile`, one gear-menu item, one `menuPressed` case.
  `splitter.go`, `monitor.go`, `events.go`, `icons.go`, `theme.go` and `internal/render/*` are untouched.
- `internal/importers/easyworship`: `Options.BundledBibles` and one `switch` case in `importBibles`; `ewb.go`'s
  public-domain list moved to `models.IsPublicDomainBible` (`isPublicDomain` delegates).
- `test/smoke.sh`: one block before `# ---- quit`. New: `internal/importers/bibletext`, `internal/bibles/bundled`
  (+ `data/*.json.gz`, `gen/main.go`), `internal/app/bundled.go`, `internal/app/biblefile.go`,
  `internal/app/core/bundled.go`, `internal/app/core/biblefile.go`, `internal/models/bible_license.go`,
  `docs/BIBLES.md`.

## Search (songs, media, presentations)

Recorded 2026-10-19 after the report "search should be case insensitive, and for songs not just the title but the
lyrics if it matches". Diagnosis with the real EasyWorship library (223 songs, imported into a temp store by
`TestSearchSongsLocalEW`): matching was already case-insensitive, prefix-per-word and over title, author and lyrics
(FTS5 `unicode61 remove_diacritics 2`), and the user's `library.db` had all 223 lyrics indexed. What was wrong was the
**ranking and the feedback**: results came in title order, so `AMAZING` listed "Alas And Did My Savior Bleed" (a lyric
match) above "Amazing Grace", `O Holy Night` came after "Day Is Dying In The West", `it is well` put "It Is Well With
My Soul" third, and nothing said why a row was there, so the search looked case-sensitive and title-only. Two smaller
faults: a contraction such as `I'll` became `"i"* "ll"*`, two independent words anywhere in the row (33 songs, one of
them through the author "Lloyd"), and the media / presentation searches used `lower(title) LIKE`, which folds ASCII
case only (`CAFÉ` did not find "café", `cafe` never found "Café"). The scripture locator was fine: bad references are
`ErrBadReference` shown in the status (`TestParseReferenceErrors`).

| Criterion | Status | Verification |
|---|---|---|
| Case- and diacritic-insensitive, every term a word prefix, title + author + lyrics | **pass** | `store.SearchSongs` (FTS5 unicode61). `TestSearchSongs`: `GRACE`, `amaz`, `señor`/`senor`, `newton wretch` across fields, `eventide` (lyrics only). Smoke: `search is case-insensitive and ranks the title match first`, `a prefix finds the same song first`, `a lyrics-only word finds the song and says so`, `two terms across title and lyrics`. |
| Ranking: every term in the title, then title + author, then rows that needed their lyrics (a mixed title + lyrics match above lyrics-only); each tier in title order | **pass** | `searchSongsFTS`: one query; the tiers are uncorrelated `IN` subqueries with FTS5 column filters (`{title} : (…)`, `{title author} : (… OR …)`) in `ORDER BY`. `TestSearchSongs` ("title tier then lyrics tier", "limit applies after ranking"), `TestSearchSongsMatched` (three tiers, mixed matches), `TestSearchSongsLocalEW` (`AMAZING`, `O Holy Night`, `it is well`, `night divine`, `newton` first rows on the real data). |
| Why a row matched: `SongSummary.Matched` (`title` / `author` / `lyrics`) and a lyric `Snippet` | **pass** | `snippet()` over the lyrics column, one line. UI: a narrow **Match** column in the song list ("author", "lyrics"), the excerpt in the row tooltip ("Matched in the lyrics: …"), the footer "5 songs · 2 matched in lyrics" (`core.SongsFooterMatches`). `/state.songs.first`, `/state.songs.lyricsMatches`. Smoke: `title first, the lyric matches counted in the footer`, screenshot `search-lyrics.png`. |
| Phrases and contractions | **pass** | `searchTerms`: whitespace splits terms, a double-quoted run is one phrase (an unbalanced quote runs to the end), the words of a term form one prefix phrase `"w1 w2"*` that FTS5 tokenizes again, so `I'll` matches the adjacent tokens of I'll (22 songs instead of 33 on the real data; `Lord's` 10 instead of 106) and `John 3:16` matches 3:16. `TestFTSQuery`, `TestSearchSongs` ("quoted phrase", "quoted phrase keeps word order", "contraction is one phrase", curly apostrophe). Smoke: `a quoted phrase`, `a contraction is one phrase (fewer songs than i + ll)`. |
| No input can produce an FTS5 syntax error | **pass** | Only letters, numbers, marks and spaces reach the quoted phrases; terms and words are capped at 16. `TestFTSQueryNeverErrors` (`"`, `*`, `(`, `-`, `:`, `^`, `NEAR(`, NUL, invalid UTF-8, a lone combining mark, …). Should the FTS query ever fail, `searchSongsPlain` (fold + substring over title and author) answers instead. Smoke: `punctuation and FTS syntax are harmless`, `only punctuation lists the library`. |
| Media and presentations fold the same way | **pass** | `SearchMedia` / `SearchPresentations` match every word in Go with `fold()` (lower-case, NFD minus combining marks, ø/ł/đ/ß/æ/œ) as a substring of the title or path, so prefixes and literal `100%` / `_done` keep working; the limit applies to the matches. `TestFold`, `TestSearchMedia` / `TestSearchPresentations` ("diacritics folded", "accented query, any case", "limit applies to the matches"). Smoke: `media search ignores case`, `presentation search ignores case`. |

Not done: indexing CCLI numbers and copyright (the FTS table has title, author, lyrics), a per-collection filter, and
EasyWorship's search-mode choice (title / lyrics / author); the Match column is the simpler equivalent.

### Shared files touched (search)

- `internal/models/song.go`: `SongSummary.Matched` / `Snippet` (own block) and the `Matched*` constants.
- `internal/app/core/state.go` (`SongsState.First`, `LyricsMatches`, own block), `core/deck.go` (`SongsFooterMatches`,
  `LyricsMatches`, `FirstSongTitle`; `SongsFooter` delegates), `internal/app/events.go` (one line in `searchSongs`),
  `internal/app/control.go` (one line in the `SongsState` literal), `internal/app/hooks.go` (`isSongTable` matches the
  multi-column Tree, no longer exactly four columns).
- `internal/ui/songs.go`: the fifth column and the tooltip in `render`; `internal/ui/uicore/match.go` is new.
  `splitter.go`, `monitor.go`, `events.go`, `icons.go`, `theme.go` and `internal/render` are untouched.
- `test/smoke.sh`: one block before `# ---- quit`.

## Tile menus (media and presentations)

Recorded 2026-10-19 on the same machine (the HDMI TV was not detected: the output ran windowed at 960×540 and the
pixel checks read the output window). The right-click and footer-gear menus of the media and presentation tiles now
follow EasyWorship's (UI-SPEC §3.6 lists both menus and what is deliberately left out). "smoke: X" names a check in the
`# ---- item menus` section of `test/smoke.sh` (100 checks, between the search and quit sections; the full run is
586 checks and passed on 2026-10-19 with log hygiene clean; the Logo screen's default is the FreeBeams logo on the brand gradient since the rename, see docs/branding/README.md).

| Criterion | Status | Verification |
|---|---|---|
| Aspect Ratio ▸ Fill / Fit / Stretch and Repeating ▸ Loop / Play Once per media item, persisted | **pass** | Migration 5 (`media.aspect`, `media.repeat`), `models.Media.Aspect` / `Repeat` (`MediaAspect`, `MediaRepeat` with `Valid` / `Normalized` / `Parse…`), `store.SetMediaOptions` (targeted update; the pipeline re-reads the choices before its whole-row `SaveMedia`). `TestMediaAspectAndRepeat`, `TestMediaOptions`. Smoke: `select the square: Fill and Loop by default`, `Aspect Ratio ▸ Fit`, `Aspect Ratio ▸ Stretch`, `Repeating ▸ Play Once`, `Repeating ▸ Loop`, `the aspect ratio is read back from the library`. |
| The choice reaches the slide: `Background.Fit` / `Background.Loop` from `Media.Background()`, thumbnails included; the renderer letterboxes / stretches images | **pass** | `render.stretchFor` (`KeepAspectCovered` / `KeepAspectCentered` / `Scale`), `normalizeBackground` keeps `Fit` and `Loop` (a video layer re-applies when they change; playback itself reads `Loop` in `video.go`). Smoke, on a 200×200 image with a green top quarter: `Fill covers the canvas: no bars`, `Fill crops the band to the top edge`, `Fit letterboxes: black bars beside the square`, `Fit shows the whole image: the band is a quarter high`, `Stretch fills the canvas: no bars`, `Stretch keeps the band a quarter high` (screenshots `menu-fill-output.png`, `menu-fit-output.png`, `menu-stretch-output.png`). |
| Use As Logo Background (media item with its aspect ratio; a presentation's page 1) | **pass** | `core.MediaLogo`, `core.PresentationLogo` → `App.setLogo` (the same path as the Options dialog). `TestItemLogos`. Smoke: `Use As Logo Background (media)`, `the logo screen shows the square`, `the logo screen letterboxes like the slide`, `Use As Logo Background (presentation): page 1`, `the logo screen shows the PDF's first page`. |
| Delete File… moves the file to the Trash and removes the item (macOS: `NSFileManager trashItemAtURL`, no Automation prompt); other platforms leave the item out of the menu | **pass** (macOS) | `trash_darwin.go` / `trash_other.go`; asks first. Smoke: `Delete File… asks first`, `cancel the delete`, `Delete File… (confirmed) trashes the file and drops the item` (the file is checked gone from disk; it lands in the user's Trash as `smoke-trash.png`). |
| Update items in Schedule rewrites the schedule's snapshots of the item (title, aspect ratio, repeat, poster) | **pass** | `core.UpdateMediaItems` (`TestUpdateMediaItems`); the selected row reloads into Preview. Smoke: `the snapshot still letterboxes (Fit)` (the schedule kept Fit after the library item went back to Fill), `Update items in Schedule`, `the updated snapshot fills the canvas (Fill)`. |
| Open File Location (Finder `open -R`, Explorer `/select,`, `xdg-open` of the folder) | **pass** | `core.RevealCommand` (`TestRevealCommand`), `App.revealFile` in the background. Smoke: `Open File Location shows the square in the Finder` (last in the section: the Finder window takes the focus). |
| View ▸ Large / Medium / Small Thumbnails / Details and Sort by ▸ Name / Date Added / Kind, per tab, persisted, also from the footer sizer | **pass** | `core.ItemView` / `ItemSort` (`TestItemViewAndSort`), `core.SortMedia` / `SortPresentations` (`TestSortMedia`, `TestSortPresentations`), settings `ui.media.view|sort`, `ui.presentations.view|sort`. Details is a real list: `uicore.TileGrid.Rows` (`TestTileGridRows`; the row height never depends on the width), `SlideGrid.SetDetails`. Smoke: `View ▸ Small Thumbnails`, `View ▸ Details`, `the media Details list has content` (`media-details.png`), `Sort by ▸ Date Added puts the newest item first`, `Sort by ▸ Kind`, `view and sort survive a reload of the list`, and the presentation equivalents (`presentations-details.png`). |
| Refresh re-checks every file: missing ones are removed after a question, failed derivations retried, the list reloaded | **pass** | `core.MissingMedia` / `MissingPresentations` / `RefreshQuestion` (`TestMissingFilesAndRefreshQuestion`). Smoke: `Refresh asks about the missing file`, `cancel the refresh`, `Refresh (confirmed) drops the missing file`, `Refresh with nothing missing asks nothing`, `Refresh asks about the missing deck`, `Refresh (confirmed) drops the missing deck`. |
| Every menu item is a control action | **pass** | `media-menu {item, index\|title, confirm}`, `presentation-menu {…}` (`internal/app/menus_items.go`, chained after the settings actions); `/state.media.aspect|repeat|view|sort`, `/state.presentations.view|sort`. Smoke: `an unknown menu item is refused`, `an unknown tile is refused`, `a presentation menu item needs a tile`. |

Left out on purpose (no disabled placeholders): Edit Video Properties…, Media Usage ▸ and Copy to Theme ▸ (media);
New / Edit Presentation… (no editor), Import PowerPoint File… (Add Presentation… takes every type), Export
Presentation File… (no native format) and Update items in Schedule (presentation items re-resolve their pages by
path). Changing an item's aspect ratio updates the library deck and Preview when it shows the library item; Live and
the schedule keep their snapshots (EasyWorship's behaviour). The Songs and Scriptures lists already had their menus.

### Shared files touched (tile menus)

- `internal/models/media.go` (`Aspect` / `Repeat` in their own block, the two enums), `internal/models/slide.go`
  (`Background.Fit` / `Loop`, own block), `internal/store/migrations.go` (migration 5 appended), `internal/store/media.go`
  (the two columns, `SetMediaOptions`).
- `internal/render/slideview.go` (+14: `Fit` / `Loop` in `normalizeBackground`, `stretchFor`); `video.go` untouched.
- `internal/app/control.go` (one line: `handleItemMenuCommand` in the default chain), `core/state.go` (two own
  blocks), `view.go` (own block in `viewCache`, two projection blocks), `media.go` / `presentations.go` (the `view` /
  `sort` fields, `wire…Menu()` calls, the row fields, the state fields, the sort on load, the prefs on init; the
  pipeline's `save` re-reads the options), `core/media.go` (`Fit` in `ThumbBackground` / `StillBackground`).
- `internal/ui/slidegrid.go` (`SetDetails`, the label rect from `TileGrid.Label`), `uicore/grid.go` (`Rows` /
  `RowThumbW`, `Label`); `ui/media.go` and `ui/presentations.go` rebuilt around one menu builder each;
  `ui/itemmenus.go` is new. `mainwindow.go`, `splitter.go`, `monitor.go`, `events.go`, `icons.go` are untouched.
- `test/smoke.sh`: the `# ---- item menus` block before `# ---- quit`.

## Per-song themes

Recorded 2026-10-22 on the same machine. "smoke: X" names a check in the `# ---- song themes` section of
`test/smoke.sh` (before `# ---- item menus`). Before this, `Song.ThemeID` was stored and then ignored.

| Criterion | Status | Verification |
|---|---|---|
| A song deck uses the song's own theme when `ThemeID != 0` and the theme exists, else the configured default song theme. A deleted or unknown id falls back silently | **pass** | `core.SongTheme` (`internal/app/core/songthemes.go`), used by `App.newSongDeck`. `TestSongTheme` covers no theme, its own theme, a deleted id and a list that is not loaded yet. Smoke: `save-song with a theme (editor OK path)`, `the themed song goes live`, `the output shows the song's own theme` (an exact pixel check of `#c46a1f` in the output's top-left corner). |
| Song editor Theme picker: "(Default)" followed by every theme. OK saves `ThemeID`. The editor stays a dumb view | **pass** | `ui.SongEditor` gets a `Theme` row in its field grid (an `OptionButton` like the theme editor's, 26 px high): `SetThemeChoices([]ui.ThemeChoice)` comes from the app's render pass, and `Open(song, …)` takes the song's id. A pick only changes the editor's own state. `OptionButton.select` projects it and does not emit a signal. A theme deleted while the editor is open saves as 0. Screenshot `song-editor-theme.png`. Smoke: `open the song editor (song themes)` (`dialog.songEditorTheme`), `pick (Default) in the editor's Theme picker`, `OK saves the song`, `the song uses the default theme again`, `the output background changed back`. |
| Songs list context menu: Theme ▸ with "(Default)" and every theme, the song's own theme checked. It sets the theme without opening the editor | **pass** | `SongLibraryPanel.addThemeMenu` / `projectThemeMenu` (`internal/ui/songthemes.go`) build a named submenu (`AddSubmenuItem(label, name, songMenuTheme)` with an explicit id). It is re-projected from the selected row's `SongSummary.ThemeID` on every right-click. `OnSetTheme(row, themeID)` → `App.setSongTheme` → `store.SetSongTheme`, a targeted UPDATE that leaves the lyrics and the FTS index alone. Screenshots `song-menu.png`, `song-theme-menu.png`. Smoke: `the Theme submenu lists (Default) and every theme`, `Theme ▸ sets the song's theme from the list`, `the submenu checks the song's theme`, `the output follows the list's Theme ▸ choice`. |
| A changed theme re-previews the song when it is in Preview. Schedule items keep their snapshot, and adding the song again uses the new theme | **pass** | `App.songThemeChanged` restyles Preview in place when it shows the song as library content (`core.RestyleLibraryDeck`, the selection is kept), together with the resource preview and the list rows. Live and the schedule are left alone. `TestRestyleLibraryDeck`. Smoke: `the schedule item keeps its theme`, `the new item uses the song's new theme`. |
| Deleting a theme leaves no dangling song theme | **pass** | `store.DeleteTheme` runs `UPDATE songs SET theme_id = 0` in the same transaction. The app clears the rows and the loaded song (`App.forgetSongTheme`, `core.ForgetSongTheme`). `core.SongTheme` also ignores an unknown id. `TestSongThemes` (store), `TestForgetSongTheme`. Smoke: `delete the song's theme`, `the song falls back to the default`, `a new deck of the song uses the default`, `the editor shows (Default) after the delete`, `the output no longer shows the deleted theme`, `the submenu checks (Default) after the delete`. |
| EasyWorship import of song themes | **skipped** | All 223 local songs have `presentation_id` 0, so the link cannot be verified. EW themes are multi-slide layouts with flag-only element properties and external media, and the text style was not found. Details: [formats/easyworship-songs.md](formats/easyworship-songs.md) "Per-song themes: not imported". |
| Control actions and `/state` | **pass** | `set-song-theme {index\|title, themeName}`, `song-editor-theme {themeName}` (a pick in the open editor), `song-menu {mode: "open"\|"theme"\|"close"}` (replies with the submenu entries and the checked one), `save-song {…, themeName}`. In all of them `""` or `"(Default)"` means the default song theme, and an unknown name is refused. `screenshot?window=song-menu\|song-theme-menu`. `/state.songs.theme` (the loaded song's own theme, `""` for the default), `/state.preview.theme` / `/state.live.theme` (the deck's theme name), `/state.dialog.songEditorTheme`. |

### Shared files touched (per-song themes)

- New files: `internal/app/songthemes.go`, `internal/app/core/songthemes.go` (+ test), `internal/store/songthemes.go`
  (+ test), `internal/ui/songthemes.go`.
- `internal/models/song.go` (`SongSummary.ThemeID`, own block), `internal/store/songs.go` (`theme_id` in the two list
  queries), `internal/store/themes.go` (`DeleteTheme` clears songs).
- `internal/app/themes.go` (`newSongDeck`, `wireSongThemes()` call, `forgetSongTheme` on delete,
  `handleSongThemeCommand` as `handleThemeCommand`'s default), `view.go` (two `SetThemeChoices` lines in the Themes
  block), `control.go` (`SongsState.Theme`, `dialog.SongEditorTheme`, `actionArgs.ThemeName` in its own block, the
  two menu screenshot windows), `hooks.go` (`save-song` takes `themeName`), `core/state.go` (`DeckState.Theme`,
  `SongsState.Theme`, `DialogState.SongEditorTheme`, own blocks).
- `internal/ui/dialogs.go` (SongEditor fields in their own block, the picker row, the id in `Open` / `save`),
  `internal/ui/songs.go` (`songMenuTheme`, the panel fields in their own block, one line each in the constructor and
  `showMenu`).
- `test/smoke.sh`: the `# ---- song themes` block before `# ---- item menus`.

## Stage display (PLAN §4 v0.2 item 1)

Recorded 2026-10-22 on the same machine, with the TV unplugged (one display: the output and the stage display were
both windows on the built-in display). "smoke: X" names a check in the `# ---- stage display (v0.2)` section of
`test/smoke.sh` (before `# ---- item menus`, 98 checks; the full run passed with 786 after merging the remote control). The stage display is EasyWorship's **Foldback output**: the decoded
forms give its names (Options ▸ Foldback, "Enable support for Foldback Output", the Live Output monitor's
`ToolButton_LiveFoldBackOuputSelect` with Live Output · Alternate Output · FoldBack Output, Action_FoldBackAlert) but
not its layout, which is ours.

| Criterion | Status | Verification |
|---|---|---|
| A text-only confidence monitor: the live slide's text (white, large, auto-fitted with every lyric line kept whole when it can), the next slide (amber `#ffd15c`), item title · slide label, wall clock and service timer, white on black, never the slide's background or video | **pass** | `models.StageFrame` (comparable, so an unchanged frame costs a compare), `core.BuildStageFrame`, `layout.StageLayout` (canvas units on a canvas with the screen's aspect ratio, `TestStageLayout`: no overlaps, everything inside, 4:3, 200 %), `render.StageView` (`fitStageText` prefers the largest size with no wrapped line). Screenshot `stage-display.png`. Smoke: `the stage shows the live slide and the next one`, `the stage is black`, `the stage shows the live text in white`, `the stage shows the next slide in amber`. |
| Next slide across the item boundary: the next schedule item's first slide, headed "NEXT ITEM · How Great Thou Art · Verse 1"; "LAST SLIDE" at the end | **pass** | `core.NextStageSlide` skips items without slides (`TestNextStageSlide`), `StageFrame.NextHeader` (`TestStageFrameHeaders`). Smoke: `End on the stage item (last slide)`, `the stage announces the next item`. |
| Output modes: the musicians keep the text during Black / Clear / Logo and see a badge (BLACK `#d32f2f`, CLEAR, LOGO) | **pass** | `core.StageBadge`. Screenshot `stage-black.png`. Smoke: `black (stage)`, `the stage shows the BLACK badge`, `the stage keeps the text during Black`, `logo (stage)`. |
| Service timer: start, stop (holds), reset, toggle; green while running | **pass** | `core.StageTimer` takes the time as an argument (`TestStageTimer`), `FormatTimer` (`MM:SS`, `H:MM:SS` from an hour). `App.tickStage` re-projects every frame; the view skips equal frames. Smoke: `start the service timer`, `the timer runs`, `stop the timer`, `a stopped timer holds`, `reset the timer`, `toggle starts it`, `toggle stops it`, `an unknown timer action is refused`. |
| Notes: the operator's stage message (EasyWorship's FoldBack alert), a white-on-teal (`#00796b`) band along the bottom that only the stage display shows | **pass** | Live ▸ Stage Message… (`ui.StageMessageDialog`: Show / Enter, Clear, Cancel / Esc; 200 characters, one paragraph: `core.NormalizeStageNote`, `TestStageNote`). The body gives the band its room (`StageBoxes.NoteBand`). Not persisted. Screenshots `stage-note.png`, `stage-message.png`. Smoke: `a stage message`, `the stage shows the message band`, `the message is white on the band`, `the live text stays above the band`, `the message never reaches the output`, `the band is gone`, `Live ▸ Stage Message… opens the dialog`, `typing in the dialog` (real key events into the dialog's window), `Enter shows the typed message`, `Cancel changes nothing`, `Show (OK) puts it up`, `Clear takes it down`. |
| Its own output: a second display is covered like the projector (borderless, unfocusable, above the menu bar, `hidesOnDeactivate` off); on the primary display it is a 16:9 window that gives the keyboard back to the operator window; it never vsyncs (the output or the operator window paces the frame loop) | **pass** (window) / **untested** (a third display) | `render.Stage`, `core.StageScreenFor` (Automatic = the first non-primary display the output does not use, `TestStageScreenFor`). With the TV as the only extra display the output keeps it, so the stage is a window on the built-in display. Smoke: `Live ▸ Stage Display turns it on` (`open`, a display, no conflict, windowed unless there are three displays), `the windowed stage gives the keyboard back to the operator window`. The covered (fullscreen) path is the projector's code and is unexercised here: no third display. |
| The stage display and the output never share a display | **pass** (logic) | `core.StageConflict` refuses a stage display the output covers (and `setOutputScreen` refuses the stage's display while it is on); the Options page shows why and snaps back. `TestStageScreenFor`. Smoke: `an unconnected stage display is refused`, `the stage keeps its display`. |
| Hotkeys in the stage window | **pass** | `Stage.OnInput` is the output's handler. Smoke: `Left in the stage window is Prev`, `Right in the stage window is Next`. |
| The Live Output monitor previews the Foldback output (EasyWorship's output selector) | **pass** | `ui.MainWindow.wireOutputSelect`: the swap button opens Live Output · Alternate Output (disabled) · FoldBack Output; the caption follows. A `StageView` over the live view, which keeps its player. Screenshot `stage-monitor.png`. Smoke: `the Live Output monitor shows Foldback`, `the live monitor shows the stage's amber next slide`, `back to the Live Output`, `the live monitor no longer shows the stage`. |
| Options ▸ Stage Display: enable, display, next slide, clock, 24-hour clock, timer, text size 50–200 %; persisted as one JSON setting; Cancel restores | **pass** | `ui.SettingsDialog` `buildStagePage` (dumb: `SetStage(StageValues)` / `OnStage`), `core.StageSettings.Normalized` (`TestStageSettingsNormalized`), setting `stage.settings`. Screenshots `stage-options.png`, `stage-big.png`, `stage-plain.png`. Smoke: `the Stage Display options page`, `a 24-hour clock and bigger text`, `the stage settings are persisted` (the stored JSON), `the bigger stage text still has its next slide`, `no next slide, no timer`, `without the next slide there is no amber`, `the clock stays inside the right margin`, `Cancel restores the stage settings`. |
| Live menu: Go Live, Go Back, Show Logo, Black Output, Clear Output, Show Live, Stage Display (checked while on), Stage Message… | **pass** | `ui.Menus.buildLive` (no accelerators: the app's hotkeys own the keys). The native menu cannot be clicked by automation; `menu {item}` runs the same handlers. Smoke: `Live ▸ Stage Display turns it on` / `turns it off`, `Live ▸ Black Output turns it off`, `Live ▸ Stage Message… opens the dialog`. |
| Control actions and `/state` | **pass** | `stage {on?, screen?}` (toggle without `on`), `stage-timer {action}`, `stage-monitor {on?}`, `stage-note {text}` / `{mode: "open"\|"clear"}`, `stage-native` (the OS window state, macOS), `set-setting {key: "stage", value: {…}}` (fields merge), `options {page: "stage"}`, `native-key {window: "stage"\|"stage-message"}`, `screenshot?window=stage\|stage-message`. `/state.stage`: `settings, open, fullscreen, screen, focused, conflict, monitor, title, label, current, next, nextLabel, nextTitle, nextHeader, badge, clock, timer, timerRunning, timerSeconds, note`; `/state.dialog.stageMessage`. |

Known limits: no per-item stage layout or EasyWorship-style layout editor; slide notes do not exist in the data
model yet (the stage message is the only "notes"); the phone remote (v0.2 item 2, "Remote control") does not show
the stage frame yet (it could push the same `StageFrame`); the timer counts up only (countdowns are v0.2 item 4).

### Shared files touched (stage display)

- New: `internal/models/stage.go` (+ test), `internal/app/core/stage.go` (+ test), `internal/app/stage.go`,
  `internal/render/layout/stage.go` (+ test), `internal/render/stage.go`, `internal/render/stage_view.go`,
  `internal/ui/settings_stage.go`, `internal/ui/menus_live.go`, `internal/ui/output_select.go`,
  `internal/ui/stage_message.go`.
- `internal/app/app.go` (`stage` field, `loadStageSettings`, `tickStage`, `closeStage`), `view.go`
  (`liveMonitorContent`, `wireStage`, `renderStage`), `settings.go` (Options snapshot / Cancel, `stage` setting key,
  `onLiveMenu` as `onMenu`'s default, `stageAllowsOutputScreen`), `control.go` (`/state.stage`,
  `dialog.stageMessage`, `actionArgs.Screen` / `Action` in their own block, `handleStageCommand`, the two screenshot
  windows), `hooks.go` (`native-key` windows `stage` and `stage-message`), `core/state.go` (`State.Stage`,
  `DialogState.StageMessage`, own blocks).
- `internal/render/output.go` (`classdb.Register[StageView]`), `internal/ui/menus.go` (the Live menu's actions in
  their own block), `internal/ui/mainwindow.go` (`OnMonitorOutput`, `StageMessage` in their own blocks, the dialog
  list), `internal/ui/settings.go` (the Stage Display page).
- `test/smoke.sh`: the `# ---- stage display (v0.2)` block before `# ---- item menus`.

## Remote control (PLAN v0.2 item 2)

Recorded 2026-10-22 on the same machine (a full smoke run passed 688 checks, 41 of them in the remote block).
"smoke: X" names a check in the `# ---- remote control` section of `test/smoke.sh` (before `# ---- item menus`).
Design and security model: [REMOTE.md](REMOTE.md).

| Criterion | Status | Verification |
|---|---|---|
| LAN server (`net/http` + `gorilla/websocket`) serving an embedded phone web app with no external resources, off until turned on | **pass** | `internal/remote` (pure Go): `server.go`, `ws.go`, `web/`. `TestWebAppIsSelfContained`. Smoke: `the remote is off by default`, `turn the remote on` (on a free port), `the remote serves the phone app`, `turn the remote off`, `nothing listens on the remote port any more`. |
| Started / stopped from settings: `remote.enabled`, `remote.port` (Options ▸ Remote, toolbar Remote ▾ ▸ Remote Control On), persisted; Cancel in Options restores them | **pass** | `App.initRemote` / `syncRemoteServer` / `setRemoteEnabled` / `setRemotePort` (`internal/app/remote.go`); the Options page in `ui.SettingsDialog` (`SettingsPageRemote`). `ValidRemotePort` refuses ports below 1024. Smoke: `a port below 1024 is refused`, `the Options dialog has a Remote page` (`test/out/remote-options.png`). |
| Pairing: six-digit code + operator approval by name; per-device 256-bit tokens stored as SHA-256 hashes (`remote.devices`); revocation closes the device's sockets at once | **pass** | `pairing.go`, `devices.go`. `TestPairingFlow`, `TestDenyAndExpiry`, `TestTokenAuth`, `TestRevocation`, `TestRateLimit`, `TestLimiter`. Smoke: `a wrong pairing code is refused` (403, 4 tries left), `the right code asks the operator`, `the phone waits for approval` (kind `phone` from the User-Agent), `approve the phone` (the code changes), `the approved phone collects its token`, `an unpaired phone gets no state (401)`, `the phone is told it was removed and disconnected`, `the revoked token no longer works (401)`. |
| Network guards: local peers only, IP / localhost / `.local` Host (DNS rebinding), same-origin JSON POSTs and WebSocket upgrades, whitelisted phone actions only | **pass** | `net.go`, `actions.go`. `TestRequestGuards`, `TestAddressChecks`, `TestWebSocketOriginCheck`, `TestActionsAreWhitelisted`, `TestPermissions`. Smoke: `black off from the phone; control-only actions are unknown to it` (`quit`, `open-schedule` refused over the socket). |
| The app shows a QR code with the URL and the pairing code; waiting phones are approved / denied and paired devices removed there (`ui.RemoteDialog`, a dumb view) | **pass** | `internal/ui/remote.go`; the QR is `remote.QRPixels` (one pixel per module, nearest filtering) in a `TextureRect`. `TestQRCode`. Screenshots `test/out/remote-dialog.png`, `remote-dialog-pending.png`. Smoke: `Remote ▸ Show Pairing Code…`, `the pairing dialog shows the QR code`, `the QR code has dark modules`, `close the pairing dialog`. |
| The phone shows the schedule, the Live / Preview slide lists and the live text, pushed within a frame of every change | **pass** | `remote.BuildState` from the presenter and the schedule, published from `App.render` (`renderRemote`); `Server.Publish` sends only changed JSON. `TestBuildState`, `TestWebSocketStateAndCommands`. Checked once by hand in Chromium at phone size (Playwright): pair, approve, Schedule list, ▶ goes live, Slides tab follows Live, Next, Black, Songs search. |
| Phone actions take the operator's event path: next / prev / black (set, not toggled), go live with a schedule item, add a library song | **pass** | Commands drained in `App.Process` → `handleCommand` (`processRemote`); `black`/`clear`/`logo` take `{on}` (`App.setMode`); `remote-live-item`, `remote-add-song`. Smoke: `the phone searches the song library`, `the phone adds a song and sends it live`, `the song is live in the app`, `next, next, prev from the phone`, `the app follows the phone (slide 2)`, `the output shows the live slide`, `black from the phone (set, not toggled)`, `the app is black`, `the output is black (phone)`, `the output shows the slide again`. |
| Control actions and `/state` | **pass** | `remote {on, port}`, `remote-dialog {on}`, `remote-approve` / `remote-deny` / `remote-revoke {device \| name}`, `remote-new-code`, `set-setting remote.enabled \| remote.port`, `options {page: "remote"}`, `screenshot?window=remote`. `/state.remote`: `enabled, configuredPort, error, dialog, running, port, addresses, url, code, pending, devices, clients`. `test/remoteclient` is the smoke test's fake phone. |
| TLS | **not done** | Plain HTTP on the LAN by design (a self-signed certificate means a warning page on every phone); see REMOTE.md "No TLS". |

### Shared files touched (remote control)

- New: `internal/remote/` (whole package), `internal/app/remote.go`, `internal/app/core/remote.go` (+ test),
  `internal/ui/remote.go`, `test/remoteclient/`, `docs/REMOTE.md`. `go.mod`: `gorilla/websocket`, `rsc.io/qr` direct.
- `internal/control/server.go` (`NewCommand`, `Command.Wait` at the end).
- `internal/app/app.go` (`rm` field in its own block, `initRemote` after `loadSettings`, `processRemote` in
  `Process`, `stopRemote` in `shutdown`), `view.go` (the `remote` view cache block, `wireRemote`, `renderRemote`
  after `renderSettings`, the Remote button no longer disabled), `events.go` (`setMode`, two toolbar cases),
  `control.go` (`black`/`clear`/`logo` take `{on}`, `actionArgs.Port`/`Device` in their own block,
  `handleRemoteCommand` in the default chain, `Remote` in the state input, the `remote` screenshot window),
  `settings.go` (the Options snapshot / Cancel, `settingsValues`, two `set-setting` keys), `core/state.go`
  (`State.Remote`, own block).
- `internal/ui/mainwindow.go` (`Remote` dialog field in its own block, `dialogs()`, `dialogName`),
  `internal/ui/toolbar.go` (Remote ▾ menu, `ActionRemotePair` / `ActionRemoteToggle`, `SetRemoteOn`),
  `internal/ui/settings.go` (the Remote page).
- `test/smoke.sh`: the `# ---- remote control` block before `# ---- item menus`.

## Alerts, countdown and nursery (PLAN v0.2 item 4)

Recorded 2026-10-22 on the same machine (this run with the built-in display only: the output opened as a window).
"smoke: X" names a check in the `# ---- alerts` section of `test/smoke.sh` (before `# ---- item menus`, 91
checks; the full run passed with 877). The
feature follows EasyWorship's Alerts (`Form_AlertMgr`, `Form_AlertEdit`, `Form_AlertTagFill`, the nursery half of
`Frame_AlertsList`, the Options ▸ Alerts cards).

| Criterion | Status | Verification |
|---|---|---|
| Alert model: saved alerts with `{token}` placeholders, Display Method (Static / Scrolling / Scroll when needed), Display Count 1–9 / Until I Remove / Until Token Expiration / a duration, text alignment; tag fill; ticker (marquee) timeline; countdown and clock formats; the nursery queue | **pass** | `internal/models/alert.go`. `TestAlertNormalized`, `TestAlertTokens`, `TestFillAlertTokens`, `TestResolveDynamicTokens`, `TestMarquee`, `TestFormatCountdown`, `TestFormatClock`, `TestParseClockTime`, `TestNurseryQueue`, `TestParseAlertDisplayAndPosition`, `TestAlertStylesNormalized`. |
| Live alert state is plain Go, ticked with an injected clock: a message queue (the first on screen, Next / clear), nursery codes (auto-remove), one countdown (to a time or for a length; hide / hold at 00:00 / overrun), the clock | **pass** | `core.Alerts` (`internal/app/core/alerts.go`): `Show`, `Next`, `Tick`, `Frame`, `State`. `TestAlertTickerTimeline` (exact x and passes at 0/5/10/19/20 s), `TestAlertQueueAndEnds`, `TestAlertScrollWhenNeeded`, `TestAlertClearRemovesOneRun`, `TestCountdown`, `TestCountdownToTimeAndTokenEnd`, `TestNurseryAutoRemoveAndHide`, `TestClockAndClearAll`, `TestBannerFits`. |
| Saved alerts persist in SQLite (migration 6, three examples seeded) | **pass** | `internal/store/alerts.go`; `TestAlertCRUD`. Smoke: `three example alerts are saved`. |
| Overlay drawn above the slide on the Live monitor and the output, through transitions, video and Black / Clear / Logo; the Preview monitor and thumbnails never show it | **pass** | `render.Overlay` (`internal/render/overlay.go`): a message band (ticker or banner, top or bottom), the nursery badge, the countdown (with caption) and the clock, laid out in canvas units by `layout.PlaceOverlay` (badges in one corner stack; `TestPlaceOverlayBand`, `TestPlaceOverlayBadges`, `TestPlaceOverlayCenterAndTopBand`) and scaled to each view. It is the SlideView's last child and is raised again after every layer reorder (`raiseOverlay`). A moving ticker costs one `SetPosition` per frame and holds `AcquireAnimating`. Smoke (exact colours over Black): `the output shows the message band`, `… the nursery badge top right`, `… the countdown in the centre`, `… the clock top left`, `the live monitor shows the band`, `the live monitor shows the nursery badge`, `the preview monitor has no alerts`, `the banner band is at the top`, `the countdown moved to the bottom left`, `Black hides the alerts`, `the output is black again`. Screenshots `alerts-output.png`, `alerts-main.png`, `alerts-banner-output.png`, `alerts-output-2.png`. |
| Toolbar Alerts ▾ menu: the saved alerts, New Alert…, Alert Manager…, Nursery…, Countdown…, Clock (check), Clear All Alerts | **pass** | `MainToolbar.SetAlertsMenu` (`internal/ui/alerts.go`), projected from state in render. Smoke: `the Alerts menu opens` (entries and the Clock check), `Alerts ▸ Clock turns the clock off`, `Alerts ▸ Clear All Alerts`. Screenshot `alerts-menu.png`. |
| Dialogs: Live Alert Editor (manager: list, Add, Delete, Use, Edit…), New / Edit Alert (Tokens ▾ menu, Show without saving), Fill Alert Text Tokens (live preview, refuses empty values), Nursery (code, Add, Remove, Clear All), Countdown (to a time / for a length, caption, position, at zero, Stop) | **pass** | Dumb views in `internal/ui/alerts.go`, wired in `internal/app/alerts.go`; dialogs over dialogs use `dialogHost`. Smoke: `Alerts ▸ Vehicle asks for the plate`, `USE with an empty plate is refused`, `type the plate`, `USE shows the alert`, `type a code and Add`, `Start with a bad time keeps the dialog`, `Start replaces the countdown`, `OK without a name is refused`, `OK saves it`, `Delete asks first`, `OK deletes the alert`. Screenshots `alert-fill.png`, `alerts-nursery.png`, `alert-manager.png`. |
| Options ▸ Alerts: message location, size, scroll speed, banner time; nursery position and auto-remove; countdown format, position and end; clock format and position; hide on Black | **pass** | `SettingsDialog` Alerts page (`internal/ui/alertsettings.go`), `core.Settings.Alerts` persisted under `settings.alerts`, restored by Cancel. Smoke: `alert styles with opaque test colors`, `hide alerts on Black`, `restore the alert styles`. Screenshot `options-alerts.png`. |
| A scrolling ticker does not keep the app busy | **pass** | The overlay is projected every frame after render (`App.renderOverlay`) without marking the app dirty. Smoke: `a scrolling ticker does not keep the app busy`, `the ticker moves left`. |
| Control actions and `/state` | **pass** | `alert {text \| id \| name, values, display, location, align, count, seconds, end}`, `alert-next`, `alert-clear {run \| kind}`, `nursery {code}`, `nursery-remove {code}`, `countdown {seconds \| minutes \| time, caption, position, end}`, `countdown-stop`, `clock {on}`, `save-alert {alert}`, `delete-alert {id \| name, confirm}`, `alerts-menu {item, index \| name}`, and the dialog drivers `alert-manager {index, mode}`, `alert-editor {alert, mode}`, `alert-fill {values}`, `nursery-type {code}`, `countdown-form {…}` (OK / USE / Start is `accept`); `set-setting {key: "alerts"}` merges a partial style. `screenshot?window=alerts-menu\|alert-manager\|alert-editor\|alert-fill\|nursery\|countdown`. `/state.alerts`: `active[]` (run, text, style, location, pass, remainingPasses, remaining, x, showing), `message`, `nursery`, `countdown`, `clock`, `saved`, `showing`, `moving`, `hidden`; `/state.dialog.alertDialog`, `/state.dialog.alertsMenu`. |

Known limits: alert text is one line in one font (no RTF formatting from EasyWorship's `Alerts.db`, which is not
imported); the styles' font family has no picker yet (the JSON setting takes one); a ticker's speed is in canvas
units per second rather than EasyWorship's slow / medium / fast; there is no hotkey for Next / Clear.

### Shared files touched (alerts)

- New files: `internal/models/alert.go`, `internal/app/core/alerts.go`, `internal/store/alerts.go`,
  `internal/render/layout/overlay.go` (each + test), `internal/render/overlay.go`, `internal/ui/alerts.go`,
  `internal/ui/alertsettings.go`, `internal/app/alerts.go`.
- `internal/store/migrations.go` (migration 6), `internal/render/slideview.go` (the `overlay` field, two
  `raiseOverlay` / `relayoutOverlay` lines), `internal/render/transitions.go` (one line), `internal/ui/toolbar.go`
  (`OnAlertsMenu`, `alertsMenu`, the Alerts press), `internal/ui/mainwindow.go` (dialog fields in their own block,
  `buildAlertDialogs`, `dialogs` / `dialogName`), `internal/ui/settings.go` (the Alerts page, `SettingsValues.Alerts`).
- `internal/app/app.go` (`al` field, `initAlerts`, `tickAlerts` / `renderOverlay` in `Process`), `view.go` (view
  memo block, `wireAlerts`, `renderAlerts`; the Alerts button is no longer disabled), `control.go` (`actionArgs`
  block, `/state.alerts`, `dialog.alertDialog`, the screenshot windows, `handleAlertsCommand`), `settings.go`
  (load / restore / `set-setting alerts`), `core/settings.go` (`Settings.Alerts`), `core/state.go` (own blocks).
- `test/smoke.sh`: the `# ---- alerts` block before `# ---- item menus`.

## Audio playlists (PLAN v0.2 item 3, second half)

Recorded 2026-10-22 on the same machine (HDMI TV as the projector). "smoke: X" names a check in the
`# ---- audio playlist` section of `test/smoke.sh` (before `# ---- item menus`, 93 checks; the full run passed with 970). EasyWorship has no
playlist (its audio files are schedule items with Media Usage / Repeating, `Form_VideoProperties`), so the design
is our own and minimal; the decisions are in [MEDIA.md](MEDIA.md) "Audio playlist".

| Criterion | Status | Verification |
|---|---|---|
| Playlist model: ordered library audio files with stable track ids; repeat off / all / one, shuffle, fade-out on stop; saved with the schedule (`playlist` key, left out while empty; old schedules open with an empty one; version stays 1) | **pass** | `internal/models/playlist.go`. `TestPlaylistEdits`, `TestRepeatModes`, `TestPlaylistNormalize`, `TestTrackFromMedia`, `TestSchedulePlaylistRoundTrip` (empty playlist not written, a pre-playlist `.fws` opens, the settings round-trip, edits mark dirty). Smoke: `the saved schedule has the playlist`, `a new schedule has no playlist; the music stops`, `open the schedule again`, `the reopened playlist plays`. |
| Player logic in pure Go: track order, the end of the list, repeat, shuffle without repeats, skipping missing / failing files, Prev restart, fade-out, edits while playing | **pass** | `core.AudioPlayer` (`internal/app/core/audio.go`, injected `Exists` / `Rand`). `TestAudioOrderAndEnd`, `TestAudioSkipsMissingFiles`, `TestAudioShuffleWithoutRepeats` (30 passes of 7 tracks: each pass a permutation, never a track twice in a row; shuffle on mid-list; a track added mid-pass; a picked track), `TestAudioPrevNextStopped`, `TestAudioFadeOut`, `TestAudioSyncAfterEdits`, `TestAudioState`. |
| Plays independently of the Live slide: a song going live, Black, Clear and Logo leave it playing | **pass** | Its own audio-only player role (`render.RoleAudio`, `internal/render/audiosource.go`; additive, `nativePlayer` unchanged), opened and closed by `App.renderAudio`, pumped by the app every frame. Smoke: `the song goes live; the music plays on`, `black; the music plays on`, `the music advances under Black`, `logo; the music plays on`, `back to normal (audio)`. |
| Position, end of track → next, next / prev / seek / volume / repeat / shuffle / stop / fade-out | **pass** | Smoke: `the first track plays`, `the position advances`, `the first track ends and the second starts`, `next skips the missing file`, `prev goes back over the missing file`, `play a track by index`, `a missing track cannot be played`, `the list ends: stopped, the first track cued`, `repeat all wraps to the first track`, `repeat one plays the track again`, `shuffle on` (the order is a permutation led by the current track), `the position holds while paused`, `stop at once`, `stop fades out`, `the volume ramps down`, `the fade ends stopped`, `a deleted file is marked missing`. |
| Operator UI: the Audio Playlist panel under the schedule (track list, its own transport), Media tab ▸ Add to Audio Playlist | **pass** | `ui.AudioPlaylist` (`internal/ui/playlist.go`), dumb view; icons `stop`, `shuffle`, `repeat_one`. Smoke: `add a folder of tones to the playlist` (files added to the library too), `adding a library item again (Media tab menu)`, `a video cannot join the playlist`, `move the last track to the top`, `remove the missing track`, `Clear Playlist asks first`, `clear the playlist (confirmed)`. Screenshots `audio-playlist-main.png` (missing file dimmed), `audio-playlist-playing.png` (repeat all). |
| Phone remote: play / pause, next and previous track | **pass** | `audio-toggle`, `audio-next`, `audio-prev` in `remote/actions.go` (`TestActionsAreWhitelisted`); `remote.State.audio {title, status, tracks}` (`TestBuildState`); the phone's music bar (`web/index.html`, `app.js`). Smoke: `the phone plays the music and skips a track`, `the phone goes back a track and pauses`. |
| Control actions and `/state.audio` | **pass** | `audio-play {index}`, `audio-pause`, `audio-toggle`, `audio-stop {fade}`, `audio-next`, `audio-prev`, `audio-seek {seconds}`, `audio-volume {value}`, `audio-repeat {mode}`, `audio-shuffle {on}`, `audio-fade-out {seconds}`, `playlist-add {index \| title \| path \| paths}`, `playlist-remove {index}`, `playlist-move {from, to}`, `playlist-clear {confirm}`, `media-menu {item: "add-to-playlist"}`. `/state.audio`: `status, playing, track, title, next, order, position, duration, volume, gain, fading, repeat, shuffle, fadeOut, tracks [{title, path, seconds, missing}], native, loaded, failed`. Smoke: `an empty playlist cannot play`, `a seek needs a playing track`, `a bad repeat mode is refused`, `a fade-out over 30 s is refused`, `a volume that is not a number is refused (audio)`, `a move out of range is refused`. |

Known limits: it plays through the same player layer as the monitors, so the ffmpeg player covers Windows / Linux
(verified on macOS with `FREEBEAMS_PLAYER=ffmpeg`, see "Video and audio off macOS" below); no drag-and-drop into the list, no crossfade between tracks, no
library of saved playlists, and the music goes to the default output device like the monitors' audio.

### Shared files touched (audio playlists)

- New files: `internal/models/playlist.go`, `internal/app/core/audio.go` (each + test), `internal/render/audiosource.go`,
  `internal/ui/playlist.go`, `internal/app/audio.go`.
- `internal/models/schedule.go` (the `Playlist` field in its own block), `schedule_ops.go` (+1: `Playlist.Normalize`
  on load); `internal/render/videosource.go` untouched (the new role and `OpenAudio` / `CloseAudio` / `PumpMedia` live in
  `audiosource.go`); `internal/ui/mainwindow.go` (the `AudioPlaylist` field block, +2 in `buildSchedule`),
  `internal/ui/media.go` (the `Add to Audio Playlist` entry, id and callback in their own blocks),
  `internal/ui/uicore/icons.go` (an `audio playlist` block after the transport icons).
- `internal/app/app.go` (`au` field block, `initAudio`, `audioEvents` and the per-frame `renderAudio` in `Process`),
  `view.go` (view-cache block, `wireAudio`, `renderAudio` in `render`), `control.go` (`Fade` arg block, `/state.audio`,
  `handleAudioCommand` in the dispatcher chain), `menus_items.go` (`media-menu add-to-playlist`), `remote.go` (+1),
  `core/state.go` (own blocks); `internal/remote/state.go`, `actions.go`, `remote_test.go`, `web/*` (the music bar).
- `test/smoke.sh`: the `# ---- audio playlist` block before `# ---- item menus`. Docs: `MEDIA.md`, `REMOTE.md`,
  `UI-SPEC.md` §3.3, `CLAUDE.md` (gotchas appended).

## Video and audio off macOS: the ffmpeg player (PLAN v0.2 item 3, PLAN §3 option B)

Recorded 2026-10-22 on the same machine (HDMI TV as the projector, no Screen Recording permission). Windows and
Linux play every video and audio file through ffmpeg processes behind the `nativePlayer` interface, so the
transport, looping, seeking, volume, the crossfade and the audio playlist work as on macOS; the design is in
[MEDIA.md](MEDIA.md) "the ffmpeg player". On this Mac it was verified with `FREEBEAMS_PLAYER=ffmpeg`.

| Criterion | Status | Verification |
|---|---|---|
| Decoding, clock and pipes in a pure-Go package, unit-tested against clips ffmpeg generates | **pass** (macOS) / **untested** (CI on Windows, Linux) | `internal/media/ffplayer`: `TestOpenPausedShowsFirstFrame` (Loading → Ready, frame 0 shown while paused, position exactly 0), `TestExactColor` (BT.709 `#2a7f62` ±3), `TestScaledToFit` (1080p → 640×360), `TestWallClockPlayback` (no sound: wall clock, pause holds, end reported once, Play after the end restarts at 0), `TestAudioClockAndGaplessLoop` (the audio clock drives the frames; across the loop point the position wraps to 0.3 s; loop off mid-pass ends at exactly 2 × duration samples), `TestOnePassIsExactlyTheDuration`, `TestSeek` / `TestSeekBeforeReady` (position = target at once, the target's frame while paused, a new audio epoch), `TestAudioOnly` (mono upmixed), `TestWallClockWhenAudioIsNotConsumed` (a hitch is not a stall; a stall hands over; the audio clock takes back without going backwards), `TestFailures`, `TestCloseStopsEverything` (Done closes, no goroutines left), `TestParseProbe` (cover art skipped, rotation, SAR, fps cap), `TestFitSize`, `TestRing`; all pass with `-race`, and also against the bundled LGPL build (`FREEBEAMS_FFMPEG_DIR`). CI runs them on `windows-latest` and `ubuntu-latest` (`ci.yml` job `media`), not yet run. |
| Video, audio and sync: RGBA frames into the source texture, f32le into an `AudioStreamGenerator`, the audio clock as master | **pass** (macOS, `FREEBEAMS_PLAYER=ffmpeg`) | `internal/render/ffplayer.go`. Smoke (playback block with the ffmpeg player): `FREEBEAMS_PLAYER=ffmpeg selects that player`, `the preview player opened the clip (video and audio)`, `the preview is still paused at 0`, `the live player plays the clip`, `the live position advances`, `the output shows the clip` / `the live monitor shows the clip` / `the preview monitor shows the clip's first frame` (exact `#2a7f62` pixel checks), `pause`, `the position holds while paused`, `the position follows the seek`, `the clip stops at its end with loop off`, `black off brings the clip back`, `the clip plays again from the start`, `the preview plays while live plays` (two decoders), `the live player plays the tone (audio only)`, `the tone's position advances in real time` (the audio clock), `seek the tone while it plays` / `the tone plays on from the seek` (the generator is restarted on a new epoch), `the tone stops at its end`, `the ffmpeg player decoded every sound track`. |
| The audio playlist on the ffmpeg player (`RoleAudio`, no view) | **pass** (macOS, `FREEBEAMS_PLAYER=ffmpeg`) | The adapter finds the scene root through `Engine.GetMainLoop`, so a source no view holds has sound. Smoke: the whole `# ---- audio playlist` block (track ends advance, next / prev, repeat, shuffle, seek, fade-out) plus `the stopped playlist leaves no decoder running` (after the fade), `the ffmpeg player decodes the playing track`, `stop kills the track's decoder`. |
| Selection: AVFoundation on macOS, ffmpeg on Windows / Linux (bundled copy, then PATH), the Theora fallback without ffmpeg; `FREEBEAMS_PLAYER` override | **pass** | `core.ChoosePlayer` (`TestChoosePlayer`), `media.Locate` (`TestLocate`: `$FREEBEAMS_FFMPEG_DIR`, the executable's folder, PATH), `render.UseMediaPlayer`; logged `[freebeams] media player: ffmpeg (/opt/homebrew/bin/ffmpeg)`; `/state.media.player`, `/state.playback.*.player`. |
| Badges and footer truthful per backend | **pass** | `MediaCaps.Native` is true for AVFoundation and ffmpeg; on the Theora fallback audio items say `Needs ffmpeg` and the footer `ffmpeg not found: audio cannot play` / `videos and audio cannot play` (`TestMediaBadge`, `TestMediaFooter`). |
| Every spawned ffmpeg / ffprobe (and pdftoppm / soffice) hides its console on Windows | **pass** (cross-vetted) / **untested** (Windows) | `internal/sysproc` (`HideWindow` + `CREATE_NO_WINDOW`, `TestHideWindow`); `media.Tools`, the player and `presentations` use `sysproc.CommandContext`. `GOOS=windows go vet` is clean. |
| Bundled LGPL ffmpeg in the Windows zip / installer and the Linux tar.gz / AppImage, pinned and checksummed | **partial** | `bin/build-ffmpeg` (FFmpeg 8.1.3, SHA-256 checked, no GPL / nonfree, static, trimmed encoders / muxers; fails unless configure reports LGPL and the needed muxers / encoders exist), `bin/build-windows`, `bin/build-linux`, `packaging/ffmpeg/NOTICE.txt`, `release.yml` (cache, MSYS2 / apt build deps, the player tests against the bundle), README licensing, [RELEASE.md](RELEASE.md) "Bundled ffmpeg". Built and tested on macOS arm64 only; the Windows (MSYS2) and Linux (static glibc) builds have not run. |

Smoke on 2026-10-22, after merging main (audio playlists): `FREEBEAMS_PLAYER=ffmpeg test/smoke.sh` **979/979
PASS** (Homebrew ffmpeg 8.1.1; three checks run only with the ffmpeg player), and the default AVFoundation run
**976/976 PASS**, log hygiene clean in both. Earlier attempts failed outside the media blocks: `close the theme
editor` (ffmpeg run, a focus check), and three AVFoundation runs while an installed `/Applications/FreeBeams.app`
was open and frontmost (`the song is live in the app` with `"mode": "black"`, the app quitting itself at `select the
clip`, `Cmd+C in a text field is not Clear`); each passed on the rerun once it had closed.

Not verified here: the app on a real Windows or Linux machine (audio devices, the Windows process kill, the static
ffmpeg builds), the CI jobs, a long-running loop for drift (the unit tests check that a pass is exactly the
container duration in samples), and CPU cost at 1080p (two decoders per open file plus RGBA conversion in ffmpeg).

### Shared files touched (ffmpeg player)

- New: `internal/media/ffplayer/` (player, decoders, probe, ring, tests), `internal/media/locate.go` (+ test),
  `internal/sysproc/`, `internal/render/player.go`, `internal/render/ffplayer.go`, `internal/app/player.go`,
  `internal/app/core/player.go` (+ test), `bin/build-ffmpeg`, `packaging/ffmpeg/NOTICE.txt`.
- `internal/render/videosource.go` (`ticker` hook in `pump`, ffmpeg's error in the failure log;
  `NativeVideoSupported` moved to `player.go`, `acquireSource` unchanged), `avplayer_darwin.go` /
  `avplayer_other.go` (`openAVPlayer`, `avFoundationAvailable`), `video.go` (`NativeVideoSupported()`).
- `internal/app/playback.go` (+1 in `initPlayback`, `Player` in the state doc), `media.go` (`Player` in caps and
  state), `core/media.go` (audio badge / footer off the media player, `MediaCaps.Player`), `core/state.go`,
  `core/playback.go` (`Player` fields in their own blocks).
- `internal/media/ffmpeg.go` (`Detect` uses `Locate`, `sysproc`), `media_test.go` (a portable cache path),
  `internal/presentations/tools.go` (`sysproc`).
- `test/smoke.sh` (playback block: the player check, audio-clock / seek / end checks; playlist block: the ffmpeg
  process checks), `bin/build-windows`, `bin/build-linux`, `bin/setup-windows.ps1`, `.github/workflows/ci.yml`,
  `release.yml`. Docs: `MEDIA.md`, `RELEASE.md`, `README.md`, `CLAUDE.md` (gotchas appended).

## Preview Output menu: restyling one item

Recorded 2026-10-22 on the same machine. "smoke: X" names a check in the `# ---- preview restyle` section of
`test/smoke.sh` (89 checks, before `# ---- item menus`; the full run passed with 1065). The operator's case: the default theme and the words do not match, so
the item in Preview gets another background or text style before it goes live.

| Criterion | Status | Verification |
|---|---|---|
| Right-click (and Control-click on macOS) on the Preview Output monitor opens a context menu; left and double clicks are left alone | **pass** | `ui.PreviewMenu` (`internal/ui/previewmenu.go`) connects the monitor's `gui_input` signal (no method on `MonitorHost`), opens on `uicore.IsContextClick` (`TestIsContextClick`) at the click point and accepts only that event. The menu is an internal child of the monitor; its submenus are named children with explicit ids. Screenshots `preview-menu.png`, `preview-text-menu.png`, `preview-theme-menu.png`, `preview-background-menu.png`. Smoke: `a right-click on the Preview monitor opens its menu` and `a Control-click opens it too` (real `InputEventMouseButton`s through `Input.ParseInputEvent`), `a right-click on the Live monitor opens no menu`. |
| Entries after EasyWorship's names: Change Theme ▸, Background ▸ (Select Media…, Solid Color…, Reset to Theme Background), Text ▸ (Increase / Decrease Font, Font Color…, Bold, Italic, Outline, Shadow, Align Left / Center / Right, Reset to Theme Text), Edit Theme for This Item…, Revert to Global Theme | **pass** | UI-SPEC §3.4. The checks, radios and disabled entries are projected from state (`App.previewMenuState`). Smoke: `Text ▸ shows the item's text style` (Bold, Outline, Shadow, Align Center checked; Reset disabled), `Change Theme ▸ checks the item's theme`, `the Reset entries are enabled now`, `the menu is disabled for media`, `restyling a media item is refused`. |
| Background: a solid color (color dialog) or a library image / video (thumbnail picker); reset | **pass** | `ui.ColorDialog` (a `ColorPicker`, sampler hidden: it needs Screen Recording permission), `ui.MediaPicker` (a `SlideGrid` of the library's images and videos, `core.BackgroundMedia`); `core.SolidBackground`, `core.MediaBackground` (audio refused; a video loops), `core.RestyleTheme` (`TestRestyleTheme`, `TestMediaBackground`). Screenshots `preview-color.png`, `media-picker.png`, `preview-solid.png`, `preview-image.png`. Smoke: `Solid Color… opens the color dialog`, `pick #1f3f9f`, `OK restyles the preview`, `the Preview monitor shows the solid background` (exact pixels), `Select Media… opens the media picker`, `OK sets the item's background`, `the Preview monitor shows the library image`, `the schedule thumbnail shows it too`. A video background (`Fall Skies 1`) was checked by hand: it plays in the monitor, thumbnails show its poster. |
| Text: color, size, outline / shadow, alignment, reset; font family and the rest in the item's theme editor | **pass** | `core.RestyleTheme` (Increase / Decrease Font step 8 canvas units, clamped 16..400, `MinSize` scaled along), `ThemeEditor.OpenItem` (fixed name; OK / Apply restyle the item through `App.applyItemEditor`, never `saveTheme`). Screenshots `preview-text.png`, `preview-item-editor.png`. Smoke: `Font Color… #ffd400`, `Increase Font`, `Decrease Font`, `Shadow off`, `Shadow on`, `the Preview monitor shows the yellow text`, `Edit Theme for This Item… opens the editor`, `its live preview shows the item's image`, `OK keeps the item's look and saves no theme`, `preview-theme sets a size and the alignment`, `Reset to Theme Text keeps the background`. |
| Scope: only the item. Schedule item → its theme copy, dirty, saved in the `.fbs`; library item → carried to Live and into the schedule; the saved theme never changes | **pass** | `App.restylePreview` (`internal/app/previewstyle.go`): `core.ApplyItemTheme` for a schedule item, else the library deck behind the resource preview; `core.CarryStyle` in `useSong` / `addSongToSchedule` (`TestCarryStyle`). `models.Theme.Custom` marks the copy (`TestThemeSameLook`, `TestScheduleKeepsCustomTheme`); `core.RestyleSchedule` / `RestyleByThemeID` skip custom copies (`TestRestyleSkipsCustomCopies`); `store.SaveTheme` clears the flag (`TestSaveThemeClearsCustom`). Smoke: `the restyled library item goes live`, `the output shows the solid background`, `the output shows the yellow text`, `the saved theme is unchanged` (byte-equal `get-theme`), `another verse keeps the override (library deck)`, `the restyled song goes into the schedule`, `select the restyled schedule item`, `the schedule file keeps the override`, `the reopened item keeps its look`. |
| Live changes only when sent live again; no menu on the Live monitor | **pass** | `Presenter.RestylePreview` changes the Preview deck only (`TestPresenterRestylePreview`). Smoke: `Live keeps its look until sent live`, `the Live monitor does not`, `Live keeps the solid background`, `the output keeps the solid background until sent live`, `send it live again`, `now the output shows the image`. |
| Monitors and thumbnails update at once | **pass** | Render refreshes a grid's thumbnails when its deck's theme changed in place (`viewCache.previewTheme` / `liveTheme`; before this, a restyled deck kept stale tiles); the schedule row rebuilds through `schedVer`; a theme video background is drawn as its poster in thumbnails. |
| Revert | **pass** | `core.BaseTheme` (`TestBaseThemeAndCustomize`). Smoke: `Change Theme ▸ Default Scripture`, `Change Theme ▸ Default Song`, `Revert to Global Theme`, `the reverted Preview has no solid background`, `nor the image`, `the saved theme is still unchanged`. |
| Control actions and `/state` | **pass** | `preview-menu {item, sub, name\|index, title, color, on}` (items `open\|close\|theme\|background-media\|background-color\|background-reset\|font-bigger\|font-smaller\|text-color\|bold\|italic\|outline\|shadow\|align-left\|align-center\|align-right\|text-reset\|edit-theme\|revert`; the reply lists the menus' entries), `preview-theme {theme \| backgroundColor, resetBackground, resetText, font, size, color, bold, italic, outline, shadow, align}`, `preview-color {color}` / `preview-media {title\|index}` (a pick in the open dialog; `accept` is OK), `monitor-context-click {monitor, ctrl}`, `screenshot?window=preview-menu\|preview-theme-menu\|preview-background-menu\|preview-text-menu\|color\|media-picker`. `/state.preview.style` / `/state.live.style` (`custom, background, backgroundColor, backgroundPath, font, size, minSize, color, bold, italic, outline, shadow, align`), `/state.dialog.previewMenu`, `/state.dialog.previewDialog` (`color\|media\|item-theme`). |

### Shared files touched (Preview Output menu)

- New files: `internal/ui/previewmenu.go`, `internal/app/previewstyle.go`, `internal/app/core/itemstyle.go` (+ test),
  `internal/ui/uicore/contextclick.go` (+ test).
- `internal/models/theme.go` (`Theme.Custom` in its own block, `Theme.SameLook`), `presenter_impl.go`
  (`RestylePreview`), `internal/store/themes.go` (one line in `SaveTheme`), `internal/app/core/themes.go` (custom
  copies skipped), `core/state.go` (`DeckState.Style`, `DialogState.PreviewMenu/PreviewDialog`, own blocks).
- `internal/app/app.go` (the `ps` field), `view.go` (the `wirePreviewStyle` and `renderPreviewStyle` calls, the grid
  theme check, the thumbnail poster for a theme video, the editor frame, `viewCache` fields in their own block),
  `events.go` (`core.CarryStyle` in `useSong` / `addSongToSchedule`), `themes.go` (one condition), `songthemes.go`
  (`handlePreviewStyleCommand` as the default), `control.go` (one line in `state()`, the screenshot default).
- `internal/ui/mainwindow.go` (three fields in their own block, `buildPreviewMenu`, the two dialogs), `themeeditor.go`
  (`OpenItem` / `ItemMode`). `internal/ui/monitor.go` is untouched.
- `test/smoke.sh`: the `# ---- preview restyle` block before `# ---- item menus`.

## Scripture locator: book suggestions, whole chapters, multi-verse selection

Recorded 2026-10-22 on the same machine (HDMI TV as the projector, no Screen Recording permission). The design is in
[UI-SPEC.md](UI-SPEC.md) §3.6 "Scripture locator".

| Criterion | Status | Verification |
|---|---|---|
| A partial or ambiguous book lists the matching books instead of an error; picking one completes the field | **pass** | `models.BooksMatching` / `BookCompletions` / `CompleteBook` (`TestBooksMatching`, `TestBookCompletions`, `TestCompleteBook`); `FindBook`'s ambiguity is an `*AmbiguousBookError` (`TestFindBookAmbiguousError`). The dropdown is `ui.suggestionList` (`FwSuggestionList`, over the translations list, never focused); keys in `ScripturePanel.referenceKey`; state in `App.refText` / `suggestSel` / `suggestOff` (`internal/app/scripture.go`). Smoke (typed through Godot's input pipeline): `type j` … `type h` (John, 1 John, 2 John, 3 John shown), `joh is not an error (John 1 is listed)`, `Down highlights 1 John`, `Enter picks 1 John`, `1 John 1 is listed`, `Esc closes the suggestions`, `click the Genesis suggestion`, `xyz 3 is an error`, `jo 3 is ambiguous, not an error`. Screenshot `scripture-suggestions.png`. |
| A book alone shows its first chapter | **pass** | `models.ParseLocator` (`TestParseLocator`: `gen` → Genesis 1, `John ` → John 1, `1` ambiguous). Smoke: `a book alone lists its first chapter` (Genesis 1, 31 verses). |
| `john 3` lists the whole chapter; `john 3:16` / `john 3:16-18` list chapter 3 with those verses selected | **pass** | `Passage.Chapters` / `Passage.Contains` (`TestPassageChapters`, `TestPassageContains`), `core.ListedVerses` / `VersesIn` (`TestListedVersesAndVersesIn`); the first selected verse is centred in the list. Smoke: `reference John 3:16-18` (36 rows, 15–17 selected), `john 3 lists the chapter` (36 verses, all selected, Preview `John 3` with 36 slides), `john 3:16 selects verse 16`. |
| Multi-verse selection (Shift-click, Cmd/Ctrl-click, Shift+arrows, Cmd+A) previews exactly those verses as one passage | **pass** | `core.RowSelection` (`TestRowSelectionClick`, `TestRowSelectionStep`, `TestRowSelectionAll`) is app state; the Tree (`SelectMulti`) only projects it, its clicks and keys are taken in `ScripturePanel.verseInput`. `models.SelectionPassage` names the selection (`TestSelectionPassage`: `John 3:16-18, 20`, `John 3:36-4:2`, `Psalms 1-2`, round trip through `ParseReference`); the field shows it. Smoke (real clicks, `click {target: "verse"}`): `click verse 16`, `Shift-click verse 18` (Preview `John 3:16-18`, 3 slides), `Cmd-click verse 20 adds it`, `Cmd-click verse 17 removes it` (`John 3:16, 18, 20`), `Shift+Down extends from the anchor`, `a plain click in the selection` / `narrows it once no double-click follows`. Screenshot `scripture-multi-preview.png`. |
| Go Live, double-click, Enter and Add to Schedule use the selected verses | **pass** | `App.goLiveVerse` / `activateVerse` / `addScriptureToSchedule`. Smoke: `Go Live sends the three verses` (live `John 3:16-18`, 3 slides; `the passage is on the output`, screenshot `scripture-multi-output.png`), `a double-click in the selection sends it all live` (at the double-clicked verse), `add the passage to the schedule`; the existing `resource Go Live sends the highlighted verse` / `Go Live again follows the preview`. |
| A double-click on the Preview monitor is Go Live; single clicks unchanged | **pass** | `ui.MonitorHost.OnDoubleClick` (left double-click only, accepted; other clicks pass on). Smoke: `a single click on the Preview monitor does nothing`, `a double-click on the Preview monitor goes live`. |
| Control actions and `/state` | **pass** | `reference {text, bible}` (also sets the field), `pick-suggestion {index}`, `select-verses {rows}`, `click {target: "verse"\|"suggestion"\|"preview-monitor", index, shift, meta, ctrl, double, button}`. `/state.scripture` adds `listed`, `rows`, `field`, `suggestions`, `suggestion`, `suggestionsShown`; `selected` is now the list row of the current verse and `verses` the listed rows. |

Smoke on 2026-10-22, after merging main: `test/smoke.sh` **1017/1017 PASS**, log hygiene clean (the locator block
adds 41 checks; five existing scripture checks now expect whole-chapter lists and list rows). One earlier run after
the merge failed outside this work, `screenshot song-menu is not a PNG: {"ok":false,"error":"screenshot: viewport
\"\" has no image yet"}`, while Slack was the frontmost app; it passed on the rerun.

### Shared files touched (scripture locator)

- New: `internal/models/locator.go` (+ test), `internal/app/core/selection.go` (+ test), `internal/app/core/scripture_test.go`,
  `internal/app/scripture.go` (the lookup and verse handlers moved out of `events.go`), `internal/ui/suggest.go`.
- `internal/models/scripture_books.go` (`AmbiguousBookError`, `BooksMatching`), `reference.go` (`splitBook`).
- `internal/app/core/deck.go` (`ListedVerses`, `VersesIn`, `PassageStatus`, `ChooseBookText`), `state.go` (a
  `ScriptureState` block), `internal/app/app.go` (a field block; `verseSel` is a `core.RowSelection`), `control.go`
  (state fields, `pick-suggestion`, `select-verses`, an `actionArgs` block), `hooks.go` (`click`, `isRowTree`
  matches `SelectMulti`), `view.go` (wiring, the projection, a `viewCache` block).
- `internal/ui/scripture.go` (rewritten input and projection; `SetVerses(rows)` + `SetVerseSelection`), `search.go`
  (`onEdit`, caret at the end), `monitor.go` (`OnDoubleClick`), `register.go`, `demo.go`.
- `test/smoke.sh` (scripture block), `docs/UI-SPEC.md` §3.6, `CLAUDE.md` (gotchas appended).

## Spell check in the song editor

EasyWorship's Presentation Editor checks spelling as you type (a 250 ms timer), has a SpellCheck mini button with
a ▾ (Check Spelling, Spelling Options…) and a Spelling dialog (`Form_SpellCheckDialog`, 337 × 237). FreeBeams
does the same in the song editor.

- **Engine** (`internal/spell`, pure Go apart from the darwin cgo): `Engine` implements `spell.Checker`
  (`Check(text, lang) []Misspelling{Start, Len, Word, Line, Col}`, `Suggest`, `Learn`, `Ignore`) over a `Backend`
  that judges single words. `Tokenize` skips section header lines in every form (`[Verse 1]`, `Chorus:`, `V1`,
  `C2`: `models.IsSectionHeader`), bracketed text and any run with a digit, and keeps apostrophes inside words
  (`I'll`, `Lord’s`). Personal words, session-ignored words and the worship vocabulary (`spell.Seed`: Hallelujah,
  Hosanna, Emmanuel, Jehovah, Yahweh, Selah, Maranatha, Thee/Thy/Thine, …) never reach the backend; verdicts are
  cached per word.
- **macOS backend**: `NSSpellChecker` (`nsspell_darwin.go`). One `requestCheckingOfString` for all the words of a
  check (joined by newlines), awaited on a semaphore, so it runs on a goroutine; guesses come from
  `guessesForWordRange`. "Add to Dictionary" stays in FreeBeams's settings (`spell.personal`): the user's
  system-wide dictionary is never written.
- **Fallback (Windows, Linux, and the unit tests)**: the embedded SCOWL American English list up to size 60 (the
  size of the usual en_US Hunspell dictionary; 123,000 words, 330 KB gzipped; licence in
  `internal/spell/data/SCOWL-LICENSE`, generator in `internal/spell/gen`) with a Norvig-style suggester (one edit,
  else two; transpositions first, then by word frequency rank). Chosen over "unavailable" because Windows and
  Linux are release targets, the licence is permissive, the size is small, and it makes the checker testable
  everywhere.
- **Editor**: the lyrics TextEdit's own context menu is off; a right click opens ours (suggestions, Ignore, Add to
  Dictionary, then Undo/Redo/Cut/Copy/Paste/Select All through `TextEdit.menu_option`). Underlines are a red wavy
  polyline drawn from the TextEdit's `draw` signal (see CLAUDE.md). The Lyrics header has the SpellCheck split
  button; its ▾ has Check Spelling…, Check Spelling as I Type (`spell.live`) and Language ▸ (`spell.language`,
  the checker's languages). The Spelling dialog walks the lyrics with `spell.Session`: Change / Change All /
  Ignore / Ignore All / Add to Dictionary / Close (Change All stands where EasyWorship has Find Next), editing the
  TextEdit in place as undoable operations and selecting each misspelling.
- **Control**: `spell-check {text}` (the misspellings), `spell-suggest`, `spell-learn`, `spell-forget`,
  `spell-ignore`, `spell-live {on}`, `spell-language {name}`, `editor-lyrics {lyrics}`, `spell-menu {index}`,
  `spell-menu-pick {item}`, `spell-dialog {mode, text}`; `dialog.spell` in `/state` while the editor is open
  (flagged words, checking, personal words, menu entries, the dialog); screenshots `spelling` and `spell-menu`.
- **Tests**: `internal/spell` unit tests (tokenizing, tags, apostrophes, personal and ignored words, the session,
  the word list and its suggestions), a darwin test hitting `NSSpellChecker` ("recieve" → "receive"), and the
  smoke block `# ---- spell check` (flagged, underline pixels, menu, dialog, learn / forget, tags not flagged).
- **Limitations**: no grammar checking; NSSpellChecker accepts some typos it would autocorrect (e.g. "teh"); the
  fallback has one language (en_US) and no affix rules beyond its expanded list; the dialog checks on the main
  thread (a user-initiated, short check); a flag shifted by an edit earlier on its line disappears until the next
  check (250 ms) rather than being drawn on the wrong letters.

### Shared files touched (spell check)

- `internal/models/lyrics.go` (`IsSectionHeader`), `internal/ui/dialogs.go` (SongEditor fields, header row, Open /
  Close), `internal/ui/mainwindow.go` (the Spelling dialog in `dialogs()` and `dialogName`),
  `internal/ui/uicore/icons.go` (`spellcheck`), `internal/app/app.go` (field, `initSpell`, `busy`),
  `internal/app/control.go` (dispatch, screenshot, `dialog.Spell`), `internal/app/view.go` (`renderSpell`),
  `internal/app/core/state.go` (`DialogState.Spell`), `test/smoke.sh`, `CLAUDE.md`.

## Editable hotkeys, default themes per item type and the Zoom transition (Options)

Recorded 2026-10-22 on the same machine (HDMI TV as the projector).

| Criterion | Status | Verification |
|---|---|---|
| Hotkeys are data: a `core.Keymap` (action id → key chords) with EasyWorship's keys as the defaults, applied live to `core.Hotkeys` | **pass** | `internal/app/core/keymap.go`: 20 bindable actions (`HotkeyActions`: next, prev, first, last, golive, schedule-next/prev, golive-next/prev, black, clear, logo, the seven section jumps and jump-slide), each with any number of `Chord`s (key + Ctrl/⌘, Alt, Shift; Ctrl and ⌘ are one chord, as before). `Hotkeys.SetKeymap` builds the lookup; the zero value still uses the defaults. Section jumps and the slide-number jump work from whatever key they are bound to, with the same 1 s digit window; navigation keys a focused list or grid keeps are still passed through. `TestHotkeys` (unchanged, defaults), `TestHotkeysCustom` (rebound Next, Verse on Q then 2 then 1 = Verse 21, slide jump on G and on Alt+J with digits, F-keys, Shift chords, a moved Ctrl+C, an unbound Backspace in `Unhandled`). |
| Parsing and the stored form | **pass** | `ParseChord` reads `ctrl+b`, `Cmd+B`, `⌘+L`, `→`, `Page Down`, `F12`; `String` is the stored form, `Label` the table text (`TestParseChord`, both round-trip). Persisted under `settings.hotkeys` as `{"next":["right","space","n"],…}`; loading is lenient (unknown actions and keys dropped, missing actions get their defaults: `TestKeymapJSON`). |
| Several keys per action, conflicts and reserved keys | **pass** | `Keymap.WithKey` / `WithoutKey` / `WithKeys` never change the receiver; a key another action has is a `*ConflictError` ("Ctrl+B is already assigned to Black") unless `reassign`; `Reserved` refuses digits (the numbers after a jump key), Esc, Tab and Ctrl/⌘ Q, W, H, M, N, O, S, ⇧S, V, X, A, Z (`TestReserved`, `TestKeymapEditing`). Ctrl+C stays EasyWorship's Clear: hotkeys are off in text fields, so ⌘C still copies there. |
| Options ▸ Hotkeys is editable: Add Key… (captures the next key press with its modifiers), Remove ▾ (one of the action's keys), Reset to Defaults, Import from EasyWorship | **pass** | `ui.HotkeysPage` (`internal/ui/settings_hotkeys.go`, a dumb view: `SetRows` / `SetCapturing` / `SetStatus`, `On…` callbacks). While a capture runs the dialog window's input hook (`dialogWindow.keyHook`) hands every key press to the app and marks it handled, so Space / Enter never press a button and Esc cancels the capture rather than closing Options. A key another action has opens the Reassign Key question (OK moves it, Cancel keeps it); a refusal shows in red under the table. Cancel in Options restores the hotkeys from when it opened. Screenshots `settings-hotkeys-capture.png`, `settings-hotkeys-refused.png`, `settings-hotkeys-conflict.png`. |
| Import from EasyWorship (`HotKeys.2.ini`: Windows virtual-key codes, `C-` = Ctrl) | **pass** | `core.ParseEWHotkeys` maps letters, digits, keypad digits, F1–F12 and the named keys, the `C-`/`S-`/`A-` prefixes; Go Live (not in EasyWorship) keeps its keys; unreadable and reserved keys are skipped and counted. EasyWorship 8's own file is exactly our defaults (`TestParseEWHotkeys`). The button is enabled when `/Users/Shared/Softouch/Easyworship.v8/Settings/HotKeys.2.ini` exists; `import-hotkeys {path}`. |
| Default theme per item type in Options (Songs, Scriptures: what `store.DefaultThemeKinds` covers) | **pass** | `ui.DefaultThemesPage` (`internal/ui/settings_themes.go`, section "Themes"), one dropdown per kind, projected from `a.th.defaults` and calling the same `setDefaultTheme` as the Themes tab's Set as Default (applied at once, not undone by Cancel, like Set as Default). On a change, `followDefaultTheme` re-themes the library deck behind the resource preview and Preview when it shows that song (without a theme of its own) or passage with the old default (`core.FollowsDefault`, `TestFollowsDefault`); schedule items keep their snapshot and Live is left alone. Smoke `# ---- default themes`: `Preview follows the new default`, `Live keeps its theme`, `a new schedule item uses the new default`, `the earlier item keeps its snapshot`, the scripture default, `an item type without a default is refused`; screenshots `settings-themes.png`, `default-theme-main.png`. |
| Zoom In / Zoom Out transitions | **pass** | `models.TransitionZoomIn` / `ZoomOut` (family `zoom`, `zoom` alone = zoom-in, `TestTransitionKinds`, `TestParseTransitionKind`). `layout.Transition` adds `InScale` / `OutScale`: zoom in grows the new slide from the centre over the old one; zoom out shrinks the old slide into the centre above the new one, with the same duration and smoothstep easing (`TestTransitionEndpoints`, `TestTransitionMidway`, `TestTransitionMonotonic`). `render` scales each layer's `inner` content node about its centre (pivot set in `placeAt`; a 0.001 floor keeps the transform invertible), so text, images and video layers zoom together on the output and the live monitor. Listed in Options ▸ Transition (from `models.TransitionKinds`); FreeBeams has no per-item transitions. |
| Robust mid-transition screenshots | **pass** | `wait-transition {percent}` answers once the output's transition (the live monitor's when the output is closed) has reached that progress, and fails when none runs; `/state.transitionProgress`. The zoom checks use it instead of `sleep 1`: `mid-zoom-in: the centre shows the new slide`, `… the corners still show the old slide`, `mid-zoom-in on the live monitor …`, `mid-zoom-out: the centre still shows the old slide`, `the zoom-in ends on the new slide`. Screenshots `zoomin-mid.png`, `zoomin-mid-main.png`, `zoomout-mid.png`. |
| Control and `/state` | **pass** | `set-hotkey {action, keys, reassign}` (the action's whole list; refusals and conflicts are errors), `reset-hotkeys`, `hotkey-capture {action}` / `{on:false}` (Add Key…), `import-hotkeys {path}`, `hotkeys`; `native-key {window:"settings"}` types into Options; `set-default-theme {kind, themeName}` (also `name`; `songs` / `scriptures` accepted); `transition {"kind":"zoom-in"}`. `/state.settings.hotkeys`, `/state.options` (`hotkeyCapture`, `hotkeyStatus`, `hotkeyRefused`), `/state.transitionProgress`. Smoke `# ---- hotkeys editing`: Next rebound to F through a capture and pressed natively in the main window (live index advances, Space no longer does), Cmd+Q held in a capture is refused, Esc cancels a capture, a conflict asks and both answers work, the keymap is persisted, a custom Verse key takes its number, Cancel restores, `reset-hotkeys`. |

Smoke on 2026-10-22: `test/smoke.sh` **1196/1196 PASS** on the first run, log hygiene and the keepalive walk clean
(the zoom checks add 21 lines, `# ---- hotkeys editing` and `# ---- default themes` 69 checks); `test/splitters.sh`
PASS. The hotkey editor, refusal, conflict question, Themes page and mid-zoom screenshots were checked by eye.

### Shared files touched (Options: hotkeys, default themes, zoom)

- New: `internal/app/core/keymap.go` (+ test), `internal/app/hotkeys.go`, `internal/ui/settings_hotkeys.go`,
  `internal/ui/settings_themes.go`.
- `internal/app/core/hotkeys.go` (the key tables became `Keymap` lookups), `core/settings.go` (`Settings.Hotkeys`
  in its own block; `Bindings` moved to keymap.go), `core/state.go` (`Options` and `TransitionProgress` blocks),
  `core/themes.go` (`FollowsDefault` appended).
- `internal/models/transition.go` (zoom kinds in their own const blocks), `internal/render/layout/transition.go`
  (`InScale` / `OutScale`), `internal/render/transitions.go`, `slideview.go` (one `layer.scale` field).
- `internal/app/app.go` (the `opt` field block, `serveProgressWaiters` after `serveWaiters`), `settings.go` (load,
  restore, `wireOptions`, `renderOptions`, `handleOptionsCommand` as the default), `control.go` (an `actionArgs`
  block, two `state()` lines), `hooks.go` (`native-key` window `settings`), `themes.go` (`followDefaultTheme` in
  `setDefaultTheme`, `set-default-theme` kinds and `themeName`).
- `internal/ui/settings.go` (the Themes section, `Hotkeys` / `DefaultThemes` fields, the key hook), `dialogs.go`
  (`dialogWindow.keyHook`).
- `test/smoke.sh`: zoom checks in the transitions block; `# ---- hotkeys editing` and `# ---- default themes` after it.

## Drag and drop

Recorded 2026-10-22 on the same machine (no TV attached for this run). As in EasyWorship, resources are dragged
into the schedule and schedule rows are dragged to reorder.

| Criterion | Status | Verification |
|---|---|---|
| A song row, the selected verses, a media tile and a presentation tile drag onto the schedule and insert at the drop position | **pass** | Sources hand Godot a `uicore.DragPayload` (`TestDragPayloadRoundTrip`): `slideTile.GetDragData` (grids with a `DragSource`), `SetDragForwarding` on the song and verse Trees (`internal/ui/dragdrop.go`). The list reports `OnDropInsert(source, index, gap)`; the app runs the Add to Schedule path with an insert index (`App.insertingAt` → `addToSchedule` uses `Schedule.Insert`; songs through `selectSongAt`). Smoke: `drag a song into the empty schedule`, `drag the verses to the top`, `drag a song into the middle`, `drag a media tile to the end`, `release the held drag` (a verse drop at gap 2). Presentations share the media path (no deck in the smoke library at that point). |
| An insertion line between rows marks the drop position; a labelled preview follows the pointer | **pass** | `uicore.DropGap` (`TestDropGap`; upper half of a row inserts before it); a `FwDragPreview` chip with the title. Smoke: `no insertion line before the drag` / `the insertion line shows mid-drag` (screenshots `drag-before.png`, `drag-mid.png`). |
| Reordering by drag inside the schedule, through the move path | **pass** | `uicore.DropMove` (`TestDropMove`) → `OnMove` → `App.moveScheduleItem`. Smoke: `reorder: drag the last row to the top`, `drag the top row into the middle`, `reorder onto its own place changes nothing`. |
| A theme dropped onto a schedule row applies to that item (stretch goal) | **pass** | `OnDropOnto` → `App.applyThemeToItem` (the Theme submenu's path); the target row is outlined and the gaps do not accept a theme. Smoke: `a theme dropped onto a row applies to that item`, `the item has the theme`, `a theme dropped between rows does nothing`. |
| Dropping outside the list does nothing | **pass** | Only the list and its rows accept the payload. Smoke: `a song dropped outside the list does nothing`, `a row dropped outside the list stays`. |
| The list auto-scrolls near its top and bottom edges | **pass** | `uicore.AutoScroll` (`TestAutoScroll`), run by a ticker while the drop feedback shows; the feedback clears when the pointer leaves or the drag ends. Smoke: `hold a drag at the bottom edge` / `the drop went after the last row (auto-scrolled)` with 15 rows (screenshot `drag-autoscroll.png`). |
| Clicks, double-clicks, context menus and keyboard focus unchanged | **pass** | Drags start only after 10 px of held motion; the existing click / double-click / context-menu / focus checks all pass, plus `a click still selects a schedule row` after the drags. A press on a row of a multi-verse selection drags the whole selection (the pending narrow-on-click is cancelled). |
| Control hook | **pass** | `drag-to-schedule {source: song\|scripture\|media\|presentation\|theme\|schedule, index\|title, at\|row\|target: outside\|top-edge\|bottom-edge, mode: hold\|release, steps}`: press, one motion per frame (with `relative`), release, through Godot's input pipeline. It warps the real pointer along: Godot 4 picks a drag's drop target from the OS cursor (CLAUDE.md). |

Smoke on 2026-10-22: `test/smoke.sh` **PASS** (1145 checks; the drag and drop block adds 36), log hygiene clean;
`test/splitters.sh` **PASS**.

Limitations: the hook moves the real mouse pointer while it drags (an operator touching the mouse during a smoke
run can disturb it); one item per drag (no multi-select drag of songs or tiles); dropping files from the Finder
into the schedule is not handled; a drag that starts on an unselected row selects it (and previews it) first, as
a click does.

### Shared files touched (drag and drop)

- New: `internal/ui/dragdrop.go`, `internal/ui/uicore/dragdrop.go` (+ test), `internal/app/dragdrop.go`.
- `internal/ui/schedule.go` (a field block, `initDrop`), `slidegrid.go` (`DragSource` block), `songs.go` /
  `scripture.go` (`initDrag` calls), `media.go` / `presentations.go` / `themes.go` (`DragSource`), `register.go`.
- `internal/app/app.go` (a `drop` field block), `events.go` (`addToSchedule` inserts while dropping;
  `selectSongAt`), `view.go` (`wireDragDrop`), `hooks.go` (`drag-to-schedule`), `control.go` (an `actionArgs` block: `at`).
- `test/smoke.sh` (the `# ---- drag and drop` block, before Open File Location), `CLAUDE.md` (gotchas appended).

## Bible Manager: add, remove and package Bibles

Recorded 2026-10-22 on the same machine. Formats, policy and behaviour: [BIBLES.md](BIBLES.md) ("Bible packages",
"The Bible Manager").

| Criterion | Status | Verification |
|---|---|---|
| A one-file Bible package with metadata, small, with one reader for built-ins and user packages | **pass** | `internal/bibles/biblepkg`: `.fbbible` = gzip of the single-file Bible JSON with a first-line header (`format`, `version`, `abbrev`, `name`, `language`, `copyright`, `verses`). `TestRoundTrip` (accents, quotes, unsorted input, reproducible bytes), `TestReadErrors` (not gzip, plain JSON, other format, newer version, no abbrev, verse-count mismatch, broken JSON, truncated, corrupt byte), `TestEveryTruncationFails` (every prefix), `TestFiles`, `TestFileName`. The bundled data is packages too (`data/*.fbbible`; KJV and ASV re-packed losslessly). |
| The RVA is bundled, public domain only, with correct accents | **pass** | `bundled.Translations` adds RVA (es, 31,102 verses, `Origin: OriginEWB`), generated from EasyWorship's `RVA.ewb` by `gen/main.go -ewb … -only RVA`; identical verse for verse to the RVA in the user's library; two source patches (`> ` before Proverbs 10:1, the closing `FIN.`). `TestBundledTranslations` (only KJV/ASV/RVA, all public domain, headers, sizes, `JEHOVÁ es mi pastor; nada me faltará.`, `amó Dios … unigénito`, `Châm`), `bundled.Version` 2. Size: 2,519,018 → 3,811,158 bytes (+1,292,140). |
| The Bibles folder: per user, discoverable, created on first use, any supported file appears | **pass** | `appdirs.Bibles` (`~/Documents/FreeBeams/Bibles`, Windows/Linux equivalents, `FREEBEAMS_BIBLES_DIR`; `TestBiblesPath`, `TestBiblesCreatesTheFolder`). `biblefolder.Scan` lists packages and `.ewb` from their headers (`easyworship.ReadBibleInfo`), JSON and translation folders by decoding; duplicates and unreadable files become problems (`TestScan`, `TestRead`, `TestEWB`). |
| The manager dialog: merged list, Add with progress off the main thread, licence question, Remove with confirm, Save as Package, Add from File…, Open Bibles Folder, Refresh | **pass** | `ui.BibleManager` (dumb view, 780 × 500, Name / Language / Source / Status / Size), state in `App.bm`, one projection `renderBibleManager`. `core.MergeBibleEntries` (`TestMergeBibleEntries`), texts (`TestBibleManagerTexts`). Add: `store.CreateBibleWithVersesProgress` in a goroutine, progress through the results queue. Remove: `store.DeleteBible` (row, verses and FTS rows in one transaction; `TestDeleteBible`). A removed built-in is recorded (`bundled.SetRemoved`, `TestInstallSkipsRemoved`). Save: `store.AllVerses` + `biblepkg.WriteFile` (atomic). |
| Removing the selected / default Bible | **pass** | The tab falls back to `core.DefaultBible` (first installed built-in in bundle order, else the first; `TestDefaultBibleFallsBackInBundleOrder`) and re-runs the reference there. Schedule items keep their slides. |
| Remove Bible… on the translations list (context menu, Delete / Backspace) | **pass** | `ScripturePanel.initBibleMenu`; both ask first. |
| Control actions, `/state`, smoke | **pass** | `bibles-available`, `bible-manager {action}`, `bible-select`, `bible-add {abbrev\|path, confirm}`, `bible-remove {abbrev, confirm}`, `bible-save-package {abbrev}`, `bible-open-folder`; `/state.bibleManager`; screenshot window `bible-manager`; focus targets `bible-list`, `bible-manager-list`. The `# ---- bible manager` smoke block: SMOKE_RESULT |

### Shared files touched (Bible Manager)

- New: `internal/bibles/biblepkg`, `internal/bibles/biblefolder`, `internal/app/biblemanager.go`,
  `internal/app/core/biblemanager.go` (+ tests), `internal/ui/biblemanager.go`, `internal/store/bibles_delete_test.go`,
  `internal/bibles/bundled/data/{kjv,asv,rva}.fbbible` (replacing `*.json.gz`).
- `internal/bibles/bundled` (packages, RVA, removed list; `gen/main.go` rewritten), `internal/importers/bibletext`
  (`EncodeWithHeader`), `internal/importers/easyworship/ewb.go` (`ReadBibleInfo`), `internal/store/bibles.go`,
  `internal/appdirs` (`Bibles`).
- `internal/app/events.go` (`loadBibles` → `refreshBibles` / `applyBibles`), `biblefile.go` (`biblefolder.Read`,
  `.fbbible` filter), `bundled.go` (texts), `app.go` / `view.go` / `control.go` (field, view-cache and `actionArgs`
  blocks, dispatch, screenshot), `hooks.go` (focus targets), `core/deck.go` (`DefaultBible`), `core/state.go`.
- `internal/ui/scripture.go` (+ opens the manager, gear ▸ Manage Bibles…, context menu, Delete key),
  `mainwindow.go`, `theme.go` (ProgressBar).
- `test/smoke.sh` (`FREEBEAMS_BIBLES_DIR`, the bundled checks now expect RVA, the `# ---- bible manager` block),
  `docs/BIBLES.md`.

## Remaining for Phase 2

- **Off macOS**: run the ffmpeg player and the bundled ffmpeg builds on real Windows and Linux machines (the
  release workflow is the first run); the Theora fallback (`render/video.go`, no ffmpeg) is still unverified.
- **Media polish**: poster frames through `AVAssetImageGenerator` on macOS (videos still need ffmpeg for a
  thumbnail), a per-item Repeat / Aspect property (`models.Background.Loop` is the hook), keeping a video's
  position across Black (today a Black longer than the fade re-opens the file from 0), a transport in the resource
  preview (UI-SPEC §3 mentions mini controls there).
- **Themes polish**: a video background in the editor ("use as background" from the Media tab is the natural
  hook: `Media.Background()`), the font picker's search bar (`OptionButton.search_bar_enabled`, Godot 4.7) once the
  list grows, importing EasyWorship's per-song themes (needs a library whose songs have them, see "Per-song
  themes"), and EasyWorship's "Apply changes to items in schedule" choice (today a saved theme always
  restyles the schedule items that use it).
- **Settings**: the Options dialog has Output, Transition (with Zoom), Logo, Library, editable Hotkeys and default
  Themes per item type. Still open: EasyWorship's separate Black / Clear / Logo transitions (one transition serves
  all), default themes for media and presentations (they have no theme setting yet), per-item transitions, and
  telling ⌘ and Ctrl apart in a binding (they are one chord). The native menus cannot be driven by automation (the
  `menu {item}` action covers the handlers).
- **Packaging** (PLAN §4 M6): the release build, scripts, CI workflows and docs are in place, see
  [RELEASE.md](RELEASE.md). Still open: signing and notarization (needs credentials), and the Windows and Linux
  builds, which are unverified until the release workflow has run.
- Media library polish: audio (EasyWorship's `Audio` folder), dropping Finder files into the schedule, per-item titles and
  tags, removing items whose files have gone missing.
- Presentations polish: verify `pdftoppm` and LibreOffice on a machine that has them (or bundle poppler in the
  release for Windows / Linux); per-page thumbnails for the Preview / Live grids; re-render a schedule
  presentation whose cache is gone and whose document is no longer in the library; speaker notes; a Windows PDF
  backend (PDFium or poppler) so PDFs render without poppler on PATH.
