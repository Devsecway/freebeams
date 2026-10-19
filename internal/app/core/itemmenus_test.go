package core

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"freeworship/internal/models"
)

func TestItemViewAndSort(t *testing.T) {
	views := []struct {
		in      string
		want    ItemView
		wantErr bool
	}{
		{"large", ViewLarge, false}, {"Medium", ViewMedium, false}, {" small ", ViewSmall, false}, {"DETAILS", ViewDetails, false},
		{"", "", true}, {"huge", "", true},
	}
	for _, tc := range views {
		got, err := ParseItemView(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("ParseItemView(%q) = %q, %v; want %q, err %v", tc.in, got, err, tc.want, tc.wantErr)
		}
	}
	sorts := []struct {
		in      string
		want    ItemSort
		wantErr bool
	}{
		{"name", SortName, false}, {"Date", SortDate, false}, {"KIND", SortKind, false}, {"", "", true}, {"size", "", true},
	}
	for _, tc := range sorts {
		got, err := ParseItemSort(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("ParseItemSort(%q) = %q, %v; want %q, err %v", tc.in, got, err, tc.want, tc.wantErr)
		}
	}
	if ItemView("").Normalized() != ViewMedium || ItemView("x").Normalized() != ViewMedium || ViewDetails.Normalized() != ViewDetails {
		t.Error("ItemView.Normalized")
	}
	if ItemSort("").Normalized() != SortName || ItemSort("x").Normalized() != SortName || SortKind.Normalized() != SortKind {
		t.Error("ItemSort.Normalized")
	}
	if len(ItemViews) != 4 || len(ItemSorts) != 3 || ItemView("").Valid() || ItemSort("").Valid() {
		t.Error("menu lists / Valid")
	}
}

func TestSortMedia(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	seed := []models.Media{
		{ID: 1, Kind: models.MediaVideo, Title: "Plexus", Added: day(1)},
		{ID: 2, Kind: models.MediaImage, Title: "cross", Added: day(3)},
		{ID: 3, Kind: models.MediaImage, Title: "Snow", Added: day(2)},
		{ID: 4, Kind: models.MediaVideo, Title: "Boho", Added: day(3)},
		{ID: 5, Kind: models.MediaImage, Title: "Cross", Added: day(2)}, // same title as 2, case aside
	}
	titles := func(list []models.Media) []string {
		out := make([]string, len(list))
		for i, m := range list {
			out[i] = m.Title
		}
		return out
	}
	tests := []struct {
		by   ItemSort
		want []string
	}{
		{SortName, []string{"Boho", "cross", "Cross", "Plexus", "Snow"}},
		{"", []string{"Boho", "cross", "Cross", "Plexus", "Snow"}},
		{SortDate, []string{"Boho", "cross", "Cross", "Snow", "Plexus"}}, // newest first; ties by title
		{SortKind, []string{"cross", "Cross", "Snow", "Boho", "Plexus"}}, // images, then videos; each by title
	}
	for _, tc := range tests {
		list := append([]models.Media(nil), seed...)
		if got := titles(SortMedia(list, tc.by)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("SortMedia(%q) = %v, want %v", tc.by, got, tc.want)
		}
	}
	if SortMedia(nil, SortDate) != nil {
		t.Error("nil list")
	}
}

