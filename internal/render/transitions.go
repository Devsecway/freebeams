package render

import (
	"freebeams/internal/models"
	"freebeams/internal/render/layout"

	"graphics.gd/classdb/CanvasItem"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Rect2"
	"graphics.gd/variant/Vector2"
)

// Transitions. A SlideView change swaps its two layers (show in slideview.go):
// cur becomes the incoming state and prev the outgoing one, both visible, and
// Process drives the progress from 0 to 1. The geometry of every kind lives in
// layout.Transition (pure, tested); this file projects a layout.TransitionFrame
// onto the layers:
//
//   - the offset of each layer moves its inner content holder inside the
//     layer root, which stays at the letterbox rect and clips (push, cover,
//     reveal), so nothing shows over the letterbox bars;
//   - the incoming layer's alpha is its root's modulate (blend);
//   - the mask (wipe, iris) is drawn by the layer root itself, which clips its
//     children to what it draws (CanvasItem.clip_children = only);
//   - the scale of each layer (zoom) scales its inner content holder about
//     the centre, so text, images and video layers zoom together.
//
// Thumbnails keep cutting: their duration is 0, so show never animates.

// SetTransition selects the transition for subsequent changes. The kind and
// duration are normalized (models.Transition.Normalized); a cut has no
// duration.
func (v *SlideView) SetTransition(t models.Transition) {
	t = t.Normalized()
	v.kind = t.Kind
	v.SetTransitionDuration(t.Duration)
}

// SetScreenTransitions selects the Black, Clear and Logo transitions for
// subsequent changes (the default follows SetTransition). The change into
// or out of a screen picks its transition when it starts
// (models.ScreenTransitions.For), so changing them mid-way never alters a
// running transition.
func (v *SlideView) SetScreenTransitions(s models.ScreenTransitions) { v.screens = s }

// changeTransition is the transition of a change from mode from to the
// current frame's mode.
func (v *SlideView) changeTransition(from, to models.OutputMode) models.Transition {
	slide := models.Transition{Kind: v.transitionKind(), Duration: v.transitionDuration()}
	return v.screens.For(slide, from, to) // thumbnails: zero screens follow their 0 ms slide
}

// RunningTransition is the transition running now (ok false when none).
func (v *SlideView) RunningTransition() (models.Transition, bool) {
	return v.run, v.transitioning
}

// Transition returns the configured transition.
func (v *SlideView) Transition() models.Transition {
	return models.Transition{Kind: v.transitionKind(), Duration: v.transitionDuration()}
}

func (v *SlideView) transitionKind() models.TransitionKind {
	if !v.kind.Valid() {
		return models.TransitionBlend
	}
	return v.kind
}

// frameAt is the layers' state at progress t for the current size, for
// the running change's kind.
func (v *SlideView) frameAt(t float64) layout.TransitionFrame {
	r, _ := v.area()
	kind := v.run.Kind
	if !kind.Valid() {
		kind = models.TransitionBlend
	}
	return layout.Transition(kind, t, r.W, r.H)
}

// beginTransition puts the layers in their starting state. show calls it once
// cur (incoming) and prev (outgoing) are both visible and cur is on top.
func (v *SlideView) beginTransition() {
	fr := v.frameAt(0)
	v.orderLayers(fr.InOnTop)
	v.applyFrame(fr)
}

// applyTransition projects the state at progress t onto the layers.
func (v *SlideView) applyTransition(t float64) { v.applyFrame(v.frameAt(t)) }

// endTransition restores the rest state: both layers in place, opaque and
// unmasked (finishTransition then hides prev).
func (v *SlideView) endTransition() {
	v.applyFrame(layout.Rest)
	v.orderLayers(true)
}

func (v *SlideView) applyFrame(fr layout.TransitionFrame) {
	v.cur.setOffset(fr.In)
	v.cur.setAlpha(fr.InAlpha)
	v.cur.setMask(fr.Mask)
	v.cur.setScale(fr.InScale)
	if v.prev != nil {
		v.prev.setScale(fr.OutScale)
		v.prev.setOffset(fr.Out)
		v.prev.setAlpha(1)
		v.prev.setMask(layout.Mask{})
	}
}

// orderLayers puts the incoming layer above (or, for a reveal, below) the
// outgoing one.
func (v *SlideView) orderLayers(inOnTop bool) {
	if v.prev == nil {
		return
	}
	top := v.cur
	if !inOnTop {
		top = v.prev
	}
	v.AsNode().MoveChild(top.root.AsNode(), -1)
	v.raiseOverlay()
}

// ── Layer side ───────────────────────────────────────────────────────────

// placeAt puts the layer at the letterbox rect r with its content at rest.
// apply calls it; a running transition re-applies its offset afterwards
// (relayout).
func (l *layer) placeAt(r layout.Rect) {
	place(l.root, r)
	l.offset = layout.Vec{}
	place(l.inner, layout.Rect{W: r.W, H: r.H})
	l.inner.SetPivotOffset(Vector2.New(r.W/2, r.H/2)) // zoom about the centre
}

// minScale keeps a zoom's first frame invertible: a zero scale makes the
// control's transform singular, which GUI picking cannot invert.
const minScale = 1e-3

// setScale scales the layer's content about its centre (1 = rest size).
func (l *layer) setScale(k float64) {
	k = max(k, minScale)
	if l.scale == k {
		return
	}
	l.scale = k
	l.inner.SetScale(Vector2.New(k, k))
}

// setOffset moves the layer's content by d from its rest position.
func (l *layer) setOffset(d layout.Vec) {
	if l.offset == d {
		return
	}
	l.offset = d
	l.inner.SetPosition(Vector2.New(d.X, d.Y))
}

// setMask clips the layer to m (MaskNone shows everything).
func (l *layer) setMask(m layout.Mask) {
	if l.mask == m {
		return
	}
	wasMasked := l.mask.Kind != layout.MaskNone
	l.mask = m
	ci := l.root.AsCanvasItem()
	if masked := m.Kind != layout.MaskNone; masked != wasMasked {
		mode := CanvasItem.ClipChildrenDisabled
		if masked {
			mode = CanvasItem.ClipChildrenOnly
		}
		ci.SetClipChildren(mode)
	}
	ci.QueueRedraw()
}

var maskColor = Color.RGBA{R: 1, G: 1, B: 1, A: 1}

// drawMask draws the layer root's clip shape (its draw signal). With
// clip_children = only the shape is not shown; the children show through it.
// The handler ends with a getter so graphics.gd flushes the buffered draw
// calls while the engine is still inside this item's draw pass.
func (l *layer) drawMask() {
	if l.mask.Kind == layout.MaskNone {
		return
	}
	ci := l.root.AsCanvasItem()
	switch m := l.mask; m.Kind {
	case layout.MaskRect:
		r := m.Rect
		ci.DrawRect(Rect2.PositionSize{Position: Vector2.New(r.X, r.Y), Size: Vector2.New(r.W, r.H)}, maskColor)
	case layout.MaskCircle:
		ci.MoreArgs().DrawCircle(Vector2.New(m.Center.X, m.Center.Y), Float.X(m.Radius), maskColor, true, -1, true)
	}
	_ = ci.Visible() // flush
}
