# CLAUDE.md

FreeWorship is a native worship presentation app (an EasyWorship 8 clone) built with **Go + graphics.gd (Godot 4.6)
+ pure-Go SQLite (`modernc.org/sqlite`)**. The stack and conventions come from Bufflehead
(`~/Documents/Work/Apex/bufflehead`). Read its `CLAUDE.md` and `internal/ui`, `internal/control` for working
graphics.gd idioms.

- Plan: `PLAN.md`. Current goal: `docs/PHASE1.md`. UI source of truth: `docs/UI-SPEC.md`
  (the decoded EasyWorship forms in `docs/ew-ui/forms/*.dfm` are git-ignored reference material).

## Build & run
```bash
go build ./... && go vet ./... && go test ./...
gd run                  # builds graphics/darwin_*.dylib and launches Godot (~/gd/bin/godot.app)
GOOS=macos gd build     # release build into releases/
```
`gd` generates `graphics/library.gdextension` and the dylibs. Never hand-write or commit them.
The first run on a clean checkout runs a headless Godot import. That import needs no dylib, and `gd` builds it afterwards.

## Package layout & dependency rules
```
main.go                    startup only: startup.LoadingScene(), register classes, add app.App, startup.Scene()
internal/models            pure Go domain types + logic (songs, scripture refs, slides, themes, schedule, presenter)
internal/store             SQLite persistence (songs, bibles, themes, settings), FTS5 search
internal/importers/...     EasyWorship (RTF lyrics, .ewb bibles), generic formats
internal/render            Godot: SlideView (draws a models.Slide with a models.Theme), transitions, output window
internal/ui                Godot: operator UI widgets matching docs/UI-SPEC.md (dumb views)
internal/control           HTTP control server (bearer key) for tests/automation
internal/app               Godot: the App node that wires store + presenter + ui + render + control
```
- `models`, `store` and `importers` **must not import graphics.gd**. They are unit-tested with plain `go test`.
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

## Conventions
- Errors are wrapped with context (`fmt.Errorf("load song %d: %w", id, err)`). No panics in library code.
- Table-driven tests. Tests that need the local EasyWorship install (`/Users/Shared/Softouch/...`) call `t.Skip` when it is absent.
- Logical slide canvas is **1920×1080**. Font sizes, margins and outline sizes in `models.Theme` are in canvas units.
- Commit messages: conventional commits (`feat(store): ...`).
