package layout

import (
	"math"

	"freebeams/internal/models"
)

// Stage display geometry (render.StageView). Everything is in canvas units:
// the logical 1920×1080 canvas, extended along one side to the screen's own
// aspect ratio (StageCanvas), so a 4:3 or 16:10 confidence monitor is filled
// edge to edge rather than letterboxed. Text sizes are auto-fitted into the
// boxes by the same fitText the slides use.
const (
	StagePad            = 40.0 // margin around everything
	StageGap            = 28.0 // between areas
	StageSlotGap        = 44.0 // between the header's right-hand slots
	StageRuleH          = 3.0  // the lines under the header and above "Next"
	StageHeaderSize     = 54.0 // title, label, badge, timer and clock
	StageNextHeaderSize = 40.0 // "NEXT · Chorus"
	StageBadgePadX      = 22.0 // the badge box around its text
	StageCurrentMax     = 150  // the live text's largest size
	StageCurrentMin     = 20
	StageNextMax        = 84 // the next slide's largest size
	StageNextMin        = 18
	// StageCurrentShare is the current text's share of the body when the
	// next slide is shown below it.
	StageCurrentShare = 0.62
)

// The stage message band (StageInput.Note) along the bottom.
const (
	StageNoteH    = 132.0 // the band's height (grows with the header scale)
	StageNotePadX = 28.0  // the text's inset from the band's sides
	StageNoteMax  = 64    // the message's largest size
	StageNoteMin  = 24
)

// StageCanvas returns the stage canvas for a w×h pixel area and the
// canvas→pixel scale s: the canvas is at least 1920×1080 and has the area's
// aspect ratio (cw*s == w, ch*s == h). Degenerate areas give zeros.
func StageCanvas(w, h float64) (cw, ch, s float64) {
	if w <= 0 || h <= 0 {
		return 0, 0, 0
	}
	s = math.Min(w/models.CanvasWidth, h/models.CanvasHeight)
	return w / s, h / s, s
}

// StageInput is what StageLayout needs: the canvas, the font scale, whether
// the next slide is shown, and the header's measured text widths in canvas
// units at HeaderSize (0 hides that slot).
type StageInput struct {
	W, H      float64
	FontScale float64
	Next      bool
	ClockW    float64
	TimerW    float64
	BadgeW    float64 // the badge text; its box adds StageBadgePadX on each side

	// Note reserves the stage message band at the bottom.
	Note bool
}

// StageBoxes is the stage layout. Hidden parts have zero rects.
type StageBoxes struct {
	HeaderSize float64 // font size of the header row (canvas units)
	Title      Rect    // item title and slide label, left of the slots
	Badge      Rect    // the BLACK / CLEAR / LOGO box
	Timer      Rect
	Clock      Rect
	Rule       Rect // under the header

	Current    Rect // the live text
	CurrentMax int  // its auto-fit range (canvas units)
	CurrentMin int

	NextRule       Rect
	NextHeader     Rect // "NEXT · Chorus" / "NEXT ITEM · How Great Thou Art"
	NextHeaderSize float64
	Next           Rect
	NextMax        int
	NextMin        int

	NoteBand Rect // the stage message's background band
	Note     Rect // its text, inset in the band
	NoteMax  int
	NoteMin  int
}

// StageTextScale clamps a font scale for the text areas (0.5 to 2; anything
// else, NaN included, is 1).
func StageTextScale(s float64) float64 {
	if !(s > 0) || math.IsInf(s, 0) {
		return 1
	}
	return math.Min(2, math.Max(0.5, s))
}

// stageHeaderScale is the header's share of the font scale: the header row
// grows less than the text, so the clock and the title still fit side by side.
func stageHeaderScale(s float64) float64 {
	return math.Min(1.5, StageTextScale(s))
}

// StageHeaderFontSize is the header font size for a font scale.
func StageHeaderFontSize(fontScale float64) float64 {
	return math.Round(StageHeaderSize * stageHeaderScale(fontScale))
}

// StageLayout lays the stage display out on a cw×ch canvas: a header row
// (title on the left; badge, timer and clock right-aligned), a rule, the live
// text, optionally a rule, the "Next" header and the next slide's text, and
// optionally the stage message band along the bottom.
func StageLayout(in StageInput) StageBoxes {
	var b StageBoxes
	if in.W <= 0 || in.H <= 0 {
		return b
	}
	ts := StageTextScale(in.FontScale)
	hs := stageHeaderScale(in.FontScale)
	b.HeaderSize = StageHeaderFontSize(in.FontScale)
	headerH := math.Round(b.HeaderSize * 1.4)
	inner := math.Max(0, in.W-2*StagePad)

	// Header: slots from the right edge leftwards.
	right := in.W - StagePad // the title ends here
	x := right               // the next slot ends here
	slot := func(w float64) Rect {
		if w <= 0 {
			return Rect{}
		}
		w = math.Min(w, x-StagePad)
		if w <= 0 {
			return Rect{} // no room left in the row
		}
		r := Rect{X: x - w, Y: StagePad, W: w, H: headerH}
		x = r.X - StageSlotGap*hs
		right = r.X - StageGap
		return r
	}
	b.Clock = slot(in.ClockW)
	b.Timer = slot(in.TimerW)
	if in.BadgeW > 0 {
		b.Badge = slot(in.BadgeW + 2*StageBadgePadX*hs)
	}
	b.Title = Rect{X: StagePad, Y: StagePad, W: math.Max(0, right-StagePad), H: headerH}
	b.Rule = Rect{X: StagePad, Y: StagePad + headerH + 10, W: inner, H: StageRuleH}

	top := b.Rule.Y + b.Rule.H + StageGap
	bottom := in.H - StagePad
	if in.Note {
		h := math.Round(StageNoteH * hs)
		b.NoteBand = Rect{X: StagePad, Y: bottom - h, W: inner, H: h}
		padX := math.Round(StageNotePadX * hs)
		b.Note = Rect{X: StagePad + padX, Y: b.NoteBand.Y, W: math.Max(0, inner-2*padX), H: h}
		b.NoteMax = int(math.Round(StageNoteMax * hs))
		b.NoteMin = int(math.Round(StageNoteMin * hs))
		bottom = b.NoteBand.Y - StageGap
	}
	avail := math.Max(0, bottom-top)
	b.CurrentMax = int(math.Round(StageCurrentMax * ts))
	b.CurrentMin = int(math.Round(StageCurrentMin * ts))
	if !in.Next {
		b.Current = Rect{X: StagePad, Y: top, W: inner, H: avail}
		return b
	}
	b.NextHeaderSize = math.Round(StageNextHeaderSize * hs)
	nextHeaderH := math.Round(b.NextHeaderSize * 1.35)
	fixed := 2*StageGap + StageRuleH + nextHeaderH
	body := math.Max(0, avail-fixed)
	curH := math.Round(body * StageCurrentShare)
	b.Current = Rect{X: StagePad, Y: top, W: inner, H: curH}
	y := top + curH + StageGap
	b.NextRule = Rect{X: StagePad, Y: y, W: inner, H: StageRuleH}
	y += StageRuleH + StageGap/2
	b.NextHeader = Rect{X: StagePad, Y: y, W: inner, H: nextHeaderH}
	y += nextHeaderH + StageGap/2
	b.Next = Rect{X: StagePad, Y: y, W: inner, H: math.Max(0, bottom-y)}
	b.NextMax = int(math.Round(StageNextMax * ts))
	b.NextMin = int(math.Round(StageNextMin * ts))
	return b
}
