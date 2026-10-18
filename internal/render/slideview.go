package render

import (
	"strings"
	"time"

	"freeworship/internal/models"
	"freeworship/internal/render/layout"

	"graphics.gd/classdb/CanvasItem"
	"graphics.gd/classdb/ColorRect"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/LabelSettings"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/TextureRect"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Vector2"
)

// DefaultTransition is the crossfade duration of a new SlideView.
const DefaultTransition = 500 * time.Millisecond

// SlideView draws one models.Frame: the slide laid out on the logical
// 1920×1080 canvas, scaled uniformly to fit the control's rect and
// letterboxed with black bars. The same view draws thumbnails, the preview and
// live monitors and the projector output, so what you preview is what goes
// live.
//
// Text is not rendered at canvas size and resampled: font sizes, margins,
// outline and shadow are scaled to the target pixel size so glyphs are
// rasterized crisply at every size. Auto-fit and line breaking are computed
// once in canvas units (and cached), so text wraps identically at every size.
//
// The view does no per-frame work when idle. Its mouse filter is Ignore, so
// clicks reach the containing widget.
type SlideView struct {
	Control.Extension[SlideView] `gd:"FwSlideView"`

	built bool
	bars  ColorRect.Instance
	cur   *layer // what is shown (on top during a transition)
	prev  *layer // outgoing state during a transition; nil until first needed

	frame    models.Frame
	hasFrame bool
	want     visual // resolved state of frame

	fade    time.Duration
	fadeSet bool

	transitioning bool
	progress      float64
	skipFrame     bool // ignore the (often long) frame that started the fade
}

// NewSlideView returns a SlideView showing black, with the default 500ms
// crossfade. Add it to the tree with AsNode().
func NewSlideView() *SlideView {
	v := new(SlideView)
	v.fade, v.fadeSet = DefaultTransition, true
	v.build()
	return v
}

// SetTransitionDuration sets the crossfade duration for subsequent changes.
// 0 means cut (use it for thumbnails).
func (v *SlideView) SetTransitionDuration(d time.Duration) {
	if d < 0 {
		d = 0
	}
	v.fade, v.fadeSet = d, true
}

func (v *SlideView) transitionDuration() time.Duration {
	if !v.fadeSet {
		return DefaultTransition
	}
	return v.fade
}

// SetFrame shows f. It is idempotent: a frame with the same non-zero Seq as the
// current one is ignored, and a frame that looks the same as what is shown
// (e.g. a Seq bump from an unrelated preview change) does not re-transition.
// Seq 0 means "unversioned" and is always compared by content.
func (v *SlideView) SetFrame(f models.Frame) {
	if v.hasFrame && f.Seq != 0 && f.Seq == v.frame.Seq {
		return
	}
	v.frame, v.hasFrame = f, true
	v.want = resolveVisual(f)
	v.build()
	v.show()
}

// Frame returns the last frame passed to SetFrame.
func (v *SlideView) Frame() models.Frame { return v.frame }

// Transitioning reports whether a crossfade is running.
func (v *SlideView) Transitioning() bool { return v.transitioning }

// TransitionProgress returns the running crossfade's progress in [0, 1]
// (1 when idle).
func (v *SlideView) TransitionProgress() float64 {
	if !v.transitioning {
		return 1
	}
	return v.progress
}

// maxFadeStep caps one frame's progress so a hitch (image decode, font
// rasterization) shortens the fade rather than skipping it.
const maxFadeStep = 1.0 / 10

// Ready implements Node.
func (v *SlideView) Ready() {
	v.build()
	v.AsNode().SetProcess(v.transitioning)
	v.relayout()
}

// ExitTree implements Node: a view leaving the tree (or being freed) never
// leaves a transition, and the low-processor refcount, dangling.
func (v *SlideView) ExitTree() {
	if v.transitioning {
		v.finishTransition()
	}
}

// Process implements Node. It only runs while a transition is in progress.
func (v *SlideView) Process(delta Float.X) {
	if !v.transitioning {
		v.AsNode().SetProcess(false)
		return
	}
	d := v.transitionDuration().Seconds()
	if d <= 0 {
		v.finishTransition()
		return
	}
	if v.skipFrame {
		// The frame that started the fade did the layout work; its delta
		// says nothing about the fade's own timing.
		v.skipFrame = false
		return
	}
	v.progress += min(float64(delta), maxFadeStep) / d
	if v.progress >= 1 {
		v.finishTransition()
		return
	}
	v.cur.setAlpha(layout.Ease(v.progress))
}

