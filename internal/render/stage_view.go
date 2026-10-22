package render

import (
	"math"
	"strings"

	"freebeams/internal/models"
	"freebeams/internal/render/layout"

	"graphics.gd/classdb/ColorRect"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/Font"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/LabelSettings"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Vector2"
)

// Stage display colors: white on black for the live text, amber for what is
// next (distinct at a glance from across a stage), dimmer greys for the
// header, and a solid badge color per output mode.
var (
	stageBg          = Color.RGBA{R: 0, G: 0, B: 0, A: 1}
	stageCurrent     = Color.RGBA{R: 1, G: 1, B: 1, A: 1}
	stageNext        = Color.RGBA{R: 1, G: 0.82, B: 0.36, A: 1}    // #ffd15c
	stageNextHeader  = Color.RGBA{R: 0.62, G: 0.62, B: 0.62, A: 1} // #9e9e9e
	stageTitle       = Color.RGBA{R: 0.80, G: 0.80, B: 0.80, A: 1} // #cccccc
	stageRule        = Color.RGBA{R: 0.27, G: 0.27, B: 0.27, A: 1} // #454545
	stageClock       = Color.RGBA{R: 1, G: 1, B: 1, A: 1}
	stageTimerOn     = Color.RGBA{R: 0.42, G: 0.88, B: 0.48, A: 1} // #6be07a
	stageTimerOff    = Color.RGBA{R: 0.60, G: 0.60, B: 0.60, A: 1}
	stageBadgeText   = Color.RGBA{R: 1, G: 1, B: 1, A: 1}
	stageBadgeColors = map[string]Color.RGBA{
		models.StageBadgeBlack: {R: 0.83, G: 0.18, B: 0.18, A: 1}, // #d32f2f
		models.StageBadgeClear: {R: 0.90, G: 0.49, B: 0.13, A: 1}, // #e67e22
		models.StageBadgeLogo:  {R: 0.18, G: 0.44, B: 0.85, A: 1}, // #2e6fd8
	}
)

// StageView draws a models.StageFrame: the stage display's text-only view
// (current slide large, next slide below, item title, slide label, clock,
// service timer and the output-mode badge), white on black. It lays out on a
// stage canvas that keeps the slide canvas's units but takes the control's
// aspect ratio (layout.StageCanvas), and auto-fits the slide text with the
// same fitText (canvas units, cached) and pixel-crisp rasterization the
// SlideView uses. The same view is the stage window's content and the Live
// Output monitor's Foldback preview. It does no per-frame work.
type StageView struct {
	Control.Extension[StageView] `gd:"FwStageView"`

	built bool
	bg    ColorRect.Instance
	rule  ColorRect.Instance
	rule2 ColorRect.Instance
	box   ColorRect.Instance // the badge's background

	title   Label.Instance
	badge   Label.Instance
	timer   Label.Instance
	clock   Label.Instance
	current Label.Instance
	head    Label.Instance // "NEXT · Chorus"
	next    Label.Instance

	frame    models.StageFrame
	hasFrame bool
	applied  models.StageFrame
	size     Vector2.XY
	laidOut  bool
}

// NewStageView returns a black StageView. Add it to the tree with AsNode().
func NewStageView() *StageView {
	v := new(StageView)
	v.build()
	return v
}

// Ready implements Node.
func (v *StageView) Ready() {
	v.build()
	v.relayout()
}

// Frame returns the frame last passed to SetFrame.
func (v *StageView) Frame() models.StageFrame { return v.frame }

// SetFrame shows f. It is idempotent: an equal frame does nothing.
func (v *StageView) SetFrame(f models.StageFrame) {
	if v.hasFrame && f == v.frame {
		return
	}
	v.frame, v.hasFrame = f, true
	v.build()
	v.relayout()
}

