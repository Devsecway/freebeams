package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestScreenTransitionFor(t *testing.T) {
	slide := Transition{Kind: TransitionPushLeft, Duration: 750 * time.Millisecond}
	black := Transition{Kind: TransitionIris, Duration: time.Second}
	logo := Transition{Kind: TransitionCut}
	own := DefaultScreenTransitions().With(ModeBlack, OwnTransition(black)).With(ModeLogo, OwnTransition(logo))
	tests := []struct {
		name     string
		s        ScreenTransitions
		from, to OutputMode
		want     Transition
	}{
		{"defaults: a slide change", DefaultScreenTransitions(), ModeNormal, ModeNormal, slide},
		{"defaults: into Black is the slide transition", DefaultScreenTransitions(), ModeNormal, ModeBlack, slide},
		{"defaults: out of Logo", DefaultScreenTransitions(), ModeLogo, ModeNormal, slide},
		{"into Black", own, ModeNormal, ModeBlack, black},
		{"out of Black", own, ModeBlack, ModeNormal, black},
		{"behind Black", own, ModeBlack, ModeBlack, black},
		{"into Logo (a cut)", own, ModeNormal, ModeLogo, logo},
		{"Black to Logo: the screen entered", own, ModeBlack, ModeLogo, logo},
		{"Logo to Black", own, ModeLogo, ModeBlack, black},
		{"Clear follows the slide", own, ModeNormal, ModeClear, slide},
		{"out of Clear", own, ModeClear, ModeNormal, slide},
		{"a slide change", own, ModeNormal, ModeNormal, slide},
		{"an unknown mode is a slide", own, "", ModeNormal, slide},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.For(slide, tt.from, tt.to); got != tt.want {
				t.Fatalf("For(%s → %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestScreenTransitionJSON(t *testing.T) {
	tests := []struct {
		in   string
		want ScreenTransition
		out  string
		err  bool
	}{
		{`"same"`, SameTransition(), `"same"`, false},
		{`"Same as Slide"`, SameTransition(), `"same"`, false},
		{`{"kind":"same"}`, SameTransition(), `"same"`, false},
		{`{"kind":"wipe-up","duration":250}`, OwnTransition(Transition{Kind: TransitionWipeUp, Duration: 250 * time.Millisecond}), `{"kind":"wipe-up","duration":250}`, false},
		{`{"kind":"cut","duration":900}`, OwnTransition(Transition{Kind: TransitionCut}), `{"kind":"cut","duration":0}`, false},
		{`{"kind":"Push Left"}`, OwnTransition(Transition{Kind: TransitionPushLeft, Duration: DefaultTransitionDuration}), `{"kind":"push-left","duration":500}`, false},
		{`"blend"`, ScreenTransition{}, "", true},
		{`{"kind":"spin"}`, ScreenTransition{}, "", true},
		{`[1]`, ScreenTransition{}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			var got ScreenTransition
			err := json.Unmarshal([]byte(tt.in), &got)
			if (err != nil) != tt.err {
				t.Fatalf("err = %v, want error %v", err, tt.err)
			}
			if tt.err {
				return
			}
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
			b, err := json.Marshal(got)
			if err != nil || string(b) != tt.out {
				t.Fatalf("marshal = %s, %v; want %s", b, err, tt.out)
			}
		})
	}
	// Nested: the three screens.
	b, _ := json.Marshal(DefaultScreenTransitions().With(ModeClear, OwnTransition(Transition{Kind: TransitionZoomIn, Duration: time.Second})))
	if string(b) != `{"black":"same","clear":{"kind":"zoom-in","duration":1000},"logo":"same"}` {
		t.Fatalf("screens JSON = %s", b)
	}
}

func TestScreenTransitionsWith(t *testing.T) {
	s := DefaultScreenTransitions().With(ModeNormal, OwnTransition(Transition{Kind: TransitionIris}))
	if s != DefaultScreenTransitions() {
		t.Errorf("ModeNormal changed the screens: %+v", s)
	}
	if got := (ScreenTransition{Same: true, Transition: Transition{Kind: TransitionIris}}).Normalized(); got != SameTransition() {
		t.Errorf("a Same keeps a transition: %+v", got)
	}
	for in, want := range map[string]OutputMode{"black": ModeBlack, " Clear ": ModeClear, "LOGO": ModeLogo} {
		if got, err := ParseScreenMode(in); err != nil || got != want {
			t.Errorf("ParseScreenMode(%q) = %q, %v", in, got, err)
		}
	}
	for _, in := range []string{"normal", "slide", ""} {
		if _, err := ParseScreenMode(in); err == nil {
			t.Errorf("ParseScreenMode(%q) accepted", in)
		}
	}
}
