package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTransitionKinds(t *testing.T) {
	tests := []struct {
		kind   TransitionKind
		family TransitionFamily
		dir    Direction
		label  string
	}{
		{TransitionBlend, FamilyBlend, DirNone, "Blend"},
		{TransitionCut, FamilyCut, DirNone, "Cut"},
		{TransitionPushLeft, FamilyPush, DirLeft, "Push Left"},
		{TransitionWipeDown, FamilyWipe, DirDown, "Wipe Down"},
		{TransitionIris, FamilyIris, DirNone, "Iris"},
		{TransitionCoverUp, FamilyCover, DirUp, "Cover Up"},
		{TransitionRevealRight, FamilyReveal, DirRight, "Reveal Right"},
		{TransitionKind("spin"), "", DirNone, "Unknown"},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			if got := tt.kind.Family(); got != tt.family {
				t.Errorf("Family() = %q, want %q", got, tt.family)
			}
			if got := tt.kind.Direction(); got != tt.dir {
				t.Errorf("Direction() = %q, want %q", got, tt.dir)
			}
			if got := tt.kind.Label(); got != tt.label {
				t.Errorf("Label() = %q, want %q", got, tt.label)
			}
			if tt.kind.Valid() != (tt.family != "") {
				t.Errorf("Valid() = %v", tt.kind.Valid())
			}
		})
	}
	kinds := TransitionKinds()
	if len(kinds) != 19 || kinds[0] != TransitionBlend {
		t.Errorf("TransitionKinds() = %v", kinds)
	}
	seen := map[TransitionKind]bool{}
	for _, k := range kinds {
		if seen[k] || !k.Valid() {
			t.Errorf("kind %q duplicated or invalid", k)
		}
		seen[k] = true
	}
}

func TestParseTransitionKind(t *testing.T) {
	tests := []struct {
		in   string
		want TransitionKind
		err  bool
	}{
		{"blend", TransitionBlend, false},
		{"Blend", TransitionBlend, false},
		{"crossfade", TransitionBlend, false},
		{"FADE", TransitionBlend, false},
		{"cut", TransitionCut, false},
		{"none", TransitionCut, false},
		{"", TransitionCut, false},
		{"push-left", TransitionPushLeft, false},
		{"Push Left", TransitionPushLeft, false},
		{"push_left", TransitionPushLeft, false},
		{" push ", TransitionPushLeft, false},
		{"wipe", TransitionWipeLeft, false},
		{"Wipe Up", TransitionWipeUp, false},
		{"iris", TransitionIris, false},
		{"circle", TransitionIris, false},
		{"cover-down", TransitionCoverDown, false},
		{"reveal right", TransitionRevealRight, false},
		{"cube", "", true},
		{"push-sideways", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseTransitionKind(tt.in)
			if (err != nil) != tt.err {
				t.Fatalf("err = %v, want error %v", err, tt.err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTransitionNormalized(t *testing.T) {
	tests := []struct {
		name string
		in   Transition
		want Transition
	}{
		{"default", DefaultTransition(), Transition{TransitionBlend, 500 * time.Millisecond}},
		{"unknown kind", Transition{"cube", time.Second}, Transition{TransitionBlend, time.Second}},
		{"cut has no duration", Transition{TransitionCut, time.Second}, Transition{TransitionCut, 0}},
		{"negative", Transition{TransitionPushLeft, -time.Second}, Transition{TransitionPushLeft, 0}},
		{"clamped", Transition{TransitionIris, time.Minute}, Transition{TransitionIris, MaxTransitionDuration}},
		{"kept", Transition{TransitionWipeUp, 700 * time.Millisecond}, Transition{TransitionWipeUp, 700 * time.Millisecond}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.Normalized(); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
	if s := (Transition{TransitionPushLeft, 1500 * time.Millisecond}).String(); s != "push-left 1500ms" {
		t.Errorf("String() = %q", s)
	}
}

func TestTransitionJSON(t *testing.T) {
	in := Transition{Kind: TransitionPushLeft, Duration: 750 * time.Millisecond}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"kind":"push-left","duration":750}` {
		t.Fatalf("marshal = %s", b)
	}
	var out Transition
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("round trip = %v, want %v", out, in)
	}

	tests := []struct {
		name string
		js   string
		want Transition
		err  bool
	}{
		{"lenient kind", `{"kind":"Wipe Up","duration":300}`, Transition{TransitionWipeUp, 300 * time.Millisecond}, false},
		{"missing duration keeps the default", `{"kind":"iris"}`, Transition{TransitionIris, DefaultTransitionDuration}, false},
		{"unknown kind", `{"kind":"cube","duration":100}`, Transition{}, true},
		{"not an object", `"push-left"`, Transition{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got Transition
			err := json.Unmarshal([]byte(tt.js), &got)
			if (err != nil) != tt.err {
				t.Fatalf("err = %v, want error %v", err, tt.err)
			}
			if !tt.err && got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}

	// Inside a struct (the settings record).
	type rec struct {
		T Transition `json:"transition"`
	}
	b, _ = json.Marshal(rec{T: DefaultTransition()})
	var r rec
	if err := json.Unmarshal(b, &r); err != nil || r.T != DefaultTransition() {
		t.Fatalf("nested round trip: %s %v %v", b, r, err)
	}
}
