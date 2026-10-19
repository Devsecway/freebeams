package easyworship

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"freeworship/internal/models"
)

// TestSearchSongsLocalEW runs the song search over the real EasyWorship
// library (223 songs): the song an operator means comes first whatever the
// case, lyrics matches are marked as such, and typed punctuation is harmless.
// Before the ranking, "AMAZING" listed "Alas And Did My Savior Bleed" (a
// lyric match) above "Amazing Grace", and "O Holy Night" came after "Day Is
// Dying In The West".
func TestSearchSongsLocalEW(t *testing.T) {
	requireLocalEW(t)
	ctx := context.Background()
	st := openStore(t)
	rep, err := ImportInto(ctx, st, Options{SongsDir: localSongsDir(), BiblesDir: t.TempDir(), SkipMedia: true})
	if err != nil {
		t.Fatal(err)
	}
	if rep.SongsImported == 0 {
		t.Fatalf("no songs imported: %+v", rep)
	}
	tests := []struct {
		query   string
		first   string // the title ranked first
		matched string // and where it matched
		min     int    // at least this many results
	}{
		{"AMAZING", "Amazing Grace", models.MatchedTitle, 2},
		{"amaz", "Amazing Grace", models.MatchedTitle, 2},
		{"Amazing Grac", "Amazing Grace", models.MatchedTitle, 1},
		{"amazing grace", "Amazing Grace", models.MatchedTitle, 1},
		{`"amazing grace"`, "Amazing Grace", models.MatchedTitle, 1},
		{"wretch", "Amazing Grace", models.MatchedLyrics, 1},
		{"wretch like me", "Amazing Grace", models.MatchedLyrics, 1},
		{"grace sweet", "Amazing Grace", models.MatchedLyrics, 2},
		{"how sweet the", "How Sweet The Name Of Jesus Sounds", models.MatchedTitle, 2},
		{"O Holy Night", "O Holy Night", models.MatchedTitle, 2},
		{"o holy night", "O Holy Night", models.MatchedTitle, 2},
		{"it is well", "It Is Well With My Soul", models.MatchedTitle, 2},
		{"night divine", "O Holy Night", models.MatchedLyrics, 2},
		{"newton", "Amazing Grace", models.MatchedAuthor, 1},
		{"John Newton", "Amazing Grace", models.MatchedAuthor, 1},
	}
	for _, tc := range tests {
		res, err := st.SearchSongs(ctx, tc.query, 0)
		if err != nil {
			t.Errorf("SearchSongs(%q): %v", tc.query, err)
			continue
		}
		if len(res) < tc.min {
			t.Errorf("SearchSongs(%q) = %d results, want at least %d", tc.query, len(res), tc.min)
			continue
		}
		if res[0].Title != tc.first || res[0].Matched != tc.matched {
			t.Errorf("SearchSongs(%q) first = %q (%s), want %q (%s)", tc.query, res[0].Title, res[0].Matched, tc.first, tc.matched)
		}
		if tc.matched == models.MatchedLyrics && res[0].Snippet == "" {
			t.Errorf("SearchSongs(%q): no lyrics snippet", tc.query)
		}
	}

	// A contraction is one phrase: I'll matches the adjacent tokens i, ll…
	// (I'll; and, the last token being a prefix, "Lord's" also "Lord
	// Sabaoth"), not an i… and an ll… anywhere in the song, which is what the
	// two separate words match.
	for _, c := range []struct {
		tight, loose string
		adjacent     *regexp.Regexp
	}{
		{"I'll", "i ll", regexp.MustCompile(`(?i)\bi\W+ll`)},
		{"Lord's", "lord s", regexp.MustCompile(`(?i)\blord\W+s`)},
	} {
		tight, err := st.SearchSongs(ctx, c.tight, 0)
		if err != nil {
			t.Fatal(err)
		}
		loose, err := st.SearchSongs(ctx, c.loose, 0)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%q: %d results (%q: %d)", c.tight, len(tight), c.loose, len(loose))
		if len(tight) == 0 || len(tight) >= len(loose) {
			t.Errorf("SearchSongs(%q) = %d results, %q = %d: want some, and fewer", c.tight, len(tight), c.loose, len(loose))
		}
		for _, r := range tight {
			if r.Matched != models.MatchedLyrics {
				continue
			}
			song, err := st.GetSong(ctx, r.ID)
			if err != nil {
				t.Fatal(err)
			}
			var lyrics strings.Builder
			for _, sec := range song.Sections {
				lyrics.WriteString(strings.Join(sec.Lines, "\n") + "\n")
			}
			if !c.adjacent.MatchString(lyrics.String()) {
				t.Errorf("SearchSongs(%q): %q matched in lyrics without the adjacent words (snippet %q)", c.tight, r.Title, r.Snippet)
			}
		}
	}

	// Typed punctuation and FTS5 syntax never fail.
	for _, q := range []string{`"amazing`, `Lord's "grace`, `(grace)*`, `grace -sweet`, `NEAR(grace)`, `grace OR`, `"" * ^`} {
		if _, err := st.SearchSongs(ctx, q, 0); err != nil {
			t.Errorf("SearchSongs(%q): %v", q, err)
		}
	}
}
