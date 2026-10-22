package store

import (
	"context"
	"errors"
	"testing"

	"freebeams/internal/models"
)

func TestSongThemes(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)

	sunday := models.DefaultSongTheme()
	sunday.Name = "Sunday"
	if err := st.SaveTheme(ctx, &sunday); err != nil {
		t.Fatal(err)
	}
	grace := sampleSong("Amazing Grace", "John Newton", verse(1, "Amazing grace how sweet the sound"))
	other := sampleSong("Be Thou My Vision", "Dallan Forgaill", verse(1, "Be Thou my vision"))
	if err := st.SaveSongs(ctx, []*models.Song{grace, other}); err != nil {
		t.Fatal(err)
	}
	themeOf := func(id int64) int64 {
		t.Helper()
		s, err := st.GetSong(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return s.ThemeID
	}

	cases := []struct {
		name    string
		song    int64
		theme   int64
		wantErr error
		want    int64
	}{
		{"set its own theme", grace.ID, sunday.ID, nil, sunday.ID},
		{"a missing theme is refused", grace.ID, 9999, ErrNotFound, sunday.ID},
		{"a missing song is refused", 9999, sunday.ID, ErrNotFound, sunday.ID},
		{"back to the default", grace.ID, 0, nil, 0},
		{"and again", grace.ID, sunday.ID, nil, sunday.ID},
	}
	for _, tc := range cases {
		err := st.SetSongTheme(ctx, tc.song, tc.theme)
		if (tc.wantErr == nil) != (err == nil) || (tc.wantErr != nil && !errors.Is(err, tc.wantErr)) {
			t.Fatalf("%s: SetSongTheme = %v, want %v", tc.name, err, tc.wantErr)
		}
		if got := themeOf(grace.ID); got != tc.want {
			t.Fatalf("%s: theme = %d, want %d", tc.name, got, tc.want)
		}
	}
	// The lyrics and the search index are untouched.
	if rows, err := st.SearchSongs(ctx, "sweet", 0); err != nil || len(rows) != 1 || rows[0].ID != grace.ID {
		t.Fatalf("search after SetSongTheme = %+v, %v", rows, err)
	}

	// The list rows carry the theme (the Songs list's Theme submenu), from the
	// plain listing and from a search alike.
	for _, q := range []string{"", "amazing", "sweet"} {
		rows, err := st.SearchSongs(ctx, q, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			want := int64(0)
			if r.ID == grace.ID {
				want = sunday.ID
			}
			if r.ThemeID != want {
				t.Errorf("SearchSongs(%q) row %q theme = %d, want %d", q, r.Title, r.ThemeID, want)
			}
		}
	}

	// Deleting the theme sends its songs back to the default: no dangling id.
	if err := st.DeleteTheme(ctx, sunday.ID); err != nil {
		t.Fatal(err)
	}
	if got := themeOf(grace.ID); got != 0 {
		t.Fatalf("theme after delete = %d, want 0", got)
	}
	if got := themeOf(other.ID); got != 0 {
		t.Fatalf("other song's theme = %d, want 0", got)
	}
}
