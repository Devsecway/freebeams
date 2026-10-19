# Phase 2 status

Status of Phase 2 (PLAN.md §4, **M4: Media & Themes**), recorded 2026-10-19 on macOS (Apple M3 Max, built-in
Retina display plus a 1080p TV over HDMI, EasyWorship 8 data at `/Users/Shared/Softouch`, Homebrew ffmpeg 8.1.1
**without** the `libtheora` encoder). This document covers the Media milestone; the rest of Phase 2 is listed at
the end.

Legend as in [PHASE1-STATUS.md](PHASE1-STATUS.md): **pass**, **partial**, **fail**, **untested**.

How to reproduce:
```bash
gofmt -l . ; go build ./... && go vet ./... && go test -count=1 ./...
test/smoke.sh          # 269 checks (228 before the Presentations block); screenshots and app.log in test/out/
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
| Smoke checks | **pass** | `test/smoke.sh` runs 228 checks (Phase 1 alone: 191); the media section is between the schedule and song-editor sections. The launch section is unchanged. Both full runs on 2026-10-19 passed first time, with the TV output open fullscreen. |

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

## Remaining for Phase 2

- **Video playback verification** on a machine with a Theora-capable ffmpeg (see above), then: a progress
  indicator in the resource preview's mini toolbar (the state has `media.working` and per-item `Converting NN%`
  badges already), and hot-swapping a live poster for the finished transcode.
- **Video option B** (PLAN §3: ffmpeg pipe → YUV textures) for machines without libtheora, or bundling an ffmpeg
  with libtheora in the release.
- **Theme editor** (background color / gradient / image / video, font, outline, shadow, alignment, margins, live
  preview; default theme per item type) and the **Themes** tab. The media library's `Media.Background()` is the
  hook for "use as background".
- **Settings UI** (output display, default themes, transition and duration, copyrighted-Bible opt-in).
- **More transitions** (push, wipe, iris, zoom) and the Logo image setting.
- **Packaging** (PLAN §4 M6): the release build, scripts, CI workflows and docs are in place, see
  [RELEASE.md](RELEASE.md). Still open: signing and notarization (needs credentials), and the Windows and Linux
  builds, which are unverified until the release workflow has run.
- Media library polish: audio (EasyWorship's `Audio` folder), drag-and-drop into the schedule, per-item titles and
  tags, removing items whose files have gone missing.
- Presentations polish: verify `pdftoppm` and LibreOffice on a machine that has them (or bundle poppler in the
  release for Windows / Linux); per-page thumbnails for the Preview / Live grids; re-render a schedule
  presentation whose cache is gone and whose document is no longer in the library; speaker notes; a Windows PDF
  backend (PDFium or poppler) so PDFs render without poppler on PATH.
