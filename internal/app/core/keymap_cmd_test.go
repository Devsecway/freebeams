package core

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// ⌘ and Control as separate modifiers (macOS) and folded together
// (Windows, Linux).

func TestParseChordModifiers(t *testing.T) {
	tests := []struct {
		mac   bool
		in    string
		want  Chord
		str   string
		label string
		err   bool
	}{
		{true, "cmd+b", Chord{Key: "b", Cmd: true}, "cmd+b", "⌘B", false},
		{true, "ctrl+b", Chord{Key: "b", Ctrl: true}, "ctrl+b", "⌃B", false},
		{true, "Control+B", Chord{Key: "b", Ctrl: true}, "ctrl+b", "⌃B", false},
		{true, "⌘+L", Chord{Key: "l", Cmd: true}, "cmd+l", "⌘L", false},
		{true, "⌘L", Chord{Key: "l", Cmd: true}, "cmd+l", "⌘L", false},
		{true, "⌃L", Chord{Key: "l", Ctrl: true}, "ctrl+l", "⌃L", false},
		{true, "⇧⌘S", Chord{Key: "s", Shift: true, Cmd: true}, "shift+cmd+s", "⇧⌘S", false},
		{true, "Meta+Shift+K", Chord{Key: "k", Shift: true, Cmd: true}, "shift+cmd+k", "⇧⌘K", false},
		{true, "command+ctrl+alt+shift+f5", Chord{Key: "f5", Ctrl: true, Alt: true, Shift: true, Cmd: true}, "ctrl+alt+shift+cmd+f5", "⌃⌥⇧⌘F5", false},
		{true, "Command+→", Chord{Key: "right", Cmd: true}, "cmd+right", "⌘→", false},
		{true, "⌥Space", Chord{Key: "space", Alt: true}, "alt+space", "⌥Space", false},
		{true, "Shift+F5", Chord{Key: "f5", Shift: true}, "shift+f5", "⇧F5", false},
		{true, "→", Chord{Key: "right"}, "right", "→", false},
		{true, "⌘", Chord{}, "", "", true},
		{true, "⌘+", Chord{}, "", "", true},
		// Off macOS ⌘ is Ctrl: a keymap from a Mac works there.
		{false, "cmd+b", Chord{Key: "b", Ctrl: true}, "ctrl+b", "Ctrl+B", false},
		{false, "⌘B", Chord{Key: "b", Ctrl: true}, "ctrl+b", "Ctrl+B", false},
		{false, "ctrl+cmd+b", Chord{Key: "b", Ctrl: true}, "ctrl+b", "Ctrl+B", false},
		{false, "⇧⌘S", Chord{Key: "s", Ctrl: true, Shift: true}, "ctrl+shift+s", "Ctrl+Shift+S", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			setMacModifiers(t, tt.mac)
			got, err := ParseChord(tt.in)
			if (err != nil) != tt.err {
				t.Fatalf("err = %v, want error %v", err, tt.err)
			}
			if tt.err {
				return
			}
			if got != tt.want || got.String() != tt.str || got.Label() != tt.label {
				t.Fatalf("got %+v (%q, %q), want %+v (%q, %q)", got, got.String(), got.Label(), tt.want, tt.str, tt.label)
			}
			for _, s := range []string{got.String(), got.Label()} {
				if back, err := ParseChord(s); err != nil || back != got {
					t.Errorf("ParseChord(%q) = %+v, %v", s, back, err)
				}
			}
		})
	}
}

func TestChordOfModifiers(t *testing.T) {
	tests := []struct {
		mac  bool
		ev   KeyEvent
		want Chord
	}{
		{true, KeyEvent{Key: "b", Meta: true}, Chord{Key: "b", Cmd: true}},
		{true, KeyEvent{Key: "b", Ctrl: true}, Chord{Key: "b", Ctrl: true}},
		{true, KeyEvent{Key: "b", Ctrl: true, Meta: true, Shift: true}, Chord{Key: "b", Ctrl: true, Cmd: true, Shift: true}},
		{false, KeyEvent{Key: "b", Meta: true}, Chord{Key: "b", Ctrl: true}}, // the Windows / Super key
		{false, KeyEvent{Key: "b", Ctrl: true}, Chord{Key: "b", Ctrl: true}},
	}
	for _, tt := range tests {
		setMacModifiers(t, tt.mac)
		if got := ChordOf(tt.ev); got != tt.want {
			t.Errorf("mac=%v ChordOf(%+v) = %+v, want %+v", tt.mac, tt.ev, got, tt.want)
		}
	}
}

