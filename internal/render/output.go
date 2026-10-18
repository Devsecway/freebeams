package render

import (
	"errors"
	"fmt"
	"math"
	"time"

	"freeworship/internal/models"

	"graphics.gd/classdb"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/DisplayServer"
	"graphics.gd/classdb/Image"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Vector2i"
)

// RegisterAll registers every Godot extension class in this package. Call it
// once at startup, before creating any SlideView.
func RegisterAll() {
	classdb.Register[SlideView]()
}

// OutputTitle is the title of the output window in windowed mode.
const OutputTitle = "FreeWorship Output"

// ScreenInfo describes one display.
type ScreenInfo struct {
	Index    int
	Position [2]int // top-left in DisplayServer (pixel) coordinates
	Size     [2]int // pixels
	Scale    float64
	Primary  bool
}

// Screens lists the connected displays. Main thread only.
func Screens() []ScreenInfo {
	n := DisplayServer.GetScreenCount()
	primary := DisplayServer.GetPrimaryScreen()
	out := make([]ScreenInfo, 0, n)
	for i := 0; i < n; i++ {
		s := DisplayServer.Screen(i)
		p := DisplayServer.ScreenGetPositionOptions(s)
		z := DisplayServer.ScreenGetSizeOptions(s)
		out = append(out, ScreenInfo{
			Index:    i,
			Position: [2]int{int(p.X), int(p.Y)},
			Size:     [2]int{int(z.X), int(z.Y)},
			Scale:    float64(DisplayServer.ScreenGetScaleOptions(s)),
			Primary:  i == primary,
		})
	}
	return out
}

// DefaultOutputScreen returns the first non-primary screen, else the primary.
func DefaultOutputScreen() int {
	return defaultScreen(Screens())
}

func defaultScreen(screens []ScreenInfo) int {
	primary := 0
	for _, s := range screens {
		if s.Primary {
			primary = s.Index
		} else {
			return s.Index
		}
	}
	return primary
}

// FullscreenStrategy selects how the output covers a secondary display.
type FullscreenStrategy int

const (
	// Borderless positions a borderless, always-on-top window over the whole
	// screen rect. On macOS this stays on the current Space (no Spaces
	// animation) and keeps the operator window usable; it is the default.
	Borderless FullscreenStrategy = iota
	// NativeFullscreen uses Window.ModeFullscreen on the target screen (a new
	// Space on macOS).
	NativeFullscreen
	// ExclusiveFullscreen uses Window.ModeExclusiveFullscreen (macOS also
	// hides the Dock and menu bar while the app is active).
	ExclusiveFullscreen
)

// Output manages the projector window: one OS window hosting a SlideView.
// All methods must be called on the main thread.
type Output struct {
	// OnClosed is called when the user closes the output window (not when
	// Close is called).
	OnClosed func()
	// Strategy selects how a secondary display is covered (default Borderless).
	Strategy FullscreenStrategy

	parent     Node.Instance
	win        Window.Instance
	view       *SlideView
	screen     int
	fullscreen bool
	frame      models.Frame
	hasFrame   bool
	fade       time.Duration
	cursorHid  bool
}

// NewOutput returns a closed output whose window, once opened, is added under
// parent (typically the scene root or the App node).
func NewOutput(parent Node.Instance) *Output {
	return &Output{parent: parent, fade: DefaultTransition, screen: -1}
}

// IsOpen reports whether the output window exists.
func (o *Output) IsOpen() bool { return o.win != Window.Nil }

// Screen returns the screen the output was opened on (-1 when closed).
func (o *Output) Screen() int {
	if !o.IsOpen() {
		return -1
	}
	return o.screen
}

// Fullscreen reports whether the output covers a whole secondary screen
// (false for the single-screen windowed fallback).
func (o *Output) Fullscreen() bool { return o.IsOpen() && o.fullscreen }

// View returns the output's SlideView (nil when closed).
func (o *Output) View() *SlideView { return o.view }

// SetTransitionDuration sets the output's crossfade duration (default 500ms).
func (o *Output) SetTransitionDuration(d time.Duration) {
	o.fade = d
	if o.view != nil {
		o.view.SetTransitionDuration(d)
	}
}

