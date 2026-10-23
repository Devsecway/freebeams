package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Screen transitions: EasyWorship's Options ▸ Transitions has four tabs,
// Slide / Black / Clear / Logo. The slide transition is Transition; the
// other three are a ScreenTransition each, which by default follows the
// slide transition ("same"), so nothing changes until the operator picks
// one of its own.

// SameAsSlide is the JSON and control-API name of "same as the slide
// transition".
const SameAsSlide = "same"

// ScreenTransition is the transition into and out of the Black, Clear or
// Logo screen: Same follows the slide transition, otherwise Transition.
//
// Its JSON form is "same" or a Transition ({"kind":"push-left","duration":500});
// {"kind":"same"} is read as "same" too. The zero value (no kind) is Same.
type ScreenTransition struct {
	Same       bool
	Transition Transition
}

// SameTransition is the default: follow the slide transition.
func SameTransition() ScreenTransition { return ScreenTransition{Same: true} }

// OwnTransition is a screen transition of its own.
func OwnTransition(t Transition) ScreenTransition {
	return ScreenTransition{Transition: t.Normalized()}
}

// same reports whether s follows the slide transition (Same, or the zero
// value).
func (s ScreenTransition) same() bool { return s.Same || s.Transition.Kind == "" }

// Normalized returns s with its own transition normalized; a Same keeps no
// transition of its own, so two Same values are equal.
func (s ScreenTransition) Normalized() ScreenTransition {
	if s.same() {
		return SameTransition()
	}
	return OwnTransition(s.Transition)
}

// Resolve is the transition to run: slide when Same.
func (s ScreenTransition) Resolve(slide Transition) Transition {
	if s.same() {
		return slide.Normalized()
	}
	return s.Transition.Normalized()
}

// String describes it ("same", "push-left 500ms").
func (s ScreenTransition) String() string {
	if s.same() {
		return SameAsSlide
	}
	return s.Transition.String()
}

// MarshalJSON writes "same" or the transition.
func (s ScreenTransition) MarshalJSON() ([]byte, error) {
	if s.same() {
		return json.Marshal(SameAsSlide)
	}
	return json.Marshal(s.Transition)
}

// UnmarshalJSON reads "same", {"kind":"same"} or a transition (leniently,
// like Transition).
func (s *ScreenTransition) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '"' {
		var name string
		if err := json.Unmarshal(b, &name); err != nil {
			return fmt.Errorf("screen transition: %w", err)
		}
		if !isSame(name) {
			return fmt.Errorf("screen transition: want %q or a transition object, got %q", SameAsSlide, name)
		}
		*s = SameTransition()
		return nil
	}
	var probe struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(b, &probe); err != nil {
		return fmt.Errorf("screen transition: %w", err)
	}
	if isSame(probe.Kind) {
		*s = SameTransition()
		return nil
	}
	var t Transition
	if err := json.Unmarshal(b, &t); err != nil {
		return fmt.Errorf("screen %w", err)
	}
	*s = OwnTransition(t)
	return nil
}

// isSame reads the spellings of "same as the slide transition".
func isSame(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case SameAsSlide, "same-as-slide", "same as slide", "slide", "default":
		return true
	}
	return false
}

// ScreenTransitions are the Black, Clear and Logo transitions.
type ScreenTransitions struct {
	Black ScreenTransition `json:"black"`
	Clear ScreenTransition `json:"clear"`
	Logo  ScreenTransition `json:"logo"`
}

// DefaultScreenTransitions follow the slide transition.
func DefaultScreenTransitions() ScreenTransitions {
	return ScreenTransitions{Black: SameTransition(), Clear: SameTransition(), Logo: SameTransition()}
}

// Get returns the screen transition of mode (ok false for ModeNormal and
// unknown modes).
func (s ScreenTransitions) Get(mode OutputMode) (ScreenTransition, bool) {
	switch mode {
	case ModeBlack:
		return s.Black, true
	case ModeClear:
		return s.Clear, true
	case ModeLogo:
		return s.Logo, true
	}
	return ScreenTransition{}, false
}

// With returns s with mode's transition replaced (s itself for a mode
// without one).
func (s ScreenTransitions) With(mode OutputMode, t ScreenTransition) ScreenTransitions {
	t = t.Normalized()
	switch mode {
	case ModeBlack:
		s.Black = t
	case ModeClear:
		s.Clear = t
	case ModeLogo:
		s.Logo = t
	}
	return s
}

// For picks the transition of a change on the output from mode from to
// mode to: going into Black, Clear or Logo uses that screen's transition;
// coming out of one (back to the slide, or a slide change that ends Clear)
// uses the screen being left; a slide change uses slide. Black to Logo is
// the Logo transition (the screen being entered wins).
func (s ScreenTransitions) For(slide Transition, from, to OutputMode) Transition {
	if st, ok := s.Get(to); ok && to != from {
		return st.Resolve(slide)
	}
	if st, ok := s.Get(from); ok && to != from {
		return st.Resolve(slide)
	}
	if st, ok := s.Get(to); ok { // a change behind the screen (Black stays Black)
		return st.Resolve(slide)
	}
	return slide.Normalized()
}

// ParseScreenMode reads the screen of a screen transition: "black",
// "clear" or "logo" (case-insensitive).
func ParseScreenMode(s string) (OutputMode, error) {
	switch m := OutputMode(strings.ToLower(strings.TrimSpace(s))); m {
	case ModeBlack, ModeClear, ModeLogo:
		return m, nil
	}
	return "", fmt.Errorf("unknown screen %q (black|clear|logo)", s)
}
