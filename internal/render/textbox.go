package render

import (
	"freebeams/internal/models"
	"freebeams/internal/render/layout"

	"graphics.gd/classdb/ColorRect"
	"graphics.gd/classdb/Node"
	"graphics.gd/variant/Color"
)

// The theme's text box band (models.TextBox): a translucent ColorRect over
// the background (and a playing video) and under the words.

func newBand(parent Node.Instance) ColorRect.Instance {
	b := ColorRect.New()
	ignoreMouse(b.AsControl())
	b.AsCanvasItem().SetVisible(false)
	parent.AddChild(b.AsNode())
	return b
}

// applyBand shows the band of th when the layer has text, at canvas scale s.
func (l *layer) applyBand(th models.Theme, hasText bool, s float64) {
	r, ok := layout.BandRect(th.Margins, th.Box)
	if !hasText || !ok || s <= 0 {
		l.band.AsCanvasItem().SetVisible(false)
		return
	}
	c := layout.ClampColor(th.Box.Color)
	l.band.SetColor(Color.RGBA{R: c.R, G: c.G, B: c.B, A: c.A})
	place(l.band.AsControl(), r.Scale(s))
	l.band.AsCanvasItem().SetVisible(true)
}