func (v *SlideView) build() {
	if v.built {
		return
	}
	v.built = true
	c := v.AsControl()
	c.SetClipContents(true)
	c.SetMouseFilter(Control.MouseFilterIgnore)
	v.bars = ColorRect.New()
	v.bars.SetColor(black)
	bc := v.bars.AsControl()
	bc.SetMouseFilter(Control.MouseFilterIgnore)
	bc.SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	v.AsNode().AddChild(v.bars.AsNode())
	v.cur = newLayer(v.AsNode())
	v.want = blackVisual
	c.OnResized(v.relayout)
}

// area returns the letterboxed canvas rect in local pixels and its scale.
func (v *SlideView) area() (layout.Rect, float64) {
	sz := v.AsControl().Size()
	r := layout.CanvasLetterbox(float64(sz.X), float64(sz.Y))
	return r, r.W / models.CanvasWidth
}

// relayout re-applies the shown layers at the current size.
func (v *SlideView) relayout() {
	if !v.built {
		return
	}
	r, s := v.area()
	v.cur.apply(v.cur.vis, r, s)
	if v.transitioning && v.prev != nil {
		v.prev.apply(v.prev.vis, r, s)
	}
}

// show projects v.want onto the layers, crossfading when appropriate.
func (v *SlideView) show() {
	if v.cur.applied && v.cur.vis == v.want {
		return
	}
	if v.transitioning {
		// A change mid-transition: jump the running fade to its end, then
		// fade from there. Never blend three states.
		v.finishTransition()
	}
	r, s := v.area()
	animate := v.cur.applied && v.transitionDuration() > 0 &&
		v.AsNode().IsInsideTree() && v.AsCanvasItem().IsVisibleInTree() && r.W > 0
	if !animate {
		v.cur.apply(v.want, r, s)
		v.cur.setAlpha(1)
		return
	}
	if v.prev == nil {
		v.prev = newLayer(v.AsNode())
	}
	v.prev, v.cur = v.cur, v.prev
	v.cur.apply(v.want, r, s)
	v.cur.setAlpha(0)
	v.cur.root.AsCanvasItem().SetVisible(true)
	v.prev.root.AsCanvasItem().SetVisible(true)
	v.AsNode().MoveChild(v.cur.root.AsNode(), -1)
	v.transitioning = true
	v.progress = 0
	v.skipFrame = true
	AcquireAnimating()
	v.AsNode().SetProcess(true)
}

func (v *SlideView) finishTransition() {
	if !v.transitioning {
		return
	}
	v.transitioning = false
	v.progress = 0
	v.cur.setAlpha(1)
	if v.prev != nil {
		v.prev.root.AsCanvasItem().SetVisible(false)
	}
	v.AsNode().SetProcess(false)
	ReleaseAnimating()
}

// ── Visual state ────────────────────────────────────────────────────────

// visual is the resolved, comparable description of what a frame looks like.
// Two frames with equal visuals render identically.
type visual struct {
	bg     models.Background
	text   string
	footer string
	theme  models.Theme // zero unless text or footer is shown
}

var blackVisual = visual{bg: models.Background{Kind: models.BackgroundColor, Color: models.RGBA{A: 1}}}

func normalizeBackground(b models.Background) models.Background {
	switch b.Kind {
	case models.BackgroundGradient:
		return models.Background{Kind: b.Kind, Color: b.Color, Color2: b.Color2}
	case models.BackgroundImage:
		return models.Background{Kind: b.Kind, Color: b.Color, Path: b.Path}
	default: // color, video (Phase 2 falls back to Color), unknown
		return models.Background{Kind: models.BackgroundColor, Color: b.Color}
	}
}

func normalizeText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSpace(s)
}

func resolveVisual(f models.Frame) visual {
	switch {
	case f.Mode == models.ModeBlack:
		return blackVisual
	case f.Mode == models.ModeLogo:
		if f.Logo.Kind == "" {
			return blackVisual
		}
		return visual{bg: normalizeBackground(f.Logo)}
	case f.Slide == nil:
		return blackVisual
	}
	bg := f.Theme.Background
	if f.Slide.Background != nil {
		bg = *f.Slide.Background
	}
	v := visual{bg: normalizeBackground(bg)}
	if f.Mode == models.ModeClear {
		return v
	}
	v.text = normalizeText(f.Slide.Text)
	v.footer = strings.TrimSpace(strings.ReplaceAll(normalizeText(f.Slide.Footer), "\n", " "))
	if v.text != "" || v.footer != "" {
		v.theme = f.Theme
		// Fields that don't affect the text's look.
		v.theme.ID, v.theme.Name, v.theme.Background = 0, "", models.Background{}
	}
	return v
}

// ── Layer: one complete visual state (background + body + footer) ─────

type layer struct {
	root   Control.Instance
	bg     ColorRect.Instance
	tex    TextureRect.Instance
	body   Label.Instance
	footer Label.Instance
	// LabelSettings are deliberately not kept here: a Go reference reachable
	// from the SlideView is kept alive by graphics.gd's keepalive every frame
	// and leaks at exit. The labels own them (Label.LabelSettings()).

	applied bool
	vis     visual
	rect    layout.Rect
}