// Open shows the output on screen. A non-primary screen gets a borderless
// window covering it (see Strategy). The primary (or only) screen gets a
// normal resizable 16:9 window titled "FreeWorship Output". An out-of-range
// screen falls back to DefaultOutputScreen. Opening an open output moves it.
func (o *Output) Open(screen int) {
	screens := Screens()
	if screen < 0 || screen >= len(screens) {
		screen = defaultScreen(screens)
	}
	if o.IsOpen() {
		o.closeWindow()
	}
	var info ScreenInfo
	if screen < len(screens) {
		info = screens[screen]
	} else {
		info = ScreenInfo{Size: [2]int{1920, 1080}, Scale: 1, Primary: true}
	}
	o.screen = screen
	o.fullscreen = !info.Primary

	win := Window.New()
	win.SetTitle(OutputTitle)
	win.SetTransient(false)
	win.SetExclusive(false)
	win.SetWrapControls(false)
	// Render at the window's native pixel size: crisp text, true 1080p/4K.
	win.SetContentScaleFactor(1)
	win.AsViewport().SetTransparentBg(false)
	win.SetCurrentScreen(screen)
	win.SetInitialPosition(Window.WindowInitialPositionAbsolute)

	if o.fullscreen {
		win.SetBorderless(true)
		win.SetUnresizable(true)
		win.SetUnfocusable(true) // keep keyboard focus in the operator window
		win.SetPosition(Vector2i.New(info.Position[0], info.Position[1]))
		win.SetSize(Vector2i.New(info.Size[0], info.Size[1]))
		switch o.Strategy {
		case NativeFullscreen:
			win.SetMode(Window.ModeFullscreen)
		case ExclusiveFullscreen:
			win.SetMode(Window.ModeExclusiveFullscreen)
		default:
			win.SetAlwaysOnTop(true)
		}
	} else {
		scale := info.Scale
		if scale <= 0 {
			scale = 1
		}
		w := int(math.Round(960 * scale))
		h := int(math.Round(540 * scale))
		win.SetBorderless(false)
		win.SetUnresizable(false)
		win.SetMinSize(Vector2i.New(int(320*scale), int(180*scale)))
		win.SetSize(Vector2i.New(w, h))
		x := info.Position[0] + (info.Size[0]-w)/2
		y := info.Position[1] + (info.Size[1]-h)/2
		win.SetPosition(Vector2i.New(x, y))
	}

	root := Control.New()
	root.SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	root.SetMouseFilter(Control.MouseFilterIgnore)
	view := NewSlideView()
	view.SetTransitionDuration(o.fade)
	view.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	root.AsNode().AddChild(view.AsNode())
	win.AsNode().AddChild(root.AsNode())

	win.OnCloseRequested(func() {
		o.closeWindow()
		if o.OnClosed != nil {
			o.OnClosed()
		}
	})
	win.OnMouseEntered(func() {
		if o.fullscreen && !o.cursorHid {
			Input.SetMouseMode(Input.MouseModeHidden)
			o.cursorHid = true
		}
	})
	win.OnMouseExited(o.showCursor)

	o.win = win
	o.view = view
	o.parent.AddChild(win.AsNode())
	if o.hasFrame {
		view.SetFrame(o.frame) // first frame is a cut
	}
}

func (o *Output) showCursor() {
	if o.cursorHid {
		Input.SetMouseMode(Input.MouseModeVisible)
		o.cursorHid = false
	}
}

// Close frees the output window. The last frame is remembered and shown again
// on the next Open.
func (o *Output) Close() { o.closeWindow() }

func (o *Output) closeWindow() {
	if !o.IsOpen() {
		return
	}
	o.showCursor()
	win := o.win
	o.win, o.view, o.screen = Window.Nil, nil, -1
	win.Hide()
	win.AsNode().QueueFree()
}

// SetFrame shows f on the output (crossfading). It is remembered while closed.
func (o *Output) SetFrame(f models.Frame) {
	o.frame, o.hasFrame = f, true
	if o.view != nil {
		o.view.SetFrame(f)
	}
}

// ErrOutputClosed is returned by Screenshot when the output is not open.
var ErrOutputClosed = errors.New("output window is not open")

// Screenshot returns a PNG of the output window's viewport as last drawn.
func (o *Output) Screenshot() ([]byte, error) {
	if !o.IsOpen() {
		return nil, ErrOutputClosed
	}
	return viewportPNG(o.win)
}

// WindowScreenshot returns a PNG of any window's viewport (e.g. the main
// window: SceneTree root).
func WindowScreenshot(w Window.Instance) ([]byte, error) {
	if w == Window.Nil {
		return nil, errors.New("screenshot: nil window")
	}
	return viewportPNG(w)
}

func viewportPNG(w Window.Instance) ([]byte, error) {
	tex := w.AsViewport().GetTexture()
	img := tex.AsTexture2D().GetImage()
	if img == Image.Nil || img.IsEmpty() {
		return nil, fmt.Errorf("screenshot: viewport %q has no image yet", w.Title())
	}
	png := img.SavePngToBuffer()
	if len(png) == 0 {
		return nil, fmt.Errorf("screenshot: encode png for %q", w.Title())
	}
	return png, nil
}
