# Phase 2 status

Status of Phase 2 (PLAN.md §4, **M4: Media & Themes**), recorded 2026-10-19 on macOS (Apple M3 Max, built-in
Retina display plus a 1080p TV over HDMI, EasyWorship 8 data at `/Users/Shared/Softouch`, Homebrew ffmpeg 8.1.1
**without** the `libtheora` encoder). This document covers the Media milestone and the Themes milestone; the rest
of Phase 2 is listed at the end.

Legend as in [PHASE1-STATUS.md](PHASE1-STATUS.md): **pass**, **partial**, **fail**, **untested**.

How to reproduce:
```bash
gofmt -l . ; go build ./... && go vet ./... && go test -count=1 ./...
test/smoke.sh          # 273 checks; screenshots and app.log in test/out/
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
| Control actions and `/state` | **pass** | Actions (`App.handleThemeCommand`): `themes`, `select-theme {index\|name}`, `get-theme {index\|name}` (the full theme JSON, for `save-theme` round trips), `save-theme {theme}`, `duplicate-theme {index\|name}`, `delete-theme {index\|name, confirm}`, `set-default-theme {index\|name, kind: "song"\|"scripture"}`, `apply-theme {index\|name, item}` (default: the selected schedule row), `editor {mode: "theme"\|"new-theme"\|"close"}`, `focus {target: "theme-grid"}`, `screenshot?window=theme-editor`. `/state.themes`: `count, names, selected, current, songDefault, scriptureDefault, countText, editor, editing`; `/state.dialog.themeEditor`. |
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
- **Settings UI** (output display, default themes, transition and duration, copyrighted-Bible opt-in).
- **More transitions** (push, wipe, iris, zoom) and the Logo image setting.
- **Packaging**: signed DMG, Windows, Linux (PLAN §4 M6).
- Media library polish: audio (EasyWorship's `Audio` folder), drag-and-drop into the schedule, per-item titles and
  tags, removing items whose files have gone missing.
