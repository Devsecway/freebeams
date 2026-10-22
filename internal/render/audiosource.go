package render

// Background audio (the schedule's playlist, docs/MEDIA.md): an audio-only
// MediaSource with a role of its own, so it never shares a decoder with the
// monitors (RoleLive / RolePreview) and nothing the Live slide does (a new
// item, Black, Clear, Logo, the end of a crossfade) opens or closes it. No
// layer draws it: the app opens and closes it from its render pass and pumps
// it every frame (PumpMedia), since no SlideView may be holding a source.
//
// This file only adds to the shared player layer (videosource.go): the
// nativePlayer interface and the monitors' sources are unchanged, so every
// platform backend that plays a file for the monitors plays the playlist too.

// RoleAudio is the playlist player's role: sound only, no texture.
const RoleAudio VideoRole = RolePreview + 1

// OpenAudio opens path for the playlist (an audio-only source, paused at 0
// until Play). Close it with CloseAudio.
func OpenAudio(path string) *MediaSource { return acquireSource(RoleAudio, path) }

// CloseAudio closes a source OpenAudio returned (nil is a no-op).
func CloseAudio(s *MediaSource) {
	if s != nil && s.refs > 0 {
		s.release()
	}
}

// PumpMedia advances every open source once this engine frame (status, end
// of file, video frames); further calls in the same frame do nothing. The
// app calls it every frame so the playlist plays with no video on screen.
func PumpMedia() { pumpSources() }
