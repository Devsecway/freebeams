package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"freebeams/internal/models"
)

// Editable hotkeys. A Keymap binds each hotkey action (HotkeyActions) to any
// number of key chords; core.Hotkeys looks key presses up in it. The defaults
// are EasyWorship's (HotKeys.2.ini) plus Enter for Go Live. The keymap is
// persisted under SettingHotkeys as {"next": ["right", "space", "n"], ...};
// an action missing from the stored map keeps its default keys, so actions
// added later get theirs.

// SettingHotkeys is the setting that holds the Keymap.
const SettingHotkeys = "settings.hotkeys"

// Chord is one key with its modifiers. Ctrl means Ctrl or ⌘: EasyWorship's
// Ctrl hotkeys are ⌘ on a Mac and both work, so they are one chord.
type Chord struct {
	Key   string // a core key name (KeyEvent.Key)
	Ctrl  bool
	Alt   bool
	Shift bool
}

// ChordOf is the chord of a key press.
func ChordOf(ev KeyEvent) Chord {
	return Chord{Key: ev.Key, Ctrl: ev.Ctrl || ev.Meta, Alt: ev.Alt, Shift: ev.Shift}
}

// String is the chord's stored form: lower-case modifiers in the order ctrl,
// alt, shift, then the key name ("ctrl+b", "right", "alt+shift+f5").
func (c Chord) String() string {
	var b strings.Builder
	if c.Ctrl {
		b.WriteString("ctrl+")
	}
	if c.Alt {
		b.WriteString("alt+")
	}
	if c.Shift {
		b.WriteString("shift+")
	}
	b.WriteString(c.Key)
	return b.String()
}

// Label is the chord's display name ("Ctrl+B", "→", "Shift+F5").
func (c Chord) Label() string {
	var b strings.Builder
	if c.Ctrl {
		b.WriteString("Ctrl+")
	}
	if c.Alt {
		b.WriteString("Alt+")
	}
	if c.Shift {
		b.WriteString("Shift+")
	}
	b.WriteString(KeyLabel(c.Key))
	return b.String()
}

// MarshalText writes the stored form.
func (c Chord) MarshalText() ([]byte, error) { return []byte(c.String()), nil }

// UnmarshalText reads any form ParseChord accepts.
func (c *Chord) UnmarshalText(b []byte) error {
	ch, err := ParseChord(string(b))
	if err != nil {
		return err
	}
	*c = ch
	return nil
}

// modifierNames maps the modifier spellings ParseChord accepts.
var modifierNames = map[string]string{
	"ctrl": "ctrl", "control": "ctrl", "cmd": "ctrl", "command": "ctrl", "meta": "ctrl", "⌘": "ctrl",
	"alt": "alt", "option": "alt", "opt": "alt", "⌥": "alt",
	"shift": "shift", "⇧": "shift",
}

// ParseChord reads a chord: "Ctrl+B", "cmd+b", "⌘+B", "Right", "→",
// "Shift+F5", "PgDn". Cmd and Meta are Ctrl (see Chord). The key must be one
// the hotkeys know (KnownKey).
func ParseChord(s string) (Chord, error) {
	parts := strings.Split(strings.TrimSpace(s), "+")
	if len(parts) == 0 || strings.TrimSpace(parts[len(parts)-1]) == "" {
		return Chord{}, fmt.Errorf("key %q: no key named", s)
	}
	var c Chord
	for _, m := range parts[:len(parts)-1] {
		switch modifierNames[strings.ToLower(strings.TrimSpace(m))] {
		case "ctrl":
			c.Ctrl = true
		case "alt":
			c.Alt = true
		case "shift":
			c.Shift = true
		default:
			return Chord{}, fmt.Errorf("key %q: unknown modifier %q", s, m)
		}
	}
	c.Key = keyFromLabel(parts[len(parts)-1])
	if !KnownKey(c.Key) {
		return Chord{}, fmt.Errorf("key %q: unknown key %q", s, parts[len(parts)-1])
	}
	return c, nil
}

// keyFromLabel reads a key name or its display label ("→", "Page Down").
func keyFromLabel(s string) string {
	t := strings.TrimSpace(s)
	for k, l := range keyLabels {
		if strings.EqualFold(l, t) {
			return k
		}
	}
	return NormalizeKeyName(strings.ReplaceAll(t, " ", ""))
}

