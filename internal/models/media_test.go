package models

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMediaKindOf(t *testing.T) {
	tests := []struct {
		path string
		kind MediaKind
		ok   bool
	}{
		{"a.png", MediaImage, true},
		{"/x/y/Photo.JPG", MediaImage, true},
		{"a.jpeg", MediaImage, true},
		{"a.webp", MediaImage, true},
		{"a.bmp", MediaImage, true},
		{"clip.mp4", MediaVideo, true},
		{"clip.MOV", MediaVideo, true},
		{"clip.m4v", MediaVideo, true},
		{"clip.ogv", MediaVideo, true},
		{"clip.webm", MediaVideo, true},
		{"song.mp3", MediaAudio, true},
		{"Track.M4A", MediaAudio, true},
		{"a.wav", MediaAudio, true},
		{"a.aiff", MediaAudio, true},
		{"a.aif", MediaAudio, true},
		{"a.flac", MediaAudio, true},
		{"a.ogg", MediaAudio, true}, // .ogg is audio; .ogv stays video
		{"notes.txt", "", false},
		{"noext", "", false},
		{"", "", false},
		{".png", MediaImage, true}, // a dot file with a known extension still counts
	}
	for _, tc := range tests {
		kind, ok := MediaKindOf(tc.path)
		if kind != tc.kind || ok != tc.ok {
			t.Errorf("MediaKindOf(%q) = %q, %v; want %q, %v", tc.path, kind, ok, tc.kind, tc.ok)
		}
	}
}

