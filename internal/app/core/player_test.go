package core

import "testing"

func TestChoosePlayer(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		av, ff   bool
		want     string
		wantNote bool
	}{
		{"macOS default", "", true, true, PlayerAVFoundation, false},
		{"macOS without ffmpeg", "", true, false, PlayerAVFoundation, false},
		{"Windows / Linux with ffmpeg", "", false, true, PlayerFFmpeg, false},
		{"Windows / Linux without ffmpeg", "", false, false, PlayerTheora, false},
		{"forced ffmpeg on macOS", "ffmpeg", true, true, PlayerFFmpeg, false},
		{"forced ffmpeg, case and spaces", " FFmpeg ", true, true, PlayerFFmpeg, false},
		{"forced ffmpeg without it", "ffmpeg", true, false, PlayerAVFoundation, true},
		{"forced avfoundation off macOS", "avfoundation", false, true, PlayerFFmpeg, true},
		{"native is avfoundation", "native", true, true, PlayerAVFoundation, false},
		{"forced theora", "theora", true, true, PlayerTheora, false},
		{"unknown", "vlc", false, true, PlayerFFmpeg, true},
	}
	for _, tt := range tests {
		got, note := ChoosePlayer(tt.env, tt.av, tt.ff)
		if got != tt.want || (note != "") != tt.wantNote {
			t.Errorf("%s: ChoosePlayer(%q, %v, %v) = %q, %q", tt.name, tt.env, tt.av, tt.ff, got, note)
		}
	}
}
