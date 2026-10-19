# CLAUDE.md

FreeWorship is a native worship presentation app (an EasyWorship 8 clone) built with **Go + graphics.gd (Godot 4.7)
+ pure-Go SQLite (`modernc.org/sqlite`)**. The stack and conventions come from Bufflehead
(`~/Documents/Work/Apex/bufflehead`). Read its `CLAUDE.md` and `internal/ui`, `internal/control` for working
graphics.gd idioms.

- Plan: `PLAN.md`. Current goal: `docs/PHASE1.md`. UI source of truth: `docs/UI-SPEC.md`
  (the decoded EasyWorship forms in `docs/ew-ui/forms/*.dfm` are git-ignored reference material).

## Build & run
```bash
go build ./... && go vet ./... && go test ./...
bin/gd run              # builds graphics/darwin_*.dylib and launches Godot 4.7 (~/gd/freeworship/bin/Godot.app)
GOOS=macos bin/gd build # release build: releases/darwin/universal/FreeWorship.app (also GOOS=windows, linux)
bin/release-dmg         # the app + an Applications link as releases/FreeWorship-<version>-macOS.dmg
bin/version             # the release version (config/version in graphics/project.godot)
test/smoke.sh           # end to end through the control API; screenshots and app.log in test/out/
test/splitters.sh       # drags every splitter through Godot's input pipeline (the layout-crash regression test)
```
Releases: `docs/RELEASE.md` (scripts in `bin/`, Inno Setup and AppImage recipes in `packaging/`, GitHub Actions in
`.github/workflows/`). The icon set is generated from `graphics/icon.svg` by `packaging/icons/make-icons.sh`.
`bin/gd` runs the graphics.gd build tool pinned in `go.mod` (`tool graphics.gd/cmd/gd`) with `GDPATH=~/gd/freeworship`,
so it installs and uses its own Godot and leaves Bufflehead's `~/gd/bin` alone. Keep the local Go toolchain at the
`go.mod` version: with an auto-downloaded toolchain `gd` says so and builds without its resident-callback runtime patch.
`gd` generates `graphics/library.gdextension`, `graphics/graphics.gen.go` and the dylibs. Never hand-write or commit
them (`graphics/icon.svg` and `main.tscn` are template files it writes once; they are committed).
The first run on a clean checkout runs a headless Godot import. That import needs no dylib, and `gd` builds it afterwards.

## Package layout & dependency rules
```
main.go                    startup only: startup.LoadingScene(), register classes, add app.App, startup.Scene()
internal/models            pure Go domain types + logic (songs, scripture refs, slides, themes, schedule, presenter)
internal/store             SQLite persistence (songs, bibles, themes, settings), FTS5 search
internal/importers/...     EasyWorship (RTF lyrics, .ewb bibles), generic formats
internal/media             pure Go: media folder scan, thumbnails, ffmpeg probe/poster/transcode behind interfaces
internal/render            Godot: SlideView (draws a models.Slide with a models.Theme), transitions, output window
internal/ui                Godot: operator UI widgets matching docs/UI-SPEC.md (dumb views)
internal/control           HTTP control server (bearer key) for tests/automation
internal/app               Godot: the App node that wires store + presenter + ui + render + control
```
- `models`, `store`, `importers` and `media` **must not import graphics.gd**. They are unit-tested with plain `go test`.
- `ui` widgets are **dumb views**. They receive data through setters (`SetItems(...)`) and report user intent through
  exported callback fields (`OnSelect func(i int)`). They never touch the store, the presenter or the render package.
  Slide thumbnails come from an injected factory func.
- Only `internal/app` knows about every package.

## UI rule: state machine, not procedural mutation (from Bufflehead)
1. Authoritative state lives in plain Go (`models.Presenter`, `models.Schedule`, app fields). Godot nodes are a projection of it.
2. Events (clicks, hotkeys, control commands) mutate state only.
3. One idempotent `render()` pass updates nodes from state. It must be safe to call repeatedly and must **never write state**.
4. Godot signals are translated into events. Suppress re-emitted signals in one place around render, not with scattered flags.

## graphics.gd idioms
- Extension node: `type X struct { Control.Extension[X] \`gd:"FwX"\` }` with `Ready()` and `Process(delta Float.X)`. Register
  every extension with `classdb.Register[X]()` in that package's `RegisterAll()`. Prefix Godot class names with `Fw`.
