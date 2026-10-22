package render

import (
	"errors"
	"log"
	"math"

	"freebeams/internal/models"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/DisplayServer"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEvent"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Callable"
	"graphics.gd/variant/Vector2i"
)

// StageTitle is the title of the stage display window in windowed mode.
const StageTitle = "FreeBeams Stage Display"

// Stage manages the stage display window (EasyWorship's Foldback output): one
// OS window hosting a StageView, modelled on Output. A non-primary display is
// covered like the projector (borderless, unfocusable, above the menu bar on
// macOS); the primary (or only) display gets a normal 16:9 window that does
// not take the focus when it opens.
//
// Frame pacing: the stage window never waits for vsync. The projector output
// paces the frame loop while it is fullscreen (Output.syncToOutput) and the
// operator window otherwise; a third vsynced window on a display with yet
// another refresh rate would make every frame wait for it too. The stage only
// changes when a slide or the clock does, so a torn frame there is invisible.
//
// All methods must be called on the main thread.
type Stage struct {
	// OnClosed is called when the operator closes the windowed stage display
	// (not when Close is called).
	OnClosed func()
	// OnInput receives the windowed stage display's input events while it has
	// keyboard focus, so the hotkeys work there too (as on the output).
	OnInput func(ev InputEvent.Instance)

	parent     Node.Instance
	win        Window.Instance
	view       *StageView
	screen     int
	fullscreen bool
	frame      models.StageFrame
	hasFrame   bool
	cursorHid  bool
}

// NewStage returns a closed stage display whose window, once opened, is added
// under parent.
func NewStage(parent Node.Instance) *Stage {
	return &Stage{parent: parent, screen: -1}
}

// IsOpen reports whether the stage window exists.
func (st *Stage) IsOpen() bool { return st.win != Window.Nil }

// Screen returns the screen the stage window was opened on (-1 when closed).
func (st *Stage) Screen() int {
	if !st.IsOpen() {
		return -1
	}
	return st.screen
}

// Fullscreen reports whether the stage covers a secondary display.
func (st *Stage) Fullscreen() bool { return st.IsOpen() && st.fullscreen }

// Focused reports whether the stage window has keyboard focus.
func (st *Stage) Focused() bool { return st.IsOpen() && st.win.HasFocus() }

// WindowID returns the stage window's DisplayServer id (-1 when closed).
func (st *Stage) WindowID() int {
	if !st.IsOpen() {
		return -1
	}
	return st.win.GetWindowId()
}

// View returns the stage's StageView (nil when closed).
func (st *Stage) View() *StageView { return st.view }

// Open shows the stage display on screen (see Stage). An out-of-range screen
// falls back to the primary display. Opening an open stage moves it.
func (st *Stage) Open(screen int) {
	screens := Screens()
	if screen < 0 || screen >= len(screens) {
		screen = 0
		for _, s := range screens {
			if s.Primary {
				screen = s.Index
			}
		}
	}
	if st.IsOpen() {
		st.closeWindow()
	}
	info := ScreenInfo{Size: [2]int{1920, 1080}, Scale: 1, Primary: true}
	if screen < len(screens) {
		info = screens[screen]
	}
	st.screen = screen
	st.fullscreen = !info.Primary

	win := Window.New()
	win.SetTitle(StageTitle)
	win.SetTransient(false)
	win.SetExclusive(false)
	win.SetWrapControls(false)
	win.SetContentScaleFactor(1) // native pixels: crisp text at any size
	win.AsViewport().SetTransparentBg(false)
	win.SetCurrentScreen(screen)
	win.SetInitialPosition(Window.WindowInitialPositionAbsolute)
	if st.fullscreen {
		win.SetBorderless(true)
		win.SetUnresizable(true)
		win.SetUnfocusable(true) // the operator window keeps the keyboard
		win.SetPosition(Vector2i.New(info.Position[0], info.Position[1]))
		win.SetSize(Vector2i.New(info.Size[0], info.Size[1]))
		win.SetAlwaysOnTop(!coverSupported)
	} else {
		// A 16:9 window in the bottom-right corner of the primary display,
		// clear of the windowed output (which opens centred).
		scale := info.Scale
		if scale <= 0 {
			scale = 1
		}
		w, h := int(math.Round(640*scale)), int(math.Round(360*scale))
		margin := int(math.Round(48 * scale))
		win.SetBorderless(false)
		win.SetUnresizable(false)
		win.SetMinSize(Vector2i.New(int(320*scale), int(180*scale)))
		win.SetSize(Vector2i.New(w, h))
		x := info.Position[0] + max(0, info.Size[0]-w-margin)
		y := info.Position[1] + max(0, info.Size[1]-h-2*margin)
		win.SetPosition(Vector2i.New(x, y))
	}

	root := Control.New()
	root.SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	root.SetMouseFilter(Control.MouseFilterIgnore)
	view := NewStageView()
	view.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	root.AsNode().AddChild(view.AsNode())
	win.AsNode().AddChild(root.AsNode())

	win.OnCloseRequested(func() {
		st.closeWindow()
		if st.OnClosed != nil {
			st.OnClosed()
		}
	})
	win.OnWindowInput(func(ev InputEvent.Instance) {
		if st.OnInput != nil {
			st.OnInput(ev)
		}
	})
	win.OnMouseEntered(func() {
		if st.fullscreen && !st.cursorHid {
			Input.SetMouseMode(Input.MouseModeHidden)
			st.cursorHid = true
		}
	})
	win.OnMouseExited(st.showCursor)

	st.win = win
	st.view = view
	st.parent.AddChild(win.AsNode())
	// Never vsync the stage (see Stage): the output or the operator window
	// paces the frame loop, whichever has vsync.
	DisplayServer.WindowSetVsyncMode(DisplayServer.VsyncDisabled, DisplayServer.Window(win.GetWindowId()))
	if st.fullscreen && coverSupported {
		st.cover()
	}
	if !st.fullscreen {
		st.keepOperatorFocus()
	}
	if st.hasFrame {
		view.SetFrame(st.frame)
	}
}

