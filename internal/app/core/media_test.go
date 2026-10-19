package core

import (
	"reflect"
	"testing"
	"time"

	"freeworship/internal/models"
)

func TestMediaDeckAndItem(t *testing.T) {
	video := models.Media{ID: 4, Kind: models.MediaVideo, Path: "/v/clip.mp4", Title: "Clip", Duration: 65 * time.Second, ThumbPath: "/c/clip.jpg"}
	lookup := func(src string) (string, bool) {
		if src == "/v/clip.mp4" {
			return "/c/clip.ogv", true
		}
		return "", false
	}
	d := MediaDeck(video, lookup)
	if d.ItemID != "" || d.Title != "Clip" || d.Theme.Name != "Media" || len(d.Slides) != 1 {
		t.Fatalf("MediaDeck = %+v", d)
	}
	bg := d.Slides[0].Background
	if bg == nil || bg.Kind != models.BackgroundVideo || bg.Path != "/c/clip.ogv" || bg.Poster != "/c/clip.jpg" {
		t.Fatalf("deck background = %+v (want the playable path)", bg)
	}
	if d2 := MediaDeck(video, nil); d2.Slides[0].Background.Path != "/v/clip.mp4" {
		t.Fatalf("MediaDeck without lookup = %+v", d2.Slides[0].Background)
	}

	it := MediaItem(video)
	if it.Kind != models.ItemMedia || it.MediaID != 4 || it.Title != "Clip" || it.Subtitle != "Video · 1:05" || len(it.Slides) != 1 {
		t.Fatalf("MediaItem = %+v", it)
	}
	if it.Slides[0].Background.Path != "/v/clip.mp4" {
		t.Fatalf("schedule snapshot must keep the source path: %+v", it.Slides[0].Background)
	}
	img := models.Media{ID: 5, Kind: models.MediaImage, Path: "/i/a.jpg", Title: "A"}
	if it := MediaItem(img); it.Subtitle != "Image" || it.Slides[0].Background.Kind != models.BackgroundImage {
		t.Fatalf("image MediaItem = %+v", it)
	}
	if it := MediaItem(models.Media{Kind: models.MediaVideo, Path: "/v/x.mp4", Title: "X"}); it.Subtitle != "Video" {
		t.Fatalf("video without duration: subtitle %q", it.Subtitle)
	}
	// A video loops by default; an audio file plays once over a black slide.
	if bg := it.Slides[0].Background; !bg.Loop {
		t.Fatalf("video item must loop by default: %+v", bg)
	}
	snd := models.Media{ID: 6, Kind: models.MediaAudio, Path: "/a/tone.wav", Title: "Tone", Duration: 3 * time.Second}
	if it := MediaItem(snd); it.Subtitle != "Audio · 0:03" || it.Slides[0].Background.Kind != models.BackgroundAudio ||
		it.Slides[0].Background.Path != "/a/tone.wav" || it.Slides[0].Background.Loop || it.Slides[0].Background.Poster != "" {
		t.Fatalf("audio MediaItem = %+v / %+v", it, it.Slides[0].Background)
	}
	if d := MediaDeck(snd, lookup); d.Slides[0].Background.Kind != models.BackgroundAudio || d.Slides[0].Background.Path != "/a/tone.wav" {
		t.Fatalf("audio MediaDeck = %+v", d.Slides[0].Background)
	}

	// ItemDeck → ResolveDeck gives the playable path; the item is untouched.
	deck := ResolveDeck(ItemDeck(it), lookup)
	if deck.ItemID != it.ID || len(deck.Slides) != 1 {
		t.Fatalf("resolved deck = %+v", deck)
	}
	if deck.Slides[0].Background.Path != "/c/clip.ogv" || it.Slides[0].Background.Path != "/v/clip.mp4" {
		t.Fatalf("ResolveDeck: deck %q, item %q", deck.Slides[0].Background.Path, it.Slides[0].Background.Path)
	}
}

func TestResolveDeck(t *testing.T) {
	lookup := func(src string) (string, bool) { return "/c/out.ogv", src == "/v/in.mp4" }
	songDeck := SongDeck(models.Song{Title: "S", Sections: []models.Section{{Kind: models.SectionVerse, Lines: []string{"a"}}}})
	if got := ResolveDeck(&songDeck, lookup); got != &songDeck {
		t.Fatal("a deck without videos must be returned as is")
	}
	if got := ResolveDeck(nil, lookup); got != nil {
		t.Fatal("nil deck")
	}
	unknown := models.Deck{Slides: []models.Slide{{Background: &models.Background{Kind: models.BackgroundVideo, Path: "/v/other.mp4"}}}}
	if got := ResolveDeck(&unknown, lookup); got != &unknown {
		t.Fatal("a deck whose videos have no playable file must be returned as is")
	}
	mixed := models.Deck{Title: "M", Slides: []models.Slide{
		{Text: "plain"},
		{Background: &models.Background{Kind: models.BackgroundImage, Path: "/i.jpg"}},
		{Background: &models.Background{Kind: models.BackgroundVideo, Path: "/v/in.mp4", Poster: "/p.jpg"}},
	}}
	before := *mixed.Slides[2].Background
	got := ResolveDeck(&mixed, lookup)
	if got == &mixed || got.Title != "M" || len(got.Slides) != 3 {
		t.Fatalf("ResolveDeck = %+v", got)
	}
	if got.Slides[2].Background.Path != "/c/out.ogv" || got.Slides[2].Background.Poster != "/p.jpg" {
		t.Fatalf("resolved slide = %+v", got.Slides[2].Background)
	}
	if *mixed.Slides[2].Background != before || got.Slides[1].Background == mixed.Slides[1].Background {
		t.Fatal("ResolveDeck modified the input deck or shared its backgrounds")
	}
	if got := ResolveDeck(&mixed, nil); got != &mixed {
		t.Fatal("nil lookup must return the deck as is")
	}
}