- All Godot calls happen on the main thread. Goroutines (DB, import, file IO) send results over channels that are
  drained in `Process()`.
- New OS windows: `Window.New()` added under the root; `embed_subwindows=false` is set in `project.godot`.
- Low-processor mode is on. Call `OS.SetLowProcessorUsageMode(false)` while a transition or video is animating, and turn it back on afterwards.

## graphics.gd gotchas (learned the hard way)
- Go references to RefCounted objects created on the main thread are released at the end of the frame, unless they are
  reachable from a live extension instance's fields. Package-level caches must pin them with `Object.Leak`.
- A reference that *is* reachable from an extension struct is reported as leaked at exit. Don't store resources the
  engine already owns (e.g. `LabelSettings`); fetch them with getters (`Label.LabelSettings()`).
- `Object.Free` does nothing on an `Object.Leak`-pinned RefCounted. Release it with `Object.Call(obj, "call", "unreference")`.
  A bare `Object.Call(obj, "unreference")` with no args panics.
- `Viewport.GetTexture().GetImage()` returns the *previous* frame. Wait a frame (or on state such as
  `render.SlideView.TransitionProgress()`) before capturing.
- `Tween.TweenProperty` is not exposed on the simple API. Animate from `Process` instead.
- `SceneTree.Get(node)` is the equivalent of `GetTree()`.
- Call `render.ReleaseCaches()` at shutdown to keep the exit log free of leak warnings.
- Exported node-typed fields on an extension struct are auto-instantiated as children. Tag such fields `gd:"-"`.
- Don't keep RefCounted resources (Theme, StyleBox) in extension struct fields: they leak at exit, while Go-only
  ones die after a frame. Store them in an engine-owned place (e.g. the node's Theme) and fetch them when needed.
- `go test` cannot link packages that import `graphics.gd/classdb/...`. Keep testable logic in pure packages
  (`internal/ui/uicore`, `internal/render/layout`) that import at most `graphics.gd/variant/*`.
- `SystemFont` with no names falls back to Godot's font. Use names `[".AppleSystemUIFont", ...]` for SF. Use `DPITexture` for SVG icons.
- Separate OS windows (dialogs) need the Theme set explicitly and the parent's content scale copied.
- The root window's content scale must be set to `DisplayServer.ScreenGetScale()` at startup (HiDPI).
- graphics.gd buffers void engine calls, and `_gui_input` does not flush that buffer when the handler returns. Follow
  `AcceptEvent()` / `SetInputAsHandled()` with any getter to force the flush (see `internal/ui/events.go`
  `acceptEvent` / `setInputHandled`). Otherwise Godot moves focus before the accept lands.
- Container sort handlers (`NotificationSortChildren`) must end with a getter (`flushCalls` in `internal/ui/events.go`).
  Otherwise the buffered `FitChildInRect` calls are flushed inside a child's own `Draw`. Godot then moves the child
  mid-draw and drops the redraw that updates its canvas transform, so the child keeps rendering at its old offset.
- Programmatic Tree selection: wrap it in `quietly` (`internal/ui/events.go`). It blocks the Tree's signals and flushes
  before unblocking, so `item_selected` is never reported as a user action.
- Godot gives a window only one exclusive child. A dialog opened over another dialog must be that dialog's child
  (`MainWindow.dialogHost` reparents it on popup).
- `PropagateNotification` and `Input.ParseInputEvent` deliver after the current Go call returns.
- A synthesized key event needs `unicode` set to type into a LineEdit.
- Changing a Tree selection in code (`set_selected`/`deselect_all`) needs `QueueRedraw()` to show.
- There is no `timeout` command on macOS. Run `gd run` directly, and have demos quit themselves.
- `DPITexture.SetSource(svg)` keeps the SVG and re-rasterises it for the drawing viewport's oversampling (the
  window's content scale); its logical size is the SVG `width`/`height`. Draw it at whole-pixel positions: a
  fractional offset (e.g. a 16 px icon centred in a 27 px button) resamples the bitmap and blurs it.
- One white single-color SVG tinted through the `modulate` argument of `DrawTextureRect` replaces a texture per
  color/state; the tint multiplies the anti-aliased edges correctly. An icon with colors of its own must be drawn
  with a white modulate (alpha only). See `docs/ICONS.md`.
- `DrawColoredPolygon` is not anti-aliased: tiny shapes (the ▾ of dropdowns) look jagged at 1×. Rasterise them as
  an SVG `DPITexture` instead.
- Godot emits `theme_changed` *before* it invalidates the control's theme-item cache, so `GetThemeIcon` /
  `GetThemeColor` inside an `OnThemeChanged` handler return what an earlier lookup cached: the red fallback icon if
  that lookup ran before the control entered the tree (e.g. a Tree built in a constructor), or the previous
  palette's item. Fetch theme items for stored textures from `onThemeReady` (`internal/ui/widgets.go`), which
  defers past the notification with `Callable.Defer`. Custom `Draw` code is unaffected (it runs after).
- A `GetMinimumSize` must not depend on the control's own size inside a `ScrollContainer`. The slide grid's height
  came from its width, the height decided the scrollbar, and the scrollbar changed the width: at some widths Godot
  toggled the scrollbar forever inside one message-queue flush, until `Message queue out of memory` and a crash while a
  splitter was dragged. Lay out from a width the scrollbar cannot change (`SlideGrid.layoutWidth`).
- Millions of `Object was deleted while awaiting a callback.` lines are Godot dumping its message queue after that
  out-of-memory, not a graphics.gd leak. The cause is above them: `Failed method: ... Message queue out of memory`.
- To find a layout loop, count Godot→Go callbacks per frame and `debug.PrintStack()` past a threshold: the Go frames
  name the handlers even though Godot's frames are stripped. For a hang, `sample <godot pid>` (see `test/splitters.sh`).
- graphics.gd before 2026-10 could corrupt its call batch when a Go signal handler (e.g. `resized`) made engine calls
  during a flush (`crash in ring buffer flush at entry N`). The pinned version handles nested flushes; never go back.
- Fullscreen modes let the macOS menu bar slide in at the top of the projector while the output has no focus.
  `render.CoverScreen` raises the output's NSWindow above the menu bar instead (`cover_darwin.go`, cgo).
- Vsync in two windows on displays with different refresh rates makes every frame wait for both. The output disables
  vsync on the operator window while it is fullscreen (`Output.syncToOutput`).
- Godot marks a borderless window that covers its screen `hidesOnDeactivate` (`_update_window_style`), so the
  projector vanished whenever another app was active. `cover_darwin.go` sets `hidesOnDeactivate:NO` and `canHide:NO`.
  `NSWindow.isVisible` still says YES for a window hidden that way; `occlusionState` and
  `CGWindowListCopyWindowInfo(kCGWindowListOptionOnScreenOnly)` tell the truth (`Output.NativeState`, hook `output-native`).
- `PopupMenu.AddSubmenuNodeItem(label, node)` trips graphics.gd's "illegal transfer of ownership" check and panics (and a
  panic in `buildUI` leaves every later frame with nil views). Name the child PopupMenu and use `AddSubmenuItem(label, "Name")`.