// KnownKey reports whether name is a key the hotkeys can bind: a letter, a
// digit, F1–F12 or one of the named keys.
func KnownKey(name string) bool {
	if len(name) == 1 {
		c := name[0]
		return c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
	}
	if _, ok := keyLabels[name]; ok {
		return true
	}
	return fKey(name) > 0
}

// fKey returns n for "f<n>" with n in 1..12, else 0.
func fKey(name string) int {
	var n int
	if len(name) < 2 || name[0] != 'f' {
		return 0
	}
	if _, err := fmt.Sscanf(name[1:], "%d", &n); err != nil || fmt.Sprint(n) != name[1:] || n < 1 || n > 12 {
		return 0
	}
	return n
}

// HotkeyAction is one bindable action: a command, a section jump that then
// takes a number, or the slide-number jump.
type HotkeyAction struct {
	ID    string // stable name: the Keymap key and the control API's action
	Label string // the Options table's text
	EW    string // EasyWorship's HotKeys.2.ini entry ("SlideNextKeys")

	act     Action             // ActJumpSection / ActJumpSlide for the jumps
	section models.SectionKind // ActJumpSection
	def     []Chord
}

// jumps reports whether the action waits for a number afterwards.
func (h HotkeyAction) jumps() bool { return h.act == ActJumpSection || h.act == ActJumpSlide }

func plain(keys ...string) []Chord {
	out := make([]Chord, len(keys))
	for i, k := range keys {
		out[i] = Chord{Key: k}
	}
	return out
}

func ctrlKey(k string) []Chord { return []Chord{{Key: k, Ctrl: true}} }

// hotkeyActions lists the actions in the Options table's order, with their
// default keys (UI-SPEC §3.7, HotKeys.2.ini).
var hotkeyActions = []HotkeyAction{
	{ID: "next", Label: "Next slide", EW: "SlideNextKeys", act: ActNext, def: plain("right", "space", "n")},
	{ID: "prev", Label: "Previous slide", EW: "SlidePrevKeys", act: ActPrev, def: plain("left", "backspace", "p")},
	{ID: "first", Label: "First slide", EW: "SlideFirstKeys", act: ActFirst, def: plain("home")},
	{ID: "last", Label: "Last slide", EW: "SlideLastKeys", act: ActLast, def: plain("end")},
	{ID: "golive", Label: "Go live", act: ActGoLive, def: plain("enter")},
	{ID: "schedule-next", Label: "Next schedule item", EW: "ScheduleNextKeys", act: ActScheduleNext, def: plain("down")},
	{ID: "schedule-prev", Label: "Previous schedule item", EW: "SchedulePrevKeys", act: ActSchedulePrev, def: plain("up")},
	{ID: "golive-next", Label: "Go live with the next item", EW: "GoLiveNextKeys", act: ActGoLiveNext, def: plain("pagedown")},
	{ID: "golive-prev", Label: "Go live with the previous item", EW: "GoLivePrevKeys", act: ActGoLivePrev, def: plain("pageup")},
	{ID: "black", Label: "Black", EW: "BlackScreenKeys", act: ActBlack, def: ctrlKey("b")},
	{ID: "clear", Label: "Clear", EW: "ClearScreenKeys", act: ActClear, def: ctrlKey("c")},
	{ID: "logo", Label: "Logo", EW: "LogoScreenKeys", act: ActLogo, def: ctrlKey("l")},
	{ID: "jump-verse", Label: "Jump to Verse (then a number)", EW: "JumpVerseKeys", act: ActJumpSection, section: models.SectionVerse, def: plain("v")},
	{ID: "jump-chorus", Label: "Jump to Chorus (then a number)", EW: "JumpChorusKeys", act: ActJumpSection, section: models.SectionChorus, def: plain("c")},
	{ID: "jump-bridge", Label: "Jump to Bridge (then a number)", EW: "JumpBridgeKeys", act: ActJumpSection, section: models.SectionBridge, def: plain("b")},
	{ID: "jump-prechorus", Label: "Jump to Pre-Chorus (then a number)", EW: "JumpPreChorusKeys", act: ActJumpSection, section: models.SectionPreChorus, def: plain("r")},
	{ID: "jump-tag", Label: "Jump to Tag (then a number)", EW: "JumpTagKeys", act: ActJumpSection, section: models.SectionTag, def: plain("t")},
	{ID: "jump-intro", Label: "Jump to Intro (then a number)", EW: "JumpIntroKeys", act: ActJumpSection, section: models.SectionIntro, def: plain("i")},
	{ID: "jump-ending", Label: "Jump to Ending (then a number)", EW: "JumpEndingKeys", act: ActJumpSection, section: models.SectionEnding, def: plain("e")},
	{ID: "jump-slide", Label: "Jump to slide number (then digits)", EW: "JumpSlideKeys", act: ActJumpSlide, def: plain("s")},
}

