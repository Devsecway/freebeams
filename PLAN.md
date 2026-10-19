# FreeBeams — Implementation Plan

A native, cross-platform worship presentation app (EasyWorship-class) written in Go,
using the same stack as Bufflehead: **Go + graphics.gd (Godot 4.6) + pure-Go SQLite**.

---

## 1. What we found

### Bufflehead (the reference stack)
`~/Documents/Work/Apex/bufflehead`

| Concern | Bufflehead's choice | Reuse in FreeBeams |
|---|---|---|
| UI / rendering | `graphics.gd` (Godot 4.6, all UI in Go, no `.gd` scripts) | Yes, as the core. Godot gives us GPU rendering, shaders, multi-window, video, and audio |
| Build tool | `gd` CLI (`go install graphics.gd/cmd/gd@release`), `gd run`, `GOOS=macos gd build` | Yes (already installed at `~/go/bin/gd`) |
| Go | 1.26 (`go1.26.1 darwin/arm64` installed) | Yes |
| Storage | `modernc.org/sqlite` (pure Go, no cgo) | Yes: library, schedules, Bibles, settings |
| Config/files | `gopkg.in/yaml.v3`, JSON in user config dir (`models/config_dir.go`) | Yes |
| IDs | `github.com/google/uuid` (indirect) | Yes |
| WebSocket | `github.com/gorilla/websocket` (indirect) | Yes: remote control / stage display |
| Control API | `internal/control`, an HTTP server with a bearer key used to drive integration tests | Yes, copied as is. It also becomes the base for the remote control |
| Tests | `go test ./...` plus pytest integration through the control API | Same approach |
| Packaging | `bin/release-dmg`, `bin/sign-notarize`, GH Actions for macOS/Win/Linux, Inno Setup | Copy and rename |
| UI convention | **State machine: events mutate state, one idempotent `render()`** | Adopt as a hard rule. It matters even more here (preview/live/schedule sync) |

Not needed: AWS SDK, BigQuery, DuckDB, pgx, mysql, keyring, ssh.

**Bufflehead `project.godot` settings to change:**
- `audio/driver/driver="Dummy"` → remove (we need real audio for video and song playback).
- `run/low_processor_mode=true` → keep it for the operator window, but turn it **off** with
  `OS.SetLowProcessorUsageMode(false)` while anything is animating on the output
  (video, transitions). Otherwise output drops to about 30 fps.
- `viewport/transparent_background=true` → off for the output window (it must be opaque black).
- Keep `window/subwindows/embed_subwindows=false`. That is what makes real OS windows,
  and the projector output needs one.

graphics.gd classes we rely on, all confirmed present in the module cache:
`Window`, `SubViewport`, `Shader`/`ShaderMaterial`, `Tween`, `RichTextLabel`, `Font`/`FontFile`/`SystemFont`,
`ImageTexture`, `VideoStreamPlayer`/`VideoStreamTheora`, `AudioStreamPlayer`/`AudioStreamMP3`/`OggVorbis`,
`DisplayServer` (screen enumeration).

### EasyWorship 8 (what we're cloning)
Installed at `/Applications/EasyWorship.app` (v8.0.49). Data is in `/Users/Shared/Softouch/`.

- **Songs**: `Easyworship/Default/v6.1/Databases/Data/Songs.db` (SQLite, 223 songs:
  title/author/copyright/administrator/ccli `reference_number`/tags). Lyrics are in `SongWords.db → word.words` as **RTF**
  with slide breaks.
- **Bibles**: `Easyworship.v8/Plugins/Bibles/*.ewb`, which are SQLite (`header`, `books`, `streams`, `words`).
  `streams` holds one **zlib-compressed** blob per book (`78DA…`). `words` is a search index.
  KJV, RVA, HCSB are installed. HCSB is copyrighted, so we import public-domain texts only and
  let users import their own.
