package core

import (
	"reflect"
	"testing"

	"freeworship/internal/models"
)

func namedTheme(id int64, name string) models.Theme {
	t := models.DefaultSongTheme()
	t.ID, t.Name = id, name
	return t
}

func TestThemeDeckAndSample(t *testing.T) {
	th := namedTheme(3, "Sunday")
	d := ThemeDeck(th)
	if d.Title != "Sunday" || d.ItemID != "" || d.Theme != th || len(d.Slides) != 1 {
		t.Fatalf("ThemeDeck = %+v", d)
	}
	s := d.Slides[0]
	if s.Kind != models.SlideSong || s.Label != "Verse 1" || s.Footer == "" || s.Text == "" || s.Group != models.SectionVerse {
		t.Fatalf("sample slide = %+v", s)
	}
	song := SongDeckWith(amazingGrace(), th)
	if song.Theme != th || song.Title != "Amazing Grace" || len(song.Slides) != 3 {
		t.Fatalf("SongDeckWith = %+v", song)
	}
	p, _ := models.ParseReference("jn 3:16")
	sc := ScriptureDeckWith(p, "KJV", []models.Verse{{Book: 43, Chapter: 3, Verse: 16, Text: "For God"}}, th)
	if sc.Theme != th || sc.Title != "John 3:16" || len(sc.Slides) != 1 {
		t.Fatalf("ScriptureDeckWith = %+v", sc)
	}
}

func TestFindThemeAndIndexOf(t *testing.T) {
	list := []models.Theme{namedTheme(10, "Default Scripture"), namedTheme(11, "Default Song"), namedTheme(12, "Sunday Night")}
	finds := []struct {
		name string
		want int
	}{
		{"Default Song", 1}, {"default song", 1}, {"Default", 0}, {"night", 2}, {"  Sunday Night ", 2}, {"", -1}, {"Nope", -1},
	}
	for _, tc := range finds {
		if got := FindTheme(list, tc.name); got != tc.want {
			t.Errorf("FindTheme(%q) = %d, want %d", tc.name, got, tc.want)
		}
	}
	ids := []struct {
		id   int64
		want int
	}{{10, 0}, {12, 2}, {0, -1}, {99, -1}}
	for _, tc := range ids {
		if got := ThemeIndexOf(list, tc.id); got != tc.want {
			t.Errorf("ThemeIndexOf(%d) = %d, want %d", tc.id, got, tc.want)
		}
	}
}

func TestCopyName(t *testing.T) {
	tests := []struct {
		name  string
		taken []string
		want  string
	}{
		{"Sunday", nil, "Sunday Copy"},
		{"Sunday", []string{"Sunday", "Sunday Copy"}, "Sunday Copy 2"},
		{"Sunday", []string{"sunday copy", "Sunday Copy 2 "}, "Sunday Copy 3"},
		{"  Sunday ", []string{"Sunday"}, "Sunday Copy"},
		{"", nil, "Theme Copy"},
	}
	for _, tc := range tests {
		if got := CopyName(tc.name, tc.taken); got != tc.want {
			t.Errorf("CopyName(%q, %v) = %q, want %q", tc.name, tc.taken, got, tc.want)
		}
	}
}

func TestThemeBadge(t *testing.T) {
	defaults := map[models.ItemKind]int64{models.ItemSong: 1, models.ItemScripture: 2}
	tests := []struct {
		theme models.Theme
		want  string
	}{
		{namedTheme(1, "a"), "Songs"},
		{namedTheme(2, "b"), "Scripture"},
		{namedTheme(3, "c"), ""},
		{namedTheme(0, "built-in"), ""},
	}
	for _, tc := range tests {
		if got := ThemeBadge(tc.theme, defaults); got != tc.want {
			t.Errorf("ThemeBadge(%d) = %q, want %q", tc.theme.ID, got, tc.want)
		}
		if got := IsDefaultTheme(tc.theme, defaults); got != (tc.want != "") {
			t.Errorf("IsDefaultTheme(%d) = %v", tc.theme.ID, got)
		}
	}
	both := map[models.ItemKind]int64{models.ItemSong: 5, models.ItemScripture: 5}
	if got := ThemeBadge(namedTheme(5, "x"), both); got != "Songs · Scripture" {
		t.Errorf("both defaults badge = %q", got)
	}
	if ThemeCountText(1) != "1 theme" || ThemeCountText(2) != "2 themes" {
		t.Error("ThemeCountText")
	}
}