func TestSortPresentations(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	seed := []models.Presentation{
		{ID: 1, Path: "/d/sermon.pptx", Title: "Sermon", Added: day(1)},
		{ID: 2, Path: "/d/announcements.pdf", Title: "Announcements", Added: day(3)},
		{ID: 3, Path: "/d/welcome.key", Title: "Welcome", Added: day(2)},
		{ID: 4, Path: "/d/budget.pdf", Title: "Budget", Added: day(2)},
	}
	titles := func(list []models.Presentation) []string {
		out := make([]string, len(list))
		for i, p := range list {
			out[i] = p.Title
		}
		return out
	}
	tests := []struct {
		by   ItemSort
		want []string
	}{
		{SortName, []string{"Announcements", "Budget", "Sermon", "Welcome"}},
		{SortDate, []string{"Announcements", "Budget", "Welcome", "Sermon"}},
		{SortKind, []string{"Welcome", "Announcements", "Budget", "Sermon"}}, // Keynote, PDF, PowerPoint
	}
	for _, tc := range tests {
		list := append([]models.Presentation(nil), seed...)
		if got := titles(SortPresentations(list, tc.by)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("SortPresentations(%q) = %v, want %v", tc.by, got, tc.want)
		}
	}
	kinds := map[string]string{"/a.pdf": "PDF", "/a.PPTX": "PowerPoint", "/a.ppt": "PowerPoint", "/a.key": "Keynote", "/a.odp": "OpenDocument", "/a.xyz": "XYZ"}
	for path, want := range kinds {
		if got := PresentationKind(path); got != want {
			t.Errorf("PresentationKind(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestItemLogos(t *testing.T) {
	img := models.Media{ID: 1, Kind: models.MediaImage, Path: "/i/a.jpg", Aspect: models.AspectFit}
	bg := MediaLogo(img)
	if bg.Kind != models.BackgroundImage || bg.Path != "/i/a.jpg" || bg.Fit != models.AspectFit || bg.Color != LogoGradient().Color {
		t.Fatalf("MediaLogo(image) = %+v", bg)
	}
	vid := models.Media{ID: 2, Kind: models.MediaVideo, Path: "/v/a.mp4", ThumbPath: "/t/a.jpg"}
	if bg := MediaLogo(vid); bg.Kind != models.BackgroundVideo || bg.Poster != "/t/a.jpg" || !bg.Loop || bg.Fit != models.AspectFill {
		t.Fatalf("MediaLogo(video) = %+v", bg)
	}
	if bg, ok := PresentationLogo(models.Presentation{Path: "/d/a.pdf"}, t.TempDir()); ok || bg.Kind != "" {
		t.Fatalf("PresentationLogo(unrendered) = %+v, %v", bg, ok)
	}
	p := models.Presentation{Path: "/d/a.pdf", Pages: 2, CacheKey: "k"}
	bg, ok := PresentationLogo(p, "/cache")
	if !ok || bg.Kind != models.BackgroundImage || bg.Path != PresentationPages(p, "/cache")[0] || bg.Color != LogoGradient().Color {
		t.Fatalf("PresentationLogo = %+v, %v", bg, ok)
	}
}

func TestMissingFilesAndRefreshQuestion(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "here.png")
	if err := os.WriteFile(present, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(dir, "gone.png")
	if FileMissing(present) || !FileMissing(gone) || !FileMissing(dir) {
		t.Fatal("FileMissing")
	}
	media := []models.Media{{ID: 1, Path: present}, {ID: 2, Path: gone}}
	if got := MissingMedia(media); len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("MissingMedia = %+v", got)
	}
	pres := []models.Presentation{{ID: 1, Path: gone}, {ID: 2, Path: present}, {ID: 3, Path: filepath.Join(dir, "nope.pdf")}}
	if got := MissingPresentations(pres); len(got) != 2 || got[0].ID != 1 || got[1].ID != 3 {
		t.Fatalf("MissingPresentations = %+v", got)
	}
	if q := RefreshQuestion(1, "media"); !strings.HasPrefix(q, "1 media file is missing") || !strings.Contains(q, "Remove it") {
		t.Errorf("RefreshQuestion(1) = %q", q)
	}
	if q := RefreshQuestion(3, "presentation"); !strings.HasPrefix(q, "3 presentation files are missing") || !strings.Contains(q, "Remove them") {
		t.Errorf("RefreshQuestion(3) = %q", q)
	}
}

func TestUpdateMediaItems(t *testing.T) {
	m := models.Media{ID: 7, Kind: models.MediaVideo, Path: "/v/clip.mp4", Title: "Clip", Duration: 30 * time.Second}
	s := models.NewSchedule()
	s.Add(MediaItem(m))
	s.Add(SongItemFor(t))
	other := models.Media{ID: 8, Kind: models.MediaImage, Path: "/i/a.jpg", Title: "Other"}
	s.Add(MediaItem(other))
	s.Add(MediaItem(m))
	s.Dirty = false

	if n := UpdateMediaItems(s, m); n != 0 || s.Dirty {
		t.Fatalf("unchanged item: n = %d, dirty %v", n, s.Dirty)
	}
	m.Aspect, m.Repeat, m.Title, m.ThumbPath = models.AspectFit, models.RepeatOnce, "Clip (fit)", "/t/clip.jpg"
	n := UpdateMediaItems(s, m)
	if n != 2 || !s.Dirty {
		t.Fatalf("n = %d, dirty %v", n, s.Dirty)
	}
	for _, i := range []int{0, 3} {
		it := s.Items[i]
		bg := it.Slides[0].Background
		if it.Title != "Clip (fit)" || it.Subtitle != "Video · 0:30" || it.MediaID != 7 || bg.Fit != models.AspectFit || bg.Loop || bg.Poster != "/t/clip.jpg" {
			t.Errorf("item %d = %+v (bg %+v)", i, it, bg)
		}
	}
	if s.Items[2].Title != "Other" || s.Items[2].Slides[0].Background.Fit != models.AspectFill {
		t.Errorf("another media item changed: %+v", s.Items[2])
	}
	if s.Items[1].Kind != models.ItemSong {
		t.Errorf("the song changed: %+v", s.Items[1])
	}
	// The two items are independent copies.
	s.Items[0].Slides[0].Label = "x"
	if s.Items[3].Slides[0].Label == "x" {
		t.Error("items share slides")
	}
	if UpdateMediaItems(nil, m) != 0 || UpdateMediaItems(s, models.Media{}) != 0 {
		t.Error("nil schedule / unsaved item")
	}
}

// SongItemFor is a minimal song schedule item for tests.
func SongItemFor(t *testing.T) models.ScheduleItem {
	t.Helper()
	song := models.Song{ID: 1, Title: "S", Sections: []models.Section{{Kind: models.SectionVerse, Lines: []string{"a"}}}}
	return SongItem(song, SongDeck(song))
}

func TestRevealCommand(t *testing.T) {
	tests := []struct {
		goos string
		name string
		args []string
	}{
		{"darwin", "open", []string{"-R", "/m/a.png"}},
		{"windows", "explorer", []string{"/select,/m/a.png"}},
		{"linux", "xdg-open", []string{"/m"}},
		{"freebsd", "xdg-open", []string{"/m"}},
	}
	for _, tc := range tests {
		name, args := RevealCommand("/m/a.png", tc.goos)
		if name != tc.name || !reflect.DeepEqual(args, tc.args) {
			t.Errorf("RevealCommand(%s) = %s %v, want %s %v", tc.goos, name, args, tc.name, tc.args)
		}
	}
}
