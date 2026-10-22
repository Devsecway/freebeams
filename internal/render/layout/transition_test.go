package layout

import (
	"math"
	"testing"

	"freebeams/internal/models"
)

func vecNear(a, b Vec) bool { return near(a.X, b.X) && near(a.Y, b.Y) }

func TestTransitionEndpoints(t *testing.T) {
	// Every kind starts with the incoming layer invisible (off screen, fully
	// transparent or fully masked) and ends at rest.
	const w, h = 1920.0, 1080.0
	for _, kind := range models.TransitionKinds() {
		t.Run(string(kind), func(t *testing.T) {
			start := Transition(kind, 0, w, h)
			end := Transition(kind, 1, w, h)
			if end != Rest {
				t.Errorf("end = %+v, want Rest", end)
			}
			hidden := start.InAlpha == 0 || // blend, cut
				math.Abs(start.In.X) >= w || math.Abs(start.In.Y) >= h || // push, cover: off screen
				(start.Mask.Kind == MaskRect && (start.Mask.Rect.W == 0 || start.Mask.Rect.H == 0)) || // wipe
				(start.Mask.Kind == MaskCircle && start.Mask.Radius == 0) || // iris
				start.InScale == 0 // zoom in
			if kind.Family() == models.FamilyReveal || kind == models.TransitionZoomOut {
				// The outgoing layer covers the incoming one completely.
				hidden = !start.InOnTop && start.Out == (Vec{}) && start.OutScale == 1
			}
			if !hidden {
				t.Errorf("start shows the incoming layer: %+v", start)
			}
			// Clamping.
			if Transition(kind, -1, w, h) != start || Transition(kind, 2, w, h) != end {
				t.Errorf("progress is not clamped")
			}
		})
	}
}

func TestTransitionMidway(t *testing.T) {
	const w, h = 1000.0, 500.0
	e := Ease(0.5) // 0.5
	tests := []struct {
		kind models.TransitionKind
		want TransitionFrame
	}{
		{models.TransitionBlend, TransitionFrame{InAlpha: e, InOnTop: true}},
		{models.TransitionCut, Rest},
		// Push: everything moves in the named direction.
		{models.TransitionPushLeft, TransitionFrame{In: Vec{X: (1 - e) * w}, Out: Vec{X: -e * w}, InAlpha: 1, InOnTop: true}},
		{models.TransitionPushRight, TransitionFrame{In: Vec{X: -(1 - e) * w}, Out: Vec{X: e * w}, InAlpha: 1, InOnTop: true}},
		{models.TransitionPushUp, TransitionFrame{In: Vec{Y: (1 - e) * h}, Out: Vec{Y: -e * h}, InAlpha: 1, InOnTop: true}},
		{models.TransitionPushDown, TransitionFrame{In: Vec{Y: -(1 - e) * h}, Out: Vec{Y: e * h}, InAlpha: 1, InOnTop: true}},
		// Cover: the incoming slides over a static outgoing layer.
		{models.TransitionCoverLeft, TransitionFrame{In: Vec{X: (1 - e) * w}, InAlpha: 1, InOnTop: true}},
		{models.TransitionCoverDown, TransitionFrame{In: Vec{Y: -(1 - e) * h}, InAlpha: 1, InOnTop: true}},
		// Reveal: the outgoing slides away, above a static incoming layer.
		{models.TransitionRevealLeft, TransitionFrame{Out: Vec{X: -e * w}, InAlpha: 1, InOnTop: false}},
		{models.TransitionRevealUp, TransitionFrame{Out: Vec{Y: -e * h}, InAlpha: 1, InOnTop: false}},
		// Wipe: the revealed region grows from the edge opposite the motion.
		{models.TransitionWipeLeft, TransitionFrame{InAlpha: 1, InOnTop: true, Mask: Mask{Kind: MaskRect, Rect: Rect{X: w * (1 - e), W: w * e, H: h}}}},
		{models.TransitionWipeRight, TransitionFrame{InAlpha: 1, InOnTop: true, Mask: Mask{Kind: MaskRect, Rect: Rect{W: w * e, H: h}}}},
		{models.TransitionWipeUp, TransitionFrame{InAlpha: 1, InOnTop: true, Mask: Mask{Kind: MaskRect, Rect: Rect{Y: h * (1 - e), W: w, H: h * e}}}},
		{models.TransitionWipeDown, TransitionFrame{InAlpha: 1, InOnTop: true, Mask: Mask{Kind: MaskRect, Rect: Rect{W: w, H: h * e}}}},
		// Iris: a circle from the centre that reaches the corners at the end.
		{models.TransitionIris, TransitionFrame{InAlpha: 1, InOnTop: true, Mask: Mask{Kind: MaskCircle, Center: Vec{X: w / 2, Y: h / 2}, Radius: e * math.Hypot(w/2, h/2)}}},
		// Zoom: in grows the incoming layer over the old one, out shrinks the
		// old layer above the new one.
		{models.TransitionZoomIn, TransitionFrame{InAlpha: 1, InOnTop: true, InScale: e, OutScale: 1}},
		{models.TransitionZoomOut, TransitionFrame{InAlpha: 1, InOnTop: false, InScale: 1, OutScale: 1 - e}},
		// Unknown kinds blend.
		{models.TransitionKind("cube"), TransitionFrame{InAlpha: e, InOnTop: true, InScale: 1, OutScale: 1}},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			if tt.want.InScale == 0 && tt.want.OutScale == 0 {
				tt.want.InScale, tt.want.OutScale = 1, 1 // every kind but zoom keeps the size
			}
			got := Transition(tt.kind, 0.5, w, h)
			if !vecNear(got.In, tt.want.In) || !vecNear(got.Out, tt.want.Out) || !near(got.InAlpha, tt.want.InAlpha) ||
				got.InOnTop != tt.want.InOnTop || got.Mask.Kind != tt.want.Mask.Kind ||
				!rectNear(got.Mask.Rect, tt.want.Mask.Rect) || !vecNear(got.Mask.Center, tt.want.Mask.Center) ||
				!near(got.Mask.Radius, tt.want.Mask.Radius) || !near(got.InScale, tt.want.InScale) || !near(got.OutScale, tt.want.OutScale) {
				t.Fatalf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestTransitionMonotonic(t *testing.T) {
	// The revealed area / travelled distance never shrinks as t grows (up to
	// the last frame before Rest).
	const w, h = 1920.0, 1080.0
	for _, kind := range models.TransitionKinds() {
		prev := -1.0
		for i := 0; i < 50; i++ {
			fr := Transition(kind, float64(i)/50, w, h)
			var v float64
			switch {
			case fr.Mask.Kind == MaskRect:
				v = fr.Mask.Rect.W * fr.Mask.Rect.H
			case fr.Mask.Kind == MaskCircle:
				v = fr.Mask.Radius
			case kind == models.TransitionZoomIn:
				v = fr.InScale
			case kind == models.TransitionZoomOut:
				v = 1 - fr.OutScale
			case kind.Family() == models.FamilyReveal:
				v = math.Hypot(fr.Out.X, fr.Out.Y)
			case kind.Family() == models.FamilyPush, kind.Family() == models.FamilyCover:
				v = w + h - math.Hypot(fr.In.X, fr.In.Y)
			default:
				v = fr.InAlpha
			}
			if v < prev-1e-9 {
				t.Fatalf("%s: not monotonic at step %d (%v < %v)", kind, i, v, prev)
			}
			prev = v
		}
	}
}
