package layout

import (
	"math"

	"freebeams/internal/models"
)

// Transition geometry. A SlideView animates a slide change with two layers:
// the incoming one (the new state) and the outgoing one (what was shown).
// TransitionFrame says where each layer is at a moment of the transition, as
// an offset from its rest position over the letterboxed area, how opaque the
// incoming layer is and what part of it is visible. All lengths are in the
// same units as the area (pixels of the view).

// Vec is a 2-D offset.
type Vec struct{ X, Y float64 }

// MaskKind selects the shape the incoming layer is clipped to.
type MaskKind int

const (
	MaskNone   MaskKind = iota // the whole layer shows
	MaskRect                   // only Rect shows (wipe)
	MaskCircle                 // only the circle Center/Radius shows (iris)
)

// Mask is the region of the incoming layer that shows, in layer-local
// coordinates (0,0 is the layer's top-left corner).
type Mask struct {
	Kind   MaskKind
	Rect   Rect
	Center Vec
	Radius float64
}

// TransitionFrame is the state of both layers at one moment.
type TransitionFrame struct {
	In      Vec     // offset of the incoming layer from its rest position
	Out     Vec     // offset of the outgoing layer from its rest position
	InAlpha float64 // opacity of the incoming layer (1 except for blend)
	InOnTop bool    // the incoming layer is drawn above the outgoing one
	Mask    Mask    // what part of the incoming layer shows
}

// Rest is the frame of a finished transition: the incoming layer in place,
// opaque, unmasked and on top.
var Rest = TransitionFrame{InAlpha: 1, InOnTop: true}

// unit returns the unit vector of a direction of motion (zero for DirNone).
func unit(d models.Direction) Vec {
	switch d {
	case models.DirLeft:
		return Vec{X: -1}
	case models.DirRight:
		return Vec{X: 1}
	case models.DirUp:
		return Vec{Y: -1}
	case models.DirDown:
		return Vec{Y: 1}
	}
	return Vec{}
}

// span is the distance a layer travels for a direction: the width for
// horizontal motion, the height for vertical.
func span(d models.Direction, w, h float64) float64 {
	if d == models.DirUp || d == models.DirDown {
		return h
	}
	return w
}

func (v Vec) scale(k float64) Vec { return Vec{X: v.X * k, Y: v.Y * k} }

// Transition returns the layers' state at progress t (0 = start, 1 = end;
// clamped and eased) of a kind over a w×h area. A finished transition (t >= 1)
// is Rest for every kind. An unknown kind blends; a cut is finished at any
// t > 0.
func Transition(kind models.TransitionKind, t, w, h float64) TransitionFrame {
	if t >= 1 {
		return Rest
	}
	e := Ease(t)
	d := unit(kind.Direction())
	s := span(kind.Direction(), w, h)
	fr := Rest
	switch kind.Family() {
	case models.FamilyCut:
		if t <= 0 {
			fr.InAlpha = 0
		}
	case models.FamilyPush:
		// Both move in the direction d: the incoming starts one span behind
		// and the outgoing ends one span ahead.
		fr.In = d.scale(-(1 - e) * s)
		fr.Out = d.scale(e * s)
	case models.FamilyCover:
		fr.In = d.scale(-(1 - e) * s)
	case models.FamilyReveal:
		fr.Out = d.scale(e * s)
		fr.InOnTop = false
	case models.FamilyWipe:
		fr.Mask = Mask{Kind: MaskRect, Rect: wipeRect(kind.Direction(), e, w, h)}
	case models.FamilyIris:
		fr.Mask = Mask{Kind: MaskCircle, Center: Vec{X: w / 2, Y: h / 2}, Radius: e * math.Hypot(w/2, h/2)}
	default: // blend
		fr.InAlpha = e
	}
	return fr
}

// wipeRect is the revealed part of the incoming layer for a wipe whose edge
// moves in direction d: it grows from the opposite edge.
func wipeRect(d models.Direction, e, w, h float64) Rect {
	switch d {
	case models.DirLeft:
		return Rect{X: w * (1 - e), Y: 0, W: w * e, H: h}
	case models.DirRight:
		return Rect{X: 0, Y: 0, W: w * e, H: h}
	case models.DirUp:
		return Rect{X: 0, Y: h * (1 - e), W: w, H: h * e}
	default: // down
		return Rect{X: 0, Y: 0, W: w, H: h * e}
	}
}
