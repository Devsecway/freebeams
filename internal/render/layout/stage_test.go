package layout

import (
	"math"
	"testing"
)

func TestStageCanvas(t *testing.T) {
	tests := []struct {
		name          string
		w, h          float64
		cw, ch, scale float64
	}{
		{"1080p", 1920, 1080, 1920, 1080, 1},
		{"4K", 3840, 2160, 1920, 1080, 2},
		{"small window", 960, 540, 1920, 1080, 0.5},
		{"4:3 extends the height", 1024, 768, 1920, 1440, 1024.0 / 1920},
		{"16:10", 1920, 1200, 1920, 1200, 1},
		{"ultrawide extends the width", 2560, 1080, 2560, 1080, 1},
		{"zero", 0, 1080, 0, 0, 0},
		{"negative", 100, -1, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cw, ch, s := StageCanvas(tt.w, tt.h)
			if !near(cw, tt.cw) || !near(ch, tt.ch) || !near(s, tt.scale) {
				t.Fatalf("StageCanvas(%v, %v) = %v, %v, %v; want %v, %v, %v", tt.w, tt.h, cw, ch, s, tt.cw, tt.ch, tt.scale)
			}
		})
	}
}

func TestStageTextScale(t *testing.T) {
	for in, want := range map[float64]float64{1: 1, 0: 1, -2: 1, 0.25: 0.5, 1.4: 1.4, 3: 2, math.NaN(): 1, math.Inf(1): 1} {
		if got := StageTextScale(in); got != want {
			t.Errorf("StageTextScale(%v) = %v, want %v", in, got, want)
		}
	}
}

// inside reports r lies within the canvas (zero rects count as inside).
func inside(r Rect, w, h float64) bool {
	if r == (Rect{}) {
		return true
	}
	return r.X >= 0 && r.Y >= 0 && r.W >= 0 && r.H >= 0 && r.X+r.W <= w+1e-9 && r.Y+r.H <= h+1e-9
}

func overlaps(a, b Rect) bool {
	if a.W <= 0 || a.H <= 0 || b.W <= 0 || b.H <= 0 {
		return false
	}
	return a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H
}