// HotkeyActions returns the bindable actions in table order.
func HotkeyActions() []HotkeyAction { return slices.Clone(hotkeyActions) }

// hotkeyAction finds an action by id.
func hotkeyAction(id string) (HotkeyAction, bool) {
	for _, a := range hotkeyActions {
		if a.ID == id {
			return a, true
		}
	}
	return HotkeyAction{}, false
}

// ParseHotkeyAction resolves an action id or its table label
// (case-insensitive): "next", "Next slide", "jump-verse".
func ParseHotkeyAction(s string) (string, error) {
	t := strings.TrimSpace(s)
	for _, a := range hotkeyActions {
		if strings.EqualFold(a.ID, t) || strings.EqualFold(a.Label, t) {
			return a.ID, nil
		}
	}
	return "", fmt.Errorf("unknown hotkey action %q", s)
}

// HotkeyLabel is the table label of an action id (the id itself if unknown).
func HotkeyLabel(id string) string {
	if a, ok := hotkeyAction(id); ok {
		return a.Label
	}
	return id
}

// Keymap binds action ids to their key chords.
type Keymap map[string][]Chord

// DefaultKeymap returns EasyWorship's hotkeys.
func DefaultKeymap() Keymap {
	km := Keymap{}
	for _, a := range hotkeyActions {
		km[a.ID] = slices.Clone(a.def)
	}
	return km
}

// Clone returns a deep copy.
func (km Keymap) Clone() Keymap {
	out := make(Keymap, len(km))
	for id, cs := range km {
		out[id] = slices.Clone(cs)
	}
	return out
}

// Equal reports whether two keymaps bind the same keys in the same order.
func (km Keymap) Equal(o Keymap) bool {
	if len(km) != len(o) {
		return false
	}
	for id, cs := range km {
		if oc, ok := o[id]; !ok || !slices.Equal(cs, oc) {
			return false
		}
	}
	return true
}

// Normalized returns a valid keymap: unknown actions dropped, missing ones
// given their defaults (nil means every default), and invalid, reserved,
// duplicate or already-bound chords removed (the first action in table
// order keeps a chord bound twice).
func (km Keymap) Normalized() Keymap {
	out := Keymap{}
	taken := map[Chord]bool{}
	for _, a := range hotkeyActions {
		cs, ok := km[a.ID]
		if !ok {
			cs = a.def
		}
		kept := []Chord{}
		for _, c := range cs {
			if !KnownKey(c.Key) || taken[c] {
				continue
			}
			if _, reserved := Reserved(c); reserved {
				continue
			}
			taken[c] = true
			kept = append(kept, c)
		}
		out[a.ID] = kept
	}
	return out
}

// Owner returns the action bound to c ("" when none).
func (km Keymap) Owner(c Chord) string {
	for _, a := range hotkeyActions {
		if slices.Contains(km[a.ID], c) {
			return a.ID
		}
	}
	return ""
}

// ConflictError is returned when a chord is already bound to another action.
type ConflictError struct {
	Chord  Chord
	Action string // the action that has the chord now
	Want   string // the action that asked for it
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%s is already assigned to %s", e.Chord.Label(), HotkeyLabel(e.Action))
}

// ErrReserved wraps the refusal of a reserved key (see Reserved).
var ErrReserved = errors.New("reserved key")

// ErrAlreadyBound is returned when the action already has the chord.
var ErrAlreadyBound = errors.New("already assigned")

// reservedCtrl are the Ctrl/⌘ chords the app, the menus or macOS own.
var reservedCtrl = map[string]string{
	"q": "Quit", "w": "Close Window", "h": "Hide", "m": "Minimize",
	"n": "New Schedule", "o": "Open Schedule", "s": "Save Schedule",
	"v": "Paste", "x": "Cut", "a": "Select All", "z": "Undo",
	"tab": "switching windows", "space": "Spotlight",
}

