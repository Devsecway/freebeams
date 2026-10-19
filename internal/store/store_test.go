package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"freebeams/internal/models"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func sampleSong(title, author string, sections ...models.Section) *models.Song {
	return &models.Song{Title: title, Author: author, Copyright: "Public Domain", Sections: sections}
}

func verse(n int, lines ...string) models.Section {
	return models.Section{Kind: models.SectionVerse, Number: n, Label: fmt.Sprintf("Verse %d", n), Lines: lines}
}

func chorus(lines ...string) models.Section {
	return models.Section{Kind: models.SectionChorus, Label: "Chorus", Lines: lines}
}

func TestOpenMigratesAndReopens(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sub", "..", "library.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var version int
	if err := st.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != len(migrations) {
		t.Fatalf("user_version = %d, want %d", version, len(migrations))
	}
	var mode string
	if err := st.db.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("journal_mode = %q, %v; want wal", mode, err)
	}
	var fk int
	if err := st.db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys = %d, %v; want 1", fk, err)
	}
	song := sampleSong("Amazing Grace", "John Newton", verse(1, "Amazing grace"))
	if err := st.SaveSong(ctx, song); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopening must not re-run migrations or lose data.
	for i := 0; i < 2; i++ {
		st2, err := Open(path)
		if err != nil {
			t.Fatalf("reopen %d: %v", i, err)
		}
		n, err := st2.CountSongs(ctx)
		if err != nil || n != 1 {
			t.Fatalf("reopen %d: CountSongs = %d, %v; want 1", i, n, err)
		}
		st2.Close()
	}
}

func TestOpenRejectsNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, len(migrations)+1)); err != nil {
		t.Fatal(err)
	}
	st.Close()
	if _, err := Open(path); err == nil {
		t.Fatal("Open of a newer schema succeeded, want error")
	}
}

func TestDefaultPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	t.Setenv("AppData", filepath.Join(dir, "AppData"))
	p, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "library.db" || filepath.Base(filepath.Dir(p)) != "FreeBeams" {
		t.Fatalf("DefaultPath = %q", p)
	}
}

