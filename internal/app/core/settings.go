package core

import (
	"fmt"
	"path/filepath"
	"strings"

	"freebeams/internal/models"
)

// Setting keys of the Options dialog (one key per value, so each can be
// changed and read independently; SettingLogo predates the dialog).
const (
	SettingTransition     = "settings.transition" // models.Transition
	SettingOutputScreen   = "output.screen"       // int: display index, AutoScreen = first non-primary
	SettingInterfaceColor = "ui.interfaceColor"   // "dark" (default) or "light"
)

// AutoScreen is the output screen setting that picks the first non-primary
// display (or the only one).
const AutoScreen = -1

// Interface colors (View ▸ Interface Color).
const (
	ColorDark  = "dark"
	ColorLight = "light"
)

// Settings is the operator-configurable state the Options dialog edits. It is
// authoritative app state; the dialog is a projection of it, and each field is
// persisted under its own key.
type Settings struct {
	Transition     models.Transition `json:"transition"`
	OutputScreen   int               `json:"outputScreen"` // AutoScreen or a display index
	Logo           models.Background `json:"logo"`
	InterfaceColor string            `json:"interfaceColor"`

	// Alerts is Options ▸ Alerts: the look and place of the message band,
	// the nursery badge, the countdown and the clock (SettingAlertStyles).
	Alerts models.AlertStyles `json:"alerts"`

	// Hotkeys is Options ▸ Hotkeys: the keys of every hotkey action
	// (SettingHotkeys, keymap.go).
	Hotkeys Keymap `json:"hotkeys"`
}

// DefaultSettings returns the built-in settings: a 500 ms blend, the automatic
// output display, the logo from logoDir (LogoBackground) and the dark
// interface.
func DefaultSettings(logoDir string) Settings {
	return Settings{
		Transition:     models.DefaultTransition(),
		OutputScreen:   AutoScreen,
		Logo:           LogoBackground(logoDir),
		InterfaceColor: ColorDark,
		Alerts:         models.DefaultAlertStyles(),
		Hotkeys:        DefaultKeymap(),
	}
}

// Normalized returns s with every field valid: the transition normalized, a
// negative screen folded to AutoScreen, an empty logo replaced by the
// default and an unknown color replaced by dark.
func (s Settings) Normalized(logoDir string) Settings {
	s.Transition = s.Transition.Normalized()
	if s.OutputScreen < 0 {
		s.OutputScreen = AutoScreen
	}
	if s.Logo.Kind == "" {
		s.Logo = LogoBackground(logoDir)
	}
	if s.InterfaceColor != ColorLight {
		s.InterfaceColor = ColorDark
	}
	s.Alerts = s.Alerts.Normalized()
	s.Hotkeys = s.Hotkeys.Normalized()
	return s
}

// ParseInterfaceColor reads "dark" / "light" (case-insensitive).
func ParseInterfaceColor(s string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case ColorDark:
		return ColorDark, nil
	case ColorLight:
		return ColorLight, nil
	}
	return "", fmt.Errorf("unknown interface color %q (dark|light)", s)
}

// OutputScreenFor resolves the configured output screen against the connected
// displays: a configured index that exists wins, otherwise the first
// non-primary display (primary is the index of the primary one). n is the
// number of displays.
func OutputScreenFor(setting, n, primary int) int {
	if setting >= 0 && setting < n {
		return setting
	}
	for i := 0; i < n; i++ {
		if i != primary {
			return i
		}
	}
	return max(primary, 0)
}

// ScreenLabel describes a display for the output picker ("Display 2 —
// 1920×1080, primary").
func ScreenLabel(index, w, h int, primary bool) string {
	s := fmt.Sprintf("Display %d — %d×%d", index+1, w, h)
	if primary {
		s += ", primary"
	}
	return s
}

// LogoDescription names the logo setting for the dialog: the image file's
// name, "Default (FreeBeams logo)" for the built-in Logo screen, or "Default
// gradient" for the gradient.
func LogoDescription(bg models.Background) string {
	switch bg.Kind {
	case models.BackgroundImage, models.BackgroundVideo:
		if IsDefaultLogo(bg) {
			return "Default (FreeBeams logo)"
		}
		if bg.Path != "" {
			return filepath.Base(bg.Path)
		}
	}
	return "Default gradient"
}

// LogoFromPath makes the logo background for an image file ("" restores the
// default from logoDir).
func LogoFromPath(path, logoDir string) models.Background {
	if strings.TrimSpace(path) == "" {
		return LogoBackground(logoDir)
	}
	return models.Background{Kind: models.BackgroundImage, Path: path, Color: LogoGradient().Color}
}

// keyLabels are the display names of the named keys.
var keyLabels = map[string]string{
	"right": "→", "left": "←", "up": "↑", "down": "↓",
	"home": "Home", "end": "End", "pageup": "Page Up", "pagedown": "Page Down",
	"space": "Space", "backspace": "Backspace", "enter": "Enter", "delete": "Delete",
	"escape": "Esc", "tab": "Tab",
}

// KeyLabel is the display name of a core key name ("right" → "→", "n" → "N").
func KeyLabel(key string) string {
	if l, ok := keyLabels[key]; ok {
		return l
	}
	return strings.ToUpper(key)
}
