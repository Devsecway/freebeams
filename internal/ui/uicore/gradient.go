package uicore

import (
	"math"

	"graphics.gd/variant/Color"
)

// Sample returns the gradient color at t (0 = top, 1 = bottom). Offsets outside
// the first/last stop clamp to the end colors, like FireMonkey's TGradient.
func (g Gradient) Sample(t float32) Color.RGBA {
	if len(g) == 0 {
		return Color.RGBA{}
	}
	if t <= g[0].Offset {
		return g[0].Color
	}
	last := g[len(g)-1]
	if t >= last.Offset {
		return last.Color
	}
	for i := 1; i < len(g); i++ {
		a, b := g[i-1], g[i]
		if t <= b.Offset {
			span := b.Offset - a.Offset
			if span <= 0 {
				return b.Color
			}
			f := (t - a.Offset) / span
			return Color.RGBA{
				R: a.Color.R + (b.Color.R-a.Color.R)*f,
				G: a.Color.G + (b.Color.G-a.Color.G)*f,
				B: a.Color.B + (b.Color.B-a.Color.B)*f,
				A: a.Color.A + (b.Color.A-a.Color.A)*f,
			}
		}
	}
	return last.Color
}

// Edge is an optional solid border row band in a rasterised gradient strip.
type Edge struct {
	Width int // rows (or columns, for horizontal strips)
	Color Color.RGBA
}

// GradientStrip rasterises g into an RGBA8 strip of the given length (rows, top
// to bottom), with optional solid bands at the start and end. The result is
// the pixel data for a 1-pixel-wide image of height `length`. Used to build
// StyleBoxTexture backgrounds whose border rows sit in the texture margins.
func GradientStrip(g Gradient, length int, start, end Edge) []byte {
	if length < 1 {
		length = 1
	}
	out := make([]byte, 0, length*4)
	inner := length - start.Width - end.Width
	if inner < 1 {
		inner = 1
	}
	for y := 0; y < length; y++ {
		var c Color.RGBA
		switch {
		case y < start.Width:
			c = start.Color
		case y >= length-end.Width:
			c = end.Color
		default:
			t := (float32(y-start.Width) + 0.5) / float32(inner)
			c = g.Sample(t)
		}
		out = append(out, to8(c.R), to8(c.G), to8(c.B), to8(c.A))
	}
	return out
}

// GradientBox rasterises g into a small RGBA8 image for a 9-slice
// StyleBoxTexture: `height` rows of gradient with solid top/bottom bands, and
// solid left/right columns (one stretchable center column). It returns the
// image width and the pixel data. Corner pixels take the vertical band color.
func GradientBox(g Gradient, height int, top, bottom, left, right Edge) (width int, px []byte) {
	col := GradientStrip(g, height, top, bottom)
	width = left.Width + 1 + right.Width
	px = make([]byte, 0, width*height*4)
	for y := 0; y < height; y++ {
		inBand := y < top.Width || y >= height-bottom.Width
		for x := 0; x < width; x++ {
			switch {
			case !inBand && x < left.Width:
				c := left.Color
				px = append(px, to8(c.R), to8(c.G), to8(c.B), to8(c.A))
			case !inBand && x >= left.Width+1:
				c := right.Color
				px = append(px, to8(c.R), to8(c.G), to8(c.B), to8(c.A))
			default:
				px = append(px, col[y*4:y*4+4]...)
			}
		}
	}
	return width, px
}

func to8(v float32) byte {
	return byte(math.Round(float64(clamp01(v)) * 255))
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
