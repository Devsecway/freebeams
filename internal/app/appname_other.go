//go:build !darwin

package app

// setAppName is a no-op off macOS: the window title and the bundle carry the
// name there.
func setAppName(string) {}
