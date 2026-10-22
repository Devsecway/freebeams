package app

import (
	"log"
	"os"

	"freebeams/internal/app/core"
	"freebeams/internal/media"
	"freebeams/internal/render"
)

// selectMediaPlayer picks the media player backend before any view opens a
// file (docs/MEDIA.md): AVFoundation on macOS, the ffmpeg player on Windows
// and Linux when ffmpeg is found (bundled next to the executable, else on
// PATH), else the Theora fallback. FREEBEAMS_PLAYER forces one (core.PlayerEnv).
func (a *App) selectMediaPlayer() {
	tools := media.Locate()
	want, note := core.ChoosePlayer(os.Getenv(core.PlayerEnv), render.AVFoundationAvailable(), tools.Available())
	if note != "" {
		log.Printf("[freebeams] %s", note)
	}
	got := render.UseMediaPlayer(want, tools.FFmpeg, tools.FFprobe)
	switch got {
	case render.PlayerFFmpeg:
		log.Printf("[freebeams] media player: ffmpeg (%s)", tools.FFmpeg)
	default:
		log.Printf("[freebeams] media player: %s", got)
	}
}
