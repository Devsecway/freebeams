package core

import (
	"testing"

	"freebeams/internal/models"
)

// TestPlaybackItemVolume follows a monitor through items with and without a
// volume of their own: an item's volume or Mute decides its start and is not
// passed on; the operator's volume of the items without one carries over.
func TestPlaybackItemVolume(t *testing.T) {
	plain := models.Background{Kind: models.BackgroundVideo, Path: "/plain.mp4"}
	loud := models.Background{Kind: models.BackgroundVideo, Path: "/loud.mp4", Volume: 40}
	muted := models.Background{Kind: models.BackgroundAudio, Path: "/muted.mp3", Volume: 80, Mute: true}
	tooLoud := models.Background{Kind: models.BackgroundAudio, Path: "/bad.mp3", Volume: 400}
	steps := []struct {
		name    string
		bg      *models.Background // nil: a frame without media
		setVol  float64            // the operator's volume after it arrived (< 0: untouched)
		want    float64            // the volume it started at
		own     bool
		carried float64 // CarriedVolume afterwards
	}{
		{"the first item plays at full volume", &plain, 0.6, 1, false, 0.6},
		{"the next plain item carries 0.6 over", &plain, -1, 0.6, false, 0.6},
		{"an item volume of 40% overrides", &loud, 0.9, 0.4, true, 0.6},
		{"the next plain item goes back to 0.6", &plain, -1, 0.6, false, 0.6},
		{"mute starts at 0 whatever the volume", &muted, 0.5, 0, true, 0.6},
		{"a song keeps the carried volume", nil, -1, 0.6, false, 0.6},
		{"after the song: 0.6", &plain, -1, 0.6, false, 0.6},
		{"a volume over 100% is clamped", &tooLoud, -1, 1, true, 0.6},
	}
	pb := NewPlayback()
	for _, st := range steps {
		if st.bg == nil {
			pb = NoPlayback(pb)
		} else {
			pb = PlaybackDefaults(*st.bg, true, pb)
		}
		if pb.Volume != st.want || pb.Own != st.own {
			t.Fatalf("%s: started at %v own %v; want %v own %v (%+v)", st.name, pb.Volume, pb.Own, st.want, st.own, pb)
		}
		if st.setVol >= 0 {
			pb.Volume = st.setVol // the transport (it raises a muted item for this playback)
		}
		if got := pb.CarriedVolume(); got != st.carried {
			t.Fatalf("%s: CarriedVolume = %v, want %v (%+v)", st.name, got, st.carried, pb)
		}
	}
	for _, tc := range []struct {
		bg    models.Background
		carry float64
		want  float64
		own   bool
	}{
		{models.Background{}, 0.3, 0.3, false},
		{models.Background{Volume: 25}, 0.3, 0.25, true},
		{models.Background{Mute: true}, 0.3, 0, true},
		{models.Background{Volume: -5}, 2, 1, false},
	} {
		if v, own := StartVolume(tc.bg, tc.carry); v != tc.want || own != tc.own {
			t.Errorf("StartVolume(%+v, %v) = %v, %v; want %v, %v", tc.bg, tc.carry, v, own, tc.want, tc.own)
		}
	}
}

// TestPlaybackItemVolumeKeptUnderBlack: the item's playback (its volume
// raised by the operator) survives Black like the rest of the playback.
func TestPlaybackItemVolumeKeptUnderBlack(t *testing.T) {
	clip := &models.Slide{Kind: models.SlideMedia, Background: &models.Background{Kind: models.BackgroundVideo, Path: "/m.mp4", Mute: true}}
	pb, _ := AdvancePlayback(NewPlayback(), models.Frame{Mode: models.ModeNormal, Slide: clip}, true)
	if pb.Volume != 0 || !pb.Own {
		t.Fatalf("muted item = %+v", pb)
	}
	pb.Volume = 0.7
	for _, m := range []models.OutputMode{models.ModeBlack, models.ModeNormal, models.ModeLogo, models.ModeClear} {
		if next, changed := AdvancePlayback(pb, models.Frame{Mode: m, Slide: clip}, true); changed || next.Volume != 0.7 {
			t.Fatalf("%s: %+v, %v; want the raised volume kept", m, next, changed)
		}
	}
}