- **Media library**: `Media.db`, `Resources/{Images,Videos,Audio}`. The videos are H.264 `.mp4` motion backgrounds.
- **Themes / layouts**: `Themes.db`, `PresentationLayouts.db` (elements, text/image/video/gradient/shape resources).
- **Transitions**: Blend, Push, Wipe, Cover, Reveal, Iris, Drop, Cube, PageFlip, HexTile, shipped as Metal/HLSL shaders.
  We write our own Godot shaders for the same effects. We do not copy theirs.
- **Hotkeys** (`Settings/HotKeys.2.ini`): Black = Ctrl+B, Clear = Ctrl+C, Logo = Ctrl+L, Next slide = →/Space/N,
  Prev = ←/Backspace/P, First/Last = Home/End, Schedule next/prev = ↓/↑, jump to Verse/Chorus/Bridge/
  Pre-chorus/Tag/Intro/Ending = V/C/B/R/T/I/E, Go live next/prev = PgDn/PgUp.
- Other features: remote control clients, MIDI controls, NDI output, PowerPoint import, live feeds (camera), FFmpeg bundled.

---

### UI design
The operator UI copies EasyWorship 8's layout, geometry and palette exactly. These were extracted from its binary.
See **[docs/UI-SPEC.md](docs/UI-SPEC.md)** and the visual mockup `docs/mockup/main-window.html`.

---

## 2. Product scope

### MVP (v0.1, usable on a Sunday)
1. **Operator window**: Schedule (left) | Preview (center) | Live (right) | Resource library (bottom tabs:
   Songs, Scriptures, Media, Themes). This is the same layout as EasyWorship.
2. **Output window**: borderless and full screen on a selected display, showing the live slide and nothing else.
3. **Songs**: create/edit lyrics with section tags (Verse 1, Chorus…), auto split into slides, search.
4. **Scripture**: type a reference (`jn 3:16-18`), get a preview, send it live. Verse-per-slide or fit-to-slide.
5. **Media**: images and looping motion backgrounds behind text.
6. **Themes**: background, font, size, color, outline/shadow, alignment, margins; auto-shrink text to fit.
7. **Transitions**: at minimum, crossfade (Blend). Also Black / Clear / Logo screens.
8. **Schedules**: add songs/verses/media, reorder, save/open `.fbs` files.
9. **Hotkeys** matching EasyWorship.
10. **Importer** for the existing EasyWorship song library (the 223 songs on this laptop).

### v0.2+
Stage display (confidence monitor with next slide and clock), remote control (phone web UI over WebSocket),
more transitions, video items with audio and transport controls, countdown timers, alerts/nursery ticker,
presentations (PDF/PPTX → images), CCLI reporting (SongHistory), multiple outputs, NDI, MIDI, live camera feeds.

---

## 3. Architecture

