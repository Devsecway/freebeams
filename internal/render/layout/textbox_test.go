package layout

import (
	"testing"

	"freebeams/internal/models"
)

func TestBandRect(t *testing.T) {
	m := models.Margins{Left: 100, Top: 700, Right: 120, Bottom: 80}
	on := models.RGBA{A: 0.5}
	tests := []struct {
		name string
		box  models.TextBox
		want Rect
		ok   bool
	}{
		{"off", models.TextBox{Color: on, Padding: 10}, Rect{}, false},
		{"transparent", models.TextBox{Enabled: true}, Rect{}, false},
		{"margins", models.TextBox{Enabled: true, Color: on}, Rect{X: 100, Y: 700, W: 1700, H: 300}, true},
		{"padded", models.TextBox{Enabled: true, Color: on, Padding: 20}, Rect{X: 80, Y: 680, W: 1740, H: 340}, true},
		{"full width", models.TextBox{Enabled: true, Color: on, Padding: 20, FullWidth: true}, Rect{X: 0, Y: 680, W: 1920, H: 340}, true},
		{"clamped", models.TextBox{Enabled: true, Color: on, Padding: 200}, Rect{X: 0, Y: 500, W: 1920, H: 580}, true},
		{"negative padding is none", models.TextBox{Enabled: true, Color: on, Padding: -5}, Rect{X: 100, Y: 700, W: 1700, H: 300}, true},
	}
	for _, tt := range tests {
		got, ok := BandRect(m, tt.box)
		if ok != tt.ok || got != tt.want {
			t.Errorf("%s: BandRect = %+v, %v; want %+v, %v", tt.name, got, ok, tt.want, tt.ok)
		}
	}
	if _, ok := BandRect(models.Margins{Left: 1000, Right: 1000}, models.TextBox{Enabled: true, Color: on}); ok {
		t.Error("an empty margins rect has a band")
	}
}
