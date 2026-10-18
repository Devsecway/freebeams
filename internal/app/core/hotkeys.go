// Package core holds the pure, unit-testable logic of the FreeWorship app:
// the hotkey state machine, schedule item ↔ deck conversion, the control
// state snapshot and the async generation counter. It must not import
// graphics.gd classdb packages (go test cannot link them).
package core

import (
	"time"

	"freeworship/internal/models"
)

// KeyEvent is a key press, normalised away from Godot. Key is a lower-case
// name: "a".."z", "0".."9", "right", "left", "up", "down", "home", "end",
// "pageup", "pagedown", "space", "backspace", "enter", "delete", "escape",
// "tab". The control server's `key` action builds the same events, so
// synthesized keys go through exactly the same code path as real ones.
type KeyEvent struct {
	Key   string
	Ctrl  bool
	Meta  bool // ⌘ on macOS
	Shift bool
	Alt   bool
	Echo  bool // OS key repeat
}

// Focus describes where keyboard focus is, which decides whether a key is a
// hotkey or belongs to the focused widget.
type Focus int

const (
	// FocusNone: nothing focused, the Live grid, or a button. Every hotkey works.
	FocusNone Focus = iota
	// FocusText: a LineEdit / TextEdit. No hotkeys at all (⌘C copies, typing types).
	FocusText
	// FocusDialog: a dialog window is open. No hotkeys.
	FocusDialog
	// FocusList: a vertical list (song list, verse list, schedule). The list keeps
	// Up/Down/Home/End/Enter/Delete for its own navigation; the rest are hotkeys.
	FocusList
	// FocusGrid: the Preview slide grid. It keeps the arrows, Home/End and
	// Enter (select a preview slide / go live with it).
	FocusGrid
)

// String names the focus context (for logs and /state).
func (f Focus) String() string {
	switch f {
	case FocusText:
		return "text"
	case FocusDialog:
		return "dialog"
	case FocusList:
		return "list"
	case FocusGrid:
		return "grid"
	}
	return "none"
}

// Action is what a hotkey asks the app to do.
type Action int

const (
	ActNone Action = iota
	ActNext
	ActPrev
	ActFirst
	ActLast
	ActScheduleNext // select the next schedule item into Preview
	ActSchedulePrev
	ActGoLiveNext // select the next schedule item and go live with it
	ActGoLivePrev
	ActBlack
	ActClear
	ActLogo
	ActGoLive
	ActJumpSection // Section, N (0 = first slide of the section)
	ActJumpSlide   // N (1-based slide number)
)

var actionNames = [...]string{"none", "next", "prev", "first", "last", "schedule-next", "schedule-prev",
	"golive-next", "golive-prev", "black", "clear", "logo", "golive", "jump-section", "jump-slide"}

func (a Action) String() string {
	if a < 0 || int(a) >= len(actionNames) {
		return "unknown"
	}
	return actionNames[a]
}

// Command is the result of a key press.
type Command struct {
	Action  Action
	Section models.SectionKind // ActJumpSection
	N       int                // section number (ActJumpSection) or slide number (ActJumpSlide)
}

// DigitTimeout is how long after a section key (V, C, …, S) a digit still
// selects the section / slide number.
const DigitTimeout = time.Second

// maxJumpNumber bounds multi-digit numbers (S 1 2 = slide 12).
const maxJumpNumber = 999

// sectionKeys are the EasyWorship section jump keys (HotKeys.2.ini).
var sectionKeys = map[string]models.SectionKind{
	"v": models.SectionVerse,
	"c": models.SectionChorus,
	"b": models.SectionBridge,
	"r": models.SectionPreChorus,
	"t": models.SectionTag,
	"i": models.SectionIntro,
	"e": models.SectionEnding,
}

// plainKeys are the unmodified hotkeys (UI-SPEC §3.7, HotKeys.2.ini).
var plainKeys = map[string]Action{
	"right": ActNext, "space": ActNext, "n": ActNext,
	"left": ActPrev, "backspace": ActPrev, "p": ActPrev,
	"home": ActFirst, "end": ActLast,
	"down": ActScheduleNext, "up": ActSchedulePrev,
	"pagedown": ActGoLiveNext, "pageup": ActGoLivePrev,
	"enter": ActGoLive,
}

