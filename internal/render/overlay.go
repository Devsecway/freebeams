package render

import (
	"math"

	"freebeams/internal/models"
	"freebeams/internal/render/layout"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/Panel"
	"graphics.gd/classdb/StyleBoxFlat"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Vector2"
)

// Overlay is a SlideView's alert layer: the message band (a ticker or a
// banner), the nursery badge, the countdown and the clock, drawn above both
// crossfading layers so a transition, a video or Black / Clear / Logo never
// hides it. The app projects one models.OverlayFrame per frame onto the
// Live monitor's and the output's overlays, so both show the same thing.
//
// Geometry comes from layout.PlaceOverlay in canvas units and is scaled to
// the view; text is rasterized at the target pixel size like slide text. A
// moving ticker costs one SetPosition per frame: no measuring, no strings,
// no allocations.
type Overlay struct {
	root  Control.Instance // at the letterbox rect; clips
	band  Panel.Instance   // the message band; clips the ticker
	msg   Label.Instance
	boxes [3]overlayBox // nursery, countdown, clock

	frame models.OverlayFrame
	style models.AlertStyles
	has   bool

	rect layout.Rect // the letterbox rect in the view
	s    float64     // canvas → view pixels
	met  layout.OverlayMetrics
	lay  layout.OverlayLayout
	msgK float64 // the message label's node scale (small views)

	skinStyle models.AlertStyles // what the styleboxes / label settings were built for
	skinS     float64
	skinned   bool
}

type overlayBox struct {
	panel   Panel.Instance
	text    Label.Instance
	caption Label.Instance
}

const (
	boxNursery = iota
	boxCountdown
	boxClock
)

func newOverlay(parent Node.Instance) *Overlay {
	o := &Overlay{}
	o.root = Control.New()
	o.root.SetClipContents(true)
	ignoreMouse(o.root)
	o.root.AsCanvasItem().SetVisible(false)

	o.band = Panel.New()
	ignoreMouse(o.band.AsControl())
	o.band.AsControl().SetClipContents(true)
	o.msg = newOverlayLabel()
	o.band.AsNode().AddChild(o.msg.AsNode())
	o.root.AsNode().AddChild(o.band.AsNode())

	for i := range o.boxes {
		b := &o.boxes[i]
		b.panel = Panel.New()
		ignoreMouse(b.panel.AsControl())
		b.text = newOverlayLabel()
		b.text.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
		b.caption = newOverlayLabel()
		b.caption.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
		b.panel.AsNode().AddChild(b.caption.AsNode())
		b.panel.AsNode().AddChild(b.text.AsNode())
		o.root.AsNode().AddChild(b.panel.AsNode())
	}
	parent.AddChild(o.root.AsNode())
	return o
}

func newOverlayLabel() Label.Instance {
	lb := newTextLabel()
	lb.SetVerticalAlignment(GUI.VerticalAlignmentCenter)
	lb.SetClipText(false)
	lb.AsCanvasItem().SetVisible(true)
	return lb
}

// MeasureAlertText is a message's width in canvas units in a style (the
// core's Alerts.Measure).
func MeasureAlertText(text string, st models.AlertBoxStyle) float64 {
	return measureOverlayText(text, st, st.Size).W
}

func measureOverlayText(text string, st models.AlertBoxStyle, size float32) layout.TextMetrics {
	f := fontFor(st.Family, st.Bold, false)
	px := max(1, int(math.Round(float64(size))))
	sz := f.MoreArgs().GetStringSize(text, GUI.HorizontalAlignmentLeft, -1, px,
		TextServer.JustificationNone, TextServer.DirectionAuto, TextServer.OrientationHorizontal)
	return layout.TextMetrics{W: float64(sz.X), Line: float64(f.MoreArgs().GetHeight(px))}
}

// Frame returns the last frame set.
func (o *Overlay) Frame() models.OverlayFrame { return o.frame }

// Set shows a frame in a style. It is idempotent; a frame that differs only
// in a ticker's X moves the text and nothing else.
func (o *Overlay) Set(f models.OverlayFrame, st models.AlertStyles) {
	if o.has && f == o.frame && st == o.style {
		return
	}
	moved := o.has && st == o.style && onlyTickerMoved(o.frame, f)
	o.frame, o.style, o.has = f, st, true
	if moved {
		o.lay.MessageText.X = f.Message.X
		o.moveTicker()
		return
	}
	o.apply()
}