func TestMediaTitle(t *testing.T) {
	tests := []struct{ path, want string }{
		{"/Videos/EWM-ewm_bg #1_14.HD720.mp4", "EWM-ewm bg #1 14.HD720"},
		{"Snowfall Loop 02 Still.jpg", "Snowfall Loop 02 Still"},
		{"/a/b/my__file.PNG", "my file"},
		{"noext", "noext"},
	}
	for _, tc := range tests {
		if got := MediaTitle(tc.path); got != tc.want {
			t.Errorf("MediaTitle(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestMediaPlayablePathAndBackground(t *testing.T) {
	tests := []struct {
		name     string
		m        Media
		playable string
		bgKind   BackgroundKind
		bgPath   string
		poster   string
	}{
		{"still", Media{Kind: MediaImage, Path: "/p/a.jpg", ThumbPath: "/t/a.jpg"}, "", BackgroundImage, "/p/a.jpg", ""},
		{"mp4 without transcode", Media{Kind: MediaVideo, Path: "/p/a.mp4", ThumbPath: "/t/a.jpg"}, "", BackgroundVideo, "/p/a.mp4", "/t/a.jpg"},
		{"mp4 with transcode", Media{Kind: MediaVideo, Path: "/p/a.mp4", TranscodedPath: "/c/a.ogv"}, "/c/a.ogv", BackgroundVideo, "/p/a.mp4", ""},
		{"ogv source", Media{Kind: MediaVideo, Path: "/p/a.OGV"}, "/p/a.OGV", BackgroundVideo, "/p/a.OGV", ""},
		{"still with a transcode field is still a still", Media{Kind: MediaImage, Path: "/p/a.png", TranscodedPath: "/c/x.ogv"}, "", BackgroundImage, "/p/a.png", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.m.PlayablePath(); got != tc.playable {
				t.Errorf("PlayablePath = %q, want %q", got, tc.playable)
			}
			bg := tc.m.Background()
			if bg.Kind != tc.bgKind || bg.Path != tc.bgPath || bg.Poster != tc.poster || bg.Color != (RGBA{A: 1}) {
				t.Errorf("Background = %+v", bg)
			}
			s := tc.m.Slide()
			if s.Kind != SlideMedia || s.Text != "" || s.Background == nil || *s.Background != bg {
				t.Errorf("Slide = %+v", s)
			}
		})
	}
}

func TestResolveVideo(t *testing.T) {
	lookup := func(src string) (string, bool) {
		if src == "/p/a.mp4" {
			return "/c/a.ogv", true
		}
		if src == "/p/empty.mp4" {
			return "", true
		}
		return "", false
	}
	tests := []struct {
		name string
		bg   Background
		want string
	}{
		{"known video", Background{Kind: BackgroundVideo, Path: "/p/a.mp4"}, "/c/a.ogv"},
		{"unknown video", Background{Kind: BackgroundVideo, Path: "/p/b.mp4"}, "/p/b.mp4"},
		{"empty playable keeps the source", Background{Kind: BackgroundVideo, Path: "/p/empty.mp4"}, "/p/empty.mp4"},
		{"image is untouched", Background{Kind: BackgroundImage, Path: "/p/a.mp4"}, "/p/a.mp4"},
	}
	for _, tc := range tests {
		got := ResolveVideo(tc.bg, lookup)
		if got.Path != tc.want || got.Kind != tc.bg.Kind || got.Poster != tc.bg.Poster {
			t.Errorf("%s: ResolveVideo = %+v, want path %q", tc.name, got, tc.want)
		}
	}
	bg := Background{Kind: BackgroundVideo, Path: "/p/a.mp4"}
	if got := ResolveVideo(bg, nil); got != bg {
		t.Errorf("nil lookup changed the background: %+v", got)
	}
}

func TestMediaScheduleItemRoundTrip(t *testing.T) {
	m := Media{ID: 7, Kind: MediaVideo, Path: "/p/clip.mp4", Title: "Clip", Duration: 30 * time.Second, ThumbPath: "/t/clip.jpg"}
	s := NewSchedule()
	s.Add(ScheduleItem{Kind: ItemMedia, Title: m.Title, Subtitle: "Video", MediaID: m.ID, Theme: DefaultMediaTheme(), Slides: []Slide{m.Slide()}})
	path := filepath.Join(t.TempDir(), "media.fbs")
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadSchedule(path)
	if err != nil {
		t.Fatal(err)
	}
	it := loaded.Items[0]
	if it.Kind != ItemMedia || it.MediaID != 7 || len(it.Slides) != 1 || it.Slides[0].Kind != SlideMedia {
		t.Fatalf("loaded item = %+v", it)
	}
	bg := it.Slides[0].Background
	if bg == nil || bg.Kind != BackgroundVideo || bg.Path != "/p/clip.mp4" || bg.Poster != "/t/clip.jpg" {
		t.Fatalf("loaded background = %+v", bg)
	}
	// The JSON names are part of the file format.
	raw, _ := json.Marshal(it)
	for _, key := range []string{`"mediaId":7`, `"poster":"/t/clip.jpg"`, `"kind":"media"`, `"fit":"fill"`, `"loop":true`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("item JSON lacks %s: %s", key, raw)
		}
	}
}

func TestMediaAspectAndRepeat(t *testing.T) {
	aspects := []struct {
		in      string
		want    MediaAspect
		wantErr bool
	}{
		{"fill", AspectFill, false}, {"Fit", AspectFit, false}, {" STRETCH ", AspectStretch, false},
		{"", "", true}, {"cover", "", true},
	}
	for _, tc := range aspects {
		got, err := ParseMediaAspect(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("ParseMediaAspect(%q) = %q, %v; want %q, err %v", tc.in, got, err, tc.want, tc.wantErr)
		}
	}
	repeats := []struct {
		in      string
		want    MediaRepeat
		wantErr bool
	}{
		{"loop", RepeatLoop, false}, {"Once", RepeatOnce, false}, {"", "", true}, {"never", "", true},
	}
	for _, tc := range repeats {
		got, err := ParseMediaRepeat(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("ParseMediaRepeat(%q) = %q, %v; want %q, err %v", tc.in, got, err, tc.want, tc.wantErr)
		}
	}
	if MediaAspect("").Normalized() != AspectFill || MediaAspect("x").Normalized() != AspectFill || AspectFit.Normalized() != AspectFit {
		t.Error("MediaAspect.Normalized")
	}
	if MediaRepeat("").Normalized() != RepeatLoop || MediaRepeat("x").Normalized() != RepeatLoop || RepeatOnce.Normalized() != RepeatOnce {
		t.Error("MediaRepeat.Normalized")
	}
	if MediaAspect("").Valid() || MediaRepeat("").Valid() || len(MediaAspects) != 3 || len(MediaRepeats) != 2 {
		t.Error("Valid / menu lists")
	}

	// The item's choices reach the slide background as Fit and Loop.
	backgrounds := []struct {
		name string
		m    Media
		fit  MediaAspect
		loop bool
	}{
		{"still defaults", Media{Kind: MediaImage, Path: "/p/a.jpg"}, AspectFill, false},
		{"still fit; repeat ignored", Media{Kind: MediaImage, Path: "/p/a.jpg", Aspect: AspectFit, Repeat: RepeatOnce}, AspectFit, false},
		{"video defaults loop", Media{Kind: MediaVideo, Path: "/p/a.mp4"}, AspectFill, true},
		{"video stretch, once", Media{Kind: MediaVideo, Path: "/p/a.mp4", Aspect: AspectStretch, Repeat: RepeatOnce}, AspectStretch, false},
		{"unknown values read as the defaults", Media{Kind: MediaVideo, Path: "/p/a.mp4", Aspect: "zoom", Repeat: "twice"}, AspectFill, true},
	}
	for _, tc := range backgrounds {
		bg := tc.m.Background()
		if bg.Fit != tc.fit || bg.Loop != tc.loop {
			t.Errorf("%s: Background = fit %q loop %v; want %q %v", tc.name, bg.Fit, bg.Loop, tc.fit, tc.loop)
		}
	}
}