func TestStillAndThumbBackground(t *testing.T) {
	if StillBackground(nil) != nil {
		t.Fatal("nil")
	}
	img := &models.Background{Kind: models.BackgroundImage, Path: "/i.jpg"}
	if StillBackground(img) != img {
		t.Fatal("images are returned as is")
	}
	v := &models.Background{Kind: models.BackgroundVideo, Path: "/v.ogv", Poster: "/p.jpg", Color: models.RGBA{A: 1}}
	got := StillBackground(v)
	if got == v || got.Kind != models.BackgroundImage || got.Path != "/p.jpg" || got.Color != v.Color {
		t.Fatalf("StillBackground(video) = %+v", got)
	}
	noPoster := &models.Background{Kind: models.BackgroundVideo, Path: "/v.ogv", Color: models.Hex(0x112233)}
	if got := StillBackground(noPoster); got.Kind != models.BackgroundColor || got.Color != noPoster.Color || got.Path != "" {
		t.Fatalf("StillBackground(video without poster) = %+v", got)
	}
	audio := &models.Background{Kind: models.BackgroundAudio, Path: "/a.mp3", Color: models.RGBA{A: 1}}
	if got := StillBackground(audio); got.Kind != models.BackgroundColor || got.Color != audio.Color || got.Path != "" {
		t.Fatalf("StillBackground(audio) = %+v", got)
	}

	tests := []struct {
		m    models.Media
		kind models.BackgroundKind
		path string
	}{
		{models.Media{Kind: models.MediaImage, Path: "/i.jpg", ThumbPath: "/t.jpg"}, models.BackgroundImage, "/t.jpg"},
		{models.Media{Kind: models.MediaImage, Path: "/i.jpg"}, models.BackgroundImage, "/i.jpg"},
		{models.Media{Kind: models.MediaVideo, Path: "/v.mp4", ThumbPath: "/t.jpg"}, models.BackgroundImage, "/t.jpg"},
		{models.Media{Kind: models.MediaVideo, Path: "/v.mp4"}, models.BackgroundColor, ""},
	}
	for _, tc := range tests {
		got := ThumbBackground(tc.m)
		if got.Kind != tc.kind || got.Path != tc.path {
			t.Errorf("ThumbBackground(%+v) = %+v", tc.m, got)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, ""}, {-time.Second, ""}, {400 * time.Millisecond, "0:00"}, {30037 * time.Millisecond, "0:30"},
		{59500 * time.Millisecond, "1:00"}, {65 * time.Second, "1:05"}, {10 * time.Minute, "10:00"},
		{time.Hour + 2*time.Minute + 3*time.Second, "1:02:03"},
	}
	for _, tc := range tests {
		if got := FormatDuration(tc.d); got != tc.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestMediaBadge(t *testing.T) {
	video := models.Media{Kind: models.MediaVideo, Duration: 30 * time.Second}
	all := MediaCaps{FFmpeg: true, Theora: true}
	tests := []struct {
		name     string
		m        models.Media
		playable bool
		caps     MediaCaps
		progress float64
		failed   bool
		want     string
	}{
		{"image", models.Media{Kind: models.MediaImage}, false, MediaCaps{}, 0, false, ""},
		{"playable video shows its duration", video, true, MediaCaps{}, 0, false, "0:30"},
		{"playable video without duration", models.Media{Kind: models.MediaVideo}, true, all, 0, false, "Video"},
		{"converting", video, false, all, 0.42, false, "Converting 42%"},
		{"converting beats playable", video, true, all, 0.5, false, "Converting 50%"},
		{"done converting", video, true, all, 1, false, "0:30"},
		{"no ffmpeg", video, false, MediaCaps{}, 0, false, "Needs ffmpeg"},
		{"no theora", video, false, MediaCaps{FFmpeg: true}, 0, false, "Needs libtheora"},
		{"failed", video, false, all, 0, true, "Unavailable"},
		{"failed but playable", video, true, all, 0, true, "0:30"},
		{"queued", video, false, all, 0, false, "Queued"},
		// A native player (macOS) plays every video as it is: never a complaint.
		{"native plays an unconverted video", video, false, MediaCaps{Native: true}, 0, false, "0:30"},
		{"native without duration", models.Media{Kind: models.MediaVideo}, false, MediaCaps{Native: true}, 0, true, "Video"},
		{"audio shows its duration", models.Media{Kind: models.MediaAudio, Duration: 3 * time.Second}, false, MediaCaps{}, 0, false, "0:03"},
		{"audio without duration", models.Media{Kind: models.MediaAudio}, false, MediaCaps{}, 0, true, "Audio"},
	}
	for _, tc := range tests {
		if got := MediaBadge(tc.m, tc.playable, tc.caps, tc.progress, tc.failed); got != tc.want {
			t.Errorf("%s: MediaBadge = %q, want %q", tc.name, got, tc.want)
		}
	}
	if !(MediaCaps{Native: true}).CanPlay(false) || (MediaCaps{}).CanPlay(false) || !(MediaCaps{}).CanPlay(true) {
		t.Error("CanPlay")
	}
}

func TestMediaFooter(t *testing.T) {
	all := MediaCaps{FFmpeg: true, Theora: true}
	native := MediaCaps{FFmpeg: true, Native: true}
	tests := []struct {
		name    string
		counts  MediaCounts
		kind    models.MediaKind
		caps    MediaCaps
		working bool
		want    string
	}{
		{"empty", MediaCounts{}, "", MediaCaps{}, false, "0 items"},
		{"all", MediaCounts{8, 15, 0}, "", all, false, "23 items · 8 videos · 15 images"},
		{"one each", MediaCounts{1, 1, 0}, "", all, false, "2 items · 1 video · 1 image"},
		{"videos", MediaCounts{8, 15, 0}, models.MediaVideo, all, false, "8 videos"},
		{"images", MediaCounts{8, 15, 0}, models.MediaImage, all, false, "15 images"},
		{"working", MediaCounts{8, 15, 0}, "", all, true, "23 items · 8 videos · 15 images · preparing media…"},
		{"no ffmpeg", MediaCounts{8, 15, 0}, "", MediaCaps{}, false, "23 items · 8 videos · 15 images · ffmpeg not found: videos cannot play"},
		{"no theora", MediaCounts{8, 15, 0}, models.MediaVideo, MediaCaps{FFmpeg: true}, false, "8 videos · ffmpeg has no Theora encoder: videos cannot play"},
		{"images ignore ffmpeg", MediaCounts{8, 15, 0}, models.MediaImage, MediaCaps{}, false, "15 images"},
		{"no videos ignore ffmpeg", MediaCounts{0, 3, 0}, "", MediaCaps{}, false, "3 items · 0 videos · 3 images"},
		{"native never complains", MediaCounts{8, 15, 0}, "", native, false, "23 items · 8 videos · 15 images"},
		{"native without ffmpeg", MediaCounts{8, 15, 0}, models.MediaVideo, MediaCaps{Native: true}, false, "8 videos"},
		{"audio counted", MediaCounts{8, 15, 2}, "", native, false, "25 items · 8 videos · 15 images · 2 audio"},
		{"audio collection", MediaCounts{8, 15, 2}, models.MediaAudio, MediaCaps{}, false, "2 audio files"},
		{"one audio file", MediaCounts{0, 0, 1}, models.MediaAudio, MediaCaps{}, false, "1 audio file"},
		{"audio collection ignores ffmpeg", MediaCounts{8, 0, 1}, models.MediaAudio, MediaCaps{}, false, "1 audio file"},
	}
	for _, tc := range tests {
		if got := MediaFooter(tc.counts, tc.kind, tc.caps, tc.working); got != tc.want {
			t.Errorf("%s: MediaFooter = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestFindMediaAndPlayables(t *testing.T) {
	list := []models.Media{
		{ID: 1, Title: "The Cross Loop 04 Still", Path: "/i/The Cross Loop 04 Still.jpg", Kind: models.MediaImage},
		{ID: 2, Title: "Boho Cross", Path: "/v/boho.mp4", Kind: models.MediaVideo, TranscodedPath: "/c/boho.ogv"},
		{ID: 3, Title: "Cross", Path: "/v/cross.mp4", Kind: models.MediaVideo},
		{ID: 4, Title: "Ready", Path: "/v/ready.ogv", Kind: models.MediaVideo},
	}
	tests := []struct {
		title string
		want  int
	}{
		{"cross", 2}, {"CROSS", 2}, {"the cross", 0}, {"boho", 1}, {"oho", 1}, {"ready.ogv", 3}, {"/v/cross.mp4", 2}, {"nope", -1}, {"", -1},
	}
	for _, tc := range tests {
		if got := FindMedia(list, tc.title); got != tc.want {
			t.Errorf("FindMedia(%q) = %d, want %d", tc.title, got, tc.want)
		}
	}
	if MediaIndexOf(list, 3) != 2 || MediaIndexOf(list, 9) != -1 || MediaIndexOf(nil, 1) != -1 {
		t.Error("MediaIndexOf")
	}
	want := map[string]string{"/v/boho.mp4": "/c/boho.ogv", "/v/ready.ogv": "/v/ready.ogv"}
	if got := Playables(list); !reflect.DeepEqual(got, want) {
		t.Errorf("Playables = %v, want %v", got, want)
	}
}
