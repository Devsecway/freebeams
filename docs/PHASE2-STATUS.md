# Phase 2 status

Status of Phase 2 (PLAN.md §4, **M4: Media & Themes**), recorded 2026-10-19 on macOS (Apple M3 Max, built-in
Retina display plus a 1080p TV over HDMI, EasyWorship 8 data at `/Users/Shared/Softouch`, Homebrew ffmpeg 8.1.1
**without** the `libtheora` encoder). This document covers the Media milestone and the Themes milestone; the rest
of Phase 2 is listed at the end.

Legend as in [PHASE1-STATUS.md](PHASE1-STATUS.md): **pass**, **partial**, **fail**, **untested**.

How to reproduce:
```bash
gofmt -l . ; go build ./... && go vet ./... && go test -count=1 ./...
test/smoke.sh          # 388 checks (Phase 1: 191; media, presentations, themes, transitions and settings blocks); screenshots and app.log in test/out/
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

## Presentations (PDF / PowerPoint / Keynote → image slides)

Recorded 2026-10-19 on the same machine. Renderers found here: **Core Graphics** (in-process, macOS) for PDF;
**no `pdftoppm`** (poppler) and **no LibreOffice** (`soffice`), so PowerPoint / Keynote decks fall back to their
embedded images. Nothing was installed. EasyWorship's `Presentations.db` holds one built-in, non-file-based
"Introduction" row and `Resources/Presentations` is empty, so there was nothing to register from it.

| Criterion | Status | Verification |
|---|---|---|
| `internal/presentations` (pure Go): `Renderer` interface, page PNGs (1920×1080, letterboxed on black) + a 640px JPEG thumbnail in `~/Library/Caches/FreeWorship/presentations/<key>/` (`FREEWORSHIP_CACHE/presentations` overrides); the key hashes path, size and mtime so an edited document renders again; `.part` folder renamed into place | **pass** | `TestKeyAndPaths`, `TestDefaultCacheDir`, `TestLetterbox` (16:9 fills, 4:3 / portrait letterboxed, small images scaled at most 2×), `TestScan`, `TestNumberedFiles`, `TestCanRender` (backend choice per type and tool set). |
| PDF through Core Graphics (cgo, `pdf_darwin.go`; `CGPDFDocument` → `CGBitmapContext` → Go `image/png`); `pdftoppm` on PATH elsewhere | **pass** (CG) / **untested** (pdftoppm) | `TestRenderPDF`: a hand-written two-page PDF renders to two pages with the exact fill colours, a 640×360 thumbnail, no `.part` left behind, the second render reuses the folder, cancellation leaves nothing, garbage is rejected. `TestRenderPDFWithoutRenderer` → `ErrUnavailable`. pdftoppm is not on this machine: the code path (`-scale-to 2160`, numeric output ordering, letterbox) is covered only by the pure helpers. |
| PPTX / PPT / Keynote / ODP through LibreOffice headless → PDF → pages, per-conversion user profile | **untested** | `soffice` is not installed; `TestRealLibreOffice` skips. `sofficeConverter` / `Service.renderOffice` are exercised only by `TestCanRender`. |
| Fallback without LibreOffice: a `.pptx` is read as a zip (`presentation.xml` slide order → slide rels → the slide's largest picture), slides without pictures skipped, `docProps/thumbnail.jpeg` when no slide has one; a `.key` shows its `preview.jpeg`; the deck is marked **Partial** ("embedded images") | **pass** | `TestRenderPPTXEmbedded` (3 slides → 2 pages: red PNG scaled 2×, green 4:3 JPEG letterboxed; thumbnail-only deck; deck without any image fails; `.ppt` → `ErrUnavailable`; garbage fails), `TestRenderKeynoteEmbedded`. Smoke: `PowerPoint pages rendered`, `select the deck (embedded images)` (`currentRenderer == "embedded"`, `currentPartial`), `the output shows the deck's slide` (exact pixel check). |
| `presentations` table (path, title, pages, cache_key, renderer, partial, added) with CRUD and search | **pass** | Migration 3; `internal/store/presentations.go`. `TestPresentationCRUD`, `TestSavePresentationsAtomic`, `TestSearchPresentations`. |
| Presentations tab (`FwPresentationPanel`, dumb view): search, grid of first-page thumbnails with badges (`12 slides`, `Rendering 3/12`, `5 slides · embedded`, `Needs LibreOffice`, `Needs poppler`), Add Presentation… (native dialog, pdf/pptx/ppt/key/odp), context menu / gear menu (Add to Schedule, Go Live, Remove from Library), footer status, view sizer | **pass** | `internal/ui/presentations.go`; `TestPresentationBadge`, `TestPresentationsFooter`. Smoke: `Presentations tab`, `presentations tab has content` (`test/out/presentations-tab.png`), `search presentations`, `all presentations`. |
| Selecting shows page 1 in the resource preview; the pages are a deck of image slides that goes live and steps with the normal hotkeys | **pass** | `core.PresentationDeck` (one `SlidePresentation` per page, `Slide N` labels; a placeholder slide while rendering). Smoke: `select the PDF` (`resource.caption == "Slide 1 · 1 / 2"`), `PDF live`, `the output shows page 1` (#2a5fbf), `Next steps to page 2`, `the output shows page 2` (#bf5f2a) / `page 1 is gone from the output` / `the live monitor shows page 2`, `Next at the last page stays`, `Prev steps back to page 1`, `Enter in the presentation grid sends the deck live`. |
| Rendering runs off the main thread in a tracked job; progress in the badges and `/state.presentations.working`; failures are logged once and not retried this session | **pass** | `App.startPresentationWork` / `runPresentationWork` (one pass over the library, one document at a time, progress posts throttled to 250 ms). Smoke: `PDF pages rendered` (`pending == 0`, `failed == 0`). |
| Schedule items snapshot the page slides with the document path and cache key; a stale or missing snapshot is rebuilt from the library's current render when the row is selected | **pass** | `core.PresentationItem`, `core.ResolvePresentationItem` (`TestPresentationDeckAndItem`, `TestResolvePresentationItem`, `TestPresentationScheduleItemRoundTrip`: `.fws` keeps `presentationId`, `source`, `cacheKey`). Smoke: `add presentation to schedule`, `select the presentation schedule item`, `presentation schedule item live` (slide 2), `remove asks for confirmation (presentation)`, `cancel the removal (presentation)`, `remove the PDF`, `the schedule still has the presentation`. An unrendered deck is refused ("wait for the slides to finish rendering"). |
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
| One icon language (32 px toolbar / 16 px mini grids, 2 / 1.5 px rounded strokes on the pixel grid), own artwork, crisp at 1× and 2× | **pass** | `internal/ui/uicore/icons.go` (artwork, pure Go) + `internal/ui/icons.go` (DPITextures). `TestIconSVGWellFormed`, `TestIconGrid`, `TestIconMulti`, `TestIconColors`. Before/after at native Retina pixels: `docs/screens/icons-{before,after}-{toolbar,panels}-{dark,light}.png`; design notes and the audit in [ICONS.md](ICONS.md). |
| State colors from palette tokens: Go Live accent, Live red only while on, Logo/Black/Clear brightened when active, hover, disabled; light palette | **pass** | Single-color icons are white masks tinted at draw time (`drawGlyph`, `glyphTint`); `live_on` / `golive` carry their own colors. Theme colors `glyph`, `glyph_hot`, `glyph_on`, `accent`, `toolbutton_text_sel` in `theme.go`; no hex in widgets. Smoke screenshots `black-main.png` (Black active), `light-main.png`. |
| Smoke | **pass** | 388/388 on 2026-10-19 after the redesign; no pixel check changed (none asserts an icon color). A first run failed once at `the output shows the image` (the media monitors still showed the previous image although `/state` had `live.title == smoke-green` and was idle; intermittent, not icon-related, passed on the rerun). |

## Remaining for Phase 2

- **Video playback verification** on a machine with a Theora-capable ffmpeg (see above), then: a progress
  indicator in the resource preview's mini toolbar (the state has `media.working` and per-item `Converting NN%`
  badges already), and hot-swapping a live poster for the finished transcode.