// ctrlKeys are the Ctrl (or ⌘) hotkeys.
var ctrlKeys = map[string]Action{"b": ActBlack, "c": ActClear, "l": ActLogo}

// repeatable keys keep working while held (natural key repeat).
var repeatable = map[string]bool{"right": true, "left": true, "up": true, "down": true}

// passThrough lists, per focus context, the keys the focused widget keeps.
var passThrough = map[Focus]map[string]bool{
	FocusList: {"up": true, "down": true, "home": true, "end": true, "enter": true, "delete": true},
	FocusGrid: {"up": true, "down": true, "left": true, "right": true, "home": true, "end": true, "enter": true},
}

// Hotkeys maps key presses to commands. It is a small state machine: a
// section key (V, C, B, R, T, I, E) jumps immediately and then waits up to
// DigitTimeout for a number (V then 2 = Verse 2); S waits for a slide number.
// Further digits within the timeout extend the number (S 1 2 = slide 12).
type Hotkeys struct {
	// Now is the clock (injected for tests); nil means time.Now.
	Now func() time.Time

	pending  string // "" or a section key / "s" awaiting a digit
	number   int
	deadline time.Time
}

func (h *Hotkeys) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// Reset forgets a pending section/slide number.
func (h *Hotkeys) Reset() { h.pending, h.number = "", 0 }

// Pending returns the key waiting for a digit ("" when none).
func (h *Hotkeys) Pending() string { return h.pending }

// Handle maps a key press in the given focus context to a command. consumed
// reports whether the key is a hotkey that the app takes (the event must then
// not reach the focused widget); a command with ActNone and consumed true is a
// swallowed key (e.g. a repeat of a non-repeatable hotkey, or S waiting for a
// number).
func (h *Hotkeys) Handle(ev KeyEvent, focus Focus) (cmd Command, consumed bool) {
	if focus == FocusText || focus == FocusDialog {
		h.Reset()
		return Command{}, false
	}
	now := h.now()
	if h.pending != "" && now.After(h.deadline) {
		h.Reset()
	}
	key := ev.Key

	if ev.Ctrl || ev.Meta {
		h.Reset()
		if ev.Alt {
			return Command{}, false
		}
		a, ok := ctrlKeys[key]
		if !ok {
			return Command{}, false // e.g. ⌘Q, ⌘S: not ours
		}
		if ev.Echo {
			return Command{}, true
		}
		return Command{Action: a}, true
	}
	if ev.Alt {
		return Command{}, false
	}

	if d, ok := digit(key); ok {
		if h.pending == "" {
			return Command{}, false
		}
		if ev.Echo {
			return Command{}, true
		}
		if n := h.number*10 + d; n <= maxJumpNumber {
			h.number = n
		}
		h.deadline = now.Add(DigitTimeout)
		if h.pending == "s" {
			return Command{Action: ActJumpSlide, N: h.number}, true
		}
		return Command{Action: ActJumpSection, Section: sectionKeys[h.pending], N: h.number}, true
	}

	if passThrough[focus][key] {
		h.Reset()
		return Command{}, false
	}

	if kind, ok := sectionKeys[key]; ok {
		if ev.Echo {
			return Command{}, true
		}
		h.pending, h.number, h.deadline = key, 0, now.Add(DigitTimeout)
		return Command{Action: ActJumpSection, Section: kind}, true
	}
	if key == "s" {
		if ev.Echo {
			return Command{}, true
		}
		h.pending, h.number, h.deadline = "s", 0, now.Add(DigitTimeout)
		return Command{}, true
	}

	a, ok := plainKeys[key]
	if !ok {
		return Command{}, false
	}
	h.Reset()
	if ev.Echo && !repeatable[key] {
		return Command{}, true
	}
	return Command{Action: a}, true
}

// digit parses "0".."9".
func digit(key string) (int, bool) {
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		return int(key[0] - '0'), true
	}
	return 0, false
}
