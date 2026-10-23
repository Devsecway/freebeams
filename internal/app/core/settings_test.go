package core

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"freebeams/internal/models"
)

func TestSettingsDefaultsAndNormalized(t *testing.T) {
	def := DefaultSettings("")
	if def.Transition != models.DefaultTransition() || def.OutputScreen != AutoScreen ||
		def.Logo != LogoGradient() || def.InterfaceColor != ColorDark || def.Alerts != models.DefaultAlertStyles() || def.PauseOnBlack {
		t.Fatalf("defaults = %+v", def)
	}
	tests := []struct {
		name string
		in   Settings
		want Settings
	}{
		{"zero value (screen 0 is the first display)", Settings{}, Settings{Transition: models.Transition{Kind: models.TransitionBlend}, OutputScreen: 0, Logo: LogoGradient(), InterfaceColor: ColorDark}},
		{"kept", Settings{Transition: models.Transition{Kind: models.TransitionIris, Duration: time.Second}, OutputScreen: 2, Logo: models.Background{Kind: models.BackgroundColor}, InterfaceColor: ColorLight},
			Settings{Transition: models.Transition{Kind: models.TransitionIris, Duration: time.Second}, OutputScreen: 2, Logo: models.Background{Kind: models.BackgroundColor}, InterfaceColor: ColorLight}},
		{"negative screen", Settings{OutputScreen: -7, InterfaceColor: "blue"}, Settings{Transition: models.Transition{Kind: models.TransitionBlend}, OutputScreen: AutoScreen, Logo: LogoGradient(), InterfaceColor: ColorDark}},

		{"pause on black is kept", Settings{PauseOnBlack: true}, Settings{Transition: models.Transition{Kind: models.TransitionBlend}, Logo: LogoGradient(), InterfaceColor: ColorDark, PauseOnBlack: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.want.Alerts = models.DefaultAlertStyles() // missing alert styles get the defaults
			tt.want.Hotkeys = DefaultKeymap()            // and missing hotkeys theirs
			if got := tt.in.Normalized(""); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
	// JSON round trip (the /state document and the control API).
	b, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	var back Settings
	if err := json.Unmarshal(b, &back); err != nil || !reflect.DeepEqual(back, def) {
		t.Fatalf("round trip: %s -> %+v (%v)", b, back, err)
	}
}

func TestParseInterfaceColor(t *testing.T) {
	for in, want := range map[string]string{"dark": ColorDark, " Light ": ColorLight, "DARK": ColorDark} {
		if got, err := ParseInterfaceColor(in); err != nil || got != want {
			t.Errorf("ParseInterfaceColor(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseInterfaceColor("blue"); err == nil {
		t.Error("blue accepted")
	}
}

func TestOutputScreenFor(t *testing.T) {
	tests := []struct {
		setting, n, primary, want int
	}{
		{AutoScreen, 1, 0, 0},  // one display: the primary
		{AutoScreen, 2, 0, 1},  // two displays: the non-primary one
		{AutoScreen, 2, 1, 0},  // the primary is the second display
		{1, 2, 0, 1},           // configured and present
		{0, 2, 0, 0},           // the primary may be chosen explicitly
		{3, 2, 0, 1},           // configured display unplugged: automatic
		{AutoScreen, 0, 0, 0},  // no displays reported
		{AutoScreen, 3, -1, 0}, // unknown primary
	}
	for _, tt := range tests {
		if got := OutputScreenFor(tt.setting, tt.n, tt.primary); got != tt.want {
			t.Errorf("OutputScreenFor(%d, %d, %d) = %d, want %d", tt.setting, tt.n, tt.primary, got, tt.want)
		}
	}
}

func TestScreenAndLogoLabels(t *testing.T) {
	if got := ScreenLabel(1, 1920, 1080, false); got != "Display 2 — 1920×1080" {
		t.Errorf("ScreenLabel = %q", got)
	}
	if got := ScreenLabel(0, 3024, 1964, true); got != "Display 1 — 3024×1964, primary" {
		t.Errorf("ScreenLabel primary = %q", got)
	}
	if got := LogoDescription(LogoGradient()); got != "Default gradient" {
		t.Errorf("LogoDescription(gradient) = %q", got)
	}
	if got := LogoDescription(models.Background{Kind: models.BackgroundImage, Path: "/a/b/logo.png"}); got != "logo.png" {
		t.Errorf("LogoDescription(image) = %q", got)
	}
	if got := LogoFromPath("", ""); got != LogoGradient() {
		t.Errorf("LogoFromPath(\"\") = %+v", got)
	}
	if got := LogoFromPath("/x/y.jpg", ""); got.Kind != models.BackgroundImage || got.Path != "/x/y.jpg" || got.Color != LogoGradient().Color {
		t.Errorf("LogoFromPath = %+v", got)
	}
}

func TestBindings(t *testing.T) {
	rows := Bindings()
	byAction := map[string]string{}
	for _, r := range rows {
		if r.Action == "" || r.Keys == "" {
			t.Errorf("empty row %+v", r)
		}
		if _, dup := byAction[r.Action]; dup {
			t.Errorf("duplicate action %q", r.Action)
		}
		byAction[r.Action] = r.Keys
	}
	want := map[string]string{
		"Next slide":                         "→ / Space / N",
		"Previous slide":                     "← / Backspace / P",
		"First slide":                        "Home",
		"Go live":                            "Enter",
		"Go live with the next item":         "Page Down",
		"Black":                              "Ctrl+B",
		"Clear":                              "Ctrl+C",
		"Logo":                               "Ctrl+L",
		"Jump to Verse (then a number)":      "V",
		"Jump to Pre-Chorus (then a number)": "R",
		"Jump to slide number (then digits)": "S",
	}
	for action, keys := range want {
		if got := byAction[action]; got != keys {
			t.Errorf("%s: keys = %q, want %q", action, got, keys)
		}
	}
	// Every default key is listed.
	all := map[string]bool{}
	for _, r := range rows {
		for _, l := range r.Labels {
			all[l] = true
		}
	}
	for _, a := range HotkeyActions() {
		for _, c := range a.def {
			if !all[c.Label()] {
				t.Errorf("%s: key %s is not listed", a.ID, c.Label())
			}
		}
	}
	if !reflect.DeepEqual(rows, Bindings()) {
		t.Error("Bindings is not deterministic")
	}
}