func onlyTickerMoved(a, b models.OverlayFrame) bool {
	if !a.Message.Scroll || !b.Message.Scroll {
		return false
	}
	a.Message.X = b.Message.X
	return a == b
}

// place puts the layer at the view's letterbox rect r with scale s.
func (o *Overlay) place(r layout.Rect, s float64) {
	if o.rect == r && o.s == s {
		return
	}
	o.rect, o.s = r, s
	place(o.root, r)
	if o.has {
		o.apply()
	}
}

func (o *Overlay) moveTicker() {
	t := o.lay.MessageText
	o.msg.AsControl().SetPosition(Vector2.New(t.X*o.s, (t.Y-o.lay.Band.Y)*o.s))
}

// apply measures the visible texts, lays the frame out and projects it.
func (o *Overlay) apply() {
	f, st := o.frame, o.style
	o.root.AsCanvasItem().SetVisible(f.Any() && o.s > 0)
	if !f.Any() || o.s <= 0 {
		return
	}
	skin := !o.skinned || o.skinStyle != st || o.skinS != o.s
	o.skinned, o.skinStyle, o.skinS = true, st, o.s

	// Measure what is shown, at the size it is drawn at.
	var m layout.OverlayMetrics
	if f.Message.Visible {
		m.Message = measureOverlayText(f.Message.Text, st.Message, st.Message.Size)
	}
	if f.Nursery.Visible {
		m.Nursery = measureOverlayText(f.Nursery.Text, st.Nursery, layout.BadgeSize(f.Nursery.Position, st.Nursery.Size))
	}
	if f.Countdown.Visible {
		size := layout.BadgeSize(f.Countdown.Position, st.Countdown.Size)
		m.Countdown = measureOverlayText(f.Countdown.Text, st.Countdown, size)
		if f.Countdown.Caption != "" {
			m.CountdownCaption = measureOverlayText(f.Countdown.Caption, st.Countdown, size*layout.CaptionRatio)
		}
	}
	if f.Clock.Visible {
		m.Clock = measureOverlayText(f.Clock.Text, st.Clock, st.Clock.Size)
	}
	o.met = m
	o.lay = layout.PlaceOverlay(f, st, m)

	o.applyBand(skin)
	o.applyBox(boxNursery, f.Nursery, st.Nursery, o.lay.Nursery, skin)
	o.applyBox(boxCountdown, f.Countdown, st.Countdown, o.lay.Countdown, skin)
	o.applyBox(boxClock, f.Clock, st.Clock, o.lay.Clock, skin)
}

func (o *Overlay) applyBand(skin bool) {
	f, st, s := o.frame.Message, o.style.Message, o.s
	o.band.AsCanvasItem().SetVisible(f.Visible)
	if !f.Visible {
		return
	}
	band := o.lay.Band
	place(o.band.AsControl(), band.Scale(s))
	if skin || !o.band.AsControl().HasThemeStyleboxOverride("panel") {
		sb := StyleBoxFlat.New()
		sb.SetBgColor(toColor(st.Background))
		// A hairline on the band's inner edge separates it from the slide.
		line := max(1, layout.Px(2, s))
		edge := Color.RGBA{R: 1, G: 1, B: 1, A: 0.14 * st.Background.A}
		sb.SetBorderColor(edge)
		if f.Top {
			sb.SetBorderWidthBottom(line)
		} else {
			sb.SetBorderWidthTop(line)
		}
		o.band.AsControl().AddThemeStyleboxOverride("panel", sb.AsStyleBox())
	}
	px, k := layout.RasterSize(float64(st.Size), s)
	o.msgK = k
	ls := o.msg.LabelSettings()
	ls.SetFont(fontFor(st.Family, st.Bold, false))
	ls.SetFontSize(px)
	ls.SetFontColor(toColor(st.Color))
	if o.msg.Text() != f.Text {
		o.msg.SetText(f.Text)
	}
	var r layout.Rect
	if f.Scroll {
		o.msg.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
		o.msg.SetHorizontalAlignment(GUI.HorizontalAlignmentLeft)
		t := o.lay.MessageText
		// A little slack: the rasterized text may come out a pixel wider.
		r = layout.Rect{X: t.X, Y: t.Y - band.Y, W: t.W + float64(st.Size), H: t.H}
	} else {
		// A banner's label spans the band's inner width and aligns the text,
		// trimming it with an ellipsis only if it really does not fit.
		pad := models.MessagePadX(st.Size)
		o.msg.SetTextOverrunBehavior(TextServer.OverrunTrimEllipsis)
		o.msg.SetHorizontalAlignment(hAlign(f.Align))
		r = layout.Rect{X: pad, Y: o.lay.MessageText.Y - band.Y, W: band.W - 2*pad, H: o.lay.MessageText.H}
	}
	placeScaled(o.msg.AsControl(), r.Scale(s), k)
}

