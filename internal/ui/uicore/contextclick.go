package uicore

// Mouse buttons as Godot numbers them (Input.MouseButton): left 1, right 2.
const (
	MouseLeft  = 1
	MouseRight = 2
)

// IsContextClick reports whether a mouse press opens a context menu: the
// right button, or on macOS a Control-click of the left button (the Mac's
// one-button convention; Godot may report it either way).
func IsContextClick(button int, ctrl, macOS bool) bool {
	switch button {
	case MouseRight:
		return true
	case MouseLeft:
		return ctrl && macOS
	}
	return false
}