// Reserved reports whether a chord cannot be a hotkey, and why. Digits are
// the numbers typed after a jump key; Escape closes dialogs; Tab moves the
// focus; Ctrl/⌘ Q, W, H, M, N, O, S, V, X, A and Z belong to the menus,
// macOS and text editing. (Ctrl+C is EasyWorship's Clear: hotkeys are off
// while a text field has focus, so ⌘C still copies there.)
func Reserved(c Chord) (string, bool) {
	if _, ok := digit(c.Key); ok && !c.Ctrl && !c.Alt {
		return "digits are the numbers typed after a jump key", true
	}
	switch c.Key {
	case "escape":
		return "Esc closes dialogs", true
	case "tab":
		if !c.Ctrl {
			return "Tab moves the keyboard focus", true
		}
	}
	if c.Ctrl {
		if what, ok := reservedCtrl[c.Key]; ok {
			if c.Key == "s" && c.Shift {
				what = "Save Schedule As"
			}
			if c.Key == "z" && c.Shift {
				what = "Redo"
			}
			return fmt.Sprintf("%s is %s", c.Label(), what), true
		}
	}
	return "", false
}

// Check validates binding c to action id: an unknown action or key, a
// reserved key (ErrReserved), a chord the action has already
// (ErrAlreadyBound) or a *ConflictError naming its current owner.
func (km Keymap) Check(id string, c Chord) error {
	if _, ok := hotkeyAction(id); !ok {
		return fmt.Errorf("unknown hotkey action %q", id)
	}
	if !KnownKey(c.Key) {
		return fmt.Errorf("unknown key %q", c.Key)
	}
	if why, ok := Reserved(c); ok {
		return fmt.Errorf("%s cannot be a hotkey: %s: %w", c.Label(), why, ErrReserved)
	}
	switch owner := km.Owner(c); owner {
	case "":
		return nil
	case id:
		return fmt.Errorf("%s is %w to %s", c.Label(), ErrAlreadyBound, HotkeyLabel(id))
	default:
		return &ConflictError{Chord: c, Action: owner, Want: id}
	}
}

// WithKey returns km with c added to action id. A chord owned by another
// action is a *ConflictError unless reassign, which moves it.
func (km Keymap) WithKey(id string, c Chord, reassign bool) (Keymap, error) {
	err := km.Check(id, c)
	var conflict *ConflictError
	switch {
	case err == nil:
	case errors.As(err, &conflict) && reassign:
	default:
		return km, err
	}
	out := km.Clone()
	if conflict != nil {
		out[conflict.Action] = slices.DeleteFunc(out[conflict.Action], func(x Chord) bool { return x == c })
	}
	out[id] = append(out[id], c)
	return out, nil
}

// WithoutKey returns km with c removed from action id.
func (km Keymap) WithoutKey(id string, c Chord) Keymap {
	out := km.Clone()
	out[id] = slices.DeleteFunc(out[id], func(x Chord) bool { return x == c })
	return out
}

// WithKeys returns km with action id bound to exactly keys (set-hotkey).
// Every chord is checked first; nothing changes on an error.
func (km Keymap) WithKeys(id string, keys []Chord, reassign bool) (Keymap, error) {
	if _, ok := hotkeyAction(id); !ok {
		return km, fmt.Errorf("unknown hotkey action %q", id)
	}
	out := km.Clone()
	out[id] = []Chord{}
	for _, c := range keys {
		next, err := out.WithKey(id, c, reassign)
		if errors.Is(err, ErrAlreadyBound) {
			continue // listed twice
		}
		if err != nil {
			return km, err
		}
		out = next
	}
	return out, nil
}

