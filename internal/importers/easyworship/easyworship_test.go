package easyworship

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"freeworship/internal/models"
	"freeworship/internal/store"
)

const localEW = "/Users/Shared/Softouch"

func requireLocalEW(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(localEW); err != nil {
		t.Skipf("EasyWorship data not installed at %s", localEW)
	}
}

func localSongsDir() string  { return filepath.Join(localEW, "Easyworship/Default/v6.1/Databases/Data") }
func localBiblesDir() string { return filepath.Join(localEW, "Easyworship.v8/Plugins/Bibles") }

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// ── Synthetic EasyWorship install (runs everywhere) ──────────────────────

const ewRTF = `{\rtf1\ansi\deff0{\fonttbl{\f0\fnil\fcharset0 Arial;}{\f1\fnil\fcharset0 Verdana;}}{\colortbl\red0\green0\blue0;}{\*\pnseclvl1\pnucrm\pnstart1\pnhang\pnindent720{\pntxtb}{\pntxta{.}}}
{\pard\li0\fi0\ri0\sb0\sl\sa0 \plain\f1\fntnamaut Verse 1\par
\li0\fi0\ri0\sb0\sl\sa0 \plain\f1\fntnamaut Caf\'e9 line one\par
\li0\fi0\ri0\sb0\sl\sa0 \plain\f1\fntnamaut Line two \par
\li0\fi0\ri0\sb0\sl\sa0 \par
\li0\fi0\ri0\sb0\sl\sa0 \plain\f1\fntnamaut Chorus 1\par
\li0\fi0\ri0\sb0\sl\sa0 \plain\f1\fntnamaut Sing it\par
\li0\fi0\ri0\sb0\sl\sa0 \plain\f1\fntnamaut \par
\li0\fi0\ri0\sb0\sl\sa0 \plain\f1\fntnamaut Sing it again}
}`

func writeSongsDBs(t *testing.T, dir string) {
	t.Helper()
	ensureCollations()
	songs, err := sql.Open("sqlite", filepath.Join(dir, songsDB))
	if err != nil {
		t.Fatal(err)
	}
	defer songs.Close()
	mustExec(t, songs, `CREATE TABLE song ("rowid" integer PRIMARY KEY AUTOINCREMENT NOT NULL UNIQUE, song_item_uid text UNIQUE, song_rev_uid text, song_uid text,
		title text NOT NULL COLLATE UTF8_U_CI, author text COLLATE UTF8_U_CI, copyright text COLLATE UTF8_U_CI, administrator text COLLATE UTF8_U_CI,
		description text, tags text COLLATE UTF8_U_CI, reference_number text COLLATE UTF8_U_CI, provider_id integer DEFAULT -1, vendor_id integer,
		presentation_id integer, layout_revision integer DEFAULT 1, revision integer DEFAULT 1)`)
	mustExec(t, songs, `CREATE INDEX song_Index01 ON song (title)`)
	mustExec(t, songs, `INSERT INTO song (rowid, title, author, copyright, administrator, reference_number, tags) VALUES
		(1, 'Test Hymn', 'Some  Author', 'Public Domain', '', '12345', 'hymn'),
		(2, 'No Words', NULL, NULL, NULL, NULL, NULL),
		(3, 'test hymn', 'some author', 'dup', '', '', '')`)
	words, err := sql.Open("sqlite", filepath.Join(dir, songWordsDB))
	if err != nil {
		t.Fatal(err)
	}
	defer words.Close()
	mustExec(t, words, `CREATE TABLE word ("rowid" integer PRIMARY KEY NOT NULL UNIQUE, song_id integer, words rtf, slide_uids text, slide_layout_revisions int64a, slide_revisions int64a)`)
	if _, err := words.Exec(`INSERT INTO word (song_id, words) VALUES (1, ?), (3, 'plain text lyrics')`, ewRTF); err != nil {
		t.Fatal(err)
	}
}

