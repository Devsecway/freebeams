package layout

import (
	"math"
	"testing"

	"freeworship/internal/models"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func rectNear(a, b Rect) bool {
	return near(a.X, b.X) && near(a.Y, b.Y) && near(a.W, b.W) && near(a.H, b.H)
}

func TestLetterbox(t *testing.T) {
	tests := []struct {
		name string
		w, h float64
		want Rect
	}{
		{"exact 16:9", 1920, 1080, Rect{0, 0, 1920, 1080}},
		{"thumbnail", 160, 90, Rect{0, 0, 160, 90}},
		{"wide pillarbox", 1000, 300, Rect{(1000 - 300*16.0/9) / 2, 0, 300 * 16.0 / 9, 300}},
		{"tall letterbox", 400, 400, Rect{0, 87.5, 400, 225}},
		{"4:3 screen", 1024, 768, Rect{0, (768 - 576) / 2, 1024, 576}},
		{"zero", 0, 100, Rect{}},
		{"negative", -5, 100, Rect{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanvasLetterbox(tt.w, tt.h)
			if !rectNear(got, tt.want) {
				t.Fatalf("CanvasLetterbox(%v,%v) = %+v, want %+v", tt.w, tt.h, got, tt.want)
			}
		})
	}
}

func TestFitScale(t *testing.T) {
	tests := []struct {
		w, h, want float64
	}{
		{1920, 1080, 1},
		{3840, 2160, 2},
		{160, 90, 160.0 / 1920},
		{400, 400, 400.0 / 1920},
		{1000, 300, 300.0 / 1080},
		{0, 0, 0},
	}
	for _, tt := range tests {
		if got := FitScale(tt.w, tt.h); !near(got, tt.want) {
			t.Errorf("FitScale(%v,%v) = %v, want %v", tt.w, tt.h, got, tt.want)
		}
	}
}

func TestPx(t *testing.T) {
	tests := []struct {
		v     float32
		scale float64
		want  int
	}{
		{96, 1, 96},
		{96, 2, 192},
		{96, 160.0 / 1920, 8},
		{4, 160.0 / 1920, 0},
		{-4, 0.5, -2},
		{3, 0.5, 2}, // 1.5 rounds away from zero
	}
	for _, tt := range tests {
		if got := Px(tt.v, tt.scale); got != tt.want {
			t.Errorf("Px(%v,%v) = %d, want %d", tt.v, tt.scale, got, tt.want)
		}
	}
}

func TestFitFontSize(t *testing.T) {
	threshold := func(max int) func(int) bool {
		return func(s int) bool { return s <= max }
	}
	tests := []struct {
		name     string
		min, max int
		limit    int // largest size that fits
		want     int
	}{
		{"everything fits", 36, 96, 1000, 96},
		{"nothing fits returns min", 36, 96, 10, 36},
		{"exactly min", 36, 96, 36, 36},
		{"middle", 36, 96, 61, 61},
		{"one below max", 36, 96, 95, 95},
		{"min > max clamps", 50, 40, 45, 50},
		{"min < 1 clamps", 0, 10, 5, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			fits := threshold(tt.limit)
			got := FitFontSize(tt.min, tt.max, func(s int) bool { calls++; return fits(s) })
			if got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
			if calls > 8 {
				t.Fatalf("too many measurements: %d", calls)
			}
		})
	}
}

func TestTextBoxes(t *testing.T) {
	m := models.Margins{Left: 96, Top: 72, Right: 96, Bottom: 96}
	b := TextBoxes(m, 0)
	if !rectNear(b.Body, Rect{96, 72, 1920 - 192, 1080 - 168}) || b.Footer != (Rect{}) {
		t.Fatalf("no footer: %+v", b)
	}
	b = TextBoxes(m, 40)
	wantFooter := Rect{96, 1080 - 96 - 40, 1920 - 192, 40}
	if !rectNear(b.Footer, wantFooter) {
		t.Fatalf("footer = %+v, want %+v", b.Footer, wantFooter)
	}
	if b.Body.Y+b.Body.H > b.Footer.Y {
		t.Fatalf("body %+v overlaps footer %+v", b.Body, b.Footer)
	}
	if !near(b.Body.H, 1080-168-40-20) {
		t.Fatalf("body height = %v", b.Body.H)
	}
	// Huge margins never produce negative sizes.
	b = TextBoxes(models.Margins{Left: 2000, Top: 2000}, 50)
	if b.Body.W < 0 || b.Body.H < 0 || b.Footer.H < 0 {
		t.Fatalf("negative box: %+v", b)
	}
}

func TestClampColor(t *testing.T) {
	nan := float32(math.NaN())
	tests := []struct {
		in, want models.RGBA
	}{
		{models.RGBA{R: 0.5, G: 0.25, B: 1, A: 1}, models.RGBA{R: 0.5, G: 0.25, B: 1, A: 1}},
		{models.RGBA{R: -1, G: 2, B: nan, A: 1.5}, models.RGBA{R: 0, G: 1, B: 0, A: 1}},
		{models.Hex(0xFF8000), models.RGBA{R: 1, G: 128.0 / 255, B: 0, A: 1}},
	}
	for _, tt := range tests {
		if got := ClampColor(tt.in); got != tt.want {
			t.Errorf("ClampColor(%+v) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestJoinLines(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		ranges [][2]int
		want   string
	}{
		{"wrap", "Amazing grace how sweet", [][2]int{{0, 14}, {14, 23}}, "Amazing grace\nhow sweet"},
		{"mandatory newline kept once", "a b\nc d", [][2]int{{0, 4}, {4, 7}}, "a b\nc d"},
		{"unicode", "Señor ñandú", [][2]int{{0, 6}, {6, 11}}, "Señor\nñandú"},
		{"clamped", "abc", [][2]int{{-2, 10}}, "abc"},
		{"empty line", "a\n\nb", [][2]int{{0, 2}, {2, 3}, {3, 4}}, "a\n\nb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := JoinLines(tt.text, tt.ranges); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEase(t *testing.T) {
	if Ease(-1) != 0 || Ease(0) != 0 || Ease(1) != 1 || Ease(2) != 1 || !near(Ease(0.5), 0.5) {
		t.Fatal("ease endpoints")
	}
	prev := 0.0
	for i := 1; i <= 100; i++ {
		v := Ease(float64(i) / 100)
		if v < prev {
			t.Fatalf("not monotonic at %d", i)
		}
		prev = v
	}
}

func TestRasterSize(t *testing.T) {
	tests := []struct {
		size, scale float64
		px          int
		k           float64
	}{
		{96, 1, 96, 1},
		{96, 2, 192, 1},
		{61, 2.0 / 3, 41, 1}, // big enough: crisp, no node scaling
		{96, 160.0 / 1920, 8, 1},
		{60, 160.0 / 1920, 5, 5.0 / 5},
		{70, 160.0 / 1920, 6, (70 * 160.0 / 1920) / 6},
		{4, 0.05, 1, 0.2},
		{0, 1, 1, 1},
	}
	for _, tt := range tests {
		px, k := RasterSize(tt.size, tt.scale)
		if px != tt.px || !near(k, tt.k) {
			t.Errorf("RasterSize(%v,%v) = %d,%v want %d,%v", tt.size, tt.scale, px, k, tt.px, tt.k)
		}
		if px < CrispMinPx && tt.size > 0 && !near(float64(px)*k, tt.size*tt.scale) {
			t.Errorf("RasterSize(%v,%v): px*k = %v, want exact %v", tt.size, tt.scale, float64(px)*k, tt.size*tt.scale)
		}
	}
}
