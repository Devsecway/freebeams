package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// TransitionKind names a live-output transition (EasyWorship's Blend, Push,
// Wipe, Cover, Reveal, Iris and Zoom). Directional kinds name the direction of
// motion: "push-left" pushes the old slide off to the left while the new one
// enters from the right; "wipe-left" reveals the new slide from the right edge
// leftwards; "cover-left" slides the new slide in, moving left, over the old
// one; "reveal-left" slides the old slide away to the left, uncovering the new
// one underneath.
type TransitionKind string

const (
	TransitionCut         TransitionKind = "cut"
	TransitionBlend       TransitionKind = "blend" // crossfade
	TransitionPushLeft    TransitionKind = "push-left"
	TransitionPushRight   TransitionKind = "push-right"
	TransitionPushUp      TransitionKind = "push-up"
	TransitionPushDown    TransitionKind = "push-down"
	TransitionWipeLeft    TransitionKind = "wipe-left"
	TransitionWipeRight   TransitionKind = "wipe-right"
	TransitionWipeUp      TransitionKind = "wipe-up"
	TransitionWipeDown    TransitionKind = "wipe-down"
	TransitionIris        TransitionKind = "iris" // a circle growing from the centre
	TransitionCoverLeft   TransitionKind = "cover-left"
	TransitionCoverRight  TransitionKind = "cover-right"
	TransitionCoverUp     TransitionKind = "cover-up"
	TransitionCoverDown   TransitionKind = "cover-down"
	TransitionRevealLeft  TransitionKind = "reveal-left"
	TransitionRevealRight TransitionKind = "reveal-right"
	TransitionRevealUp    TransitionKind = "reveal-up"
	TransitionRevealDown  TransitionKind = "reveal-down"
)

// Zoom: "zoom-in" grows the new slide from the centre over the old one;
// "zoom-out" shrinks the old slide into the centre, uncovering the new one.
const (
	TransitionZoomIn  TransitionKind = "zoom-in"
	TransitionZoomOut TransitionKind = "zoom-out"
)

// TransitionFamily is a transition kind without its direction.
type TransitionFamily string

const (
	FamilyCut    TransitionFamily = "cut"
	FamilyBlend  TransitionFamily = "blend"
	FamilyPush   TransitionFamily = "push"
	FamilyWipe   TransitionFamily = "wipe"
	FamilyIris   TransitionFamily = "iris"
	FamilyCover  TransitionFamily = "cover"
	FamilyReveal TransitionFamily = "reveal"
)

// FamilyZoom is zoom-in and zoom-out (in and out are not directions of motion).
const FamilyZoom TransitionFamily = "zoom"

// Direction is the direction of motion of a directional transition.
type Direction string

const (
	DirNone  Direction = ""
	DirLeft  Direction = "left"
	DirRight Direction = "right"
	DirUp    Direction = "up"
	DirDown  Direction = "down"
)

// transitionKinds lists every kind in picker order.
var transitionKinds = []TransitionKind{
	TransitionBlend, TransitionCut,
	TransitionPushLeft, TransitionPushRight, TransitionPushUp, TransitionPushDown,
	TransitionWipeLeft, TransitionWipeRight, TransitionWipeUp, TransitionWipeDown,
	TransitionIris,
	TransitionCoverLeft, TransitionCoverRight, TransitionCoverUp, TransitionCoverDown,
	TransitionRevealLeft, TransitionRevealRight, TransitionRevealUp, TransitionRevealDown,
	TransitionZoomIn, TransitionZoomOut,
}

// TransitionKinds returns every transition kind in the order pickers show them.
func TransitionKinds() []TransitionKind {
	out := make([]TransitionKind, len(transitionKinds))
	copy(out, transitionKinds)
	return out
}

// Valid reports whether k is a known kind.
func (k TransitionKind) Valid() bool {
	for _, known := range transitionKinds {
		if k == known {
			return true
		}
	}
	return false
}

// Family returns the kind without its direction ("" for an unknown kind).
func (k TransitionKind) Family() TransitionFamily {
	if !k.Valid() {
		return ""
	}
	name, _, _ := strings.Cut(string(k), "-")
	return TransitionFamily(name)
}