func ignoreMouse(c Control.Instance) { c.SetMouseFilter(Control.MouseFilterIgnore) }

func newLayer(parent Node.Instance) *layer {
	l := &layer{}
	l.root = Control.New()
	l.root.SetClipContents(true)
	ignoreMouse(l.root)

	l.bg = ColorRect.New()
	l.bg.SetColor(black)
	ignoreMouse(l.bg.AsControl())
	l.bg.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)

	l.tex = TextureRect.New()
	ignoreMouse(l.tex.AsControl())
	l.tex.SetExpandMode(TextureRect.ExpandIgnoreSize)
	l.tex.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	l.tex.AsCanvasItem().SetTextureFilter(CanvasItem.TextureFilterLinearWithMipmaps)
	l.tex.AsCanvasItem().SetVisible(false)

	l.body = newTextLabel()
	l.footer = newTextLabel()
	l.footer.SetTextOverrunBehavior(TextServer.OverrunTrimEllipsis)
	l.footer.SetVerticalAlignment(GUI.VerticalAlignmentCenter)

	n := l.root.AsNode()
	n.AddChild(l.bg.AsNode())
	n.AddChild(l.tex.AsNode())
	n.AddChild(l.body.AsNode())
	n.AddChild(l.footer.AsNode())
	parent.AddChild(n)
	return l
}

func newTextLabel() Label.Instance {
	lb := Label.New()
	ignoreMouse(lb.AsControl())
	lb.SetAutowrapMode(TextServer.AutowrapOff) // line breaks are forced from the canvas-unit fit
	lb.SetLabelSettings(LabelSettings.New())
	lb.AsCanvasItem().SetVisible(false)
	return lb
}

func (l *layer) setAlpha(a float64) {
	l.root.AsCanvasItem().SetModulate(Color.RGBA{R: 1, G: 1, B: 1, A: Float.X(a)})
}

// opaque composites c over black so a translucent background color never
// shows the layer underneath.
func opaque(c models.RGBA) Color.RGBA {
	k := toColor(c)
	return Color.RGBA{R: k.R * k.A, G: k.G * k.A, B: k.B * k.A, A: 1}
}

func hAlign(a models.HAlign) GUI.HorizontalAlignment {
	switch a {
	case models.AlignLeft:
		return GUI.HorizontalAlignmentLeft
	case models.AlignRight:
		return GUI.HorizontalAlignmentRight
	default:
		return GUI.HorizontalAlignmentCenter
	}
}

func vAlign(a models.VAlign) GUI.VerticalAlignment {
	switch a {
	case models.AlignTop:
		return GUI.VerticalAlignmentTop
	case models.AlignBottom:
		return GUI.VerticalAlignmentBottom
	default:
		return GUI.VerticalAlignmentCenter
	}
}

func hGrow(a models.HAlign) Control.GrowDirection {
	switch a {
	case models.AlignLeft:
		return Control.GrowDirectionEnd
	case models.AlignRight:
		return Control.GrowDirectionBegin
	default:
		return Control.GrowDirectionBoth
	}
}

func vGrow(a models.VAlign) Control.GrowDirection {
	switch a {
	case models.AlignTop:
		return Control.GrowDirectionEnd
	case models.AlignBottom:
		return Control.GrowDirectionBegin
	default:
		return Control.GrowDirectionBoth
	}
}

func place(c Control.Instance, r layout.Rect) {
	c.SetPosition(Vector2.New(r.X, r.Y))
	c.SetSize(Vector2.New(r.W, r.H))
}

// outlinePx converts a theme outline thickness (canvas units, per side) to
// Godot's outline_size, which is the total stroke width (half of it falls
// outside the glyph).
func outlinePx(size float32, scale float64) int {
	if size <= 0 {
		return 0
	}
	return layout.Px(size*2, scale)
}

// styleText configures a label's settings for the theme at a pixel size.
func styleText(ls LabelSettings.Instance, th models.Theme, color models.RGBA, sizePx int, sizeRatio float32, lineSpacingPx float64, scale float64, bold, italic bool) {
	ls.SetFont(fontFor(th.Font.Family, bold, italic))
	ls.SetFontSize(sizePx)
	ls.SetFontColor(toColor(color))
	ls.SetLineSpacing(Float.X(lineSpacingPx))
	ls.SetParagraphSpacing(0)
	if th.Outline.Enabled && th.Outline.Size > 0 {
		ls.SetOutlineSize(outlinePx(th.Outline.Size*sizeRatio, scale))
		ls.SetOutlineColor(toColor(th.Outline.Color))
	} else {
		ls.SetOutlineSize(0)
	}
	if th.Shadow.Enabled {
		ls.SetShadowColor(toColor(th.Shadow.Color))
		ls.SetShadowOffset(Vector2.New(
			float64(th.Shadow.OffsetX*sizeRatio)*scale,
			float64(th.Shadow.OffsetY*sizeRatio)*scale))
		// Godot has no text-shadow blur; a shadow outline thickens/softens it.
		ls.SetShadowSize(layout.Px(th.Shadow.Blur*sizeRatio, scale))
	} else {
		ls.SetShadowColor(Color.RGBA{})
		ls.SetShadowSize(0)
		ls.SetShadowOffset(Vector2.New(0, 0))
	}
}

