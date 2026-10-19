package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"freeworship/internal/models"
)

func TestPresentationDeckAndItem(t *testing.T) {
	cache := t.TempDir()
	p := models.Presentation{ID: 3, Path: "/docs/Sermon.pdf", Title: "Sermon", Pages: 2, CacheKey: "k1-Sermon", Renderer: "coregraphics"}
	pages := PresentationPages(p, cache)
	want := []string{filepath.Join(cache, "k1-Sermon", "page-001.png"), filepath.Join(cache, "k1-Sermon", "page-002.png")}
	if !reflect.DeepEqual(pages, want) {
		t.Fatalf("PresentationPages = %v, want %v", pages, want)
	}
	if PresentationPages(models.Presentation{Path: "/x.pdf"}, cache) != nil || PresentationPages(p, "") != nil {
		t.Fatal("unrendered / no cache must give no pages")
	}

	d := PresentationDeck(p, pages)
	if d.ItemID != "" || d.Title != "Sermon" || d.Theme.Name != "Presentation" || len(d.Slides) != 2 {
		t.Fatalf("PresentationDeck = %+v", d)
	}
	s := d.Slides[1]
	if s.Kind != models.SlidePresentation || s.Label != "Slide 2" || s.Text != "" || s.Background == nil ||
		s.Background.Kind != models.BackgroundImage || s.Background.Path != want[1] || s.Background.Color != (models.RGBA{A: 1}) {
		t.Fatalf("slide 2 = %+v (bg %+v)", s, s.Background)
	}
	// Unrendered: a single placeholder slide.
	ph := PresentationDeck(models.Presentation{Title: "New"}, nil)
	if len(ph.Slides) != 1 || ph.Slides[0].Text == "" || ph.Slides[0].Background == nil || ph.Slides[0].Background.Kind != models.BackgroundColor {
		t.Fatalf("placeholder deck = %+v", ph)
	}

	it := PresentationItem(p, pages)
	if it.Kind != models.ItemPresentation || it.PresentationID != 3 || it.Source != "/docs/Sermon.pdf" || it.CacheKey != "k1-Sermon" ||
		it.Title != "Sermon" || it.Subtitle != "Presentation · 2 slides" || len(it.Slides) != 2 || it.Theme.Name != "Presentation" {
		t.Fatalf("PresentationItem = %+v", it)
	}
	partial := p
	partial.Partial, partial.Pages = true, 1
	if got := PresentationSubtitle(partial); got != "Presentation · 1 slide · embedded images" {
		t.Errorf("PresentationSubtitle(partial) = %q", got)
	}
	if got := PresentationSubtitle(models.Presentation{}); got != "Presentation" {
		t.Errorf("PresentationSubtitle(unrendered) = %q", got)
	}

	// ItemDeck keeps the item id and slides.
	deck := ItemDeck(it)
	if deck.ItemID != it.ID || len(deck.Slides) != 2 || deck.Slides[0].Background.Path != want[0] {
		t.Fatalf("ItemDeck = %+v", deck)
	}
}

