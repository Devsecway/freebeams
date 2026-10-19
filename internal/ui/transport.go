package ui

import (
	"math"

	"freebeams/internal/app/core"
	"freebeams/internal/ui/uicore"

	"graphics.gd/classdb/BaseButton"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/HSlider"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/PanelContainer"
	"graphics.gd/classdb/StyleBox"
	"graphics.gd/classdb/StyleBoxFlat"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/TextureRect"
	"graphics.gd/classdb/Theme"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Vector2"
)

// Transport is the video / audio transport bar under an output monitor
// (UI-SPEC §3: the output monitor's video controls): a seek slider with the
// time, a round play / pause button, skip to start / end, a loop toggle and
// the volume. It is a dumb view: the app projects the player's state with
// SetState (every frame while something plays) and learns of the operator's
// intent through the On… callbacks. It is shown only while the monitor's
// slide is a video or an audio file (the app sets Visible).
type Transport struct {
	OnPlayPause func()
	OnSeek      func(frac float64) // the seek slider: fraction of the duration
	OnVolume    func(v float64)    // 0..1
	OnLoop      func(on bool)
	OnSkip      func(dir int) // -1: to the start, +1: to the end

	root    PanelContainer.Instance
	seek    HSlider.Instance
	time    Label.Instance
	play    *transportButton
	back    *miniButton
	fwd     *miniButton
	loop    *miniButton
	volIcon TextureRect.Instance
	volume  HSlider.Instance

	state   TransportState
	seeking bool // the operator is dragging the seek slider
}

// TransportState is what the bar shows.
type TransportState struct {
	Playing  bool
	Position float64 // seconds
	Duration float64 // seconds (0: unknown yet)
	Loop     bool
	Volume   float64
	// Enabled is false while the file is loading or could not be opened:
	// the controls are greyed out.
	Enabled bool
}

// transportHeight is the bar's height: a 20 px slider row over a 36 px
// button row with 4 px margins.
const transportHeight = 68