// apply projects vis onto the layer at the letterbox rect r (local pixels)
// with canvas→pixel scale s. It is a no-op when nothing changed.
func (l *layer) apply(vis visual, r layout.Rect, s float64) {
	if l.applied && l.vis == vis && l.rect == r {
		return
	}
	l.applied, l.vis, l.rect = true, vis, r
	place(l.root, r)

	// Background.
	l.bg.SetColor(opaque(vis.bg.Color))
	switch vis.bg.Kind {
	case models.BackgroundGradient:
		l.tex.SetTexture(gradientTexture(vis.bg.Color, vis.bg.Color2))
		l.tex.SetStretchMode(TextureRect.StretchScale)
		l.tex.AsCanvasItem().SetVisible(true)
	case models.BackgroundImage:
		if t, ok := imageTexture(vis.bg.Path); ok {
			l.tex.SetTexture(t)
			l.tex.SetStretchMode(TextureRect.StretchKeepAspectCovered)
			l.tex.AsCanvasItem().SetVisible(true)
		} else {
			l.tex.AsCanvasItem().SetVisible(false)
		}
	default:
		l.tex.AsCanvasItem().SetVisible(false)
	}

	th := vis.theme
	if (vis.text == "" && vis.footer == "") || s <= 0 {
		l.body.AsCanvasItem().SetVisible(false)
		l.footer.AsCanvasItem().SetVisible(false)
		return
	}

	// Footer (one line, bottom of the text box, never overlapping the body).
	footerLH := 0.0
	footerSize := th.FooterSize
	if footerSize <= 0 {
		footerSize = 28
	}
	if vis.footer != "" {
		footerLH = lineHeight(fontFor(th.Font.Family, false, th.Font.Italic), footerSize)
	}
	boxes := layout.TextBoxes(th.Margins, footerLH)

	if vis.footer != "" {
		px, k := layout.RasterSize(float64(footerSize), s)
		ratio := footerSize / max(th.Font.Size, 1)
		styleText(l.footer.LabelSettings(), th, th.FooterColor, px, ratio, 0, s/k, false, th.Font.Italic)
		l.footer.SetText(vis.footer)
		l.footer.SetHorizontalAlignment(hAlign(th.HAlign))
		placeScaled(l.footer.AsControl(), boxes.Footer.Scale(s), k)
		l.footer.AsCanvasItem().SetVisible(true)
	} else {
		l.footer.AsCanvasItem().SetVisible(false)
	}

	if vis.text != "" {
		fit := fitText(vis.text, th.Font, boxes.Body.W, boxes.Body.H)
		font := fontFor(th.Font.Family, th.Font.Bold, th.Font.Italic)
		px, k := layout.RasterSize(float64(fit.size), s)
		ratio := float32(fit.size) / max(th.Font.Size, 1)
		// Make each line advance exactly the canvas layout's advance scaled
		// to this view, whatever the rasterized font's own (rounded) line
		// height is, so text fills the box identically at every size.
		advance := (lineHeight(font, float32(fit.size)) + float64(th.Font.LineSpacing)*float64(fit.size)) * s / k
		spacing := advance - lineHeight(font, float32(px))
		styleText(l.body.LabelSettings(), th, th.Font.Color, px, ratio, spacing, s/k, th.Font.Bold, th.Font.Italic)
		l.body.SetText(fit.lines)
		l.body.SetHorizontalAlignment(hAlign(th.HAlign))
		l.body.SetVerticalAlignment(vAlign(th.VAlign))
		bc := l.body.AsControl()
		bc.SetGrowHorizontal(hGrow(th.HAlign))
		bc.SetGrowVertical(vGrow(th.VAlign))
		placeScaled(bc, boxes.Body.Scale(s), k)
		l.body.AsCanvasItem().SetVisible(true)
	} else {
		l.body.AsCanvasItem().SetVisible(false)
	}
}

// placeScaled positions c at r (parent pixels) with node scale k: its own
// size is r/k so the scaled result covers r exactly.
func placeScaled(c Control.Instance, r layout.Rect, k float64) {
	c.SetScale(Vector2.New(k, k))
	c.SetPosition(Vector2.New(r.X, r.Y))
	c.SetSize(Vector2.New(r.W/k, r.H/k))
}