- A `Label` with `clip_text` (what `ui.label()` sets) that does not expand has zero minimum width in a BoxContainer and
  vanishes. Inline captions need `SetClipText(false)` and `OverrunNoTrimming`.
- Project widget values from state with the no-signal setters (`Range.SetValueNoSignal`, `BaseButton.SetPressedNoSignal`,
  `OptionButton.Select`, `ColorPickerButton.SetColor`) and end the pass with a getter (`flushCalls`): a buffered setter
  that emits would emit at flush time, after any `applying` guard has been reset.
- `OS.GetSystemFonts()` returns installed family names (hidden ones start with ".") usable directly as `SystemFont` names.
- cgo: a CoreFoundation ref cannot be compared with `nil` (`mismatched types C.CGPDFDocumentRef and untyped nil`); test
  NULL in a one-line C helper. Core Graphics without AppKit is safe from a goroutine; it logs "CoreGraphics PDF has
  logged an error" to stderr for a non-PDF (harmless, but it lands in app.log).
- `CGPDFPageGetDrawingTransform` never scales a page up: compute the fit scale yourself (translate + scale, swapping the
  box for `/Rotate` 90/270) and give the transform a rect of exactly the rotated box size.
- A `CGBitmapContext` over a Go `[]byte` with `kCGImageAlphaPremultipliedLast | kCGBitmapByteOrder32Big` is RGBA,
  rows top-down: usable as `image.RGBA.Pix`. Fill it opaque black first so premultiplied == straight.