func TestRestyleScheduleAndApply(t *testing.T) {
	old := namedTheme(7, "Old")
	other := namedTheme(8, "Other")
	s := models.NewSchedule()
	s.Add(models.ScheduleItem{Kind: models.ItemSong, Title: "A", Theme: old, Slides: []models.Slide{{Text: "a"}}})
	s.Add(models.ScheduleItem{Kind: models.ItemScripture, Title: "B", Theme: other})
	s.Add(models.ScheduleItem{Kind: models.ItemSong, Title: "C", Theme: old})
	s.Dirty = false

	renamed := old
	renamed.Name = "Renamed"
	renamed.Font.Size = 50
	if n := RestyleSchedule(s, renamed); n != 2 || !s.Dirty {
		t.Fatalf("RestyleSchedule = %d, dirty %v; want 2, true", n, s.Dirty)
	}
	if s.Items[0].Theme != renamed || s.Items[1].Theme != other || s.Items[2].Theme != renamed {
		t.Errorf("themes after restyle: %q %q %q", s.Items[0].Theme.Name, s.Items[1].Theme.Name, s.Items[2].Theme.Name)
	}
	if s.Items[0].Slides[0].Text != "a" {
		t.Error("slides changed")
	}
	s.Dirty = false
	if n := RestyleSchedule(s, renamed); n != 0 || s.Dirty {
		t.Errorf("second restyle = %d, dirty %v", n, s.Dirty)
	}
	if RestyleSchedule(s, namedTheme(0, "built-in")) != 0 || RestyleSchedule(nil, renamed) != 0 {
		t.Error("restyle with id 0 / nil schedule changed something")
	}

	// Per-item override.
	if !ApplyItemTheme(s, 1, renamed) || s.Items[1].Theme != renamed || !s.Dirty {
		t.Fatalf("ApplyItemTheme: %+v dirty=%v", s.Items[1].Theme.Name, s.Dirty)
	}
	if ApplyItemTheme(s, 3, renamed) || ApplyItemTheme(s, -1, renamed) || ApplyItemTheme(nil, 0, renamed) {
		t.Error("ApplyItemTheme out of range succeeded")
	}

	// Presenter callbacks.
	deck := &models.Deck{ItemID: s.Items[0].ID, Theme: old}
	if th, ok := RestyleByThemeID(renamed)(deck); !ok || th != renamed {
		t.Error("RestyleByThemeID did not match the deck")
	}
	if _, ok := RestyleByThemeID(other)(deck); ok {
		t.Error("RestyleByThemeID matched another theme")
	}
	if _, ok := RestyleByThemeID(namedTheme(0, "x"))(&models.Deck{}); ok {
		t.Error("RestyleByThemeID matched id 0")
	}
	if th, ok := RestyleByItemID(s.Items[0].ID, other)(deck); !ok || th != other {
		t.Error("RestyleByItemID did not match the item")
	}
	if _, ok := RestyleByItemID("", other)(&models.Deck{}); ok {
		t.Error("RestyleByItemID matched an empty id")
	}
}

func TestBuildThemesState(t *testing.T) {
	list := []models.Theme{namedTheme(1, "Default Scripture"), namedTheme(2, "Default Song")}
	defaults := map[models.ItemKind]int64{models.ItemSong: 2, models.ItemScripture: 1}
	st := BuildThemesState(list, 1, defaults, true, "Default Song")
	want := ThemesState{Count: 2, Names: []string{"Default Scripture", "Default Song"}, Selected: 1, Current: "Default Song",
		SongDefault: "Default Song", ScriptureDefault: "Default Scripture", CountText: "2 themes", Editor: true, Editing: "Default Song"}
	if !reflect.DeepEqual(st, want) {
		t.Fatalf("BuildThemesState = %+v\nwant %+v", st, want)
	}
	empty := BuildThemesState(nil, -1, nil, false, "ignored")
	if empty.Count != 0 || empty.Names == nil || empty.Current != "" || empty.Editing != "" || empty.CountText != "0 themes" {
		t.Fatalf("empty state = %+v", empty)
	}
}