func (o *Overlay) applyBox(i int, f models.BoxOverlay, st models.AlertBoxStyle, l layout.BoxLayout, skin bool) {
	b := &o.boxes[i]
	b.panel.AsCanvasItem().SetVisible(f.Visible)
	if !f.Visible {
		return
	}
	s := o.s
	size := layout.BadgeSize(f.Position, st.Size)
	place(b.panel.AsControl(), l.Box.Scale(s))
	if skin || !b.panel.AsControl().HasThemeStyleboxOverride("panel") {
		sb := StyleBoxFlat.New()
		sb.SetBgColor(toColor(st.Background))
		sb.SetCornerRadiusAll(max(0, layout.Px(float32(layout.BadgeRadius(size)), s)))
		sb.SetAntiAliasing(true)
		b.panel.AsControl().AddThemeStyleboxOverride("panel", sb.AsStyleBox())
	}
	local := func(r layout.Rect) layout.Rect {
		return layout.Rect{X: r.X - l.Box.X, Y: r.Y - l.Box.Y, W: r.W, H: r.H}
	}
	style := func(lb Label.Instance, sz float32, alpha float32) float64 {
		px, k := layout.RasterSize(float64(sz), s)
		ls := lb.LabelSettings()
		ls.SetFont(fontFor(st.Family, st.Bold, false))
		ls.SetFontSize(px)
		c := toColor(st.Color)
		c.A *= alpha
		ls.SetFontColor(c)
		return k
	}
	k := style(b.text, size, 1)
	if b.text.Text() != f.Text {
		b.text.SetText(f.Text)
	}
	placeScaled(b.text.AsControl(), local(l.Text).Scale(s), k)
	b.caption.AsCanvasItem().SetVisible(f.Caption != "")
	if f.Caption != "" {
		kc := style(b.caption, size*layout.CaptionRatio, 0.85)
		if b.caption.Text() != f.Caption {
			b.caption.SetText(f.Caption)
		}
		placeScaled(b.caption.AsControl(), local(l.Caption).Scale(s), kc)
	}
}

// ---- SlideView integration -------------------------------------------------------

// Overlay returns the view's alert layer, creating it on first use. It is
// kept above the slide layers (raiseOverlay) and follows the letterbox.
func (v *SlideView) Overlay() *Overlay {
	if v.overlay == nil {
		v.build()
		v.overlay = newOverlay(v.AsNode())
		r, s := v.area()
		v.overlay.place(r, s)
	}
	return v.overlay
}

// raiseOverlay keeps the alert layer the last child (drawn on top) after a
// layer was moved to the front.
func (v *SlideView) raiseOverlay() {
	if v.overlay != nil {
		v.AsNode().MoveChild(v.overlay.root.AsNode(), -1)
	}
}

// relayoutOverlay follows a resize.
func (v *SlideView) relayoutOverlay(r layout.Rect, s float64) {
	if v.overlay != nil {
		v.overlay.place(r, s)
	}
}

// SetOverlay shows an alert frame above the slide. A view that never showed
// an alert gets no layer until the first visible frame.
func (v *SlideView) SetOverlay(f models.OverlayFrame, st models.AlertStyles) {
	if v.overlay == nil && !f.Any() {
		return
	}
	v.Overlay().Set(f, st)
}

// OverlayShown reports whether the view's alert layer is drawn.
func (v *SlideView) OverlayShown() bool {
	return v.overlay != nil && v.overlay.root.AsCanvasItem().Visible()
}

// SetOverlay shows an alert frame on the output's view (nothing while the
// output is closed: the app projects the frame every frame, so a newly
// opened output picks it up at once).
func (o *Output) SetOverlay(f models.OverlayFrame, st models.AlertStyles) {
	if o.view != nil {
		o.view.SetOverlay(f, st)
	}
}