func TestSongCRUD(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)

	song := sampleSong("Amazing Grace", "John Newton",
		verse(1, "Amazing grace! how sweet the sound", "That saved a wretch like me!"),
		chorus("My chains are gone"),
		verse(2, "'Twas grace that taught my heart to fear"),
	)
	song.CCLI = "22025"
	song.Tags = "hymn"
	song.Administrator = "Admin"
	song.ThemeID = 7
	if err := st.SaveSong(ctx, song); err != nil {
		t.Fatalf("SaveSong: %v", err)
	}
	if song.ID == 0 || song.UUID == "" || song.Created.IsZero() || song.Updated.IsZero() {
		t.Fatalf("SaveSong did not assign fields: %+v", song)
	}

	got, err := st.GetSong(ctx, song.ID)
	if err != nil {
		t.Fatalf("GetSong: %v", err)
	}
	if !got.Created.Equal(song.Created) || !got.Updated.Equal(song.Updated) {
		t.Fatalf("times: got %v/%v want %v/%v", got.Created, got.Updated, song.Created, song.Updated)
	}
	got.Created, got.Updated = song.Created, song.Updated
	if !reflect.DeepEqual(got, *song) {
		t.Fatalf("GetSong mismatch:\n got %+v\nwant %+v", got, *song)
	}

	// Update path: change metadata and sections.
	created := song.Created
	uuid := song.UUID
	song.Title = "Amazing Grace (My Chains Are Gone)"
	song.Sections = []models.Section{verse(1, "Amazing grace"), {Kind: models.SectionEnding, Label: "Ending", Lines: []string{"Amen"}}}
	song.UUID = ""
	if err := st.SaveSong(ctx, song); err != nil {
		t.Fatalf("SaveSong update: %v", err)
	}
	if song.UUID != uuid || !song.Created.Equal(created) {
		t.Fatalf("update changed identity: uuid %q->%q created %v->%v", uuid, song.UUID, created, song.Created)
	}
	got, err = st.GetSong(ctx, song.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != song.Title || len(got.Sections) != 2 || got.Sections[1].Kind != models.SectionEnding {
		t.Fatalf("after update: %+v", got)
	}
	if n, _ := st.CountSongs(ctx); n != 1 {
		t.Fatalf("CountSongs = %d, want 1", n)
	}
	// The index follows the update: the old lyric is gone, the new one found.
	if res, _ := st.SearchSongs(ctx, "chains", 0); len(res) != 1 {
		t.Fatalf("search title word after update: %v", res)
	}
	if res, _ := st.SearchSongs(ctx, "wretch", 0); len(res) != 0 {
		t.Fatalf("stale lyric still indexed: %v", res)
	}
	if res, _ := st.SearchSongs(ctx, "amen", 0); len(res) != 1 {
		t.Fatalf("new lyric not indexed: %v", res)
	}

	// Errors.
	if _, err := st.GetSong(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSong missing: %v", err)
	}
	if err := st.SaveSong(ctx, &models.Song{ID: 9999, Title: "x"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SaveSong missing id: %v", err)
	}
	if err := st.SaveSong(ctx, &models.Song{Title: "  "}); err == nil {
		t.Fatal("SaveSong with empty title succeeded")
	}
	dup := &models.Song{Title: "Dup", UUID: uuid}
	if err := st.SaveSong(ctx, dup); !errors.Is(err, ErrExists) {
		t.Fatalf("SaveSong duplicate uuid: %v", err)
	}
	if dup.ID != 0 {
		t.Fatalf("failed SaveSong assigned ID %d", dup.ID)
	}

	// Delete cascades to sections and the index.
	if err := st.DeleteSong(ctx, song.ID); err != nil {
		t.Fatalf("DeleteSong: %v", err)
	}
	var n int
	if err := st.db.QueryRowContext(ctx, `SELECT count(*) FROM song_sections`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("sections after delete = %d, %v", n, err)
	}
	if err := st.db.QueryRowContext(ctx, `SELECT count(*) FROM songs_fts`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("fts rows after delete = %d, %v", n, err)
	}
	if err := st.DeleteSong(ctx, song.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteSong twice: %v", err)
	}
	if _, err := st.GetSong(ctx, song.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSong after delete: %v", err)
	}
}

func TestSaveSongsAtomic(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	a := sampleSong("A", "x")
	b := sampleSong("", "bad") // invalid: no title
	if err := st.SaveSongs(ctx, []*models.Song{a, b}); err == nil {
		t.Fatal("SaveSongs with an invalid song succeeded")
	}
	if a.ID != 0 {
		t.Fatalf("rolled back SaveSongs assigned ID %d", a.ID)
	}
	if n, _ := st.CountSongs(ctx); n != 0 {
		t.Fatalf("CountSongs after rollback = %d", n)
	}
	b.Title = "B"
	if err := st.SaveSongs(ctx, []*models.Song{a, b}); err != nil {
		t.Fatal(err)
	}
	if a.ID == 0 || b.ID == 0 || a.UUID == b.UUID {
		t.Fatalf("SaveSongs ids: %+v %+v", a, b)
	}
}

func TestSearchSongs(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	songs := []*models.Song{
		sampleSong("Amazing Grace", "John Newton", verse(1, "Amazing grace! how sweet the sound", "That saved a wretch like me!")),
		sampleSong("abide With Me", "Henry F. Lyte", verse(1, "Abide with me; fast falls the eventide")),
		sampleSong("How Great Thou Art", "Carl Boberg", verse(1, "O Lord my God, when I in awesome wonder"), chorus("Then sings my soul")),
		sampleSong("A Mighty Fortress Is Our God", "Martin Luther", verse(1, "His craft and pow'r are great")),
		sampleSong("Santo, Santo, Santo", "Reginald Heber", verse(1, "¡Santo! ¡Santo! ¡Santo! Señor omnipotente")),
		// Ranking fixtures: "amazing" and "grace" in lyrics only, "great" in
		// two titles and one lyric, a contraction and an author (Lloyd) that
		// the old `"i"* "ll"*` query matched for I'll.
		sampleSong("Alas And Did My Savior Bleed", "Isaac Watts", verse(1, "Amazing pity! grace unknown!")),
		sampleSong("Grace Alone", "Scott Wesley Brown", verse(1, "Every promise we can make")),
		sampleSong("Great Is Thy Faithfulness", "Thomas Chisholm", verse(1, "Morning by morning new mercies I see")),
		sampleSong("I'll Fly Away", "Albert Brumley", verse(1, "Some glad morning when this life is o'er", "I'll fly away")),
		sampleSong("Great Day", "Lloyd Larson", verse(1, "I will rise")),
	}
	for _, s := range songs {
		if err := st.SaveSong(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	titles := func(res []models.SongSummary) []string {
		out := []string{}
		for _, r := range res {
			out = append(out, r.Title)
		}
		return out
	}
	all := []string{"A Mighty Fortress Is Our God", "abide With Me", "Alas And Did My Savior Bleed", "Amazing Grace", "Grace Alone",
		"Great Day", "Great Is Thy Faithfulness", "How Great Thou Art", "I'll Fly Away", "Santo, Santo, Santo"}
	tests := []struct {
		name  string
		query string
		limit int
		want  []string
	}{
		{"empty lists all by title nocase", "", 0, all},
		{"whitespace lists all", "   ", 2, all[:2]},
		{"limit", "", 1, all[:1]},
		{"title prefix ranks before lyrics", "amaz", 0, []string{"Amazing Grace", "Alas And Did My Savior Bleed"}},
		{"case insensitive, titles first", "GRACE", 0, []string{"Amazing Grace", "Grace Alone", "Alas And Did My Savior Bleed"}},
		{"title tier then lyrics tier, each by title", "great", 0, []string{"Great Day", "Great Is Thy Faithfulness", "How Great Thou Art", "A Mighty Fortress Is Our God"}},
		{"limit applies after ranking", "great", 2, []string{"Great Day", "Great Is Thy Faithfulness"}},
		{"title and lyrics mixed", "grace sweet", 0, []string{"Amazing Grace"}},
		{"quoted phrase", `"sweet the sound"`, 0, []string{"Amazing Grace"}},
		{"quoted phrase keeps word order", `"sound sweet"`, 0, nil},
		{"quoted phrase plus a word", `"the sound" newton`, 0, []string{"Amazing Grace"}},
		{"contraction is one phrase", "I'll", 0, []string{"I'll Fly Away"}},
		{"contraction with a curly apostrophe", "I’ll fly", 0, []string{"I'll Fly Away"}},
		{"multi word all must match", "how great", 0, []string{"How Great Thou Art"}},
		{"multi word across fields", "newton wretch", 0, []string{"Amazing Grace"}},
		{"multi word no match", "great wretch", 0, nil},
		{"author match", "luther", 0, []string{"A Mighty Fortress Is Our God"}},
		{"lyric match", "eventide", 0, []string{"abide With Me"}},
		{"chorus lyric", "sings soul", 0, []string{"How Great Thou Art"}},
		{"apostrophe", "pow'r", 0, []string{"A Mighty Fortress Is Our God"}},
		{"diacritics folded", "senor", 0, []string{"Santo, Santo, Santo"}},
		{"accented query", "señor", 0, []string{"Santo, Santo, Santo"}},
		{"punctuation", `grace!!! "how`, 0, []string{"Amazing Grace"}},
		{"fts operators are words", "AND OR NOT", 0, nil},
		{"near syntax is literal words", "NEAR(grace sweet)", 0, nil},
		{"column filter syntax", "title:grace", 0, nil},
		{"star and caret", "^gra* -x +", 0, nil},
		{"only punctuation lists all", `"*()-:^`, 1, []string{"A Mighty Fortress Is Our God"}},
		{"unbalanced quote", `"amazing`, 0, []string{"Amazing Grace", "Alas And Did My Savior Bleed"}},
		{"quote after a word", `amazing"`, 0, []string{"Amazing Grace", "Alas And Did My Savior Bleed"}},
		{"empty quotes are ignored", `"" amazing ""`, 0, []string{"Amazing Grace", "Alas And Did My Savior Bleed"}},
		{"no match", "zzzz", 0, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := st.SearchSongs(ctx, tc.query, tc.limit)
			if err != nil {
				t.Fatalf("SearchSongs(%q): %v", tc.query, err)
			}
			got := titles(res)
			want := tc.want
			if want == nil {
				want = []string{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("SearchSongs(%q) = %q, want %q", tc.query, got, want)
			}
		})
	}
	// Summary fields are populated.
	res, _ := st.SearchSongs(ctx, "amazing", 0)
	if len(res) != 2 || res[0].Author != "John Newton" || res[0].Copyright != "Public Domain" || res[0].ID != songs[0].ID {
		t.Fatalf("summary = %+v", res)
	}
}

func TestSearchSongsMatched(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	songs := []*models.Song{
		sampleSong("Morning Song", "Ann Lee", verse(1, "Sunrise over the hills")),
		sampleSong("Evening Hymn", "Morning Star Singers", verse(1, "Night falls softly")),
		sampleSong("Day Is Done", "Bob Ray", verse(1, "Gone the sun", "The morning will come again", "Rest in peace")),
	}
	for _, s := range songs {
		if err := st.SaveSong(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	type row struct{ title, matched string }
	rows := func(res []models.SongSummary) []row {
		out := []row{}
		for _, r := range res {
			out = append(out, row{r.Title, r.Matched})
		}
		return out
	}
	tests := []struct {
		name  string
		query string
		want  []row
	}{
		{"three tiers", "morning", []row{{"Morning Song", "title"}, {"Evening Hymn", "author"}, {"Day Is Done", "lyrics"}}},
		{"title plus lyrics needs the lyrics", "morning sunrise", []row{{"Morning Song", "lyrics"}}},
		{"author plus lyrics", "singers night", []row{{"Evening Hymn", "lyrics"}}},
		{"title and author together", "morning lee", []row{{"Morning Song", "author"}}},
		{"listing is not a match", "", []row{{"Day Is Done", ""}, {"Evening Hymn", ""}, {"Morning Song", ""}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := st.SearchSongs(ctx, tc.query, 0)
			if err != nil {
				t.Fatal(err)
			}
			if got := rows(res); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("SearchSongs(%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
	// The lyrics excerpt is a single line around the match; title and author
	// matches carry none.
	res, _ := st.SearchSongs(ctx, "morning", 0)
	if !strings.Contains(res[2].Snippet, "The morning will come") || strings.Contains(res[2].Snippet, "\n") {
		t.Errorf("lyrics snippet = %q", res[2].Snippet)
	}
	if res[0].Snippet != "" || res[1].Snippet != "" {
		t.Errorf("title/author snippets = %q, %q", res[0].Snippet, res[1].Snippet)
	}
	res, _ = st.SearchSongs(ctx, "come again", 0)
	if len(res) != 1 || res[0].Snippet != "Gone the sun / The morning will come again / Rest in…" {
		t.Errorf("snippet lines = %+v", res)
	}
}

func TestFTSQueryNeverErrors(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if err := st.SaveSong(ctx, sampleSong("Grace", "x", verse(1, "grace"))); err != nil {
		t.Fatal(err)
	}
	inputs := []string{`"`, `""`, `*`, `(`, `)`, `-`, `:`, `^`, `NEAR(`, `a"b`, `'`, `\`, "\x00", "\xff\xfe", `{x}`, `[x]`, `a OR`, `NOT a`, `a AND`, `💒 ✝`, `日本語`, `1:2`, `x*y`, `col:`, `+`,
		`"grace`, `grace"`, `"" ""`, `*"*`, `a"b"c"d`, `"grace" -sweet`, `NEAR("a" "b", 3)`, `{title} : grace`, `grace*`, `"a\x00b"`, "́", `"\xff"`}
	for _, in := range inputs {
		if _, err := st.SearchSongs(ctx, in, 0); err != nil {
			t.Errorf("SearchSongs(%q): %v", in, err)
		}
		if _, err := st.SearchVerses(ctx, 1, in, 0); err != nil {
			t.Errorf("SearchVerses(%q): %v", in, err)
		}
	}
}

func TestFTSQuery(t *testing.T) {
	// Whitespace separates terms, a quoted run is one term, and the words of
	// a term form one prefix phrase (FTS5 folds case and diacritics itself).
	tests := map[string]string{
		"":                  "",
		"  ":                "",
		"Grace":             `"Grace"*`,
		`how  "great`:       `"how"* "great"*`,
		"pow'r":             `"pow r"*`,
		"señor":             `"señor"*`,
		"--- ** ()":         "",
		"title:grace":       `"title grace"*`,
		"John 3:16":         `"John"* "3 16"*`,
		"NEAR(a b, 10)":     `"NEAR a"* "b"* "10"*`,
		`"amazing grace"`:   `"amazing grace"*`,
		`"sweet the" sound`: `"sweet the"* "sound"*`,
		`grace"how`:         `"grace"* "how"*`,
		`"" x ""`:           `"x"*`,
		`a-b_c.d`:           `"a b c d"*`,
	}
	for in, want := range tests {
		if got := ftsQuery(in); got != want {
			t.Errorf("ftsQuery(%q) = %s, want %s", in, got, want)
		}
	}
	terms := searchTerms(`"amazing grace" how`)
	if got := ftsColumns([]string{"title"}, terms, true); got != `{title} : ("amazing grace"* "how"*)` {
		t.Errorf("ftsColumns all = %s", got)
	}
	if got := ftsColumns([]string{"title", "author"}, terms, false); got != `{title author} : ("amazing grace"* OR "how"*)` {
		t.Errorf("ftsColumns any = %s", got)
	}
	// Pathological input is capped.
	long := strings.Repeat("w ", 100)
	if got := len(searchTerms(long)); got != maxTerms {
		t.Errorf("terms for 100 words = %d, want %d", got, maxTerms)
	}
	if got := len(searchTerms(`"` + long + `"`)[0]); got != maxTerms {
		t.Errorf("words of a 100-word phrase = %d, want %d", got, maxTerms)
	}
}

func TestFold(t *testing.T) {
	tests := map[string]string{
		"Café":             "cafe",
		"SEÑOR":            "senor",
		"naïve Lumière":    "naive lumiere",
		"Straße Øre Łódź":  "strasse ore lodz",
		"100%_Done":        "100%_done",
		"plain":            "plain",
		"":                 "",
		"café":            "cafe", // decomposed input
		"Ærø Œuvre Đakovo": "aero oeuvre dakovo",
	}
	for in, want := range tests {
		if got := fold(in); got != want {
			t.Errorf("fold(%q) = %q, want %q", in, got, want)
		}
	}
	if !matchesAll(textTerms("CAFÉ lum"), "x", "Café Lumière") || matchesAll(textTerms("cafe zebra"), "Café Lumière") {
		t.Error("matchesAll")
	}
	if !matchesAll(nil, "anything") || !matchesAll(textTerms("  "), "anything") {
		t.Error("no terms match everything")
	}
}

func vr(chapter, start, end int) models.VerseRange {
	return models.VerseRange{Chapter: chapter, VerseStart: start, VerseEnd: end}
}

func kjvSample() []models.Verse {
	return []models.Verse{
		{Book: 43, Chapter: 3, Verse: 16, Text: "For God so loved the world, that he gave his only begotten Son"},
		{Book: 43, Chapter: 3, Verse: 17, Text: "For God sent not his Son into the world to condemn the world"},
		{Book: 43, Chapter: 3, Verse: 18, Text: "He that believeth on him is not condemned"},
		{Book: 43, Chapter: 3, Verse: 36, Text: "He that believeth on the Son hath everlasting life"},
		{Book: 43, Chapter: 4, Verse: 1, Text: "When therefore the Lord knew how the Pharisees had heard"},
		{Book: 43, Chapter: 4, Verse: 2, Text: "(Though Jesus himself baptized not, but his disciples,)"},
		{Book: 43, Chapter: 4, Verse: 3, Text: "He left Judaea, and departed again into Galilee."},
		{Book: 19, Chapter: 23, Verse: 1, Text: "The LORD is my shepherd; I shall not want."},
		{Book: 19, Chapter: 23, Verse: 2, Text: "He maketh me to lie down in green pastures"},
		{Book: 1, Chapter: 1, Verse: 1, Text: "In the beginning God created the heaven and the earth."},
	}
}

func refs(vs []models.Verse) []string {
	out := []string{}
	for _, v := range vs {
		out = append(out, fmt.Sprintf("%d %d:%d", v.Book, v.Chapter, v.Verse))
	}
	return out
}

func TestBiblesAndVerses(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)

	kjv, err := st.CreateBible(ctx, models.Bible{Abbrev: "KJV", Name: "King James Version", Language: "en"})
	if err != nil || kjv.ID == 0 {
		t.Fatalf("CreateBible: %+v %v", kjv, err)
	}
	if _, err := st.CreateBible(ctx, models.Bible{Abbrev: "kjv"}); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate abbrev: %v", err)
	}
	if _, err := st.CreateBible(ctx, models.Bible{Abbrev: " "}); err == nil {
		t.Fatal("empty abbrev accepted")
	}
	asv, err := st.CreateBibleWithVerses(ctx, models.Bible{Abbrev: "ASV", Name: "American Standard Version"},
		[]models.Verse{{Book: 43, Chapter: 3, Verse: 16, Text: "For God so loved the world, that he gave his only begotten Son"}})
	if err != nil {
		t.Fatal(err)
	}
	list, err := st.ListBibles(ctx)
	if err != nil || len(list) != 2 || list[0].Abbrev != "ASV" || list[1] != kjv {
		t.Fatalf("ListBibles = %+v, %v", list, err)
	}

	if err := st.InsertVerses(ctx, kjv.ID, kjvSample()); err != nil {
		t.Fatalf("InsertVerses: %v", err)
	}
	if err := st.InsertVerses(ctx, 999, kjvSample()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("InsertVerses unknown bible: %v", err)
	}
	if err := st.InsertVerses(ctx, kjv.ID, []models.Verse{{Book: 0, Chapter: 1, Verse: 1}}); err == nil {
		t.Fatal("invalid verse ref accepted")
	}

	tests := []struct {
		name string
		p    models.Passage
		want []string
	}{
		{"single", models.Passage{Book: 43, Ranges: []models.VerseRange{vr(3, 16, 16)}}, []string{"43 3:16"}},
		{"range", models.Passage{Book: 43, Ranges: []models.VerseRange{vr(3, 16, 18)}}, []string{"43 3:16", "43 3:17", "43 3:18"}},
		{"whole chapter", models.Passage{Book: 19, Ranges: []models.VerseRange{vr(23, 0, 0)}}, []string{"19 23:1", "19 23:2"}},
		{"to end of chapter", models.Passage{Book: 43, Ranges: []models.VerseRange{vr(3, 17, 0)}}, []string{"43 3:17", "43 3:18", "43 3:36"}},
		{"cross chapter", models.Passage{Book: 43, Ranges: []models.VerseRange{vr(3, 36, 0), vr(4, 1, 2)}}, []string{"43 3:36", "43 4:1", "43 4:2"}},
		{"missing skipped", models.Passage{Book: 43, Ranges: []models.VerseRange{vr(3, 15, 19)}}, []string{"43 3:16", "43 3:17", "43 3:18"}},
		{"canonical order, no dups", models.Passage{Book: 43, Ranges: []models.VerseRange{vr(4, 3, 3), vr(3, 18, 18), vr(3, 16, 18)}}, []string{"43 3:16", "43 3:17", "43 3:18", "43 4:3"}},
		{"reversed range", models.Passage{Book: 43, Ranges: []models.VerseRange{vr(3, 18, 16)}}, []string{"43 3:16", "43 3:17", "43 3:18"}},
		{"missing chapter", models.Passage{Book: 43, Ranges: []models.VerseRange{vr(30, 0, 0)}}, nil},
		{"missing book", models.Passage{Book: 2, Ranges: []models.VerseRange{vr(1, 1, 3)}}, nil},
		{"no ranges", models.Passage{Book: 43}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := st.Verses(ctx, kjv.ID, tc.p)
			if err != nil {
				t.Fatal(err)
			}
			want := tc.want
			if want == nil {
				want = []string{}
			}
			if !reflect.DeepEqual(refs(got), want) {
				t.Fatalf("Verses = %v, want %v", refs(got), want)
			}
		})
	}
	vs, _ := st.Verses(ctx, kjv.ID, models.Passage{Book: 1, Ranges: []models.VerseRange{vr(1, 1, 1)}})
	if len(vs) != 1 || vs[0].Text != "In the beginning God created the heaven and the earth." {
		t.Fatalf("Gen 1:1 = %+v", vs)
	}
	// Bibles are isolated from each other.
	vs, _ = st.Verses(ctx, asv.ID, models.Passage{Book: 43, Ranges: []models.VerseRange{vr(3, 0, 0)}})
	if !reflect.DeepEqual(refs(vs), []string{"43 3:16"}) {
		t.Fatalf("ASV John 3 = %v", refs(vs))
	}

	if n, err := st.ChapterCount(ctx, kjv.ID, 43); err != nil || n != 4 {
		t.Fatalf("ChapterCount = %d, %v", n, err)
	}
	if n, err := st.ChapterCount(ctx, kjv.ID, 2); err != nil || n != 0 {
		t.Fatalf("ChapterCount missing = %d, %v", n, err)
	}
	if n, err := st.VerseCount(ctx, kjv.ID, 43, 3); err != nil || n != 36 {
		t.Fatalf("VerseCount = %d, %v", n, err)
	}

	search := []struct {
		q     string
		limit int
		want  []string
	}{
		{"loved world", 0, []string{"43 3:16"}},
		{"believ", 0, []string{"43 3:18", "43 3:36"}},
		{"shepherd", 0, []string{"19 23:1"}},
		{"the", 2, []string{"1 1:1", "19 23:1"}},
		{"(Though", 0, []string{"43 4:2"}},
		{"", 0, nil},
		{"zzz", 0, nil},
	}
	for _, tc := range search {
		got, err := st.SearchVerses(ctx, kjv.ID, tc.q, tc.limit)
		if err != nil {
			t.Fatalf("SearchVerses(%q): %v", tc.q, err)
		}
		want := tc.want
		if want == nil {
			want = []string{}
		}
		if !reflect.DeepEqual(refs(got), want) {
			t.Errorf("SearchVerses(%q) = %v, want %v", tc.q, refs(got), want)
		}
	}
	if got, _ := st.SearchVerses(ctx, kjv.ID, "loved", 0); len(got) != 1 || got[0].Text == "" {
		t.Fatalf("SearchVerses text: %+v", got)
	}
	if got, _ := st.SearchVerses(ctx, asv.ID, "loved", 0); len(got) != 1 {
		t.Fatalf("ASV SearchVerses: %+v", got)
	}

	// Re-inserting replaces text and keeps the index consistent.
	if err := st.InsertVerses(ctx, kjv.ID, []models.Verse{
		{Book: 19, Chapter: 23, Verse: 1, Text: "The LORD is my keeper"},
		{Book: 19, Chapter: 23, Verse: 1, Text: "The LORD is my guardian"},
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.SearchVerses(ctx, kjv.ID, "shepherd", 0); len(got) != 0 {
		t.Fatalf("stale index after replace: %v", refs(got))
	}
	if got, _ := st.SearchVerses(ctx, kjv.ID, "keeper", 0); len(got) != 0 {
		t.Fatalf("stale duplicate in index: %v", refs(got))
	}
	if got, _ := st.SearchVerses(ctx, kjv.ID, "guardian", 0); len(got) != 1 || got[0].Text != "The LORD is my guardian" {
		t.Fatalf("replaced verse: %+v", got)
	}
}

func TestSettings(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	type prefs struct {
		Theme   string
		Volume  float64
		Screens []int
	}
	var p prefs
	ok, err := st.GetSetting(ctx, "prefs", &p)
	if err != nil || ok {
		t.Fatalf("unset GetSetting = %v, %v", ok, err)
	}
	want := prefs{Theme: "dark", Volume: 0.5, Screens: []int{1, 2}}
	if err := st.SetSetting(ctx, "prefs", want); err != nil {
		t.Fatal(err)
	}
	ok, err = st.GetSetting(ctx, "prefs", &p)
	if err != nil || !ok || !reflect.DeepEqual(p, want) {
		t.Fatalf("GetSetting = %+v, %v, %v", p, ok, err)
	}
	if err := st.SetSetting(ctx, "prefs", prefs{Theme: "light"}); err != nil {
		t.Fatal(err)
	}
	p = prefs{}
	if _, err := st.GetSetting(ctx, "prefs", &p); err != nil || p.Theme != "light" {
		t.Fatalf("overwrite: %+v %v", p, err)
	}
	if err := st.SetSetting(ctx, "n", 42); err != nil {
		t.Fatal(err)
	}
	var n int
	if ok, err := st.GetSetting(ctx, "n", &n); !ok || err != nil || n != 42 {
		t.Fatalf("int setting = %d %v %v", n, ok, err)
	}
	var s string
	if _, err := st.GetSetting(ctx, "n", &s); err == nil {
		t.Fatal("type mismatch not reported")
	}
	if err := st.SetSetting(ctx, "bad", func() {}); err == nil {
		t.Fatal("unmarshalable value accepted")
	}
}

func TestConcurrentWrites(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	const workers, each = 8, 10
	var wg sync.WaitGroup
	errs := make(chan error, workers*each*2)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				s := sampleSong(fmt.Sprintf("Song %d-%d", w, i), "Author", verse(1, "line"))
				if err := st.SaveSong(ctx, s); err != nil {
					errs <- err
				}
				if _, err := st.SearchSongs(ctx, "song", 5); err != nil {
					errs <- err
				}
				if err := st.SetSetting(ctx, fmt.Sprintf("k%d", w), i); err != nil {
					errs <- err
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if n, _ := st.CountSongs(ctx); n != workers*each {
		t.Fatalf("CountSongs = %d, want %d", n, workers*each)
	}
}

// TestCloseIsBoundedWhileAnotherWriterHoldsTheLock: Close's PRAGMA optimize
// must not wait out the 10s busy timeout behind another connection's write
// transaction (e.g. an import that is still unwinding at shutdown).
func TestCloseIsBoundedWhileAnotherWriterHoldsTheLock(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "library.db")
	a, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ { // give optimize something to analyze
		if err := a.SaveSong(ctx, sampleSong(fmt.Sprintf("Song %d", i), "Author", verse(1, "line"))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.SearchSongs(ctx, "song", 0); err != nil {
		t.Fatal(err)
	}
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	tx, err := b.db.BeginTx(ctx, nil) // BEGIN IMMEDIATE: holds the write lock
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	start := time.Now()
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("Close took %v while another writer held the lock, want < 3s", d)
	}
}