```
freebeams/
├── main.go                     # startup, like bufflehead/main.go
├── graphics/                   # Godot project: project.godot, export_presets.cfg, icon, shaders/
│   └── shaders/                # blend.gdshader, push.gdshader, wipe.gdshader, ...
├── internal/
│   ├── models/                 # pure Go, no Godot imports, fully unit-tested
│   │   ├── song.go             # Song, Section{Kind, Label, Lines}, slide splitting
│   │   ├── scripture.go        # Reference parsing ("1 jn 1:9", "Ps 23"), verse ranges
│   │   ├── slide.go            # Slide = {Layers: background, text, overlays}, the unit of display
│   │   ├── theme.go            # Theme{Bg, Font, Size, Color, Outline, Shadow, Align, Margins}
│   │   ├── schedule.go         # Schedule{Items}, ScheduleItem{Kind, RefID, ThemeOverride}
│   │   └── presenter.go        # ★ PresenterState: preview item/slide, live item/slide, black/clear/logo
│   ├── store/                  # SQLite (modernc.org/sqlite): songs, bibles, media, themes, history
│   │   ├── migrations/
│   │   └── fts.go              # FTS5 search over titles, lyrics, verses
│   ├── importers/
│   │   ├── easyworship/        # Songs.db + SongWords.db (RTF → sections), .ewb (zlib streams)
│   │   ├── rtf/                # minimal RTF → plain text with paragraph breaks
│   │   ├── openlyrics/         # OpenLyrics XML (de facto interchange format)
│   │   └── bible/              # OSIS / Zefania / USFX → our bible schema
│   ├── media/                  # file scanning, thumbnails, ffmpeg transcode/probe
│   ├── render/                 # Godot: builds a slide scene from a models.Slide
│   │   ├── slideview.go        # SlideView node: bg layer + text layer + transition compositor
│   │   ├── textfit.go          # auto-shrink: binary search font size using Font.GetMultilineStringSize
│   │   └── transition.go       # two SubViewports (from/to) → ShaderMaterial, progress driven by a Tween
│   ├── ui/                     # Godot operator UI (bufflehead pattern: Extension structs)
│   │   ├── app.go              # root, menus, hotkeys
│   │   ├── schedule_panel.go
│   │   ├── preview_panel.go    # slide thumbnail grid + SlideView
│   │   ├── live_panel.go
│   │   ├── library_panel.go    # tabs: Songs / Scriptures / Media / Themes
│   │   ├── song_editor.go
│   │   ├── theme_editor.go
│   │   └── output_window.go    # borderless Window on chosen screen, hosts one SlideView
│   ├── outputs/                # display enumeration, output routing (later: stage, NDI)
│   └── control/                # copied from bufflehead: HTTP + bearer key (tests, later remote)
├── test/                       # pytest integration via control API (bufflehead harness)
├── bin/                        # release-dmg, sign-notarize, build-windows, build-linux
└── packaging/
```

### Key design rules
1. **`models` and `store` never import Godot.** Slide splitting, scripture parsing, schedule logic and
   presenter state transitions are plain Go and covered by `go test`. Godot is a projection.
2. **One `PresenterState`, many views.** Preview pane, Live pane, Output window, Stage display and
   Remote are all renderers of the same state, which matches Bufflehead's CLAUDE.md state-machine rule.
   Events (`GoLive`, `NextSlide`, `Black`, `SelectScheduleItem`) mutate state. Then one `render()` pass runs.
3. **A Slide is data, not nodes.** `models.Slide` → `render.SlideView` builds nodes. The same code
   draws thumbnails (small SubViewport), preview, and the full-screen output, so what you preview is what goes live.
4. **Main-thread discipline.** DB queries, imports and ffmpeg run in goroutines. Results are posted to a channel
   and drained in `App.Process()` (the same pattern Bufflehead uses for `pendingInit` / SQL replies).
5. **Output resolution independence.** Author slides in a logical 1920×1080 canvas.
   The output `SubViewport` renders at the target display's native resolution and the operator views scale it down.

### Output window (the most important piece)
- `DisplayServer.GetScreenCount()` / `ScreenGetSize(i)` / `ScreenGetPosition(i)` list the displays.
  The user picks one in Settings. The default is the first non-primary screen.
- Create a `Window` with `Borderless`, `AlwaysOnTop` and `Unfocusable` set, move it to the screen's rect, and
  set mode `ExclusiveFullscreen` (or windowed-borderless, which works better on macOS Spaces).
- Re-run the setup when displays are hot-plugged (poll the screen count in `Process`, or catch the window notification).
- With a single screen (developing on the laptop), show the output as a resizable normal window.

### Text rendering
- `Label`/`RichTextLabel` with `outline_size`, `font_outline_color`, `font_shadow_color`, `shadow_offset`.
- Auto-fit: binary-search the font size between theme min and max so the wrapped text fits the
  text box (`Font.GetMultilineStringSize` with width = box width). Cache the result per (slide, theme, resolution).
- Fonts: `SystemFont` for installed fonts (font picker lists system families), and bundle a few OFL fonts.

### Video (largest technical risk)
Godot 4.6 decodes **Ogg Theora only**. EasyWorship's backgrounds are H.264 MP4. Options:

