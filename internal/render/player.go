package render

import (
	"errors"
	"log"

	"freebeams/internal/media/ffplayer"

	"graphics.gd/classdb/Node"
)

// Media player backends behind MediaSource (docs/MEDIA.md). The app picks
// one at startup (core.ChoosePlayer) and tells the renderer with
// UseMediaPlayer, before any view opens a file.
const (
	PlayerAVFoundation = "avfoundation" // macOS: AVFoundation (avplayer_darwin.go)
	PlayerFFmpeg       = "ffmpeg"       // ffmpeg processes over pipes (ffplayer.go); Windows and Linux
	PlayerTheora       = "theora"       // no media player: Godot's Theora player for .ogv backgrounds (video.go), no audio
)

var (
	playerBackend = defaultPlayer()
	ffConfig      ffplayer.Config
)

func defaultPlayer() string {
	if avFoundationAvailable {
		return PlayerAVFoundation
	}
	return PlayerTheora
}

// AVFoundationAvailable reports whether this build has the AVFoundation
// player (macOS).
func AVFoundationAvailable() bool { return avFoundationAvailable }

// UseMediaPlayer selects the backend: PlayerFFmpeg needs ffmpeg and
// ffprobe; PlayerAVFoundation needs macOS. A backend that cannot work falls
// back to the platform default. It returns the backend in use.
func UseMediaPlayer(backend, ffmpeg, ffprobe string) string {
	switch {
	case backend == PlayerFFmpeg && ffmpeg != "" && ffprobe != "":
		ffConfig = ffplayer.Config{FFmpeg: ffmpeg, FFprobe: ffprobe}
		playerBackend = PlayerFFmpeg
	case backend == PlayerAVFoundation && avFoundationAvailable, backend == PlayerTheora:
		playerBackend = backend
	default:
		playerBackend = defaultPlayer()
		if backend != playerBackend {
			log.Printf("[render] media player %q is not available here; using %q", backend, playerBackend)
		}
	}
	return playerBackend
}

// MediaPlayer is the backend in use.
func MediaPlayer() string { return playerBackend }

// NativeVideoSupported reports whether a media player plays every video
// and audio file as it is (AVFoundation or ffmpeg: no transcode). False on
// the Theora fallback.
func NativeVideoSupported() bool { return playerBackend != PlayerTheora }

// openNativePlayer opens path with the selected backend. host is a node in
// the scene tree (the layer's texture rect): the ffmpeg player adds its
// AudioStreamPlayer under the tree's root.
func openNativePlayer(path string, host Node.Instance) (nativePlayer, error) {
	switch playerBackend {
	case PlayerAVFoundation:
		return openAVPlayer(path)
	case PlayerFFmpeg:
		return openFFPlayer(path, host)
	}
	return nil, errors.New("no media player on this platform")
}