func (v *StageView) build() {
	if v.built {
		return
	}
	v.built = true
	c := v.AsControl()
	c.SetClipContents(true)
	c.SetMouseFilter(Control.MouseFilterIgnore)
	rect := func(col Color.RGBA) ColorRect.Instance {
		r := ColorRect.New()
		r.SetColor(col)
		ignoreMouse(r.AsControl())
		v.AsNode().AddChild(r.AsNode())
		return r
	}
	v.bg = rect(stageBg)
	v.bg.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	v.rule = rect(stageRule)
	v.rule2 = rect(stageRule)
	v.box = rect(stageBg)
	label := func() Label.Instance {
		lb := newTextLabel()
		lb.SetTextOverrunBehavior(TextServer.OverrunTrimEllipsis)
		lb.SetVerticalAlignment(GUI.VerticalAlignmentCenter)
		v.AsNode().AddChild(lb.AsNode())
		return lb
	}
	v.title = label()
	v.badge = label()
	v.badge.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
	v.timer = label()
	v.timer.SetHorizontalAlignment(GUI.HorizontalAlignmentRight)
	v.clock = label()
	v.clock.SetHorizontalAlignment(GUI.HorizontalAlignmentRight)
	for _, l := range []Label.Instance{v.badge, v.timer, v.clock} {
		// Measured slots: never trim the clock to "9:39 A…" over a rounding
		// difference between the measurement and the rasterized size.
		l.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	}
	v.head = label()
	v.current = newTextLabel() // line breaks forced from the canvas-unit fit
	v.current.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
	v.current.SetVerticalAlignment(GUI.VerticalAlignmentCenter)
	v.AsNode().AddChild(v.current.AsNode())
	v.next = newTextLabel()
	v.next.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
	v.next.SetVerticalAlignment(GUI.VerticalAlignmentTop)
	v.AsNode().AddChild(v.next.AsNode())
	c.OnResized(v.relayout)
}

// stageFont is the stage display's typeface (the slides' default family).
func stageFont(bold bool) Font.Instance { return fontFor("", bold, false) }

// textWidth measures s in canvas units at size (canvas units).
func textWidth(f Font.Instance, s string, size float64) float64 {
	if s == "" {
		return 0
	}
	sz := f.MoreArgs().GetStringSize(s, GUI.HorizontalAlignmentLeft, -1, int(math.Round(size)), TextServer.JustificationFlag(3), 0, 0)
	return math.Ceil(float64(sz.X) + size*0.08) // slack for the rasterized size's rounding
}

// relayout projects the frame at the current size. It is a no-op when
// neither changed.
func (v *StageView) relayout() {
	if !v.built {
		return
	}
	size := v.AsControl().Size()
	if v.laidOut && size == v.size && v.applied == v.frame {
		return
	}
	v.laidOut, v.size, v.applied = true, size, v.frame
	f := v.frame
	cw, ch, s := layout.StageCanvas(float64(size.X), float64(size.Y))
	if s <= 0 {
		for _, l := range v.labels() {
			l.AsCanvasItem().SetVisible(false)
		}
		return
	}
	headerSize := layout.StageHeaderFontSize(f.FontScale)
	bold := stageFont(true)
	b := layout.StageLayout(layout.StageInput{
		W: cw, H: ch, FontScale: f.FontScale, Next: f.ShowNext,
		ClockW: textWidth(bold, f.Clock, headerSize),
		TimerW: textWidth(bold, f.Timer, headerSize),
		BadgeW: textWidth(bold, f.Badge, headerSize),
	})

	// Header.
	v.line(v.title, f.HeaderTitle(), b.Title, b.HeaderSize, s, false, stageTitle)
	v.line(v.clock, f.Clock, b.Clock, b.HeaderSize, s, true, stageClock)
	timerColor := stageTimerOff
	if f.TimerRunning {
		timerColor = stageTimerOn
	}
	v.line(v.timer, f.Timer, b.Timer, b.HeaderSize, s, true, timerColor)
	v.line(v.badge, f.Badge, b.Badge, b.HeaderSize, s, true, stageBadgeText)
	if col, ok := stageBadgeColors[f.Badge]; ok && b.Badge.W > 0 {
		v.box.SetColor(col)
		place(v.box.AsControl(), b.Badge.Scale(s))
		v.box.AsCanvasItem().SetVisible(true)
	} else {
		v.box.AsCanvasItem().SetVisible(false)
	}
	place(v.rule.AsControl(), pixelRule(b.Rule.Scale(s)))

	// The live text and the next slide.
	v.fitted(v.current, f.Current, b.Current, b.CurrentMin, b.CurrentMax, s, true, stageCurrent)
	if f.ShowNext {
		place(v.rule2.AsControl(), pixelRule(b.NextRule.Scale(s)))
		v.rule2.AsCanvasItem().SetVisible(true)
		headColor := stageNextHeader
		if f.NextTitle != "" {
			headColor = stageNext // a new item is coming: the musicians change songs
		}
		v.line(v.head, f.NextHeader(), b.NextHeader, b.NextHeaderSize, s, f.NextTitle != "", headColor)
		v.fitted(v.next, f.Next, b.Next, b.NextMin, b.NextMax, s, false, stageNext)
	} else {
		v.rule2.AsCanvasItem().SetVisible(false)
		v.head.AsCanvasItem().SetVisible(false)
		v.next.AsCanvasItem().SetVisible(false)
	}
}

