//go:build !darwin

package spell

// System returns nil: only macOS has a system checker here, so Default uses
// the embedded word list (wordlist.go).
func System() Backend { return nil }
