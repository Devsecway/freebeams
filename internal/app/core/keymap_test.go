package core

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"freebeams/internal/models"
)

func TestParseChord(t *testing.T) {
	tests := []struct {
		in    string
		want  Chord
		str   string
		label string
		err   bool
	}{
		{"right", Chord{Key: "right"}, "right", "→", false},
		{"→", Chord{Key: "right"}, "right", "→", false},
		{"ArrowLeft", Chord{Key: "left"}, "left", "←", false},
		{"N", Chord{Key: "n"}, "n", "N", false},
		{"Ctrl+B", Chord{Key: "b", Ctrl: true}, "ctrl+b", "Ctrl+B", false},
		{"cmd+b", Chord{Key: "b", Ctrl: true}, "ctrl+b", "Ctrl+B", false},
		{"⌘+L", Chord{Key: "l", Ctrl: true}, "ctrl+l", "Ctrl+L", false},
		{"Meta+Shift+K", Chord{Key: "k", Ctrl: true, Shift: true}, "ctrl+shift+k", "Ctrl+Shift+K", false},
		{"shift+alt+ctrl+f5", Chord{Key: "f5", Ctrl: true, Alt: true, Shift: true}, "ctrl+alt+shift+f5", "Ctrl+Alt+Shift+F5", false},
		{"Option+G", Chord{Key: "g", Alt: true}, "alt+g", "Alt+G", false},
		{"Page Down", Chord{Key: "pagedown"}, "pagedown", "Page Down", false},
		{"PgUp", Chord{Key: "pageup"}, "pageup", "Page Up", false},
		{"Space", Chord{Key: "space"}, "space", "Space", false},
		{"F12", Chord{Key: "f12"}, "f12", "F12", false},
		{"F13", Chord{}, "", "", true},
		{"f0", Chord{}, "", "", true},
		{"Hyper+B", Chord{}, "", "", true},
		{"Ctrl+", Chord{}, "", "", true},
		{"", Chord{}, "", "", true},
		{"volumeup", Chord{}, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
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
			// The stored form and the label both parse back.
			for _, s := range []string{got.String(), got.Label()} {
				if back, err := ParseChord(s); err != nil || back != got {
					t.Errorf("ParseChord(%q) = %+v, %v", s, back, err)
				}
			}
		})
	}
}