- **Video option B** (PLAN §3: ffmpeg pipe → YUV textures) for machines without libtheora, or bundling an ffmpeg
  with libtheora in the release.
- **Themes polish**: a video background in the editor ("use as background" from the Media tab is the natural
  hook: `Media.Background()`), the font picker's search bar (`OptionButton.search_bar_enabled`, Godot 4.7) once the
  list grows, honouring `Song.ThemeID` (per-song theme, imported from EasyWorship) in `newSongDeck`, a theme picker
  for the song editor, and EasyWorship's "Apply changes to items in schedule" choice (today a saved theme always
  restyles the schedule items that use it).
- **Settings**: default themes per item type (the Options dialog has Output, Transition, Logo, Library and a
  read-only Hotkeys table; editable hotkeys and a Zoom transition are not done). The native menus cannot be
  driven by automation (the `menu {item}` action covers the handlers).
- **Packaging** (PLAN §4 M6): the release build, scripts, CI workflows and docs are in place, see
  [RELEASE.md](RELEASE.md). Still open: signing and notarization (needs credentials), and the Windows and Linux
  builds, which are unverified until the release workflow has run.
- Media library polish: audio (EasyWorship's `Audio` folder), drag-and-drop into the schedule, per-item titles and
  tags, removing items whose files have gone missing.
- Presentations polish: verify `pdftoppm` and LibreOffice on a machine that has them (or bundle poppler in the
  release for Windows / Linux); per-page thumbnails for the Preview / Live grids; re-render a schedule
  presentation whose cache is gone and whose document is no longer in the library; speaker notes; a Windows PDF
  backend (PDFium or poppler) so PDFs render without poppler on PATH.