func (v *StageView) labels() []Label.Instance {
	return []Label.Instance{v.title, v.badge, v.timer, v.clock, v.current, v.head, v.next}
}

// pixelRule keeps a rule at least one pixel high at any scale.
func pixelRule(r layout.Rect) layout.Rect {
	r.H = math.Max(1, math.Round(r.H))
	r.Y = math.Round(r.Y)
	return r
}

// line shows one line of text in r (canvas units) at size, trimmed with an
// ellipsis when it does not fit; empty text or an empty rect hides it.
func (v *StageView) line(l Label.Instance, text string, r layout.Rect, size, s float64, bold bool, col Color.RGBA) {
	if text == "" || r.W <= 0 || r.H <= 0 {
		l.AsCanvasItem().SetVisible(false)
		return
	}
	px, k := layout.RasterSize(size, s)
	stageStyle(l.LabelSettings(), stageFont(bold), px, 0, col)
	l.SetText(text)
	placeScaled(l.AsControl(), r.Scale(s), k)
	l.AsCanvasItem().SetVisible(true)
}

// fitted auto-fits text into r (canvas units) between min and max, with the
// line breaks the fit chose, exactly as a slide's body is fitted.
func (v *StageView) fitted(l Label.Instance, text string, r layout.Rect, min, max int, s float64, bold bool, col Color.RGBA) {
	if text == "" || r.W <= 0 || r.H <= 0 {
		l.AsCanvasItem().SetVisible(false)
		return
	}
	const lineSpacing = 0.08
	f := models.Font{Size: float32(max), MinSize: float32(min), Bold: bold, LineSpacing: lineSpacing}
	fit := fitStageText(text, f, r.W, r.H)
	font := stageFont(bold)
	px, k := layout.RasterSize(float64(fit.size), s)
	advance := (lineHeight(font, float32(fit.size)) + lineSpacing*float64(fit.size)) * s / k
	stageStyle(l.LabelSettings(), font, px, advance-lineHeight(font, float32(px)), col)
	l.SetText(fit.lines)
	placeScaled(l.AsControl(), r.Scale(s), k)
	l.AsCanvasItem().SetVisible(true)
}

// stageStyle sets a label's font, size, color and line spacing; the stage
// has no outline or shadow (black background).
func stageStyle(ls LabelSettings.Instance, f Font.Instance, px int, lineSpacing float64, col Color.RGBA) {
	ls.SetFont(f)
	ls.SetFontSize(px)
	ls.SetFontColor(col)
	ls.SetLineSpacing(Float.X(lineSpacing))
	ls.SetParagraphSpacing(0)
	ls.SetOutlineSize(0)
	ls.SetShadowColor(Color.RGBA{})
	ls.SetShadowSize(0)
	ls.SetShadowOffset(Vector2.New(0, 0))
}

// stageFitCache memoizes fitStageText (canvas units; cleared when full).
var stageFitCache = map[fitKey]fitResult{}

// fitStageText is fitText for the stage display: it prefers the largest size
// at which every line of the slide stays on one line (a lyric line broken in
// two is hard to sing from), and falls back to the plain fit (wrapping) when
// even the smallest size cannot keep the lines whole.
func fitStageText(text string, f models.Font, boxW, boxH float64) fitResult {
	maxSize, minSize := int(math.Round(float64(f.Size))), int(math.Round(float64(f.MinSize)))
	k := fitKey{text: text, font: fontKey{f.Family, f.Bold, f.Italic}, maxSize: maxSize, minSize: minSize,
		lineSpacing: f.LineSpacing, boxW: int(math.Round(boxW)), boxH: int(math.Round(boxH))}
	if r, ok := stageFitCache[k]; ok {
		return r
	}
	font := fontFor(f.Family, f.Bold, f.Italic)
	lines := strings.Count(text, "\n") + 1
	whole := func(s int) bool {
		p := shape(text, font, s, f.LineSpacing, boxW)
		return float64(p.GetSize().Y) <= boxH && p.GetLineCount() <= lines
	}
	size := layout.FitFontSize(minSize, maxSize, whole)
	var res fitResult
	if whole(size) {
		res = fitResult{size: size, lines: text}
	} else {
		res = fitText(text, f, boxW, boxH)
	}
	if len(stageFitCache) >= fitCacheMax {
		clear(stageFitCache)
	}
	stageFitCache[k] = res
	return res
}
