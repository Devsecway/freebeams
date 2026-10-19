//go:build !darwin

package render

// coverSupported is false off macOS: the output window covers its screen as a
// borderless always-on-top window (there is no per-display menu bar to beat).
const coverSupported = false

func coverScreen(handle uintptr) bool { return false }
