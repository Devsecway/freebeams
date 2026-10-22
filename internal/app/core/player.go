package core

import "strings"

// PlayerEnv forces a media player backend: "ffmpeg" (also on macOS, to
// verify the Windows / Linux player there), "avfoundation" or "theora".
const PlayerEnv = "FREEBEAMS_PLAYER"

// Media player backends (render.Player*: the same names).
const (
	PlayerAVFoundation = "avfoundation"
	PlayerFFmpeg       = "ffmpeg"
	PlayerTheora       = "theora"
)

// ChoosePlayer picks the media player: what env asks for when it can work,
// else AVFoundation where there is one (macOS), else the ffmpeg player when
// ffmpeg and ffprobe were found, else the Theora fallback. note explains an
// env request that could not be honoured ("" otherwise).
func ChoosePlayer(env string, avfoundation, ffmpeg bool) (backend, note string) {
	def := PlayerTheora
	switch {
	case avfoundation:
		def = PlayerAVFoundation
	case ffmpeg:
		def = PlayerFFmpeg
	}
	switch want := strings.ToLower(strings.TrimSpace(env)); want {
	case "":
		return def, ""
	case PlayerFFmpeg:
		if ffmpeg {
			return PlayerFFmpeg, ""
		}
		return def, PlayerEnv + "=ffmpeg: ffmpeg / ffprobe not found"
	case PlayerAVFoundation, "native":
		if avfoundation {
			return PlayerAVFoundation, ""
		}
		return def, PlayerEnv + "=" + want + ": AVFoundation is macOS only"
	case PlayerTheora:
		return PlayerTheora, ""
	default:
		return def, PlayerEnv + "=" + want + ": unknown player (ffmpeg, avfoundation, theora)"
	}
}