| Option | Effort | Quality | Decision |
|---|---|---|---|
| A. Transcode to `.ogv` on import using `ffmpeg` (bundled like EW bundles FFmpeg6) | Low | Fine for motion backgrounds | **MVP** |
| B. `ffmpeg` subprocess → raw YUV420 frames over a pipe → 3 `ImageTexture`s + YUV→RGB shader | Medium | Any codec, ~90 MB/s at 1080p30 | **v0.2** for user videos with audio |
| C. Native decode via a GDExtension (e.g. godot-ffmpeg) or cgo `libav*` | High | Best | Later, only if B falls short |

Hide all three behind a single `media.VideoSource` interface so the renderer doesn't care which one is in use.

### Transitions
- Render the outgoing and incoming slides into two `SubViewport`s, composite them on a `ColorRect` with a
  `ShaderMaterial` that takes `from`, `to` and `progress` uniforms, and drive `progress` from 0 to 1 with a `Tween`.
- Write each transition as a small `.gdshader`: blend, push, wipe, cover, reveal, iris, zoom.
- Black/Clear/Logo are just target states, so they use the same transition path.

### Storage schema (sketch)
```
songs(id, uuid, title, author, copyright, ccli, tags, theme_id, created, updated)
song_sections(song_id, ord, kind, label, text)          -- kind: verse|chorus|bridge|prechorus|tag|intro|ending
songs_fts(title, author, lyrics)                         -- FTS5
bibles(id, abbrev, name, lang, copyright)
verses(bible_id, book, chapter, verse, text)             -- ~31k rows per translation
verses_fts(text)
media(id, path, kind, duration, width, height, thumb_path, transcoded_path)
themes(id, name, json)                                   -- Theme serialized as JSON
song_history(song_id, used_at)                           -- for CCLI reports
```
Schedules are **files** (`.fbs` = zip of `schedule.json` plus any referenced media), so they can be
carried to another computer, as EasyWorship does with `.ewsx`.

---

## 4. Milestones

Each milestone ends with something you can run (`gd run`) and demo.

### M0: Skeleton (1–2 days)
- `go mod init freebeams`. Add `graphics.gd`, `modernc.org/sqlite`, `google/uuid`, `yaml.v3`.
- Copy and adapt from Bufflehead: `main.go` startup, `graphics/project.godot` + `library.gdextension` +
  `export_presets.cfg` (rename, fix audio driver), `theme.go` (colors/fonts), `control/`, `bin/release-dmg`.
- Empty operator window with the 4-pane layout (`HSplitContainer`/`VSplitContainer`).
- Output window opens on the second screen, or windowed if there is only one, and shows a black background.
- **Done when:** `gd run` opens two windows and `go test ./...` passes.

### M1: Slide rendering core (3–5 days)
- `models.Slide`, `models.Theme`, `render.SlideView` (image background + auto-fit text with outline/shadow).
- `PresenterState` with GoLive/Next/Prev/Black/Clear/Logo, plus unit tests.
- Preview and Live panes plus Output render the same `SlideView` at different scales.
- Crossfade transition via SubViewport + shader + Tween.
- **Done when:** hardcoded slides can be stepped with arrow keys and crossfade on the projector.

### M2: Songs (4–6 days)
- `store` with migrations and the songs tables + FTS5.
- Song editor: plain-text lyrics with `[Verse 1]`/`[Chorus]` tags or blank-line splitting. Max lines per slide.
- Library → Songs tab: search box and list. Double-click goes to preview. Thumbnails of all slides.
- Section hotkeys (V/C/B/R/T/I/E jump to section).
- **EasyWorship importer**: read `Songs.db` + `SongWords.db`, parse RTF to text, map to sections.
  Test it against the 223 songs on this machine.
- **Done when:** your current EW song library shows up and can be projected.

### M3: Scripture (3–5 days)
- Bible schema, plus an importer for OSIS/Zefania XML (public-domain KJV, WEB, ASV from ebible.org / Crosswire).
- Optional: an EW `.ewb` importer (zlib-inflate `streams`, parse) for translations the user has a license for.
- Reference parser (`jn 3:16`, `John 3:16-18,20`, `1 Cor 13`, `Ps 23:1-3`), with book aliases in the tables.
- Scripture tab: reference box, verse list, translation picker, verse-per-slide vs. fill mode, reference footer.
- **Done when:** typing `jn 3:16` and pressing Enter shows it in preview, and pressing Enter again sends it live.

