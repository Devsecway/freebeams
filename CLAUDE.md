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
GOOS=macos bin/gd build # release build into releases/
test/smoke.sh           # end to end through the control API; screenshots and app.log in test/out/
test/splitters.sh       # drags every splitter through Godot's input pipeline (the layout-crash regression test)
```
`bin/gd` runs the graphics.gd build tool pinned in `go.mod` (`tool graphics.gd/cmd/gd`) with `GDPATH=~/gd/freeworship`,
so it installs and uses its own Godot and leaves Bufflehead's `~/gd/bin` alone. Keep the local Go toolchain at the
`go.mod` version: with an auto-downloaded toolchain `gd` says so and builds without its resident-callback runtime patch.
`gd` generates `graphics/library.gdextension` and the dylibs. Never hand-write or commit them (`graphics/icon.svg` and
`main.tscn` are template files it writes once; they are committed).
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

## Conventions
- Errors are wrapped with context (`fmt.Errorf("load song %d: %w", id, err)`). No panics in library code.
- Table-driven tests. Tests that need the local EasyWorship install (`/Users/Shared/Softouch/...`) call `t.Skip` when it is absent.
- Logical slide canvas is **1920×1080**. Font sizes, margins and outline sizes in `models.Theme` are in canvas units.
- Commit messages: conventional commits (`feat(store): ...`).