// NewTransport builds an empty, disabled bar.
func NewTransport() *Transport {
	t := &Transport{}
	t.root = panel(vBk1)
	t.root.AsControl().SetCustomMinimumSize(Vector2.New(0, transportHeight))
	col := vbox(4)
	add(t.root, withMargins(col.AsControl(), 8, 8))

	// Row 1: seek slider and the time.
	row1 := hbox(8)
	t.seek = HSlider.New()
	t.seek.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	t.seek.AsControl().SetSizeFlagsVertical(Control.SizeShrinkCenter)
	t.seek.AsControl().SetCustomMinimumSize(Vector2.New(0, 20))
	t.seek.AsControl().SetFocusMode(Control.FocusNone)
	rg := t.seek.AsRange()
	rg.SetMinValue(0)
	rg.SetMaxValue(1)
	rg.SetStep(0.001)
	t.seek.AsSlider().SetScrollable(false)
	t.seek.AsSlider().OnDragStarted(func() { t.seeking = true })
	t.seek.AsSlider().OnDragEnded(func(changed bool) {
		t.seeking = false
		if changed && t.OnSeek != nil {
			t.OnSeek(float64(t.seek.AsRange().Value()))
		}
	})
	rg.OnValueChanged(func(v Float.X) {
		// A keyboard step or a click on the track (no drag end with a
		// change) seeks at once; drags seek on release.
		if !t.seeking && t.OnSeek != nil {
			t.OnSeek(float64(v))
		}
	})
	t.time = label("0:00 / 0:00", vCaption)
	t.time.SetClipText(false)
	t.time.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	t.time.SetHorizontalAlignment(GUI.HorizontalAlignmentRight)
	t.time.AsControl().SetCustomMinimumSize(Vector2.New(84, 0))
	add(row1, t.seek.AsControl(), t.time.AsControl())

	// Row 2: ⏵ | ⏮ ⏭ … ⟲ 🔊 ━━●
	row2 := hbox(4)
	t.play = newTransportButton("play", "Play / Pause")
	t.play.AsBaseButton().OnPressed(func() {
		if t.OnPlayPause != nil {
			t.OnPlayPause()
		}
	})
	t.back = newMiniButton(miniPlain, "skip_back", 28, 24, "Skip to Start")
	t.back.AsBaseButton().OnPressed(func() {
		if t.OnSkip != nil {
			t.OnSkip(-1)
		}
	})
	t.fwd = newMiniButton(miniPlain, "skip_fwd", 28, 24, "Skip to End")
	t.fwd.AsBaseButton().OnPressed(func() {
		if t.OnSkip != nil {
			t.OnSkip(1)
		}
	})
	t.loop = newMiniButton(miniPlain, "loop", 28, 24, "Loop")
	t.loop.AsBaseButton().SetToggleMode(true)
	t.loop.AsBaseButton().OnToggled(func(on bool) {
		if t.OnLoop != nil {
			t.OnLoop(on)
		}
	})
	t.volIcon = TextureRect.New()
	t.volIcon.SetStretchMode(TextureRect.StretchKeepCentered)
	t.volIcon.AsControl().SetCustomMinimumSize(Vector2.New(20, 24))
	t.volIcon.AsControl().SetSizeFlagsVertical(Control.SizeShrinkCenter)
	t.volIcon.AsControl().SetMouseFilter(Control.MouseFilterIgnore)
	onThemeReady(t.volIcon.AsControl(), func() { t.volIcon.SetTexture(fwIcon(t.volIcon.AsControl(), "volume")) })
	t.volume = HSlider.New()
	t.volume.AsControl().SetCustomMinimumSize(Vector2.New(90, 20))
	t.volume.AsControl().SetSizeFlagsVertical(Control.SizeShrinkCenter)
	t.volume.AsControl().SetFocusMode(Control.FocusNone)
	vr := t.volume.AsRange()
	vr.SetMinValue(0)
	vr.SetMaxValue(1)
	vr.SetStep(0.01)
	vr.SetValueNoSignal(1)
	vr.OnValueChanged(func(v Float.X) {
		if t.OnVolume != nil {
			t.OnVolume(float64(v))
		}
	})
	add(row2, t.play.AsControl(), fixed(4, 0), t.back.AsControl(), t.fwd.AsControl(), spacer(),
		t.loop.AsControl(), fixed(4, 0), t.volIcon.AsControl(), t.volume.AsControl())
	add(col, row1.AsControl(), row2.AsControl())
	t.state.Volume = 1
	t.render()
	return t
}

// AsControl returns the bar's root control.
func (t *Transport) AsControl() Control.Instance { return t.root.AsControl() }

// SetState projects the player's state onto the bar. It is idempotent and
// never emits the sliders' or buttons' signals (no-signal setters, flushed
// before the next frame's events).
func (t *Transport) SetState(s TransportState) {
	t.state = s
	t.render()
}

// State returns what the bar shows.
func (t *Transport) State() TransportState { return t.state }

func (t *Transport) render() {
	s := t.state
	if !t.seeking {
		frac := 0.0
		if s.Duration > 0 {
			frac = min(max(s.Position/s.Duration, 0), 1)
		}
		t.seek.AsRange().SetValueNoSignal(Float.X(frac))
	}
	t.time.SetText(core.TransportTime(s.Position, s.Duration))
	icon := "play"
	if s.Playing {
		icon = "pause"
	}
	t.play.setIcon(icon)
	t.loop.AsBaseButton().SetPressedNoSignal(s.Loop)
	t.volume.AsRange().SetValueNoSignal(Float.X(min(max(s.Volume, 0), 1)))
	for _, b := range []BaseButton.Instance{t.play.AsBaseButton(), t.back.AsBaseButton(), t.fwd.AsBaseButton(), t.loop.AsBaseButton()} {
		b.SetDisabled(!s.Enabled)
	}
	t.seek.AsSlider().SetEditable(s.Enabled && s.Duration > 0)
	flushCalls(t.root.AsControl()) // the no-signal setters must land before the guard is gone
}

// ---- the round play / pause button --------------------------------------------