// ParseChords reads a list of chords (ParseChord).
func ParseChords(keys []string) ([]Chord, error) {
	out := make([]Chord, 0, len(keys))
	for _, k := range keys {
		c, err := ParseChord(k)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// UnmarshalJSON reads {"action": ["key", ...]} leniently: unknown actions
// and keys that do not parse are skipped (Normalized drops the rest), so a
// stored keymap from another version never fails to load.
func (km *Keymap) UnmarshalJSON(b []byte) error {
	var raw map[string][]string
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("hotkeys: %w", err)
	}
	out := Keymap{}
	for id, keys := range raw {
		cs := []Chord{}
		for _, k := range keys {
			if c, err := ParseChord(k); err == nil {
				cs = append(cs, c)
			}
		}
		out[id] = cs
	}
	*km = out
	return nil
}

// Binding is one row of the Options hotkey table.
type Binding struct {
	ID     string   `json:"id"`
	Action string   `json:"action"`
	Keys   string   `json:"keys"`   // display labels joined with " / "
	Chords []string `json:"chords"` // stored forms
	Labels []string `json:"-"`      // display labels
}

// Bindings lists the table rows for km, in table order.
func (km Keymap) Bindings() []Binding {
	out := make([]Binding, 0, len(hotkeyActions))
	for _, a := range hotkeyActions {
		b := Binding{ID: a.ID, Action: a.Label, Chords: []string{}, Labels: []string{}}
		for _, c := range km[a.ID] {
			b.Chords = append(b.Chords, c.String())
			b.Labels = append(b.Labels, c.Label())
		}
		b.Keys = strings.Join(b.Labels, " / ")
		out = append(out, b)
	}
	return out
}

// Bindings lists the default hotkeys (the table before any change).
func Bindings() []Binding { return DefaultKeymap().Bindings() }

// ---- EasyWorship import ------------------------------------------------------------

// EWHotkeysPath is where EasyWorship 8 keeps its hotkeys on a Mac.
const EWHotkeysPath = "/Users/Shared/Softouch/Easyworship.v8/Settings/HotKeys.2.ini"

// vkKeys maps the Windows virtual-key codes EasyWorship stores to key names.
var vkKeys = map[int]string{
	8: "backspace", 9: "tab", 13: "enter", 27: "escape", 32: "space",
	33: "pageup", 34: "pagedown", 35: "end", 36: "home",
	37: "left", 38: "up", 39: "right", 40: "down", 46: "delete",
}

// vkKey converts a virtual-key code ("" when the hotkeys have no such key).
func vkKey(code int) string {
	switch {
	case code >= 'A' && code <= 'Z':
		return string(rune('a' + code - 'A'))
	case code >= '0' && code <= '9':
		return string(rune(code))
	case code >= 0x60 && code <= 0x69: // numeric keypad
		return string(rune('0' + code - 0x60))
	case code >= 0x70 && code <= 0x7B:
		return fmt.Sprintf("f%d", code-0x70+1)
	}
	return vkKeys[code]
}

// ParseEWHotkeys reads EasyWorship's HotKeys.2.ini ([HotKeys] entries such as
// "SlideNextKeys=78, 39, 32" and "BlackScreenKeys=C-66": Windows virtual-key
// codes, "C-" for Ctrl, "S-" for Shift, "A-" for Alt). Actions EasyWorship
// does not have (Go Live) keep their keys from base. Keys that cannot be
// read or that FreeBeams reserves are skipped and reported in skipped.
func ParseEWHotkeys(text string, base Keymap) (km Keymap, skipped []string, err error) {
	byEW := map[string]string{}
	for _, a := range hotkeyActions {
		if a.EW != "" {
			byEW[strings.ToLower(a.EW)] = a.ID
		}
	}
	km = base.Clone()
	found := false
	section := ""
	for _, line := range strings.Split(strings.TrimPrefix(text, "\ufeff"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || section != "hotkeys" {
			continue
		}
		id, ok := byEW[strings.ToLower(strings.TrimSpace(name))]
		if !ok {
			continue
		}
		found = true
		chords := []Chord{}
		for _, code := range strings.Split(value, ",") {
			c, ok := ewChord(strings.TrimSpace(code))
			if !ok {
				if strings.TrimSpace(code) != "" {
					skipped = append(skipped, fmt.Sprintf("%s: %s", strings.TrimSpace(name), strings.TrimSpace(code)))
				}
				continue
			}
			chords = append(chords, c)
		}
		km[id] = chords
	}
	if !found {
		return base, nil, errors.New("no [HotKeys] entries found")
	}
	// Drop what FreeBeams cannot bind, reporting it; Normalized keeps the
	// first owner of a chord listed twice.
	for _, a := range hotkeyActions {
		for _, c := range km[a.ID] {
			if why, ok := Reserved(c); ok {
				skipped = append(skipped, fmt.Sprintf("%s: %s (%s)", a.Label, c.Label(), why))
			}
		}
	}
	sort.Strings(skipped)
	return km.Normalized(), skipped, nil
}

// ewChord reads one EasyWorship key ("78", "C-66", "S-C-9").
func ewChord(s string) (Chord, bool) {
	var c Chord
	for {
		switch {
		case strings.HasPrefix(s, "C-"):
			c.Ctrl = true
		case strings.HasPrefix(s, "S-"):
			c.Shift = true
		case strings.HasPrefix(s, "A-"):
			c.Alt = true
		default:
			var code int
			if _, err := fmt.Sscanf(s, "%d", &code); err != nil || fmt.Sprint(code) != s {
				return Chord{}, false
			}
			c.Key = vkKey(code)
			return c, c.Key != ""
		}
		s = s[2:]
	}
}