- Adding a field to a gofmt-aligned struct or map literal in a shared file realigns the whole block. Put new fields in
  their own blank-line-separated block to keep diffs additive and merges clean (several agents edit these files).
- A smoke block that adds a schedule item late must `save-schedule` before `# ---- quit`, or the close question makes
  "the app did not exit after quit" fail.
- Draw calls from a `draw` signal handler (`CanvasItem.OnDraw`) work, but end the handler with a getter (`ci.Visible()`)
  so the buffered draw calls flush while Godot is still inside the item's draw pass.
- `clip_children = ClipChildrenOnly` on a plain Control plus a shape drawn by that Control masks its children (wipe,
  iris); it coexists with `clip_contents` on the same node. Clipping nodes must not be nested (the CanvasGroup rule).
- A `MenuBar` with `prefer_global_menu` lives in the macOS system menu bar and has zero minimum size in the window;
  automation cannot click native menus, so every handler is also a control action (`menu {item}`).
- `debug.ReadBuildInfo().Main.Version` is the VCS pseudo-version under `gd run`, usable for an About box.
- A `//go:build ignore` generator cannot be run as a package (`go run ./pkg/gen` → "build constraints exclude all Go
  files"); run it by file: `go run internal/bibles/bundled/gen/main.go …`. `//go:embed data/*.json.gz` needs at least one
  matching file to compile, so keep the data files in place when regenerating.
- Go 1.27's `encoding/json` error strings name `jsontext.Value` internals; never assert on them. Decode objects with
  `json.Decoder` tokens to keep document order and own the error texts.
- `ui.ConfirmDialog` is a fixed 420×150 window: a question longer than a heading line plus one sentence pushes
  OK/Cancel out of view (see `core.BibleFileQuestion`).
- Two first-run imports writing the same table race on a unique key; tell the second about the first
  (`easyworship.Options.BundledBibles`) instead of relying on `ErrExists` after a full insert. SQLite's
  `_txlock=immediate` + `busy_timeout` serialize the concurrent bundled and EasyWorship imports (~1 s per translation).
- FTS5: a quoted multi-word string is re-tokenized, so `"i ll"*` is how a contraction (`I'll`) matches with unicode61
  (the apostrophe is a separator); `*` applies to the last token only. Column filters take groups:
  `{title author} : ("a"* OR "b"*)`; uncorrelated `id IN (SELECT rowid FROM fts WHERE fts MATCH ?)` flags make cheap
  rank tiers in `ORDER BY`. `snippet()`/`MATCH` resolve through the hidden column named after the table even under an alias.
- `hooks.go isSongTable` identifies the song list by its column count; a new column in `ui/songs.go` must keep it in sync.
- Inspect a WAL-mode `library.db` copy with `sqlite3 "file:x.db?immutable=1"`; `-readonly` fails when it cannot create
  the `-shm` file.
- A Godot window occluded by a fullscreen or frontmost app on the only display (VLC, a browser) stops redrawing:
  `/screenshot?window=output` returns the frozen previous frame and every output pixel check fails. Check
  `osascript … frontmost` before blaming the code; smoke runs need the Godot windows unobstructed.
- `screencapture -D 2` and `DisplayServer.ScreenGetImage` need Screen Recording permission for the terminal. Without
  it, check the TV by eye and the content with `/screenshot?window=output`.
- A node that only reacts to its own lifecycle needs no extension class: connect `OnVisibilityChanged` /
  `OnTreeExiting` closures on the plain engine instance (`render/video.go`). `visibility_changed` also fires when an
  ancestor's visibility changes, so a child can pause itself when its layer is hidden after a crossfade.
- A widget that rebuilds its tiles only when its rows change must carry every input the thumbnail factory reads in
  the row (e.g. `ui.MediaRow.Thumb`); otherwise a thumbnail that arrives later is never drawn.
- Keep `Image.LoadFromFile` on the main thread but feed it small derived files: decode/scale off-thread into the cache
  dir (`~/Library/Caches/FreeWorship/media`) and let the factory load the 640 px JPEG.
- Godot plays Ogg Theora only. Guard `VideoStreamPlayer.Play()` behind an "is .ogv and exists" check: a missing or
  unsupported file logs a Godot ERROR, which fails the smoke test's log hygiene.
- Icon SVGs (`uicore/icons.go`): cut details out of a filled shape as evenodd subpaths of the same `<path>`, never
  paint a "lighter" part over a semi-transparent fill (alphas compound: 50 % over 85 % is 92 %); a hole subpath that
  pokes outside its shape is filled there (it is inside one subpath). ThorVG honours `fill-rule`, `fill-opacity` and
  `stroke-opacity`, and those opacities survive the white-mask + modulate tint as alpha.
- Preview an SVG without the app: `qlmanage -t -s 128 -o <dir> x.svg`. QuickLook does not scale the intrinsic size,
  so set the SVG `width`/`height` to the thumbnail size (keep the `viewBox`) and put a toolbar-grey `<rect>` behind
  the body; `FW_ICON_DUMP=<dir> go test ./internal/ui/uicore/ -run Dump` writes the whole set.
- Measure EasyWorship reference PNGs with `python3 -I` + PIL (`Image.getpixel`, a 1×1 `BOX` resize for an area
  average); the Mac screenshots are 2× (3024 px wide), so scale displayed coordinates before sampling.
- The dark palette's surfaces come from EasyWorship's Mac screenshots, not only the Windows `.dfm` style tokens
  (`docs/UI-SPEC.md` §1 "Mac measurements"). Give a surface its own `uicore.Palette` token (`GridBk`, `ResourceBk`, …)
  and a `Fw…` panel variation instead of pointing it at `window.bk1/2`, and add the token to `TestPaletteMatchesSpec`.
- `PopupMenu.AddItem` / `AddSubmenuItem` with id -1 give the entry its *index* as id, while `AddSeparator` keeps -1.
  An `enableMenu` that switches on `GetItemId(idx)` must give submenu entries explicit ids
  (`MoreArgs().AddSubmenuItem(label, name, id)`), or an entry at index 2 masquerades as the item with id 2.
- A PopupMenu submenu node has one parent: a tile's context menu and the footer gear menu need their own submenu
  copies, both projected from the same state in render (`ui/itemmenus.go` `radioMenu`), never one shared node.
- A background pipeline that rewrites whole rows (`store.SaveMedia` from a pass-start snapshot) clobbers columns the
  operator changed meanwhile (`SetMediaOptions`). Re-read those columns right before the save, or write them with a
  targeted UPDATE only.
- Move a file to the Trash on macOS with `NSFileManager trashItemAtURL` (cgo, Foundation: `internal/app/trash_darwin.go`).
  `osascript -e 'tell application "Finder" to delete …'` needs the Automation (TCC) permission and hangs an unattended
  smoke run on the prompt.
- A smoke block's fixture files go in a folder of their own (`$OUT/media-menus`, recreated by the block): the startup
  `rm -f "$OUT"/*.png` cleans only the top level, and a file left in `$OUT/media` by an aborted run changes what the
  media block's folder scan adds. Later blocks read `media.count` from `/state` at their start rather than reusing
  `$MEDIA_COUNT`: the themes block adds images of its own.
- The scratchpad directory is shared by every agent of one session. Prefix scratch scripts and logs with the worktree
  id (`adbc-run-smoke.sh`), or another agent's same-named wrapper (for its own worktree) runs instead of yours.
- Native video (macOS, `render/avplayer_darwin.go`, see `docs/MEDIA.md`): an `AVPlayerItemVideoOutput` hands out BGRA
  frames; Godot's `Image` has no BGRA8, so swizzle with `vImagePermuteChannels_ARGB8888` (map `{2,1,0,3}`, honours
  `bytesPerRow` padding) straight into a Go `[]byte`, then `Image.SetData` + `ImageTexture.Update` (same size) or
  `SetImage` (size changed). A paused item still delivers its first frame once `ReadyToPlay`, which is how the preview
  shows frame 0 without a poster. Gapless looping is `actionAtItemEnd = None` plus a `DidPlayToEndTime` observer on the
  main queue that seeks to 0 (the rate stays 1); Godot's `NSApp` run loop delivers it between frames.