// transportButton is EasyWorship's round transport button: a 36 px disc
// with a 16 px glyph.
type transportButton struct {
	BaseButton.Extension[transportButton] `gd:"FwTransportButton"`

	icon string
}

const transportButtonSize = 36

func newTransportButton(icon, tooltip string) *transportButton {
	b := &transportButton{icon: icon}
	c := b.AsControl()
	c.SetCustomMinimumSize(Vector2.New(transportButtonSize, transportButtonSize))
	c.SetSizeFlagsVertical(Control.SizeShrinkCenter)
	c.SetFocusMode(Control.FocusNone)
	c.SetTooltipText(tooltip)
	return b
}

func (b *transportButton) setIcon(name string) {
	if b.icon == name {
		return
	}
	b.icon = name
	b.AsCanvasItem().QueueRedraw()
}

func (b *transportButton) Draw() {
	c := b.AsControl()
	ci := b.AsCanvasItem()
	bb := b.AsBaseButton()
	size := c.Size()
	cx, cy := float32(math.Floor(float64(size.X/2))), float32(math.Floor(float64(size.Y/2)))
	r := Float.X(min(size.X, size.Y)/2 - 1)
	tint := glyphTint(c, bb)
	fill := alpha(fwColor(c, "glyph"), 0.10)
	switch bb.GetDrawMode() {
	case BaseButton.DrawHover:
		fill = alpha(fwColor(c, "glyph"), 0.18)
	case BaseButton.DrawPressed, BaseButton.DrawHoverPressed:
		fill = alpha(fwColor(c, "highlight"), 0.6)
	}
	if bb.Disabled() {
		fill = alpha(fill, 0.5)
	}
	center := Vector2.New(cx, cy)
	ci.MoreArgs().DrawCircle(center, r, fill, true, -1, true)
	ci.MoreArgs().DrawCircle(center, r, alpha(fwColor(c, "border"), tint.A), false, 1, true)
	// The play triangle sits a touch right of centre to look centred.
	dx := float32(0)
	if b.icon == "play" {
		dx = 1
	}
	drawGlyph(ci, c, b.icon, Vector2.New(cx-8+dx, cy-8), 16, tint)
}

// ---- slider theme -------------------------------------------------------------

// buildSliderTheme styles HSlider (the transport's seek and volume sliders):
// a 4 px rounded track in window.bk0 with a 1 px border, the played part in
// the highlight color and a round grabber (the "knob" icon in the glyph color).
func buildSliderTheme(th Theme.Instance, p Palette) {
	track := func(fill, border Color.RGBA) StyleBox.Instance {
		sb := StyleBoxFlat.New()
		sb.SetBgColor(fill)
		sb.SetBorderWidthAll(1)
		sb.SetBorderColor(border)
		sb.SetCornerRadiusAll(2)
		margins(sb.AsStyleBox(), 0, 2, 0, 2)
		return sb.AsStyleBox()
	}
	th.SetStylebox("slider", "HSlider", track(p.Bk0, p.Border))
	th.SetStylebox("grabber_area", "HSlider", track(p.Highlight, p.Highlight))
	th.SetStylebox("grabber_area_highlight", "HSlider", track(p.HighlightHot, p.HighlightHot))
	knob := uicore.IconColors{Glyph: uicore.GlyphColor(p), Accent: uicore.GlyphColor(p), Red: uicore.GlyphColor(p)}
	if tex, ok := rasterIcon("knob", knob); ok {
		th.SetIcon("grabber", "HSlider", tex)
	}
	hot := uicore.IconColors{Glyph: p.TextSemibold, Accent: p.TextSemibold, Red: p.TextSemibold}
	if tex, ok := rasterIcon("knob", hot); ok {
		th.SetIcon("grabber_highlight", "HSlider", tex)
	}
	dim := uicore.IconColors{Glyph: uicore.WithAlpha(uicore.GlyphColor(p), 0.35)}
	if tex, ok := rasterIcon("knob", dim); ok {
		th.SetIcon("grabber_disabled", "HSlider", tex)
	}
	th.SetConstant("grabber_offset", "HSlider", 0)
	th.SetConstant("center_grabber", "HSlider", 0)
}
