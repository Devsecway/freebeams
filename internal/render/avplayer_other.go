//go:build !darwin

package render

import "errors"

// nativeVideoSupported is false off macOS: videos play through Godot's
// Theora player (video.go) after a transcode, and audio files have no
// player yet.
const nativeVideoSupported = false

// openNativePlayer has no implementation here.
func openNativePlayer(path string) (nativePlayer, error) {
	return nil, errors.New("no native media player on this platform")
}
