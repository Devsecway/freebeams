package core

import (
	"testing"
	"time"

	"freebeams/internal/models"
)

// step is one key press at a time offset (ms since the start).
type step struct {
	at    int // ms
	ev    KeyEvent
	focus Focus
	want  Command
	eaten bool // consumed
}

func k(name string) KeyEvent      { return KeyEvent{Key: name} }
func ctrl(name string) KeyEvent   { return KeyEvent{Key: name, Ctrl: true} }
func meta(name string) KeyEvent   { return KeyEvent{Key: name, Meta: true} }
func repeat(name string) KeyEvent { return KeyEvent{Key: name, Echo: true} }

func cmd(a Action) Command { return Command{Action: a} }
func jump(kind models.SectionKind, n int) Command {
	return Command{Action: ActJumpSection, Section: kind, N: n}
}
func slide(n int) Command { return Command{Action: ActJumpSlide, N: n} }

func TestHotkeys(t *testing.T) {
	none := Command{}
	tests := []struct {
		name  string
		steps []step
	}{
		{"next keys", []step{
			{0, k("right"), FocusNone, cmd(ActNext), true},
			{0, k("space"), FocusNone, cmd(ActNext), true},
			{0, k("n"), FocusNone, cmd(ActNext), true},
		}},
		{"prev keys", []step{
			{0, k("left"), FocusNone, cmd(ActPrev), true},
			{0, k("backspace"), FocusNone, cmd(ActPrev), true},
			{0, k("p"), FocusNone, cmd(ActPrev), true},
		}},
		{"first last", []step{
			{0, k("home"), FocusNone, cmd(ActFirst), true},
			{0, k("end"), FocusNone, cmd(ActLast), true},
		}},
		{"schedule and go-live navigation", []step{
			{0, k("down"), FocusNone, cmd(ActScheduleNext), true},
			{0, k("up"), FocusNone, cmd(ActSchedulePrev), true},
			{0, k("pagedown"), FocusNone, cmd(ActGoLiveNext), true},
			{0, k("pageup"), FocusNone, cmd(ActGoLivePrev), true},
		}},
		{"enter goes live", []step{{0, k("enter"), FocusNone, cmd(ActGoLive), true}}},
		{"ctrl and cmd toggles", []step{
			{0, ctrl("b"), FocusNone, cmd(ActBlack), true},
			{0, ctrl("c"), FocusNone, cmd(ActClear), true},
			{0, ctrl("l"), FocusNone, cmd(ActLogo), true},
			{0, meta("b"), FocusNone, cmd(ActBlack), true},
			{0, meta("c"), FocusNone, cmd(ActClear), true},
			{0, meta("l"), FocusNone, cmd(ActLogo), true},
		}},
		{"other shortcuts pass through", []step{
			{0, meta("q"), FocusNone, none, false},
			{0, ctrl("s"), FocusNone, none, false},
			{0, KeyEvent{Key: "b", Ctrl: true, Alt: true}, FocusNone, none, false},
			{0, KeyEvent{Key: "n", Alt: true}, FocusNone, none, false},
		}},
		{"unknown keys pass through", []step{
			{0, k("x"), FocusNone, none, false},
			{0, k("f5"), FocusNone, none, false},
			{0, k("5"), FocusNone, none, false}, // digit without a pending section
		}},
		{"text focus disables everything (cmd+c copies)", []step{
			{0, meta("c"), FocusText, none, false},
			{0, ctrl("b"), FocusText, none, false},
			{0, k("space"), FocusText, none, false},
			{0, k("v"), FocusText, none, false},
			{0, k("right"), FocusText, none, false},
		}},
		{"dialog focus disables everything", []step{
			{0, k("n"), FocusDialog, none, false},
			{0, ctrl("l"), FocusDialog, none, false},
		}},
		{"list keeps its navigation keys", []step{
			{0, k("up"), FocusList, none, false},
			{0, k("down"), FocusList, none, false},
			{0, k("home"), FocusList, none, false},
			{0, k("end"), FocusList, none, false},
			{0, k("enter"), FocusList, none, false},
			{0, k("delete"), FocusList, none, false},
			{0, k("backspace"), FocusList, none, false}, // remove / delete in the list
			{0, k("right"), FocusList, cmd(ActNext), true},
			{0, k("space"), FocusList, cmd(ActNext), true},
			{0, k("pagedown"), FocusList, cmd(ActGoLiveNext), true},
			{0, ctrl("b"), FocusList, cmd(ActBlack), true},
		}},
		{"preview grid keeps arrows and enter", []step{
			{0, k("right"), FocusGrid, none, false},
			{0, k("left"), FocusGrid, none, false},
			{0, k("up"), FocusGrid, none, false},
			{0, k("enter"), FocusGrid, none, false},
			{0, k("space"), FocusGrid, cmd(ActNext), true},
			{0, k("n"), FocusGrid, cmd(ActNext), true},
		}},
		{"section keys jump immediately", []step{
			{0, k("v"), FocusNone, jump(models.SectionVerse, 0), true},
			{0, k("c"), FocusNone, jump(models.SectionChorus, 0), true},
			{0, k("b"), FocusNone, jump(models.SectionBridge, 0), true},
			{0, k("r"), FocusNone, jump(models.SectionPreChorus, 0), true},
			{0, k("t"), FocusNone, jump(models.SectionTag, 0), true},
			{0, k("i"), FocusNone, jump(models.SectionIntro, 0), true},
			{0, k("e"), FocusNone, jump(models.SectionEnding, 0), true},
		}},
		{"V then 2 within 1s is Verse 2", []step{
			{0, k("v"), FocusNone, jump(models.SectionVerse, 0), true},
			{400, k("2"), FocusNone, jump(models.SectionVerse, 2), true},
		}},
		{"digit after the timeout is ignored", []step{
			{0, k("c"), FocusNone, jump(models.SectionChorus, 0), true},
			{1001, k("2"), FocusNone, none, false},
		}},
		{"digit exactly at the timeout still counts", []step{
			{0, k("v"), FocusNone, jump(models.SectionVerse, 0), true},
			{1000, k("3"), FocusNone, jump(models.SectionVerse, 3), true},
		}},
		{"multi-digit numbers extend the deadline", []step{
			{0, k("v"), FocusNone, jump(models.SectionVerse, 0), true},
			{900, k("1"), FocusNone, jump(models.SectionVerse, 1), true},
			{1800, k("2"), FocusNone, jump(models.SectionVerse, 12), true},
			{2900, k("3"), FocusNone, none, false},
		}},
		{"S waits for a slide number", []step{
			{0, k("s"), FocusNone, none, true},
			{300, k("4"), FocusNone, slide(4), true},
			{600, k("2"), FocusNone, slide(42), true},
		}},
		{"S then timeout", []step{
			{0, k("s"), FocusNone, none, true},
			{1500, k("4"), FocusNone, none, false},
		}},
		{"another key cancels a pending number", []step{
			{0, k("v"), FocusNone, jump(models.SectionVerse, 0), true},
			{100, k("right"), FocusNone, cmd(ActNext), true},
			{200, k("2"), FocusNone, none, false},
		}},
		{"a new section key restarts the number", []step{
			{0, k("v"), FocusNone, jump(models.SectionVerse, 0), true},
			{100, k("1"), FocusNone, jump(models.SectionVerse, 1), true},
			{200, k("c"), FocusNone, jump(models.SectionChorus, 0), true},
			{300, k("2"), FocusNone, jump(models.SectionChorus, 2), true},
		}},
		{"text focus cancels a pending number", []step{
			{0, k("v"), FocusNone, jump(models.SectionVerse, 0), true},
			{100, k("2"), FocusText, none, false},
			{200, k("2"), FocusNone, none, false},
		}},
		{"arrows repeat, other keys do not", []step{
			{0, repeat("right"), FocusNone, cmd(ActNext), true},
			{0, repeat("left"), FocusNone, cmd(ActPrev), true},
			{0, repeat("down"), FocusNone, cmd(ActScheduleNext), true},
			{0, repeat("space"), FocusNone, none, true},
			{0, repeat("n"), FocusNone, none, true},
			{0, repeat("pagedown"), FocusNone, none, true},
			{0, repeat("v"), FocusNone, none, true},
			{0, KeyEvent{Key: "b", Ctrl: true, Echo: true}, FocusNone, none, true},
		}},
		{"shift is ignored", []step{
			{0, KeyEvent{Key: "n", Shift: true}, FocusNone, cmd(ActNext), true},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC)
			now := start
			h := Hotkeys{Now: func() time.Time { return now }}
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
}

// TestHotkeysUnhandled covers the second chance a passed-through key gets
// when the focused widget did not consume it.
func TestHotkeysUnhandled(t *testing.T) {
	tests := []struct {
		name  string
		ev    KeyEvent
		focus Focus
		want  Command
	}{
		{"backspace in a list that ignored it is Prev", k("backspace"), FocusList, cmd(ActPrev)},
		{"held backspace does not repeat", repeat("backspace"), FocusList, Command{}},
		{"cmd+backspace is not ours", meta("backspace"), FocusList, Command{}},
		{"backspace in a text field never falls back", k("backspace"), FocusText, Command{}},
		{"backspace in a dialog never falls back", k("backspace"), FocusDialog, Command{}},
		{"backspace without focus was already a hotkey", k("backspace"), FocusNone, Command{}},
		{"backspace in the preview grid was already a hotkey", k("backspace"), FocusGrid, Command{}},
		{"other list keys belong to the list", k("down"), FocusList, Command{}},
		{"delete has no hotkey", k("delete"), FocusList, Command{}},
		{"enter in the grid belongs to the grid", k("enter"), FocusGrid, Command{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h Hotkeys
			if got := h.Unhandled(tt.ev, tt.focus); got != tt.want {
				t.Errorf("Unhandled(%+v, %v) = %+v, want %+v", tt.ev, tt.focus, got, tt.want)
			}
		})
	}
}

func TestNormalizeKeyName(t *testing.T) {
	tests := map[string]string{
		"ArrowRight": "right", "Right": "right", "PgDn": "pagedown", "PageUp": "pageup",
		"V": "v", " ": "space", "Space": "space", "Return": "enter", "2": "2", "Backspace": "backspace",
	}
	for in, want := range tests {
		if got := NormalizeKeyName(in); got != want {
			t.Errorf("NormalizeKeyName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestActionString(t *testing.T) {
	if ActJumpSlide.String() != "jump-slide" || ActNone.String() != "none" || Action(99).String() != "unknown" {
		t.Fatal("unexpected action names")
	}
	if FocusGrid.String() != "grid" || FocusNone.String() != "none" {
		t.Fatal("unexpected focus names")
	}
}