func TestKeymapJSON(t *testing.T) {
	km := DefaultKeymap()
	km, err := km.WithKey("next", Chord{Key: "f5", Shift: true}, false)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(km)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"next":["right","space","n","shift+f5"]`) || !strings.Contains(string(b), `"black":["ctrl+b"]`) {
		t.Fatalf("JSON = %s", b)
	}
	var back Keymap
	if err := json.Unmarshal(b, &back); err != nil || !back.Equal(km) {
		t.Fatalf("round trip = %v, %v", back, err)
	}
	// Lenient: unknown actions and unreadable or reserved keys are dropped,
	// missing actions get their defaults, an emptied action stays empty.
	var stored Keymap
	if err := json.Unmarshal([]byte(`{"next":["x","bogus+q","ctrl+q","3"],"prev":[],"spin":["k"]}`), &stored); err != nil {
		t.Fatal(err)
	}
	got := stored.Normalized()
	if !reflect.DeepEqual(got["next"], []Chord{{Key: "x"}}) || len(got["prev"]) != 0 || got["spin"] != nil ||
		!reflect.DeepEqual(got["black"], []Chord{{Key: "b", Ctrl: true}}) {
		t.Fatalf("normalized = %v", got)
	}
	if err := json.Unmarshal([]byte(`[1]`), &stored); err == nil {
		t.Fatal("a JSON array was accepted")
	}
	// A nil keymap is the defaults; a chord bound twice stays with the
	// first action in table order.
	if !Keymap(nil).Normalized().Equal(DefaultKeymap()) {
		t.Fatal("nil keymap is not the defaults")
	}
	dup := DefaultKeymap()
	dup["logo"] = []Chord{{Key: "n"}}
	if n := dup.Normalized(); len(n["logo"]) != 0 || n.Owner(Chord{Key: "n"}) != "next" {
		t.Fatalf("duplicate chord: %v", n)
	}
}

func TestReserved(t *testing.T) {
	tests := []struct {
		c        Chord
		reserved bool
	}{
		{Chord{Key: "q", Ctrl: true}, true},
		{Chord{Key: "v", Ctrl: true}, true},
		{Chord{Key: "x", Ctrl: true}, true},
		{Chord{Key: "a", Ctrl: true}, true},
		{Chord{Key: "z", Ctrl: true, Shift: true}, true},
		{Chord{Key: "s", Ctrl: true, Shift: true}, true},
		{Chord{Key: "w", Ctrl: true}, true},
		{Chord{Key: "escape"}, true},
		{Chord{Key: "tab"}, true},
		{Chord{Key: "tab", Shift: true}, true},
		{Chord{Key: "tab", Ctrl: true}, true},
		{Chord{Key: "5"}, true},
		{Chord{Key: "5", Shift: true}, true},
		{Chord{Key: "c", Ctrl: true}, false}, // EasyWorship's Clear (hotkeys are off in text fields)
		{Chord{Key: "b", Ctrl: true}, false},
		{Chord{Key: "q"}, false},
		{Chord{Key: "q", Alt: true}, false},
		{Chord{Key: "5", Ctrl: true}, false},
		{Chord{Key: "f5"}, false},
	}
	for _, tt := range tests {
		why, got := Reserved(tt.c)
		if got != tt.reserved || got != (why != "") {
			t.Errorf("Reserved(%s) = %q, %v; want %v", tt.c, why, got, tt.reserved)
		}
	}
	km := DefaultKeymap()
	if _, err := km.WithKey("next", Chord{Key: "q", Ctrl: true}, true); !errors.Is(err, ErrReserved) {
		t.Errorf("⌘Q accepted: %v", err)
	}
}

func TestKeymapEditing(t *testing.T) {
	def := DefaultKeymap()
	f := Chord{Key: "f"}
	ctrlB := Chord{Key: "b", Ctrl: true}

	km, err := def.WithKey("next", f, false)
	if err != nil || !reflect.DeepEqual(km["next"], []Chord{{Key: "right"}, {Key: "space"}, {Key: "n"}, f}) {
		t.Fatalf("add F: %v %v", km["next"], err)
	}
	if !def.Equal(DefaultKeymap()) {
		t.Fatal("WithKey changed its receiver")
	}
	if _, err := km.WithKey("next", f, false); !errors.Is(err, ErrAlreadyBound) {
		t.Errorf("adding F twice: %v", err)
	}

	// A conflict names the owner; reassign moves the key.
	_, err = km.WithKey("next", ctrlB, false)
	var conflict *ConflictError
	if !errors.As(err, &conflict) || conflict.Action != "black" || conflict.Want != "next" || conflict.Chord != ctrlB {
		t.Fatalf("conflict = %v", err)
	}
	if msg := err.Error(); msg != "Ctrl+B is already assigned to Black" {
		t.Errorf("conflict message %q", msg)
	}
	moved, err := km.WithKey("next", ctrlB, true)
	if err != nil || moved.Owner(ctrlB) != "next" || len(moved["black"]) != 0 {
		t.Fatalf("reassign: %v %v", moved, err)
	}

	// Remove.
	if rm := moved.WithoutKey("next", Chord{Key: "space"}); rm.Owner(Chord{Key: "space"}) != "" || len(rm["next"]) != 4 {
		t.Errorf("remove space: %v", rm["next"])
	}

	// WithKeys replaces the list; a duplicate in the list is ignored; an
	// error changes nothing.
	set, err := def.WithKeys("next", []Chord{f, f, {Key: "g"}}, false)
	if err != nil || !reflect.DeepEqual(set["next"], []Chord{f, {Key: "g"}}) || set.Owner(Chord{Key: "right"}) != "" {
		t.Fatalf("set keys: %v %v", set["next"], err)
	}
	if same, err := def.WithKeys("next", []Chord{f, ctrlB}, false); err == nil || !same.Equal(def) {
		t.Errorf("set keys with a conflict: %v %v", same["next"], err)
	}
	if _, err := def.WithKeys("spin", nil, false); err == nil {
		t.Error("unknown action accepted")
	}
	if _, err := def.WithKey("next", Chord{Key: "volumeup"}, false); err == nil {
		t.Error("unknown key accepted")
	}
}

func TestParseHotkeyAction(t *testing.T) {
	for in, want := range map[string]string{"next": "next", "Next slide": "next", "JUMP-VERSE": "jump-verse", "Black": "black"} {
		if got, err := ParseHotkeyAction(in); err != nil || got != want {
			t.Errorf("ParseHotkeyAction(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseHotkeyAction("spin"); err == nil {
		t.Error("spin accepted")
	}
}

// TestHotkeysCustom runs the state machine with edited bindings.
func TestHotkeysCustom(t *testing.T) {
	none := Command{}
	km := DefaultKeymap()
	must := func(k Keymap, err error) Keymap {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	km = must(km.WithKeys("next", []Chord{{Key: "f"}, {Key: "right"}}, false))
	km = must(km.WithKeys("jump-verse", []Chord{{Key: "q"}}, false))                        // V is free now
	km = must(km.WithKeys("jump-slide", []Chord{{Key: "g"}, {Key: "j", Alt: true}}, false)) // S too
	km = must(km.WithKey("black", Chord{Key: "f2"}, false))
	km = must(km.WithKey("logo", Chord{Key: "k", Shift: true}, false))
	km = must(km.WithKey("prev", Chord{Key: "c", Ctrl: true}, true)) // Ctrl+C moves from Clear to Prev
	km = km.WithoutKey("prev", Chord{Key: "backspace"})

	tests := []struct {
		name  string
		steps []step
	}{
		{"rebound next", []step{
			{0, k("f"), FocusNone, cmd(ActNext), true},
			{0, k("right"), FocusNone, cmd(ActNext), true},
			{0, k("n"), FocusNone, none, false},
			{0, k("space"), FocusNone, none, false},
		}},
		{"custom verse key takes a number", []step{
			{0, k("q"), FocusNone, jump(models.SectionVerse, 0), true},
			{300, k("2"), FocusNone, jump(models.SectionVerse, 2), true},
			{600, k("1"), FocusNone, jump(models.SectionVerse, 21), true},
		}},
		{"the old verse key is free", []step{
			{0, k("v"), FocusNone, none, false},
			{100, k("2"), FocusNone, none, false},
		}},
		{"custom slide key takes digits", []step{
			{0, k("g"), FocusNone, none, true},
			{300, k("1"), FocusNone, slide(1), true},
			{600, k("2"), FocusNone, slide(12), true},
			{1700, k("3"), FocusNone, none, false},
		}},
		{"a modified jump key", []step{
			{0, KeyEvent{Key: "j", Alt: true}, FocusNone, none, true},
			{200, k("7"), FocusNone, slide(7), true},
		}},
		{"chorus still on C, number restarts", []step{
			{0, k("q"), FocusNone, jump(models.SectionVerse, 0), true},
			{100, k("c"), FocusNone, jump(models.SectionChorus, 0), true},
			{200, k("3"), FocusNone, jump(models.SectionChorus, 3), true},
		}},
		{"plain function key and shift chord", []step{
			{0, k("f2"), FocusNone, cmd(ActBlack), true},
			{0, KeyEvent{Key: "k", Shift: true}, FocusNone, cmd(ActLogo), true},
			{0, k("k"), FocusNone, none, false},
			{0, KeyEvent{Key: "f", Shift: true}, FocusNone, cmd(ActNext), true}, // shift ignored when unbound
		}},
		{"reassigned ctrl+c", []step{
			{0, ctrl("c"), FocusNone, cmd(ActPrev), true},
			{0, meta("c"), FocusNone, cmd(ActPrev), true},
			{0, meta("c"), FocusText, none, false}, // copies in a text field
		}},
		{"a modified unbound key cancels a number", []step{
			{0, k("q"), FocusNone, jump(models.SectionVerse, 0), true},
			{100, meta("s"), FocusNone, none, false},
			{200, k("2"), FocusNone, none, false},
		}},
		{"list keeps navigation keys even when bound", []step{
			{0, k("right"), FocusGrid, none, false},
			{0, k("f"), FocusGrid, cmd(ActNext), true},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
			now := start
			h := Hotkeys{Now: func() time.Time { return now }}
			h.SetKeymap(km)
			for i, s := range tt.steps {
				now = start.Add(time.Duration(s.at) * time.Millisecond)
				got, eaten := h.Handle(s.ev, s.focus)
				if got != s.want || eaten != s.eaten {
					t.Errorf("step %d %+v (focus %v): got %+v consumed=%v, want %+v consumed=%v",
						i, s.ev, s.focus, got, eaten, s.want, s.eaten)
				}
			}
		})
	}

	// Backspace was unbound from Prev: a list that ignores it gets nothing.
	var h Hotkeys
	h.SetKeymap(km)
	if got := h.Unhandled(k("backspace"), FocusList); got != none {
		t.Errorf("Unhandled(backspace) with Prev unbound = %+v", got)
	}
	if got, eaten := h.Handle(k("backspace"), FocusNone); eaten || got != none {
		t.Errorf("Handle(backspace) with Prev unbound = %+v %v", got, eaten)
	}
	if !h.Keymap().Equal(km.Normalized()) {
		t.Error("Keymap() is not what was set")
	}
	// Back to the defaults.
	h.SetKeymap(DefaultKeymap())
	if got := h.Unhandled(k("backspace"), FocusList); got != cmd(ActPrev) {
		t.Errorf("default Unhandled(backspace) = %+v", got)
	}
}

func TestParseEWHotkeys(t *testing.T) {
	// EasyWorship 8's own file (BOM, CRLF), with every default.
	ew := "\ufeff[HotKeys]\r\nBlackScreenKeys=C-66\r\nClearScreenKeys=C-67\r\nGoLiveNextKeys=34\r\nGoLivePrevKeys=33\r\n" +
		"JumpBridgeKeys=66\r\nJumpChorusKeys=67\r\nJumpEndingKeys=69\r\nJumpIntroKeys=73\r\nJumpPreChorusKeys=82\r\n" +
		"JumpSlideKeys=83\r\nJumpTagKeys=84\r\nJumpVerseKeys=86\r\nLogoScreenKeys=C-76\r\nScheduleNextKeys=40\r\n" +
		"SchedulePrevKeys=38\r\nSlideFirstKeys=36\r\nSlideLastKeys=35\r\nSlideNextKeys=78, 39, 32\r\nSlidePrevKeys=80, 37, 8\r\n" +
		"\r\n[Settings]\r\nAllow_Live_Navigation_From_Schedule=1\r\n"
	km, skipped, err := ParseEWHotkeys(ew, DefaultKeymap())
	if err != nil || len(skipped) != 0 {
		t.Fatalf("err %v, skipped %v", err, skipped)
	}
	if !sameKeys(km, DefaultKeymap()) {
		t.Fatalf("EasyWorship's defaults differ from ours:\n%v\n%v", km, DefaultKeymap())
	}

	// Custom keys: F5 (0x74), Shift/Alt prefixes, keypad digits are refused
	// (numbers), an unknown code and a reserved chord are skipped; Go Live
	// keeps the base's keys.
	custom := "[HotKeys]\nSlideNextKeys=116, S-78\nBlackScreenKeys=A-C-66\nJumpVerseKeys=96\nLogoScreenKeys=C-81\nSlidePrevKeys=999, x\n"
	base := DefaultKeymap()
	base["golive"] = []Chord{{Key: "f9"}}
	km, skipped, err = ParseEWHotkeys(custom, base)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(km["next"], []Chord{{Key: "f5"}, {Key: "n", Shift: true}}) ||
		!reflect.DeepEqual(km["black"], []Chord{{Key: "b", Ctrl: true, Alt: true}}) ||
		len(km["jump-verse"]) != 0 || len(km["logo"]) != 0 || len(km["prev"]) != 0 ||
		!reflect.DeepEqual(km["golive"], []Chord{{Key: "f9"}}) || !reflect.DeepEqual(km["clear"], []Chord{{Key: "c", Ctrl: true}}) {
		t.Fatalf("custom = %v", km)
	}
	if len(skipped) != 4 {
		t.Errorf("skipped = %q", skipped)
	}
	if _, _, err := ParseEWHotkeys("[Settings]\nX=1\n", base); err == nil {
		t.Error("a file without hotkeys was accepted")
	}
}

// sameKeys compares keymaps ignoring the order of each action's keys.
func sameKeys(a, b Keymap) bool {
	if len(a) != len(b) {
		return false
	}
	for id, cs := range a {
		if len(cs) != len(b[id]) {
			return false
		}
		for _, c := range cs {
			if b.Owner(c) != id {
				return false
			}
		}
	}
	return true
}
