package layout

import (
	"math"

	"freebeams/internal/models"
)

// BandRect is where a theme's text box band (models.TextBox) is drawn, in
// canvas units: the margins rect grown by the padding on every side (from
// edge to edge with FullWidth), clamped to the canvas. ok is false when the
// band is off, fully transparent or empty.
func BandRect(m models.Margins, b models.TextBox) (r Rect, ok bool) {
	if !b.Enabled || b.Color.A <= 0 {
		return Rect{}, false
	}
	pad := math.Max(0, float64(b.Padding))
	x0, x1 := float64(m.Left)-pad, models.CanvasWidth-float64(m.Right)+pad
	if b.FullWidth {
		x0, x1 = 0, models.CanvasWidth
	}
	y0, y1 := float64(m.Top)-pad, models.CanvasHeight-float64(m.Bottom)+pad
	x0, y0 = math.Max(0, x0), math.Max(0, y0)
	x1, y1 = math.Min(models.CanvasWidth, x1), math.Min(models.CanvasHeight, y1)
	if x1 <= x0 || y1 <= y0 {
		return Rect{}, false
	}
	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}, true
}