// keepOperatorFocus gives the keyboard back to the operator window once the
// windowed stage display is up: showing a window makes it key on macOS, and
// the hotkeys belong where the operator works. (The stage window still takes
// the keys when it is clicked; OnInput runs the hotkeys there.)
func (st *Stage) keepOperatorFocus() {
	refocus := func() {
		if !st.IsOpen() || st.parent == Node.Nil || !st.parent.IsInsideTree() {
			return
		}
		root := SceneTree.Get(st.parent).Root()
		if root != Window.Nil && st.win.HasFocus() {
			root.GrabFocus()
		}
	}
	Callable.Defer(Callable.New(refocus))
}

// cover raises the borderless stage window above the menu bar (macOS), as
// the projector output does (cover_darwin.go): hidesOnDeactivate off, so the
// musicians keep their text while the operator works in another app.
func (st *Stage) cover() {
	apply := func() {
		if !st.IsOpen() {
			return
		}
		if !coverScreen(st.nativeHandle()) {
			log.Printf("[render] could not raise the stage window above the menu bar")
		}
	}
	apply()
	Callable.Defer(Callable.New(apply))
}

func (st *Stage) nativeHandle() uintptr {
	return uintptr(DisplayServer.WindowGetNativeHandle(DisplayServer.WindowHandle, DisplayServer.Window(st.win.GetWindowId())))
}

// NativeState reports the OS window state of the open stage window (ok is
// false when it is closed or the platform has no such probe).
func (st *Stage) NativeState() (NativeWindowState, bool) {
	if !st.IsOpen() || !coverSupported {
		return NativeWindowState{}, false
	}
	return coverState(st.nativeHandle())
}

func (st *Stage) showCursor() {
	if st.cursorHid {
		Input.SetMouseMode(Input.MouseModeVisible)
		st.cursorHid = false
	}
}

// Close frees the stage window. The last frame is shown again on the next Open.
func (st *Stage) Close() { st.closeWindow() }

func (st *Stage) closeWindow() {
	if !st.IsOpen() {
		return
	}
	st.showCursor()
	win := st.win
	st.win, st.view, st.screen = Window.Nil, nil, -1
	win.Hide()
	win.AsNode().QueueFree()
}

// SetFrame shows f on the stage display. It is remembered while closed.
func (st *Stage) SetFrame(f models.StageFrame) {
	st.frame, st.hasFrame = f, true
	if st.view != nil {
		st.view.SetFrame(f)
	}
}

// ErrStageClosed is returned by Screenshot when the stage window is not open.
var ErrStageClosed = errors.New("stage display window is not open")

// Screenshot returns a PNG of the stage window's viewport as last drawn.
func (st *Stage) Screenshot() ([]byte, error) {
	if !st.IsOpen() {
		return nil, ErrStageClosed
	}
	return viewportPNG(st.win)
}