- A `MediaSource`'s `Image` / `ImageTexture` are Go-held and must be `pin`ned; the engine keeps its own reference while a
  `TextureRect` shows the texture, so an outgoing crossfade layer keeps its last frame after the source is unpinned.
- `visibility_changed` on a layer root is how the outgoing side of a finished crossfade (and a view whose window
  closed) releases its player; a Black and back inside the fade keeps the decoder, a longer one re-opens from 0. Keep
  a view's `Process` on while it holds a source (`SetProcess(transitioning || holdsMedia)`); the pump is guarded by
  `Engine.GetProcessFrames()` so several views advance each source once per frame.
- HSlider: user drags come as `drag_started` … `value_changed` … `drag_ended(changed)`; a click on the track or a
  keyboard step is a bare `value_changed`. Seek on `drag_ended` (and on a bare `value_changed`), and never project the
  position into the slider while `seeking`. `Control.Instance` has no `SetVisible`: it is `AsCanvasItem().SetVisible`.
- A synthetic test clip must be tagged for the matrix the decoder assumes, or an exact pixel check misses by more than
  ±3: `-vf scale=out_color_matrix=bt709:out_range=tv,format=yuv420p -colorspace bt709 -color_primaries bt709
  -color_trc bt709 -color_range tv` makes AVFoundation reproduce `#2a7f62` exactly (`test/smoke.sh` playback block).
