package layout

import (
	"testing"

	"freebeams/internal/models"
)

func ovNear(a, b float64) bool { return a-b < 1e-6 && b-a < 1e-6 }

func ovNearRect(a, b Rect) bool {
	return ovNear(a.X, b.X) && ovNear(a.Y, b.Y) && ovNear(a.W, b.W) && ovNear(a.H, b.H)
}

func TestPlaceOverlayBand(t *testing.T) {
	st := models.DefaultAlertStyles() // message 54: padY 17.28, padX 43.2
	m := OverlayMetrics{Message: TextMetrics{W: 600, Line: 64}}
	tests := []struct {
		name string
		msg  models.MessageOverlay
		band Rect
		text Rect
	}{
		{"bottom ticker", models.MessageOverlay{Visible: true, Scroll: true, X: 300},
			Rect{0, 1080 - 98.56, 1920, 98.56}, Rect{300, 1080 - 98.56 + 17.28, 600, 64}},
		{"top banner centred", models.MessageOverlay{Visible: true, Top: true, Align: models.AlignCenter},
			Rect{0, 0, 1920, 98.56}, Rect{660, 17.28, 600, 64}},
		{"left banner", models.MessageOverlay{Visible: true, Align: models.AlignLeft},
			Rect{0, 1080 - 98.56, 1920, 98.56}, Rect{43.2, 1080 - 98.56 + 17.28, 600, 64}},
		{"right banner", models.MessageOverlay{Visible: true, Align: models.AlignRight},
			Rect{0, 1080 - 98.56, 1920, 98.56}, Rect{1920 - 43.2 - 600, 1080 - 98.56 + 17.28, 600, 64}},
	}
	for _, tt := range tests {
		l := PlaceOverlay(models.OverlayFrame{Message: tt.msg}, st, m)
		if !ovNearRect(l.Band, tt.band) || !ovNearRect(l.MessageText, tt.text) {
			t.Errorf("%s: band %+v text %+v; want %+v %+v", tt.name, l.Band, l.MessageText, tt.band, tt.text)
		}
	}
	// A banner wider than the band is clipped to it.
	wide := PlaceOverlay(models.OverlayFrame{Message: models.MessageOverlay{Visible: true}}, st, OverlayMetrics{Message: TextMetrics{W: 5000, Line: 64}})
	if !ovNear(wide.MessageText.X, 43.2) || !ovNear(wide.MessageText.W, 1920-2*43.2) {
		t.Errorf("wide banner %+v", wide.MessageText)
	}
}

func TestPlaceOverlayBadges(t *testing.T) {
	st := models.DefaultAlertStyles()
	st.NurseryPosition = models.PosBottomRight
	st.ClockPosition = models.PosBottomRight
	st.CountdownPosition = models.PosTopLeft
	m := OverlayMetrics{
		Message:          TextMetrics{W: 600, Line: 64},
		Nursery:          TextMetrics{W: 200, Line: 80}, // size 64: pad 28.8 x 11.52
		Countdown:        TextMetrics{W: 150, Line: 90}, // size 72: pad 32.4 x 12.96
		CountdownCaption: TextMetrics{W: 300, Line: 40},
		Clock:            TextMetrics{W: 120, Line: 56}, // size 44: pad 19.8 x 7.92
	}
	f := models.OverlayFrame{
		Message:   models.MessageOverlay{Visible: true}, // bottom band 98.56 high
		Nursery:   models.BoxOverlay{Visible: true, Position: models.PosBottomRight},
		Countdown: models.BoxOverlay{Visible: true, Caption: "Starts in", Position: models.PosTopLeft},
		Clock:     models.BoxOverlay{Visible: true, Position: models.PosBottomRight},
	}
	l := PlaceOverlay(f, st, m)

	// The nursery badge sits above the band at the right edge.
	nw, nh := 200+2*28.8, 80+2*11.52
	nb := Rect{1920 - 40 - nw, 1080 - 40 - 98.56 - nh, nw, nh}
	if !ovNearRect(l.Nursery.Box, nb) || !ovNearRect(l.Nursery.Text, Rect{nb.X + 28.8, nb.Y + 11.52, 200, 80}) {
		t.Errorf("nursery %+v, want box %+v", l.Nursery, nb)
	}
	// The clock stacks above it in the same corner.
	cw, ch := 120+2*19.8, 56+2*7.92
	if cb := (Rect{1920 - 40 - cw, nb.Y - OverlayGap - ch, cw, ch}); !ovNearRect(l.Clock.Box, cb) {
		t.Errorf("clock %+v, want %+v", l.Clock.Box, cb)
	}
	// The countdown: caption above the digits, as wide as the caption.
	dw, dh := 300+2*32.4, 40+90+2*12.96
	if !ovNearRect(l.Countdown.Box, Rect{40, 40, dw, dh}) ||
		!ovNearRect(l.Countdown.Caption, Rect{40 + 32.4, 40 + 12.96, 300, 40}) ||
		!ovNearRect(l.Countdown.Text, Rect{40 + 32.4, 40 + 12.96 + 40, 300, 90}) {
		t.Errorf("countdown %+v", l.Countdown)
	}
}

func TestPlaceOverlayCenterAndTopBand(t *testing.T) {
	st := models.DefaultAlertStyles()
	m := OverlayMetrics{Message: TextMetrics{W: 100, Line: 64}, Countdown: TextMetrics{W: 400, Line: 180}, Clock: TextMetrics{W: 100, Line: 50}}
	f := models.OverlayFrame{
		Message:   models.MessageOverlay{Visible: true, Top: true},
		Countdown: models.BoxOverlay{Visible: true, Position: models.PosCenter},
		Clock:     models.BoxOverlay{Visible: true, Position: models.PosTopLeft},
	}
	l := PlaceOverlay(f, st, m)
	size := BadgeSize(models.PosCenter, 72) // 144: pad 64.8 x 25.92
	if size != 144 {
		t.Fatalf("BadgeSize = %v", size)
	}
	bw, bh := 400+2*64.8, 180+2*25.92
	if !ovNearRect(l.Countdown.Box, Rect{(1920 - bw) / 2, (1080 - bh) / 2, bw, bh}) {
		t.Errorf("centred countdown %+v", l.Countdown.Box)
	}
	// The top-left clock keeps clear of the top band.
	if !ovNear(l.Clock.Box.Y, 40+98.56) || !ovNear(l.Clock.Box.X, 40) {
		t.Errorf("clock under a top band: %+v", l.Clock.Box)
	}
	if empty := PlaceOverlay(models.OverlayFrame{}, st, m); empty != (OverlayLayout{}) {
		t.Errorf("an empty frame places %+v", empty)
	}
}