func TestStageLayout(t *testing.T) {
	tests := []struct {
		name string
		in   StageInput
	}{
		{"everything", StageInput{W: 1920, H: 1080, FontScale: 1, Next: true, ClockW: 240, TimerW: 200, BadgeW: 180}},
		{"no next", StageInput{W: 1920, H: 1080, FontScale: 1, ClockW: 240}},
		{"nothing in the header", StageInput{W: 1920, H: 1080, FontScale: 1, Next: true}},
		{"big font", StageInput{W: 1920, H: 1080, FontScale: 2, Next: true, ClockW: 400, TimerW: 380, BadgeW: 300}},
		{"4:3", StageInput{W: 1920, H: 1440, FontScale: 1, Next: true, ClockW: 240, TimerW: 200}},
		{"header wider than the canvas", StageInput{W: 1920, H: 1080, FontScale: 1, ClockW: 1500, TimerW: 900, BadgeW: 600}},
		{"note", StageInput{W: 1920, H: 1080, FontScale: 1, Next: true, ClockW: 240, TimerW: 200, Note: true}},
		{"note without next", StageInput{W: 1920, H: 1080, FontScale: 1, Note: true}},
		{"note, big font, 4:3", StageInput{W: 1920, H: 1440, FontScale: 2, Next: true, ClockW: 400, Note: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := StageLayout(tt.in)
			all := map[string]Rect{"title": b.Title, "badge": b.Badge, "timer": b.Timer, "clock": b.Clock, "rule": b.Rule,
				"current": b.Current, "nextRule": b.NextRule, "nextHeader": b.NextHeader, "next": b.Next,
				"noteBand": b.NoteBand, "note": b.Note}
			for name, r := range all {
				if !inside(r, tt.in.W, tt.in.H) {
					t.Errorf("%s = %+v is outside the %vx%v canvas", name, r, tt.in.W, tt.in.H)
				}
			}
			names := []string{"title", "badge", "timer", "clock", "rule", "current", "nextRule", "nextHeader", "next", "noteBand"}
			for i, a := range names {
				for _, c := range names[i+1:] {
					if overlaps(all[a], all[c]) {
						t.Errorf("%s %+v overlaps %s %+v", a, all[a], c, all[c])
					}
				}
			}
			// Hidden slots are zero (and so are slots with no room left);
			// shown ones are right-aligned in order clock, timer, badge.
			if (tt.in.ClockW == 0 && b.Clock.W > 0) || (tt.in.TimerW == 0 && b.Timer.W > 0) || (tt.in.BadgeW == 0 && b.Badge.W > 0) {
				t.Errorf("slots shown = clock %v timer %v badge %v", b.Clock, b.Timer, b.Badge)
			}
			if b.Clock.W > 0 && !near(b.Clock.X+b.Clock.W, tt.in.W-StagePad) {
				t.Errorf("clock not at the right edge: %+v", b.Clock)
			}
			if b.Timer.W > 0 && b.Clock.W > 0 && b.Timer.X >= b.Clock.X {
				t.Errorf("timer %+v is not left of the clock %+v", b.Timer, b.Clock)
			}
			if tt.in.Next != (b.Next.H > 0) {
				t.Errorf("next shown = %v, want %v (%+v)", b.Next.H > 0, tt.in.Next, b.Next)
			}
			if tt.in.Next && !(b.Current.H > b.Next.H) {
				t.Errorf("the current text (%v high) should be bigger than the next (%v)", b.Current.H, b.Next.H)
			}
			if tt.in.Note != (b.NoteBand.H > 0 && b.Note.W > 0) || (tt.in.Note && b.NoteMin >= b.NoteMax) {
				t.Errorf("note shown = %+v / %+v (%d..%d), want %v", b.NoteBand, b.Note, b.NoteMin, b.NoteMax, tt.in.Note)
			}
			if tt.in.Note && (!near(b.NoteBand.Y+b.NoteBand.H, tt.in.H-StagePad) || b.Note.X <= b.NoteBand.X ||
				b.Note.X+b.Note.W >= b.NoteBand.X+b.NoteBand.W || b.Note.H != b.NoteBand.H) {
				t.Errorf("note band %+v / text %+v: not at the bottom margin, or the text is not inset", b.NoteBand, b.Note)
			}
			if !tt.in.Next && !tt.in.Note && !near(b.Current.Y+b.Current.H, tt.in.H-StagePad) {
				t.Errorf("without next the current text reaches the bottom margin: %+v", b.Current)
			}
			if b.CurrentMin >= b.CurrentMax || b.CurrentMin < 1 {
				t.Errorf("current fit range %d..%d", b.CurrentMin, b.CurrentMax)
			}
		})
	}

	// The title gives way to the header slots, and a bigger font scale
	// raises the fit ceilings.
	plain := StageLayout(StageInput{W: 1920, H: 1080, FontScale: 1})
	busy := StageLayout(StageInput{W: 1920, H: 1080, FontScale: 1, ClockW: 240, TimerW: 200, BadgeW: 180})
	if !(busy.Title.W < plain.Title.W) || !near(plain.Title.W, 1920-2*StagePad) {
		t.Errorf("title widths: plain %v, busy %v", plain.Title.W, busy.Title.W)
	}
	big := StageLayout(StageInput{W: 1920, H: 1080, FontScale: 1.5, Next: true})
	normal := StageLayout(StageInput{W: 1920, H: 1080, FontScale: 1, Next: true})
	if big.CurrentMax != 225 || normal.CurrentMax != StageCurrentMax || big.NextMax <= normal.NextMax || big.HeaderSize <= normal.HeaderSize {
		t.Errorf("font scale: big %+v, normal %+v", big, normal)
	}
	if z := StageLayout(StageInput{}); z != (StageBoxes{}) {
		t.Errorf("empty canvas = %+v", z)
	}
}
