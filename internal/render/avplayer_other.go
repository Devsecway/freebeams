//go:build !darwin

package render

import "errors"

// avFoundationAvailable is false off macOS: videos and audio play through
// the ffmpeg player (ffplayer.go) when ffmpeg is found, else only Ogg Theora
// backgrounds play through Godot (video.go).
const avFoundationAvailable = false

// openAVPlayer has no implementation here.
func openAVPlayer(path string) (nativePlayer, error) {
	return nil, errors.New("AVFoundation is macOS only")
}
