package ui

import (
	"freebeams/internal/app/core"

	"graphics.gd/classdb/BaseButton"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/HSlider"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/PanelContainer"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/TextureRect"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Vector2"
)

// MiniTransport is the resource preview's video controls (UI-SPEC §3,
// EasyWorship's Frame_VideoControlsMini): a 28 px mini toolbar with skip
// back, a round 24 px play / pause, skip forward, the progress slider, the
// time and the volume. It previews a clip before it goes live: it drives the
// resource preview's own player, never the Preview or Live ones. Dumb view
// like Transport: SetState, and the shared TransportCallbacks (OnLoop is
// never called: the mini bar has no loop button). The resource pane is
// narrow, so the bar does not ask for the width of all its parts: it clips
// them and drops the volume, then the time, when they do not fit (fit).
type MiniTransport struct {
	TransportCallbacks

	root    PanelContainer.Instance
	clip    Control.Instance
	core    []Control.Instance // always shown: the buttons and the progress slider
	timeBox []Control.Instance // the time and the spacer before it
	volBox  []Control.Instance // the volume icon, its slider and the spacer before them
	play    *transportButton
	back    *miniButton
	fwd     *miniButton
	seek    HSlider.Instance
	time    Label.Instance
	volIcon TextureRect.Instance
	volume  HSlider.Instance

	state   TransportState
	seeking bool
}

// NewMiniTransport builds an empty, disabled mini bar.
func NewMiniTransport() *MiniTransport {
	t := &MiniTransport{}
	t.root = panel(vMiniBarTL)
	t.root.AsControl().SetCustomMinimumSize(Vector2.New(0, 28))
	// A plain Control does not pass its children's minimum width up, so the
	// bar never widens the resource pane; the row fills it and is clipped.
	t.clip = Control.New()
	t.clip.SetClipContents(true)
	t.clip.SetMouseFilter(Control.MouseFilterIgnore)
	t.clip.SetCustomMinimumSize(Vector2.New(0, 24))
	add(t.root, t.clip)
	row := hbox(miniSeparation)
	row.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	bar := withMargins(row.AsControl(), miniMarginLeft, miniMarginRight)
	t.clip.AsNode().AddChild(bar.AsNode())
	bar.SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	t.clip.OnResized(t.fit)

	t.back = newMiniButton(miniPlain, "skip_back", 22, 24, "Skip to Start")
	t.back.AsBaseButton().OnPressed(func() {
		if t.OnSkip != nil {
			t.OnSkip(-1)
		}
	})
	t.play = newTransportButton("play", "Play / Pause")
	t.play.AsControl().SetCustomMinimumSize(Vector2.New(24, 24))
	t.play.AsBaseButton().OnPressed(func() {
		if t.OnPlayPause != nil {
			t.OnPlayPause()
		}
	})
	t.fwd = newMiniButton(miniPlain, "skip_fwd", 22, 24, "Skip to End")
	t.fwd.AsBaseButton().OnPressed(func() {
		if t.OnSkip != nil {
			t.OnSkip(1)
		}
	})

	t.seek = HSlider.New()
	t.seek.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	t.seek.AsControl().SetSizeFlagsVertical(Control.SizeShrinkCenter)
	t.seek.AsControl().SetCustomMinimumSize(Vector2.New(40, 16))
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
		if !t.seeking && t.OnSeek != nil {
			t.OnSeek(float64(v))
		}
	})

	t.time = label("0:00 / 0:00", vCaption)
	t.time.SetClipText(false)
	t.time.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	t.time.SetHorizontalAlignment(GUI.HorizontalAlignmentRight)

	t.volIcon = TextureRect.New()
	t.volIcon.SetStretchMode(TextureRect.StretchKeepCentered)
	t.volIcon.AsControl().SetCustomMinimumSize(Vector2.New(18, 24))
	t.volIcon.AsControl().SetSizeFlagsVertical(Control.SizeShrinkCenter)
	t.volIcon.AsControl().SetMouseFilter(Control.MouseFilterIgnore)
	onThemeReady(t.volIcon.AsControl(), func() { t.volIcon.SetTexture(fwIcon(t.volIcon.AsControl(), "volume")) })
	t.volume = HSlider.New()
	t.volume.AsControl().SetCustomMinimumSize(Vector2.New(48, 16))
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
	t.core = []Control.Instance{t.back.AsControl(), t.play.AsControl(), t.fwd.AsControl(), fixed(4, 0), t.seek.AsControl()}
	t.timeBox = []Control.Instance{fixed(4, 0), t.time.AsControl()}
	t.volBox = []Control.Instance{fixed(4, 0), t.volIcon.AsControl(), t.volume.AsControl()}
	add(row, t.core...)
	add(row, t.timeBox...)
	add(row, t.volBox...)
	t.state.Volume = 1
	t.render()
	return t
}

// AsControl returns the bar's root control.
func (t *MiniTransport) AsControl() Control.Instance { return t.root.AsControl() }

// Callbacks returns the bar's callbacks, to be set by the app.
func (t *MiniTransport) Callbacks() *TransportCallbacks { return &t.TransportCallbacks }

// SetState projects the player's state onto the bar (idempotent, no signals).
func (t *MiniTransport) SetState(s TransportState) {
	t.state = s
	t.render()
}

// State returns what the bar shows.
func (t *MiniTransport) State() TransportState { return t.state }

func (t *MiniTransport) render() {
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
	t.volume.AsRange().SetValueNoSignal(Float.X(min(max(s.Volume, 0), 1)))
	for _, b := range []BaseButton.Instance{t.play.AsBaseButton(), t.back.AsBaseButton(), t.fwd.AsBaseButton()} {
		b.SetDisabled(!s.Enabled)
	}
	t.seek.AsSlider().SetEditable(s.Enabled && s.Duration > 0)
	t.fit() // the time's width follows its text
	flushCalls(t.root.AsControl())
}

// The row's spacing (fit adds it up).
const (
	miniSeparation  = 2
	miniMarginLeft  = 4
	miniMarginRight = 6
)

// fit shows the time and the volume only when the bar is wide enough for
// them, the volume going first. It reads the bar's width, which the resource
// pane decides alone (the clip has no minimum width), so it cannot loop.
func (t *MiniTransport) fit() {
	t.time.GetLineCount() // shape the new text before reading its minimum width
	width := func(parts []Control.Instance) float32 {
		w := float32(0)
		for _, c := range parts {
			w += c.GetCombinedMinimumSize().X + miniSeparation
		}
		return w
	}
	avail := t.clip.Size().X
	need := miniMarginLeft + miniMarginRight - miniSeparation + width(t.core) + width(t.timeBox)
	showTime := need <= avail
	showVol := showTime && need+width(t.volBox) <= avail
	for _, c := range t.timeBox {
		c.AsCanvasItem().SetVisible(showTime)
	}
	for _, c := range t.volBox {
		c.AsCanvasItem().SetVisible(showVol)
	}
	flushCalls(t.clip)
}
