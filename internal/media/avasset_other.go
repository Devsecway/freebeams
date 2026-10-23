//go:build !darwin

package media

import "context"

// AVFoundation is macOS only; elsewhere it answers ErrUnavailable.
type AVFoundation struct{}

// AVFoundationAvailable reports whether this platform has it (macOS).
func AVFoundationAvailable() bool { return false }

// Probe is unavailable off macOS.
func (AVFoundation) Probe(context.Context, string) (Info, error) { return Info{}, ErrUnavailable }

// Thumbnail is unavailable off macOS.
func (AVFoundation) Thumbnail(context.Context, string, string, int) error { return ErrUnavailable }
