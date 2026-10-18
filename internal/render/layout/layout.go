// Package layout holds the pure geometry and text-fitting math used by the
// renderer. It must not import graphics.gd so it can be unit-tested with plain
// `go test`.
package layout

import (
	"math"
	"strings"

	"freeworship/internal/models"
)

// Rect is an axis-aligned rectangle.
type Rect struct {
	X, Y, W, H float64
}

// Letterbox returns the largest rect with aspect ratio aw:ah that fits inside
// a w×h area, centered. Degenerate inputs yield an empty rect.
func Letterbox(w, h, aw, ah float64) Rect {
	if w <= 0 || h <= 0 || aw <= 0 || ah <= 0 {
		return Rect{}
	}
	scale := math.Min(w/aw, h/ah)
	rw, rh := aw*scale, ah*scale
	return Rect{X: (w - rw) / 2, Y: (h - rh) / 2, W: rw, H: rh}
}

// CanvasLetterbox is Letterbox for the logical slide canvas.
func CanvasLetterbox(w, h float64) Rect {
	return Letterbox(w, h, models.CanvasWidth, models.CanvasHeight)
}

// FitScale is the uniform scale that maps the logical canvas onto a w×h area
// (letterboxed). It is 0 for degenerate areas.
func FitScale(w, h float64) float64 {
	r := CanvasLetterbox(w, h)
	return r.W / models.CanvasWidth
}

// Px converts a canvas-unit length to whole target pixels at scale, rounding
// to nearest. Negative values are kept (shadow offsets can be negative).
func Px(v float32, scale float64) int {
	return int(math.Round(float64(v) * scale))
}

// Scale returns r scaled by s around the origin.
func (r Rect) Scale(s float64) Rect {
	return Rect{X: r.X * s, Y: r.Y * s, W: r.W * s, H: r.H * s}
}

// FitFontSize returns the largest integer size in [min, max] for which fits
// reports true, using binary search (fits must be monotonic: true for small
// sizes, false above some threshold). If even min does not fit, min is
// returned (the text overflows rather than disappearing). min is clamped to
// at least 1 and max to at least min.
func FitFontSize(min, max int, fits func(size int) bool) int {
	if min < 1 {
		min = 1
	}
	if max < min {
		max = min
	}
	if fits(max) {
		return max
	}
	lo, hi := min, max-1
	best := min
	for lo <= hi {
		mid := lo + (hi-lo)/2
		if fits(mid) {
			best = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	return best
}

// Boxes is the canvas-unit layout of a slide's text areas.
type Boxes struct {
	Body   Rect
	Footer Rect // zero when there is no footer
}

// FooterGapFactor is the gap between body and footer as a fraction of the
// footer's line height.
const FooterGapFactor = 0.5

// TextBoxes computes the body and footer rects inside the theme margins on the
// logical canvas. footerLineHeight is the height of one footer line in canvas
// units (0 = no footer). The body box is shrunk so it never overlaps the
// footer. Sizes never go negative.
func TextBoxes(m models.Margins, footerLineHeight float64) Boxes {
	x := float64(m.Left)
	y := float64(m.Top)
	w := math.Max(0, models.CanvasWidth-float64(m.Left)-float64(m.Right))
	h := math.Max(0, models.CanvasHeight-float64(m.Top)-float64(m.Bottom))
	b := Boxes{Body: Rect{X: x, Y: y, W: w, H: h}}
	if footerLineHeight > 0 {
		fh := math.Min(footerLineHeight, h)
		b.Footer = Rect{X: x, Y: y + h - fh, W: w, H: fh}
		b.Body.H = math.Max(0, h-fh-footerLineHeight*FooterGapFactor)
	}
	return b
}

// Clamp01 clamps v into [0, 1]; NaN becomes 0.
func Clamp01(v float32) float32 {
	if v != v || v < 0 { // NaN or negative
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// ClampColor returns c with every component clamped into [0, 1].
func ClampColor(c models.RGBA) models.RGBA {
	return models.RGBA{R: Clamp01(c.R), G: Clamp01(c.G), B: Clamp01(c.B), A: Clamp01(c.A)}
}

// JoinLines rebuilds text from wrapped line ranges ([start, end) indexes into
// the rune slice of text, as reported by the text server) so the renderer can
// force identical line breaks at every scale. Trailing whitespace and line
// terminators are trimmed from each line. Out-of-range indexes are clamped.
func JoinLines(text string, ranges [][2]int) string {
	runes := []rune(text)
	n := len(runes)
	lines := make([]string, 0, len(ranges))
	for _, r := range ranges {
		s, e := r[0], r[1]
		if s < 0 {
			s = 0
		}
		if e > n {
			e = n
		}
		if s > e {
			s = e
		}
		lines = append(lines, strings.TrimRight(string(runes[s:e]), " \t\r\n"))
	}
	return strings.Join(lines, "\n")
}

// Ease is the crossfade curve for transition progress t in [0, 1]
// (smoothstep; clamped).
func Ease(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	return t * t * (3 - 2*t)
}

// CrispMinPx is the font pixel size from which text is rasterized at its
// rounded size with no node scaling (pixel-crisp; widths within ±2% of the
// canvas layout). Below it (thumbnails, small monitors) rounding would change
// widths by up to ±50%, so text is rasterized at the rounded size and the
// label node is scaled by the remaining factor to match the layout exactly.
const CrispMinPx = 24

// RasterSize returns the integer font pixel size for a canvas-unit font size
// at scale s, and the node scale k that corrects the rounding (k == 1 at and
// above CrispMinPx).
func RasterSize(canvasSize, s float64) (px int, k float64) {
	exact := canvasSize * s
	px = max(1, int(math.Round(exact)))
	if px >= CrispMinPx || exact <= 0 {
		return px, 1
	}
	return px, exact / float64(px)
}