func TestResolvePresentationItem(t *testing.T) {
	cache := t.TempDir()
	oldDir := filepath.Join(cache, "old")
	newDir := filepath.Join(cache, "new")
	for _, d := range []string{oldDir, newDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p string) {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	oldPages := []string{filepath.Join(oldDir, "page-001.png")}
	newPages := []string{filepath.Join(newDir, "page-001.png"), filepath.Join(newDir, "page-002.png")}
	for _, p := range newPages {
		write(p)
	}
	write(oldPages[0])
	p := models.Presentation{ID: 1, Path: "/d/a.pdf", Title: "A", Pages: 1, CacheKey: "old"}
	item := PresentationItem(p, oldPages)
	lookup := func(source string) ([]string, string, bool) {
		if source == "/d/a.pdf" {
			return newPages, "new", true
		}
		return nil, "", false
	}

	// Stale key: rebuilt from the library's render.
	got := ResolvePresentationItem(item, lookup)
	if got.CacheKey != "new" || len(got.Slides) != 2 || got.Slides[1].Background.Path != newPages[1] || got.ID != item.ID {
		t.Fatalf("stale item resolved = %+v", got)
	}
	if item.CacheKey != "old" || len(item.Slides) != 1 {
		t.Fatal("the input item must not change")
	}
	// Same key and pages present: untouched.
	same := item
	same.CacheKey = "new"
	same.Slides = PresentationSlides(newPages)
	if got := ResolvePresentationItem(same, lookup); !reflect.DeepEqual(got, same) {
		t.Fatalf("current item changed: %+v", got)
	}
	// Same key but the pages are gone: rebuilt.
	gone := same
	gone.Slides = PresentationSlides([]string{filepath.Join(newDir, "missing.png")})
	if got := ResolvePresentationItem(gone, lookup); len(got.Slides) != 2 || got.Slides[0].Background.Path != newPages[0] {
		t.Fatalf("item with missing pages = %+v", got)
	}
	// Unknown source, nil lookup, other kinds: untouched.
	other := item
	other.Source = "/d/unknown.pdf"
	if got := ResolvePresentationItem(other, lookup); !reflect.DeepEqual(got, other) {
		t.Fatal("unknown source must be returned as is")
	}
	if got := ResolvePresentationItem(item, nil); !reflect.DeepEqual(got, item) {
		t.Fatal("nil lookup must return the item as is")
	}
	song := models.ScheduleItem{Kind: models.ItemSong, Source: "/d/a.pdf"}
	if got := ResolvePresentationItem(song, lookup); !reflect.DeepEqual(got, song) {
		t.Fatal("a song must be returned as is")
	}
}

func TestPresentationThumb(t *testing.T) {
	cache := "/cache"
	tests := []struct {
		p    models.Presentation
		dir  string
		kind models.BackgroundKind
		path string
	}{
		{models.Presentation{Pages: 2, CacheKey: "k"}, cache, models.BackgroundImage, filepath.Join(cache, "k", "thumb.jpg")},
		{models.Presentation{Pages: 2}, cache, models.BackgroundColor, ""},
		{models.Presentation{Pages: 2, CacheKey: "k"}, "", models.BackgroundColor, ""},
		{models.Presentation{}, cache, models.BackgroundColor, ""},
	}
	for _, tc := range tests {
		got := PresentationThumb(tc.p, tc.dir)
		if got.Kind != tc.kind || got.Path != tc.path {
			t.Errorf("PresentationThumb(%+v, %q) = %+v", tc.p, tc.dir, got)
		}
	}
}

func TestPresentationBadge(t *testing.T) {
	all := PresentationCaps{PDF: true, Office: true}
	pdf := models.Presentation{Path: "/d/a.pdf"}
	pptx := models.Presentation{Path: "/d/a.pptx"}
	ppt := models.Presentation{Path: "/d/a.ppt"}
	done := models.Presentation{Path: "/d/a.pdf", Pages: 12, CacheKey: "k"}
	one := models.Presentation{Path: "/d/a.pdf", Pages: 1, CacheKey: "k"}
	emb := models.Presentation{Path: "/d/a.pptx", Pages: 5, CacheKey: "k", Partial: true}
	tests := []struct {
		name   string
		p      models.Presentation
		prog   RenderProgress
		failed bool
		caps   PresentationCaps
		want   string
	}{
		{"rendered", done, RenderProgress{}, false, all, "12 slides"},
		{"one slide", one, RenderProgress{}, false, all, "1 slide"},
		{"embedded", emb, RenderProgress{}, false, PresentationCaps{PDF: true}, "5 slides · embedded"},
		{"rendering", pdf, RenderProgress{true, 3, 12}, false, all, "Rendering 3/12"},
		{"rendering started", pdf, RenderProgress{Active: true}, false, all, "Rendering…"},
		{"inactive progress is queued", pdf, RenderProgress{false, 3, 12}, false, all, "Queued"},
		{"progress after done shows count", done, RenderProgress{true, 12, 12}, false, all, "12 slides"},
		{"failed", pdf, RenderProgress{}, true, all, "Unavailable"},
		{"failed but rendered", done, RenderProgress{}, true, all, "12 slides"},
		{"pdf without renderer", pdf, RenderProgress{}, false, PresentationCaps{}, "Needs poppler"},
		{"pptx without office is queued (fallback)", pptx, RenderProgress{}, false, PresentationCaps{PDF: true}, "Queued"},
		{"ppt without office", ppt, RenderProgress{}, false, PresentationCaps{PDF: true}, "Needs LibreOffice"},
		{"ppt with office", ppt, RenderProgress{}, false, all, "Queued"},
		{"queued", pdf, RenderProgress{}, false, all, "Queued"},
	}
	for _, tc := range tests {
		if got := PresentationBadge(tc.p, tc.prog, tc.failed, tc.caps); got != tc.want {
			t.Errorf("%s: PresentationBadge = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestPresentationsFooter(t *testing.T) {
	all := PresentationCaps{PDF: true, Office: true}
	tests := []struct {
		name          string
		count, office int
		caps          PresentationCaps
		working       bool
		want          string
	}{
		{"empty", 0, 0, PresentationCaps{}, false, "0 presentations"},
		{"one", 1, 0, all, false, "1 presentation"},
		{"many", 3, 1, all, false, "3 presentations"},
		{"working", 3, 1, all, true, "3 presentations · rendering slides…"},
		{"no pdf renderer", 2, 0, PresentationCaps{}, false, "2 presentations · no PDF renderer: install poppler (pdftoppm)"},
		{"no office with office docs", 2, 1, PresentationCaps{PDF: true}, false, "2 presentations · LibreOffice not found: PowerPoint shows its embedded images only"},
		{"no office without office docs", 2, 0, PresentationCaps{PDF: true}, false, "2 presentations"},
		{"empty ignores tools", 0, 0, PresentationCaps{}, false, "0 presentations"},
	}
	for _, tc := range tests {
		if got := PresentationsFooter(tc.count, tc.office, tc.caps, tc.working); got != tc.want {
			t.Errorf("%s: PresentationsFooter = %q, want %q", tc.name, got, tc.want)
		}
	}
	list := []models.Presentation{{Path: "/a.pdf"}, {Path: "/b.PPTX"}, {Path: "/c.key"}, {Path: "/d.PDF"}}
	if got := OfficeCount(list); got != 2 {
		t.Errorf("OfficeCount = %d, want 2", got)
	}
}

func TestFindPresentation(t *testing.T) {
	list := []models.Presentation{
		{ID: 1, Title: "Easter Sunday", Path: "/d/Easter Sunday.pdf"},
		{ID: 2, Title: "Christmas", Path: "/d/xmas.pptx"},
		{ID: 3, Title: "Sunday Notes", Path: "/d/notes.pdf"},
	}
	tests := []struct {
		title string
		want  int
	}{
		{"christmas", 1}, {"SUNDAY", 2}, {"easter", 0}, {"notes", 2}, {"xmas.pptx", 1}, {"/d/notes.pdf", 2}, {"nope", -1}, {"", -1},
	}
	for _, tc := range tests {
		if got := FindPresentation(list, tc.title); got != tc.want {
			t.Errorf("FindPresentation(%q) = %d, want %d", tc.title, got, tc.want)
		}
	}
	if PresentationIndexOf(list, 3) != 2 || PresentationIndexOf(list, 9) != -1 || PresentationIndexOf(nil, 1) != -1 {
		t.Error("PresentationIndexOf")
	}
}

func TestPresentationScheduleItemRoundTrip(t *testing.T) {
	p := models.Presentation{ID: 7, Path: "/d/Talk.pptx", Title: "Talk", Pages: 2, CacheKey: "k7", Partial: true}
	s := models.NewSchedule()
	s.Add(PresentationItem(p, []string{"/c/k7/page-001.png", "/c/k7/page-002.png"}))
	path := filepath.Join(t.TempDir(), "s.fws")
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := models.LoadSchedule(path)
	if err != nil {
		t.Fatal(err)
	}
	it := loaded.Items[0]
	if it.Kind != models.ItemPresentation || it.PresentationID != 7 || it.Source != "/d/Talk.pptx" || it.CacheKey != "k7" ||
		len(it.Slides) != 2 || it.Slides[1].Background.Path != "/c/k7/page-002.png" || it.Subtitle != "Presentation · 2 slides · embedded images" {
		t.Fatalf("loaded item = %+v", it)
	}
}