- A single-screen machine has the output closed until `output {"on":true}`; `/screenshot?window=output` then returns
  a JSON error, not a PNG (`pngcheck.py` says "not a PNG").
- `en_kjv.json`-style Bibles (the thiagobodruk/bible array layout) carry Portuguese book abbreviations that collide with
  English ones (`jn` is Jonah, `jo` John): resolve a 66-book array by position, never by abbreviation. The file name is
  the only place the translation's abbreviation lives (`bibletext.TranslationFromFilename`).

## Release builds (`gd build`) gotchas
- `bin/gd` must resolve the tool binary with `GOOS= GOARCH= go tool -n gd` and exec it with the target restored:
  `GOOS=macos go tool gd` fails inside `go` itself ("unsupported GOOS/GOARCH pair macos/arm64"), and `GOOS=windows`
  would build `gd.exe` for Windows and try to run it.
- The version has one source: `config/version` in `graphics/project.godot`. The macOS and Windows presets leave
  `short_version`/`version`/`file_version`/`product_version` empty on purpose: Godot falls back to `config/version`
  (`CFBundleShortVersionString` came out as `0.1.0`). `bin/version` prints it, `internal/app/version.go` logs it.
- Godot prefixes the macOS `application/app_category` with `public.app-category.` itself: the preset value is
  `"Productivity"`, not the full identifier (that gave `public.app-category.public.app-category.productivity`).
- `.icns`/`.ico` files are not "resources" for `export_filter="all_resources"`. The presets add
  `include_filter="*.icns"` / `"*.ico"` so `config/macos_native_icon` / `config/windows_native_icon` can be read
  from the pck at startup (a missing file is a Godot ERROR in the log).
- Export templates install into `~/Library/Application Support/Godot/export_templates/<version>.stable/` (per OS; see
  `gdpaths.ExportTemplates` in gd). `gd build` fetches only the target platform's files from the `.tpz` (macos.zip is
  124 MB). CI caches that directory plus `$GDPATH/bin`.
- `GOTOOLCHAIN=local` (what `actions/setup-go` sets) makes gd refuse every download, Godot included. CI sets
  `GOTOOLCHAIN=$(go env GOVERSION)` instead.
- Godot's export ad-hoc signs the bundle (`codesign/codesign=1`); `bin/sign-notarize` re-signs everything with
  `--force` and the entitlements in `packaging/macos/entitlements.plist`.
- Never run `gd build` while the exported app is running from `releases/`: the export replaces the bundle in place.

## Conventions
- Errors are wrapped with context (`fmt.Errorf("load song %d: %w", id, err)`). No panics in library code.
- Table-driven tests. Tests that need the local EasyWorship install (`/Users/Shared/Softouch/...`) call `t.Skip` when it is absent.
- Logical slide canvas is **1920×1080**. Font sizes, margins and outline sizes in `models.Theme` are in canvas units.
- Commit messages: conventional commits (`feat(store): ...`).
