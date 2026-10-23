//go:build !darwin && !windows

package app

// nativeLook is a no-op off macOS and Windows: Linux window managers draw
// the decorations from their own theme.
type nativeLook struct{}

func (*nativeLook) apply(bool)    {}
func (*nativeLook) state() string { return "" }