### M4: Media & Themes (4–6 days)
- Media library: scan folders, thumbnails (Godot `Image` for stills, `ffmpeg -ss` poster frames for video).
- Motion backgrounds: transcode MP4 → OGV in a background goroutine with a progress indicator, then loop with `VideoStreamPlayer`.
- Theme editor: background (color/gradient/image/video), font family/size/color, outline, shadow,
  alignment, text box margins, and a live preview. Default theme per item type (songs / scripture).
- **Done when:** a song runs over a looping motion background with a custom theme.

### M5: Schedules (3–4 days)
- Schedule panel: add from the library (drag-drop and an "Add to schedule" button), reorder, delete, per-item theme override.
- Save/Open `.fbs`, a recent-files list, autosave, and a "dirty" indicator.
- Schedule hotkeys (↑/↓, PgUp/PgDn go-live-next).
- **Done when:** a full service can be built, saved, reopened and run from the keyboard.

### M6: Polish & ship MVP (4–6 days)
- Settings dialog: output display, default themes, transition and duration, hotkeys.
- More transitions (push, wipe, iris, zoom). Logo screen image.
- Crash safety: the output window must never show the operator UI or an error dialog.
- Signed DMG (Bufflehead's `sign-notarize` + CI workflow), then Windows (Inno Setup) and Linux AppImage.
- Integration tests through the control API: open schedule → go live → next → assert `/state`.

**MVP total: about 4–6 weeks of focused work.**

### v0.2+ (ordered by value to a church)
1. **Stage display**: second output with current + next slide, clock, timer, and notes.
2. **Remote control**: `net/http` + `gorilla/websocket`. The phone web UI shows the schedule, slide grid and next/prev/black.
   Reuse the control server's bearer key and show a QR code with the URL+key in the app.
3. **Video items with audio + transport** (Video option B), plus audio playlists.
4. Countdown timers, alerts/announcement ticker overlay.
5. Presentations: PDF → images (via `pdftoppm`/MuPDF), PPTX via LibreOffice headless → PDF.
6. CCLI usage report from `song_history`. Song import from OpenLyrics, ChordPro, and SongSelect files.
7. NDI output (cgo against the NDI SDK, which EW bundles too), MIDI control, live camera feeds.

---

## 5. Risks & mitigations

| Risk | Mitigation |
|---|---|
| Video codecs (Godot = Theora only) | Transcode on import (MVP). ffmpeg-pipe decoder behind the `VideoSource` interface later |
| Output window on macOS full screen / Spaces / always-on-top quirks | Prototype in **M0** on a real second display before building anything else. Fall back to a borderless window sized to the screen |
| Low-processor mode throttling animations | Toggle it off while the output is animating, and on again when idle |
| HiDPI / scaling differences between operator and projector | Logical 1920×1080 slide canvas. The output SubViewport uses the native size of the target screen |
| graphics.gd API churn (pre-1.0) | Pin a version in `go.mod` and bump deliberately. Pinned: `v0.0.0-20261014193348-f9563e0035a9` with Godot 4.7.2 (its call ring survives nested flushes; the June 2026 version did not) |
| Bible licensing | Ship only public-domain texts. Imports of user-licensed translations stay local |
| RTF lyrics import edge cases | Small dedicated RTF tokenizer with golden tests against this machine's 223 songs |
| Live-service reliability | No modal dialogs on output. All I/O off the main thread. Autosave schedules. Test that output survives a display unplug |

---

## 6. First concrete steps (what to do next)
1. `go mod init freebeams` and copy Bufflehead's `graphics/` scaffolding, `main.go` startup and `control/`.
2. Build M0's two-window spike and **verify the output window on a real projector/TV** right away.
3. Write `models/` (song splitting, scripture parser, presenter state) test-first. It has no Godot dependency.
4. Write `CLAUDE.md` for this repo that carries over Bufflehead's state-machine UI rules.