// Direction returns the direction of motion (DirNone for blend, cut, iris,
// zoom and unknown kinds).
func (k TransitionKind) Direction() Direction {
	if !k.Valid() || k.Family() == FamilyZoom {
		return DirNone
	}
	_, dir, ok := strings.Cut(string(k), "-")
	if !ok {
		return DirNone
	}
	return Direction(dir)
}

// Label is the kind's display name ("Push Left", "Blend").
func (k TransitionKind) Label() string {
	if !k.Valid() {
		return "Unknown"
	}
	words := strings.Split(string(k), "-")
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// transitionAliases maps alternative spellings to kinds (lower case, with
// spaces and underscores folded to "-").
var transitionAliases = map[string]TransitionKind{
	"crossfade": TransitionBlend, "fade": TransitionBlend, "dissolve": TransitionBlend,
	"none": TransitionCut, "": TransitionCut,
	"push": TransitionPushLeft, "wipe": TransitionWipeLeft,
	"cover": TransitionCoverLeft, "reveal": TransitionRevealLeft,
	"iris-circle": TransitionIris, "circle": TransitionIris,
	"zoom": TransitionZoomIn,
}

// ParseTransitionKind reads a kind from its name or label, case-insensitively
// ("Push Left", "push_left", "push-left"). Family names alone mean the
// leftward variant ("push" is push-left; "zoom" is zoom-in); "crossfade" and
// "fade" are blend.
func ParseTransitionKind(s string) (TransitionKind, error) {
	key := strings.ToLower(strings.TrimSpace(s))
	key = strings.NewReplacer(" ", "-", "_", "-").Replace(key)
	if k := TransitionKind(key); k.Valid() {
		return k, nil
	}
	if k, ok := transitionAliases[key]; ok {
		return k, nil
	}
	return "", fmt.Errorf("unknown transition %q", s)
}

// DefaultTransitionDuration is the duration of the built-in crossfade.
const DefaultTransitionDuration = 500 * time.Millisecond

// MaxTransitionDuration bounds a configured duration.
const MaxTransitionDuration = 10 * time.Second

// Transition is the configured live-output transition: how slide changes on
// the output (and the live monitor) are animated, and for how long.
//
// Its JSON form is {"kind":"push-left","duration":500} with the duration in
// milliseconds, which is what the control API and the settings store use.
type Transition struct {
	Kind     TransitionKind
	Duration time.Duration
}

// DefaultTransition is the 500 ms crossfade.
func DefaultTransition() Transition {
	return Transition{Kind: TransitionBlend, Duration: DefaultTransitionDuration}
}

// Normalized returns t with an unknown kind replaced by blend, a cut given a
// zero duration and the duration clamped into [0, MaxTransitionDuration].
func (t Transition) Normalized() Transition {
	if !t.Kind.Valid() {
		t.Kind = TransitionBlend
	}
	switch {
	case t.Kind == TransitionCut, t.Duration < 0:
		t.Duration = 0
	case t.Duration > MaxTransitionDuration:
		t.Duration = MaxTransitionDuration
	}
	return t
}

// String describes the transition ("push-left 500ms").
func (t Transition) String() string {
	return fmt.Sprintf("%s %dms", t.Kind, t.Duration.Milliseconds())
}

type transitionJSON struct {
	Kind     string `json:"kind"`
	Duration *int64 `json:"duration"` // milliseconds
}

// MarshalJSON writes {"kind": ..., "duration": <ms>}.
func (t Transition) MarshalJSON() ([]byte, error) {
	ms := t.Duration.Milliseconds()
	return json.Marshal(transitionJSON{Kind: string(t.Kind), Duration: &ms})
}

// UnmarshalJSON reads the JSON form. The kind is parsed leniently (see
// ParseTransitionKind) and a missing duration keeps the default.
func (t *Transition) UnmarshalJSON(b []byte) error {
	var raw transitionJSON
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("transition: %w", err)
	}
	kind, err := ParseTransitionKind(raw.Kind)
	if err != nil {
		return fmt.Errorf("transition: %w", err)
	}
	d := DefaultTransitionDuration
	if raw.Duration != nil {
		d = time.Duration(*raw.Duration) * time.Millisecond
	}
	*t = Transition{Kind: kind, Duration: d}
	return nil
}
