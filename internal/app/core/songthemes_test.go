package core

import (
	"testing"

	"freebeams/internal/models"
)

func TestSongTheme(t *testing.T) {
	def := namedTheme(1, "Default Song")
	list := []models.Theme{def, namedTheme(2, "Sunday"), namedTheme(3, "Evening")}
	cases := []struct {
		name     string
		themeID  int64
		list     []models.Theme
		want     string // the deck's theme
		wantName string // SongThemeName: "" = uses the default
	}{
		{"no theme of its own", 0, list, "Default Song", ""},
		{"its own theme", 2, list, "Sunday", "Sunday"},
		{"another theme", 3, list, "Evening", "Evening"},
		{"the default theme chosen explicitly", 1, list, "Default Song", "Default Song"},
		{"deleted theme falls back", 99, list, "Default Song", ""},
		{"list not loaded yet", 2, nil, "Default Song", ""},
	}
	for _, tc := range cases {
		song := amazingGrace()
		song.ThemeID = tc.themeID
		if got := SongTheme(song, tc.list, def); got.Name != tc.want {
			t.Errorf("%s: SongTheme = %q, want %q", tc.name, got.Name, tc.want)
		}
		if got := SongThemeName(tc.themeID, tc.list); got != tc.wantName {
			t.Errorf("%s: SongThemeName = %q, want %q", tc.name, got, tc.wantName)
		}
	}
	// The deck carries the resolved theme.
	song := amazingGrace()
	song.ThemeID = 3
	if d := SongDeckWith(song, SongTheme(song, list, def)); d.Theme.ID != 3 {
		t.Errorf("deck theme = %d, want 3", d.Theme.ID)
	}
}

func TestSongThemeArg(t *testing.T) {
	list := []models.Theme{namedTheme(1, "Default Song"), namedTheme(2, "Sunday Night")}
	cases := []struct {
		arg    string
		wantID int64
		wantOK bool
	}{
		{"", 0, true},
		{"(Default)", 0, true},
		{" (default) ", 0, true},
		{"Sunday Night", 2, true},
		{"sunday", 2, true},
		{"Default Song", 1, true},
		{"Nope", 0, false},
	}
	for _, tc := range cases {
		id, ok := SongThemeArg(list, tc.arg)
		if id != tc.wantID || ok != tc.wantOK {
			t.Errorf("SongThemeArg(%q) = %d, %v; want %d, %v", tc.arg, id, ok, tc.wantID, tc.wantOK)
		}
	}
}

func TestRestyleLibraryDeck(t *testing.T) {
	def, own := namedTheme(1, "Default Song"), namedTheme(2, "Sunday")
	song := amazingGrace()
	old := SongDeckWith(song, def)
	fromSchedule := old
	fromSchedule.ItemID = "item-1"
	cases := []struct {
		name string
		deck models.Deck
		want bool
	}{
		{"the song's library deck", old, true},
		{"the same song from the schedule", fromSchedule, false},
		{"another song", SongDeckWith(models.Song{Title: "Other", Sections: song.Sections}, def), false},
		{"already restyled", SongDeckWith(song, own), false},
	}
	for _, tc := range cases {
		d := tc.deck
		th, ok := RestyleLibraryDeck(old, own)(&d)
		if ok != tc.want || (ok && th != own) {
			t.Errorf("%s: RestyleLibraryDeck = %q, %v; want %v", tc.name, th.Name, ok, tc.want)
		}
	}
	if _, ok := RestyleLibraryDeck(old, own)(nil); ok {
		t.Error("RestyleLibraryDeck matched a nil deck")
	}
}

func TestForgetSongTheme(t *testing.T) {
	rows := []models.SongSummary{{ID: 1, ThemeID: 5}, {ID: 2}, {ID: 3, ThemeID: 5}, {ID: 4, ThemeID: 6}}
	if n := ForgetSongTheme(rows, 5); n != 2 {
		t.Fatalf("ForgetSongTheme = %d, want 2", n)
	}
	for i, want := range []int64{0, 0, 0, 6} {
		if rows[i].ThemeID != want {
			t.Errorf("row %d theme = %d, want %d", i, rows[i].ThemeID, want)
		}
	}
	if n := ForgetSongTheme(rows, 0); n != 0 {
		t.Errorf("ForgetSongTheme(0) = %d", n)
	}
}
