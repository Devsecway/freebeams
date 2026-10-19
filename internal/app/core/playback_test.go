package core

import (
	"testing"

	"freebeams/internal/models"
)

func TestFrameMedia(t *testing.T) {
	video := &models.Background{Kind: models.BackgroundVideo, Path: "/v.mp4", Poster: "/p.jpg", Loop: true}
	audio := &models.Background{Kind: models.BackgroundAudio, Path: "/a.mp3"}
	image := &models.Background{Kind: models.BackgroundImage, Path: "/i.jpg"}
	slide := func(bg *models.Background) *models.Slide {
		return &models.Slide{Kind: models.SlideMedia, Background: bg}
	}
	tests := []struct {
		name string
		f    models.Frame
		want string
		ok   bool
	}{
		{"video", models.Frame{Mode: models.ModeNormal, Slide: slide(video)}, "/v.mp4", true},
		{"audio", models.Frame{Mode: models.ModeNormal, Slide: slide(audio)}, "/a.mp3", true},
		{"clear keeps the background playing", models.Frame{Mode: models.ModeClear, Slide: slide(video)}, "/v.mp4", true},
		{"black stops it", models.Frame{Mode: models.ModeBlack, Slide: slide(video)}, "", false},
		{"logo stops it", models.Frame{Mode: models.ModeLogo, Slide: slide(video)}, "", false},
		{"image", models.Frame{Mode: models.ModeNormal, Slide: slide(image)}, "", false},
		{"no background", models.Frame{Mode: models.ModeNormal, Slide: &models.Slide{Text: "x"}}, "", false},
		{"no slide", models.Frame{Mode: models.ModeNormal}, "", false},
		{"empty path", models.Frame{Mode: models.ModeNormal, Slide: slide(&models.Background{Kind: models.BackgroundVideo})}, "", false},
	}
	for _, tc := range tests {
		bg, ok := FrameMedia(tc.f)
		if ok != tc.ok || bg.Path != tc.want {
			t.Errorf("%s: FrameMedia = %+v, %v; want %q, %v", tc.name, bg, ok, tc.want, tc.ok)
		}
		if ok && bg.Loop != tc.f.Slide.Background.Loop {
			t.Errorf("%s: Loop lost", tc.name)
		}
	}
}

func TestPlaybackDefaults(t *testing.T) {
	video := models.Background{Kind: models.BackgroundVideo, Path: "/v.mp4", Loop: true}
	audio := models.Background{Kind: models.BackgroundAudio, Path: "/a.mp3"}
	live := PlaybackDefaults(video, true, Playback{Volume: 1})
	if live.Key != "/v.mp4" || !live.Playing || !live.Loop || live.Volume != 1 || live.SeekSeq != 0 {
		t.Fatalf("live video defaults = %+v", live)
	}
	prev := Playback{Key: "/v.mp4", Playing: true, Loop: true, Volume: 0.4, SeekSeq: 3}
	next := PlaybackDefaults(audio, true, prev)
	if next.Key != "/a.mp3" || !next.Playing || next.Loop || next.Volume != 0.4 || next.SeekSeq != 3 {
		t.Fatalf("live audio after a video = %+v (volume carries over, loop is the item's)", next)
	}
	preview := PlaybackDefaults(video, false, Playback{Volume: 1})
	if preview.Playing || !preview.Loop || preview.Volume != 1 {
		t.Fatalf("preview defaults = %+v (paused)", preview)
	}
	if none := NoPlayback(prev); none.Key != "" || none.Playing || none.Loop || none.Volume != 0.4 || none.SeekSeq != 3 {
		t.Fatalf("NoPlayback = %+v", none)
	}
}

func TestTransportTime(t *testing.T) {
	tests := []struct {
		pos, dur float64
		want     string
	}{
		{0, 0, "0:00 / 0:00"},
		{0, 30.04, "0:00 / 0:30"},
		{1.2, 30, "0:01 / 0:30"},
		{65, 125.9, "1:05 / 2:05"},
		{40, 30, "0:30 / 0:30"}, // clamped
		{-1, 30, "0:00 / 0:30"},
		{3700, 3725, "1:01:40 / 1:02:05"},
	}
	for _, tc := range tests {
		if got := TransportTime(tc.pos, tc.dur); got != tc.want {
			t.Errorf("TransportTime(%v, %v) = %q, want %q", tc.pos, tc.dur, got, tc.want)
		}
	}
	if SeekFraction(0.5, 30) != 15 || SeekFraction(2, 30) != 30 || SeekFraction(-1, 30) != 0 || SeekFraction(0.5, 0) != 0 {
		t.Error("SeekFraction")
	}
	if ClampVolume(1.5) != 1 || ClampVolume(-2) != 0 || ClampVolume(0.25) != 0.25 {
		t.Error("ClampVolume")
	}
}

func TestPlaybackTarget(t *testing.T) {
	for _, tc := range []struct {
		target   string
		live, ok bool
	}{{"", true, true}, {"live", true, true}, {"preview", false, true}, {"output", false, false}} {
		live, ok := PlaybackTarget(tc.target)
		if live != tc.live || ok != tc.ok {
			t.Errorf("PlaybackTarget(%q) = %v, %v", tc.target, live, ok)
		}
	}
	if err := PlaybackError("seek", "x"); err == nil || err.Error() != `seek: target must be "live" or "preview", got "x"` {
		t.Errorf("PlaybackError = %v", err)
	}
}