func mustExec(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	if _, err := db.Exec(q); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func TestReadSongsSynthetic(t *testing.T) {
	dir := t.TempDir()
	writeSongsDBs(t, dir)
	songs, err := ReadSongs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(songs) != 3 {
		t.Fatalf("got %d songs", len(songs))
	}
	s := songs[0]
	if s.Title != "Test Hymn" || s.Author != "Some Author" || s.CCLI != "12345" || s.Tags != "hymn" || s.Copyright != "Public Domain" {
		t.Fatalf("metadata = %+v", s)
	}
	want := []models.Section{
		{Kind: models.SectionVerse, Number: 1, Label: "Verse 1", Lines: []string{"Café line one", "Line two"}},
		{Kind: models.SectionChorus, Number: 1, Label: "Chorus 1", Lines: []string{"Sing it"}},
		{Kind: models.SectionChorus, Number: 1, Label: "Chorus 1", Lines: []string{"Sing it again"}},
	}
	if !slices.EqualFunc(s.Sections, want, func(a, b models.Section) bool {
		return a.Kind == b.Kind && a.Number == b.Number && a.Label == b.Label && slices.Equal(a.Lines, b.Lines)
	}) {
		t.Fatalf("sections = %+v", s.Sections)
	}
	if len(songs[1].Sections) != 0 || songs[1].Author != "" {
		t.Fatalf("song without words = %+v", songs[1])
	}
	if len(songs[2].Sections) != 1 || songs[2].Sections[0].Lines[0] != "plain text lyrics" {
		t.Fatalf("plain-text lyrics = %+v", songs[2].Sections)
	}
	if _, err := ReadSongs(t.TempDir()); err == nil {
		t.Fatal("ReadSongs on empty dir: no error")
	}
}

func TestImportIntoSynthetic(t *testing.T) {
	ctx := context.Background()
	songsDir, biblesDir := t.TempDir(), t.TempDir()
	writeSongsDBs(t, songsDir)
	writeEWB(t, filepath.Join(biblesDir, "KJV.ewb"), "King James Version", "KJV", "EN", "", 17,
		[][]fakeVerse{{{1, 1, 1, "In the beginning God created the heaven and the earth. "}}})
	writeEWB(t, filepath.Join(biblesDir, "HCSB.ewb"), "Holman Christian Standard Bible", "HCSB", "EN", "", 14,
		[][]fakeVerse{{{1, 1, 1, "In the beginning God created the heavens and the earth. "}}})
	os.WriteFile(filepath.Join(biblesDir, "Broken.ewb"), []byte("garbage"), 0o644)

	st := openStore(t)
	opt := Options{SongsDir: songsDir, BiblesDir: biblesDir}
	rep, err := ImportInto(ctx, st, opt)
	if err != nil {
		t.Fatal(err)
	}
	if rep.SongsImported != 2 || rep.SongsSkipped != 1 {
		t.Fatalf("songs report = %+v", rep)
	}
	if !slices.Equal(rep.BiblesImported, []string{"KJV"}) || !slices.Equal(rep.BiblesSkipped, []string{"HCSB (copyrighted)"}) {
		t.Fatalf("bibles report = %+v", rep)
	}
	if len(rep.Warnings) != 2 { // Broken.ewb + "No Words" has no lyrics
		t.Fatalf("warnings = %q", rep.Warnings)
	}
	bibles, _ := st.ListBibles(ctx)
	if len(bibles) != 1 || bibles[0].Copyright != "Public Domain" || bibles[0].Language != "en" {
		t.Fatalf("bibles = %+v", bibles)
	}

	// Second run: nothing new.
	rep, err = ImportInto(ctx, st, opt)
	if err != nil {
		t.Fatal(err)
	}
	if rep.SongsImported != 0 || rep.SongsSkipped != 3 || len(rep.BiblesImported) != 0 {
		t.Fatalf("second run report = %+v", rep)
	}
	if n, _ := st.CountSongs(ctx); n != 2 {
		t.Fatalf("CountSongs = %d", n)
	}

	// Opting in imports the copyrighted Bible.
	opt.IncludeCopyrightedBibles = true
	rep, err = ImportInto(ctx, st, opt)
	if err != nil || !slices.Equal(rep.BiblesImported, []string{"HCSB"}) {
		t.Fatalf("opt-in report = %+v, %v", rep, err)
	}

	// Missing directories are not errors.
	rep, err = ImportInto(ctx, st, Options{SongsDir: filepath.Join(songsDir, "x"), BiblesDir: filepath.Join(biblesDir, "x")})
	if err != nil || rep.SongsImported != 0 || len(rep.Warnings) != 1 {
		t.Fatalf("missing dirs: %+v, %v", rep, err)
	}
	if _, err := ImportInto(ctx, nil, opt); err == nil {
		t.Fatal("nil store: no error")
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := ImportInto(cctx, st, opt); err == nil {
		t.Fatal("cancelled context: no error")
	}
}

func TestReadOnlyNeverModifies(t *testing.T) {
	dir := t.TempDir()
	writeSongsDBs(t, dir)
	path := filepath.Join(dir, songsDB)
	before, _ := os.Stat(path)
	if _, err := ReadSongs(dir); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		t.Fatal("Songs.db modified by ReadSongs")
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Stat(path + suffix); err == nil {
			t.Fatalf("%s created", suffix)
		}
	}
}

// ── Real EasyWorship data on this machine ───────────────────────────────

func TestRealSongs(t *testing.T) {
	requireLocalEW(t)
	songs, err := ReadSongs(localSongsDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(songs) != 223 {
		t.Fatalf("read %d songs, want 223", len(songs))
	}
	byTitle := map[string]models.Song{}
	for _, s := range songs {
		byTitle[s.Title] = s
		if len(s.Sections) == 0 {
			t.Errorf("%q has no sections", s.Title)
		}
		for _, sec := range s.Sections {
			if sec.Label == "" || len(sec.Lines) == 0 {
				t.Errorf("%q: bad section %+v", s.Title, sec)
			}
			for _, l := range sec.Lines {
				if strings.ContainsAny(l, `\{}`) || l != strings.TrimSpace(l) || l == "" {
					t.Errorf("%q: bad line %q", s.Title, l)
				}
				if _, isLabel := parseLabel(l); isLabel {
					t.Errorf("%q: label left in lines: %q", s.Title, l)
				}
			}
		}
	}

	ag, ok := byTitle["Amazing Grace"]
	if !ok {
		t.Fatal("Amazing Grace missing")
	}
	if ag.Author != "John Newton" || ag.Copyright != "Public Domain" {
		t.Errorf("Amazing Grace metadata = %+v", ag)
	}
	if len(ag.Sections) != 4 {
		t.Fatalf("Amazing Grace sections = %d, want 4", len(ag.Sections))
	}
	for i, sec := range ag.Sections {
		if sec.Kind != models.SectionVerse || sec.Number != i+1 || len(sec.Lines) != 4 {
			t.Errorf("Amazing Grace section %d = %+v", i, sec)
		}
	}
	if got := ag.Sections[0].Lines[0]; got != "Amazing grace how sweet the sound," {
		t.Errorf("Amazing Grace first line = %q", got)
	}
	if got := ag.Sections[3].Lines[3]; got != "Than when we've first begun." {
		t.Errorf("Amazing Grace last line = %q", got)
	}

	abide := byTitle["Abide With Me"]
	if abide.Author != "Henry F. Lyte William H. Monk" || len(abide.Sections) < 5 {
		t.Errorf("Abide With Me = %+v", abide)
	}
	if abide.Sections[0].Label != "Verse 1" || abide.Sections[0].Lines[0] != "Abide with me!" {
		t.Errorf("Abide With Me first section = %+v", abide.Sections[0])
	}
	last := abide.Sections[len(abide.Sections)-1]
	if last.Lines[len(last.Lines)-1] != "O Lord, abide with me!" {
		t.Errorf("Abide With Me last line = %q", last.Lines[len(last.Lines)-1])
	}

	// A chorus split over two slides by a blank line.
	glory := byTitle["A New Name In Glory"]
	if len(glory.Sections) < 2 || glory.Sections[0].Label != "Chorus 1" || glory.Sections[1].Label != "Chorus 1" ||
		glory.Sections[1].Lines[0] != "For there's a new name" {
		t.Errorf("A New Name In Glory sections = %+v", glory.Sections[:2])
	}
	fortress := byTitle["A Mighty Fortress Is Our God"]
	if fortress.Author != "Hans Leo Hassler Martin Luther" {
		t.Errorf("A Mighty Fortress author = %q", fortress.Author)
	}
}

func TestRealBibles(t *testing.T) {
	requireLocalEW(t)
	files, err := FindBibles(localBiblesDir())
	if err != nil {
		t.Fatal(err)
	}
	pd := map[string]bool{}
	for _, f := range files {
		pd[f.Abbrev] = f.PublicDomain
	}
	if want := map[string]bool{"HCSB": false, "KJV": true, "RVA": true}; !mapsEqual(pd, want) {
		t.Fatalf("FindBibles = %+v", files)
	}
}

func mapsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

func verseMap(vs []models.Verse) map[[3]int]string {
	m := make(map[[3]int]string, len(vs))
	for _, v := range vs {
		m[[3]int{v.Book, v.Chapter, v.Verse}] = v.Text
	}
	return m
}

func TestRealKJV(t *testing.T) {
	requireLocalEW(t)
	start := time.Now()
	bible, verses, err := ReadBible(filepath.Join(localBiblesDir(), "KJV.ewb"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("decoded KJV in %v", time.Since(start))
	if bible.Abbrev != "KJV" || bible.Name != "King James Version" || bible.Language != "en" || bible.Copyright != "Public Domain" {
		t.Errorf("bible = %+v", bible)
	}
	if len(verses) != 31102 {
		t.Errorf("verses = %d, want 31102", len(verses))
	}
	m := verseMap(verses)
	known := map[[3]int]string{
		{1, 1, 1}:      "In the beginning God created the heaven and the earth.",
		{1, 1, 2}:      "And the earth was without form, and void; and darkness was upon the face of the deep. And the Spirit of God moved upon the face of the waters.",
		{1, 50, 26}:    "So Joseph died, being an hundred and ten years old: and they embalmed him, and he was put in a coffin in Egypt.",
		{19, 23, 1}:    "The LORD is my shepherd; I shall not want.",
		{19, 119, 176}: "I have gone astray like a lost sheep; seek thy servant; for I do not forget thy commandments.",
		{40, 1, 1}:     "The book of the generation of Jesus Christ, the son of David, the son of Abraham.",
		{43, 3, 16}:    "For God so loved the world, that he gave his only begotten Son, that whosoever believeth in him should not perish, but have everlasting life.",
		{64, 1, 14}:    "But I trust I shall shortly see thee, and we shall speak face to face. Peace be to thee. Our friends salute thee. Greet the friends by name.",
		{66, 22, 21}:   "The grace of our Lord Jesus Christ be with you all. Amen.",
	}
	for ref, want := range known {
		if got := m[ref]; got != want {
			t.Errorf("%v = %q, want %q", ref, got, want)
		}
	}
	count := func(book, chapter int) int {
		n := 0
		for k := range m {
			if k[0] == book && (chapter == 0 || k[1] == chapter) {
				n++
			}
		}
		return n
	}
	if n := count(19, 119); n != 176 {
		t.Errorf("Psalm 119 has %d verses, want 176", n)
	}
	if n := count(64, 0); n != 14 {
		t.Errorf("3 John has %d verses, want 14", n)
	}
	if n := count(1, 1); n != 31 {
		t.Errorf("Genesis 1 has %d verses, want 31", n)
	}
	books := map[int]bool{}
	for i, v := range verses {
		books[v.Book] = true
		if strings.ContainsAny(v.Text, "[]") || v.Text != strings.TrimSpace(v.Text) {
			t.Fatalf("unclean text %v: %q", v, v.Text)
		}
		if i > 0 {
			p := verses[i-1]
			if v.Book < p.Book || (v.Book == p.Book && (v.Chapter < p.Chapter || (v.Chapter == p.Chapter && v.Verse <= p.Verse))) {
				t.Fatalf("not in canonical order: %v after %v", v, p)
			}
		}
	}
	if len(books) != 66 {
		t.Errorf("books = %d, want 66", len(books))
	}
}

func TestRealRVA(t *testing.T) {
	requireLocalEW(t)
	bible, verses, err := ReadBible(filepath.Join(localBiblesDir(), "RVA.ewb"))
	if err != nil {
		t.Fatal(err)
	}
	if bible.Abbrev != "RVA" || bible.Name != "Reina-Valera Antigua" || bible.Language != "es" || bible.Copyright != "Public Domain" {
		t.Errorf("bible = %+v", bible)
	}
	if len(verses) != 31102 {
		t.Errorf("verses = %d, want 31102", len(verses))
	}
	m := verseMap(verses)
	known := map[[3]int]string{
		{1, 1, 1}:    "EN el principio crió Dios los cielos y la tierra.",
		{19, 23, 1}:  "JEHOVÁ es mi pastor; nada me faltará.",
		{43, 3, 16}:  "Porque de tal manera amó Dios al mundo, que ha dado á su Hijo unigénito, para que todo aquel que en Él cree, no se pierda, mas tenga vida eterna.",
		{66, 22, 21}: "La gracia de nuestro Señor Jesucristo sea con todos vosotros. Amén. FIN.",
	}
	for ref, want := range known {
		if got := m[ref]; got != want {
			t.Errorf("%v = %q, want %q", ref, got, want)
		}
	}
}

func TestRealHCSB(t *testing.T) {
	requireLocalEW(t)
	bible, verses, err := ReadBible(filepath.Join(localBiblesDir(), "HCSB.ewb"))
	if err != nil {
		t.Fatal(err)
	}
	if bible.Abbrev != "HCSB" || bible.Copyright == "Public Domain" || bible.Copyright == "" {
		t.Errorf("bible = %+v", bible)
	}
	m := verseMap(verses)
	// 2 Samuel 24:9 is stored with chapter 2 in the file; the decoder repairs it.
	if m[[3]int{10, 24, 9}] == "" {
		t.Error("2 Samuel 24:9 missing")
	}
	if len(verses) < 31000 {
		t.Errorf("verses = %d", len(verses))
	}
}

func TestRealImportInto(t *testing.T) {
	requireLocalEW(t)
	ctx := context.Background()
	st := openStore(t)
	opt := Options{SongsDir: localSongsDir(), BiblesDir: localBiblesDir()}
	start := time.Now()
	rep, err := ImportInto(ctx, st, opt)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("first import in %v: %+v", time.Since(start), rep)
	if rep.SongsImported != 223 || rep.SongsSkipped != 0 {
		t.Errorf("songs: %+v", rep)
	}
	slices.Sort(rep.BiblesImported)
	if !slices.Equal(rep.BiblesImported, []string{"KJV", "RVA"}) || !slices.Equal(rep.BiblesSkipped, []string{"HCSB (copyrighted)"}) {
		t.Errorf("bibles: %+v", rep)
	}
	if len(rep.Warnings) != 0 {
		t.Errorf("warnings: %q", rep.Warnings)
	}

	res, err := st.SearchSongs(ctx, "amazing grace", 0)
	if err != nil || !slices.ContainsFunc(res, func(s models.SongSummary) bool { return s.Title == "Amazing Grace" }) {
		t.Errorf("search amazing grace = %+v, %v", res, err)
	}
	if res, _ := st.SearchSongs(ctx, "wretch", 0); !slices.ContainsFunc(res, func(s models.SongSummary) bool { return s.Title == "Amazing Grace" }) {
		t.Errorf("lyric search = %+v", res)
	}
	bibles, _ := st.ListBibles(ctx)
	var kjv models.Bible
	for _, b := range bibles {
		if b.Abbrev == "KJV" {
			kjv = b
		}
	}
	vs, err := st.Verses(ctx, kjv.ID, models.Passage{Book: 43, Ranges: []models.VerseRange{{Chapter: 3, VerseStart: 16, VerseEnd: 16}}})
	if err != nil || len(vs) != 1 || !strings.HasPrefix(vs[0].Text, "For God so loved the world") {
		t.Errorf("John 3:16 = %+v, %v", vs, err)
	}
	if n, _ := st.VerseCount(ctx, kjv.ID, 19, 119); n != 176 {
		t.Errorf("Psalm 119 verse count = %d", n)
	}
	if hits, _ := st.SearchVerses(ctx, kjv.ID, "begotten son", 0); len(hits) == 0 {
		t.Error("verse search found nothing")
	}

	start = time.Now()
	rep, err = ImportInto(ctx, st, opt)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("second import in %v: %+v", time.Since(start), rep)
	if rep.SongsImported != 0 || rep.SongsSkipped != 223 || len(rep.BiblesImported) != 0 {
		t.Errorf("second run: %+v", rep)
	}
	if n, _ := st.CountSongs(ctx); n != 223 {
		t.Errorf("CountSongs = %d", n)
	}
	if bibles, _ := st.ListBibles(ctx); len(bibles) != 2 {
		t.Errorf("bibles after second run = %+v", bibles)
	}
}

func TestRealImportCopyrightedBiblesOnly(t *testing.T) {
	requireLocalEW(t)
	ctx := context.Background()
	st := openStore(t)
	opt := Options{SongsDir: localSongsDir(), BiblesDir: localBiblesDir()}
	if _, err := ImportInto(ctx, st, opt); err != nil {
		t.Fatal(err)
	}
	opt.BiblesOnly, opt.IncludeCopyrightedBibles = true, true
	rep, err := ImportInto(ctx, st, opt)
	if err != nil {
		t.Fatal(err)
	}
	if rep.SongsImported != 0 || rep.SongsSkipped != 0 {
		t.Errorf("a Bibles-only import touched songs: %+v", rep)
	}
	if !slices.Equal(rep.BiblesImported, []string{"HCSB"}) {
		t.Errorf("imported %q, want HCSB", rep.BiblesImported)
	}
	slices.Sort(rep.BiblesSkipped)
	if !slices.Equal(rep.BiblesSkipped, []string{"KJV (already installed)", "RVA (already installed)"}) {
		t.Errorf("skipped %q", rep.BiblesSkipped)
	}
	bibles, _ := st.ListBibles(ctx)
	if !slices.ContainsFunc(bibles, func(b models.Bible) bool { return b.Abbrev == "HCSB" }) {
		t.Errorf("HCSB missing from %+v", bibles)
	}
	// Idempotent.
	rep, err = ImportInto(ctx, st, opt)
	if err != nil || len(rep.BiblesImported) != 0 {
		t.Errorf("second import: %+v, %v", rep, err)
	}
}