// TestCoverPlayback is the Pause on Black option: Black / Logo pause a
// playing file and leaving them resumes it only if Black paused it.
func TestCoverPlayback(t *testing.T) {
	playing := Playback{Key: "/v.mp4", Playing: true, Volume: 1}
	paused := Playback{Key: "/v.mp4", Volume: 1}
	steps := []struct {
		name    string
		start   *Playback // nil: go on from the previous step
		hidden  bool
		on      bool // the option
		operate func(*Playback)
		playing bool
		blackP  bool
		changed bool
	}{
		{"option off: Black leaves it playing", &playing, true, false, nil, true, false, true},
		{"option off: leaving Black, still playing", nil, false, false, nil, true, false, true},
		{"option on: Black pauses the playing file", nil, true, true, nil, false, true, true},
		{"still under Black: nothing more", nil, true, true, nil, false, true, false},
		{"leaving Black resumes it", nil, false, true, nil, true, false, true},
		{"a paused file stays paused under Black", &paused, true, true, nil, false, false, true},
		{"and after it", nil, false, true, nil, false, false, true},
		{"Black pauses again", &playing, true, true, nil, false, true, true},
		{"the operator plays it under Black", nil, true, true, func(p *Playback) { p.SetPlaying(true) }, true, false, false},
		{"leaving Black keeps it playing", nil, false, true, nil, true, false, true},
		{"Black pauses it", nil, true, true, nil, false, true, true},
		{"the operator pauses explicitly", nil, true, true, func(p *Playback) { p.SetPlaying(false) }, false, false, false},
		{"leaving Black keeps the operator's pause", nil, false, true, nil, false, false, true},
		{"Black pauses, the option goes off", &playing, true, true, nil, false, true, true},
		{"leaving Black still resumes what Black paused", nil, false, false, nil, true, false, true},
		{"nothing plays: no hidden state", &Playback{Volume: 1}, true, true, nil, false, false, false},
	}
	var pb Playback
	for _, st := range steps {
		if st.start != nil {
			pb = *st.start
		}
		if st.operate != nil {
			st.operate(&pb)
		}
		next, changed := CoverPlayback(pb, st.hidden, st.on)
		if next.Playing != st.playing || next.BlackPaused != st.blackP || changed != st.changed {
			t.Fatalf("%s: CoverPlayback = %+v, %v; want playing %v blackPaused %v changed %v",
				st.name, next, changed, st.playing, st.blackP, st.changed)
		}
		pb = next
	}

	// A new item going live under Black with the option on waits paused
	// and starts when Black goes off.
	clip := &models.Slide{Kind: models.SlideMedia, Background: &models.Background{Kind: models.BackgroundVideo, Path: "/new.mp4"}}
	black := models.Frame{Mode: models.ModeBlack, Slide: clip}
	pb, _ = AdvancePlayback(NewPlayback(), black, true)
	_, shown, _ := MonitorMedia(black)
	pb, _ = CoverPlayback(pb, !shown, true)
	if pb.Playing || !pb.BlackPaused {
		t.Fatalf("new item under Black = %+v; want paused by Black", pb)
	}
	pb, _ = AdvancePlayback(pb, models.Frame{Mode: models.ModeNormal, Slide: clip}, true)
	if pb, _ = CoverPlayback(pb, false, true); !pb.Playing || pb.BlackPaused {
		t.Fatalf("new item after Black = %+v; want playing", pb)
	}
	// Another item replacing a Black-paused one starts fresh (shown: plays).
	other := &models.Slide{Kind: models.SlideMedia, Background: &models.Background{Kind: models.BackgroundVideo, Path: "/other.mp4"}}
	pb, _ = CoverPlayback(pb, true, true)
	pb, _ = AdvancePlayback(pb, models.Frame{Mode: models.ModeNormal, Slide: other}, true)
	if pb, _ = CoverPlayback(pb, false, true); !pb.Playing || pb.BlackPaused || pb.Hidden {
		t.Fatalf("next item = %+v; want playing, not Black-paused", pb)
	}
}
