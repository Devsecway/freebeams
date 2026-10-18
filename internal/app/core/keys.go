package core

import "strings"

// keyAliases maps alternative spellings (control API, DOM-style names) to the
// canonical KeyEvent names.
var keyAliases = map[string]string{
	"arrowright": "right", "arrowleft": "left", "arrowup": "up", "arrowdown": "down",
	"pgdn": "pagedown", "pgdown": "pagedown", "page_down": "pagedown", "page-down": "pagedown",
	"pgup": "pageup", "page_up": "pageup", "page-up": "pageup",
	"return": "enter", "kp_enter": "enter", "kpenter": "enter",
	" ": "space", "spacebar": "space",
	"bksp": "backspace", "back": "backspace",
	"del": "delete", "esc": "escape",
}

// NormalizeKeyName returns the canonical KeyEvent name for a user-supplied key
// name ("ArrowRight" → "right", "PgDn" → "pagedown", "V" → "v").
func NormalizeKeyName(name string) string {
	if name == " " {
		return "space"
	}
	k := strings.ToLower(strings.TrimSpace(name))
	if a, ok := keyAliases[k]; ok {
		return a
	}
	return k
}
