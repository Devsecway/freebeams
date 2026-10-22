package layout

import "freebeams/internal/models"

// Overlay geometry: where the alert layer's message band and badges go on
// the 1920×1080 canvas. The renderer measures the text (font metrics) and
// scales the result to the view, so the overlay looks the same on the Live
// monitor and on the projector.

// OverlayEdge is the gap between a badge and the canvas edge (canvas units).
const OverlayEdge = 40

// OverlayGap separates badges stacked in one corner.
const OverlayGap = 16

// CenterScale is how much larger a centred countdown is drawn than its size.
const CenterScale = 2

// BadgeSize is the font size a badge is drawn at: a centred one is
// CenterScale times its style's size.
func BadgeSize(pos models.AlertPosition, size float32) float32 {
	if pos == models.PosCenter {
		return size * CenterScale
	}
	return size
}

// BandPadY is the message band's padding above and below the text.
func BandPadY(size float32) float64 { return float64(size) * 0.32 }

// BadgePad is a badge's padding around its text (x, y).
func BadgePad(size float32) (x, y float64) { return float64(size) * 0.45, float64(size) * 0.18 }

// BadgeRadius is a badge's corner radius.
func BadgeRadius(size float32) float64 { return float64(size) * 0.22 }

// TextMetrics is one measured text: its width and line height (canvas units).
type TextMetrics struct {
	W, Line float64
}

// OverlayMetrics are the measured texts of a frame (at the sizes the badges
// are drawn at, see BadgeSize).
type OverlayMetrics struct {
	Message                     TextMetrics
	Nursery                     TextMetrics
	Countdown, CountdownCaption TextMetrics
	Clock                       TextMetrics
}

// BoxLayout places a badge: its box, its text and its caption (above the
// text; zero without a caption).
type BoxLayout struct {
	Box, Text, Caption Rect
}

// OverlayLayout is the geometry of a frame in canvas units.
type OverlayLayout struct {
	Band Rect // the message band (full width)
	// MessageText is the message's text rect: aligned in the band for a
	// banner, at the frame's X for a ticker (it may lie partly off the band).
	MessageText Rect
	Nursery     BoxLayout
	Countdown   BoxLayout
	Clock       BoxLayout
}

// PlaceOverlay lays out a frame. Hidden parts get zero rects. Badges in the
// same corner stack away from the edge in the order nursery, countdown,
// clock, and keep clear of the message band.
func PlaceOverlay(f models.OverlayFrame, st models.AlertStyles, m OverlayMetrics) OverlayLayout {
	var l OverlayLayout
	const w, h = float64(models.CanvasWidth), float64(models.CanvasHeight)
	bandH := 0.0
	if f.Message.Visible {
		size := st.Message.Size
		padY := BandPadY(size)
		bandH = m.Message.Line + 2*padY
		y := h - bandH
		if f.Message.Top {
			y = 0
		}
		l.Band = Rect{X: 0, Y: y, W: w, H: bandH}
		textY := y + padY
		if f.Message.Scroll {
			l.MessageText = Rect{X: f.Message.X, Y: textY, W: m.Message.W, H: m.Message.Line}
		} else {
			pad := models.MessagePadX(size)
			tw := min(m.Message.W, w-2*pad)
			x := (w - tw) / 2
			switch f.Message.Align {
			case models.AlignLeft:
				x = pad
			case models.AlignRight:
				x = w - pad - tw
			}
			l.MessageText = Rect{X: x, Y: textY, W: tw, H: m.Message.Line}
		}
	}

	// next y per corner: top corners grow down, bottom corners grow up.
	topY := float64(OverlayEdge)
	bottomY := h - OverlayEdge
	if f.Message.Visible && f.Message.Top {
		topY += bandH
	}
	if f.Message.Visible && !f.Message.Top {
		bottomY -= bandH
	}
	next := map[models.AlertPosition]float64{
		models.PosTopLeft: topY, models.PosTopRight: topY,
		models.PosBottomLeft: bottomY, models.PosBottomRight: bottomY,
	}
	place := func(b models.BoxOverlay, size float32, text, caption TextMetrics) BoxLayout {
		if !b.Visible {
			return BoxLayout{}
		}
		size = BadgeSize(b.Position, size)
		padX, padY := BadgePad(size)
		capH := 0.0
		if b.Caption != "" {
			capH = caption.Line
		}
		bw := max(text.W, caption.W*boolf(b.Caption != "")) + 2*padX
		bh := capH + text.Line + 2*padY
		var x, y float64
		switch b.Position {
		case models.PosCenter:
			x, y = (w-bw)/2, (h-bh)/2
		default:
			pos := b.Position
			if pos.Left() {
				x = OverlayEdge
			} else {
				x = w - OverlayEdge - bw
			}
			if pos.Top() {
				y = next[pos]
				next[pos] = y + bh + OverlayGap
			} else {
				y = next[pos] - bh
				next[pos] = y - OverlayGap
			}
		}
		box := Rect{X: x, Y: y, W: bw, H: bh}
		inner := Rect{X: x + padX, W: bw - 2*padX}
		out := BoxLayout{Box: box}
		if capH > 0 {
			out.Caption = Rect{X: inner.X, Y: y + padY, W: inner.W, H: capH}
		}
		out.Text = Rect{X: inner.X, Y: y + padY + capH, W: inner.W, H: text.Line}
		return out
	}
	l.Nursery = place(f.Nursery, st.Nursery.Size, m.Nursery, TextMetrics{})
	l.Countdown = place(f.Countdown, st.Countdown.Size, m.Countdown, m.CountdownCaption)
	l.Clock = place(f.Clock, st.Clock.Size, m.Clock, TextMetrics{})
	return l
}

// CaptionRatio is a countdown caption's size relative to the countdown.
const CaptionRatio = 0.42

func boolf(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
