package core

import (
	"os"
	"testing"
)

// TestMain pins the hotkey modifiers to the portable rules (⌘ folded into
// Ctrl) whatever the host is, so the tests mean the same on every OS; the
// macOS rules are tested with setMacModifiers.
func TestMain(m *testing.M) {
	macModifiers = false
	os.Exit(m.Run())
}

// setMacModifiers switches the ⌘ / Control rules for one test.
func setMacModifiers(t *testing.T, on bool) {
	t.Helper()
	old := macModifiers
	macModifiers = on
	t.Cleanup(func() { macModifiers = old })
}