func TestDefaultKeymapModifiers(t *testing.T) {
	setMacModifiers(t, true)
	km := DefaultKeymap()
	for id, key := range map[string]string{"black": "b", "clear": "c", "logo": "l"} {
		want := []Chord{{Key: key, Cmd: true}, {Key: key, Ctrl: true}}
		if !reflect.DeepEqual(km[id], want) {
			t.Errorf("mac default %s = %v, want %v", id, km[id], want)
		}
	}
	if !km.Normalized().Equal(km) {
		t.Error("the Mac defaults are not normalized")
	}
	b, _ := json.Marshal(km)
	if !strings.Contains(string(b), `"black":["cmd+b","ctrl+b"]`) {
		t.Errorf("JSON = %s", b)
	}
	for _, r := range km.Bindings() {
		if r.ID == "black" && r.Keys != "⌘B / ⌃B" {
			t.Errorf("Black row = %q", r.Keys)
		}
	}

	setMacModifiers(t, false)
	if km := DefaultKeymap(); !reflect.DeepEqual(km["black"], []Chord{{Key: "b", Ctrl: true}}) {
		t.Errorf("default black = %v", km["black"])
	}
}

func TestHotkeysModifiers(t *testing.T) {
	setMacModifiers(t, true)
	var h Hotkeys
	h.SetKeymap(DefaultKeymap())
	for _, ev := range []KeyEvent{{Key: "b", Meta: true}, {Key: "b", Ctrl: true}} {
		if got, ok := h.Handle(ev, FocusNone); !ok || got.Action != ActBlack {
			t.Errorf("default %+v = %+v %v, want Black", ev, got, ok)
		}
	}
	// ⌃B moves to Next; ⌘B stays Black.
	km, err := DefaultKeymap().WithKey("next", Chord{Key: "b", Ctrl: true}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(km["black"], []Chord{{Key: "b", Cmd: true}}) {
		t.Fatalf("black after the move = %v", km["black"])
	}
	h.SetKeymap(km)
	if got, _ := h.Handle(KeyEvent{Key: "b", Ctrl: true}, FocusNone); got.Action != ActNext {
		t.Errorf("⌃B = %+v, want Next", got)
	}
	if got, _ := h.Handle(KeyEvent{Key: "b", Meta: true}, FocusNone); got.Action != ActBlack {
		t.Errorf("⌘B = %+v, want Black", got)
	}
	if got, ok := h.Handle(KeyEvent{Key: "b", Ctrl: true, Meta: true}, FocusNone); ok {
		t.Errorf("⌃⌘B (unbound) = %+v", got)
	}
	// A conflict names the Mac key.
	if _, err := km.WithKey("prev", Chord{Key: "b", Cmd: true}, false); err == nil || err.Error() != "⌘B is already assigned to Black" {
		t.Errorf("conflict = %v", err)
	}

	// Off macOS ⌘ (the Super key) and Ctrl stay one chord.
	setMacModifiers(t, false)
	h.SetKeymap(DefaultKeymap())
	for _, ev := range []KeyEvent{{Key: "b", Meta: true}, {Key: "b", Ctrl: true}} {
		if got, ok := h.Handle(ev, FocusNone); !ok || got.Action != ActBlack {
			t.Errorf("portable %+v = %+v %v, want Black", ev, got, ok)
		}
	}
}

func TestReservedModifiers(t *testing.T) {
	tests := []struct {
		mac      bool
		c        Chord
		reserved bool
	}{
		{true, Chord{Key: "q", Cmd: true}, true},
		{true, Chord{Key: "q", Ctrl: true}, false}, // Control+Q is free on a Mac
		{true, Chord{Key: "s", Cmd: true, Shift: true}, true},
		{true, Chord{Key: "space", Cmd: true}, true},
		{true, Chord{Key: "space", Ctrl: true}, true}, // input sources
		{true, Chord{Key: "right", Ctrl: true}, true}, // Spaces
		{true, Chord{Key: "up", Ctrl: true}, true},    // Mission Control
		{true, Chord{Key: "right", Cmd: true}, false},
		{true, Chord{Key: "tab", Cmd: true}, true},
		{true, Chord{Key: "tab", Ctrl: true}, false},
		{true, Chord{Key: "c", Cmd: true}, false}, // Clear
		{true, Chord{Key: "5", Cmd: true}, false},
		{true, Chord{Key: "5"}, true},
		{false, Chord{Key: "q", Ctrl: true}, true},
		{false, Chord{Key: "q", Cmd: true}, true}, // folded into Ctrl
		{false, Chord{Key: "right", Ctrl: true}, false},
		{false, Chord{Key: "space", Ctrl: true}, true},
	}
	for _, tt := range tests {
		setMacModifiers(t, tt.mac)
		why, got := Reserved(tt.c)
		if got != tt.reserved || got != (why != "") {
			t.Errorf("mac=%v Reserved(%s) = %q, %v; want %v", tt.mac, tt.c, why, got, tt.reserved)
		}
	}
	setMacModifiers(t, true)
	if _, err := DefaultKeymap().WithKey("next", Chord{Key: "q", Cmd: true}, true); !errors.Is(err, ErrReserved) || err.Error() != "⌘Q is reserved (Quit)" {
		t.Errorf("⌘Q: %v", err)
	}
}

func TestMigrateKeymap(t *testing.T) {
	// Stored by a build where "ctrl" meant Ctrl or ⌘.
	legacy := `{"black":["ctrl+b"],"next":["right","ctrl+right"],"logo":["alt+ctrl+f5"],"clear":[]}`
	tests := []struct {
		mac  bool
		want map[string][]Chord
	}{
		{true, map[string][]Chord{
			"black": {{Key: "b", Cmd: true}, {Key: "b", Ctrl: true}},
			"next":  {{Key: "right"}, {Key: "right", Cmd: true}}, // ⌃→ is macOS's (Spaces)
			"logo":  {{Key: "f5", Alt: true, Cmd: true}, {Key: "f5", Ctrl: true, Alt: true}},
			"clear": {},
			"prev":  {{Key: "left"}, {Key: "backspace"}, {Key: "p"}}, // missing: the defaults
		}},
		{false, map[string][]Chord{
			"black": {{Key: "b", Ctrl: true}},
			"next":  {{Key: "right"}, {Key: "right", Ctrl: true}},
			"logo":  {{Key: "f5", Ctrl: true, Alt: true}},
			"clear": {},
			"prev":  {{Key: "left"}, {Key: "backspace"}, {Key: "p"}},
		}},
	}
	for _, tt := range tests {
		setMacModifiers(t, tt.mac)
		var km Keymap
		if err := json.Unmarshal([]byte(legacy), &km); err != nil {
			t.Fatal(err)
		}
		got := MigrateKeymap(km)
		for id, want := range tt.want {
			if !reflect.DeepEqual(got[id], want) {
				t.Errorf("mac=%v %s = %v, want %v", tt.mac, id, got[id], want)
			}
		}
		if !MigrateKeymap(nil).Equal(DefaultKeymap()) {
			t.Errorf("mac=%v: a missing legacy keymap is not the defaults", tt.mac)
		}
		// A migrated keymap stored again is read back unchanged (no second
		// migration through the new key).
		b, _ := json.Marshal(got)
		var back Keymap
		if err := json.Unmarshal(b, &back); err != nil || !back.Normalized().Equal(got) {
			t.Errorf("mac=%v round trip %s = %v, %v", tt.mac, b, back, err)
		}
	}
}

func TestParseEWHotkeysModifiers(t *testing.T) {
	setMacModifiers(t, true)
	// EasyWorship's "C-" keys are both ⌘ and Control on a Mac, like the defaults.
	ew := "[HotKeys]\nBlackScreenKeys=C-66\nClearScreenKeys=C-67\nLogoScreenKeys=C-76, C-81\n"
	km, skipped, err := ParseEWHotkeys(ew, DefaultKeymap())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(km["black"], []Chord{{Key: "b", Cmd: true}, {Key: "b", Ctrl: true}}) ||
		!reflect.DeepEqual(km["logo"], []Chord{{Key: "l", Cmd: true}, {Key: "l", Ctrl: true}, {Key: "q", Ctrl: true}}) {
		t.Fatalf("imported = %v", km)
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0], "⌘Q") { // ⌘Q is Quit; ⌃Q is free
		t.Errorf("skipped = %q", skipped)
	}
}
