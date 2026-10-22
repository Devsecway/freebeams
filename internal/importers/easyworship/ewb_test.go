package easyworship

import (
	"bytes"
	"compress/zlib"
	"database/sql"
	"encoding/binary"
	"path/filepath"
	"reflect"
	"testing"
	"unicode/utf8"

	"freebeams/internal/models"
)

type fakeVerse struct {
	book, chapter, verse int
	text                 string
}

// packRecord builds one verse_info record (see docs/formats/ewb.md).
func packRecord(off, length, bibleID, book, chapter, verse int) []byte {
	var rec [8]byte
	binary.LittleEndian.PutUint32(rec[0:], uint32(off)<<10|uint32(length))
	binary.LittleEndian.PutUint32(rec[4:], uint32(bibleID)<<25|uint32(book)<<18|uint32(chapter)<<10|uint32(verse)<<2)
	return rec[:]
}

// buildBook returns the zlib stream and verse_info for verses of one book.
func buildBook(t testing.TB, bibleID int, verses []fakeVerse) (stream, info []byte) {
	t.Helper()
	text := string([]rune{0, rune(bibleID)})
	off := 0
	for _, v := range verses {
		n := utf8.RuneCountInString(v.text)
		info = append(info, packRecord(off, n, bibleID, v.book, v.chapter, v.verse)...)
		text += v.text
		off += n
	}
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	zw.Write([]byte(text))
	zw.Close()
	return buf.Bytes(), info
}

// writeEWB creates a synthetic .ewb file with the real schema.
func writeEWB(t testing.TB, path, name, abbrev, lang, copyright string, bibleID int, books [][]fakeVerse) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		`CREATE TABLE header (rowid INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, id INT8 NOT NULL, key INT8 NOT NULL, name TEXT NOT NULL, abbrev_name TEXT NOT NULL, lang_code TEXT NOT NULL, plugin_version INT8 NOT NULL, format_version INT8 NOT NULL, copyright TEXT NULL, license TEXT NULL)`,
		`CREATE TABLE books (rowid INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, name TEXT NOT NULL, abbrev_name TEXT NOT NULL, alt_name TEXT NULL, book_info BLOB NULL, verse_info BLOB)`,
		`CREATE TABLE streams (rowid INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, stream BLOB NOT NULL)`,
		`CREATE TABLE words (rowid INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL, word TEXT NOT NULL, verse_info INT32A NOT NULL)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	var cr any
	if copyright != "" {
		cr = copyright
	}
	if _, err := db.Exec(`INSERT INTO header (id, key, name, abbrev_name, lang_code, plugin_version, format_version, copyright) VALUES (?, 0, ?, ?, ?, 1, 1, ?)`,
		bibleID, name, abbrev, lang, cr); err != nil {
		t.Fatal(err)
	}
	for i, verses := range books {
		stream, info := buildBook(t, bibleID, verses)
		if _, err := db.Exec(`INSERT INTO books (rowid, name, abbrev_name, book_info, verse_info) VALUES (?, ?, ?, ?, ?)`,
			i+1, "Book", "Bk", []byte{1, byte(len(verses))}, info); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO streams (rowid, stream) VALUES (?, ?)`, i+1, stream); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDecodeVerseInfoRealRecords(t *testing.T) {
	// The first two KJV Genesis records, verbatim.
	blob := []byte{0x37, 0, 0, 0, 0x04, 0x04, 0x04, 0x22, 0x91, 0xDC, 0, 0, 0x08, 0x04, 0x04, 0x22}
	got := decodeVerseInfo(blob)
	want := []ewbRecord{
		{Offset: 0, Length: 55, Book: 1, Chapter: 1, Verse: 1, BibleID: 17},
		{Offset: 55, Length: 145, Book: 1, Chapter: 1, Verse: 2, BibleID: 17},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decodeVerseInfo = %+v, want %+v", got, want)
	}
	if len(decodeVerseInfo(blob[:7])) != 0 || len(decodeVerseInfo(nil)) != 0 {
		t.Fatal("short blob produced records")
	}
}

func TestDecodeBook(t *testing.T) {
	verses := []fakeVerse{
		{43, 3, 16, "Porque de tal manera amó Dios al mundo. "},
		{43, 3, 17, "Darkness [was] upon  the deep. "},
		{43, 4, 1, "Ñandú «x» ¿y? "},
	}
	stream, info := buildBook(t, 36, verses)
	got, err := decodeBook(stream, info, 43)
	if err != nil {
		t.Fatal(err)
	}
	want := []models.Verse{
		{Book: 43, Chapter: 3, Verse: 16, Text: "Porque de tal manera amó Dios al mundo."},
		{Book: 43, Chapter: 3, Verse: 17, Text: "Darkness was upon the deep."},
		{Book: 43, Chapter: 4, Verse: 1, Text: "Ñandú «x» ¿y?"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decodeBook = %+v, want %+v", got, want)
	}
}

func TestDecodeBookRepairsAndSkips(t *testing.T) {
	stream, _ := buildBook(t, 14, []fakeVerse{{10, 24, 8, "a "}, {10, 24, 9, "b "}, {10, 24, 10, "c "}, {10, 24, 11, "d "}})
	var info []byte
	info = append(info, packRecord(0, 2, 14, 10, 24, 8)...)
	info = append(info, packRecord(2, 2, 14, 10, 2, 9)...)     // HCSB-style corrupt chapter: repaired to 24:9
	info = append(info, packRecord(4, 2, 14, 10, 24, 9)...)    // duplicate: dropped
	info = append(info, packRecord(6, 2, 14, 0, 24, 10)...)    // invalid book: falls back to position
	info = append(info, packRecord(500, 2, 14, 10, 24, 11)...) // out of bounds: dropped
	info = append(info, packRecord(0, 2, 14, 10, 0, 0)...)     // zero chapter/verse: dropped
	info = append(info, 1, 2, 3)                               // trailing partial record: ignored
	got, err := decodeBook(stream, info, 7)
	if err != nil {
		t.Fatal(err)
	}
	want := []models.Verse{
		{Book: 10, Chapter: 24, Verse: 8, Text: "a"},
		{Book: 10, Chapter: 24, Verse: 9, Text: "b"},
		{Book: 7, Chapter: 24, Verse: 10, Text: "d"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decodeBook = %+v, want %+v", got, want)
	}
}

func TestDecodeBookCorrupt(t *testing.T) {
	for _, stream := range [][]byte{nil, {0x78}, {0x78, 0xDA, 1, 2, 3, 4}, []byte("not zlib at all")} {
		if _, err := decodeBook(stream, packRecord(0, 5, 1, 1, 1, 1), 1); err == nil {
			t.Errorf("decodeBook(%x) succeeded", stream)
		}
	}
	// Truncated zlib stream.
	stream, info := buildBook(t, 1, []fakeVerse{{1, 1, 1, "In the beginning"}})
	if _, err := decodeBook(stream[:len(stream)-6], info, 1); err == nil {
		t.Error("truncated stream decoded without error")
	}
}

func TestCleanVerseText(t *testing.T) {
	tests := map[string]string{
		"The LORD [is] my shepherd; I shall not want. ": "The LORD is my shepherd; I shall not want.",
		"[Our] friends salute thee.":                    "Our friends salute thee.",
		"  a \t b\n c  ":                                "a b c",
		"[]":                                            "",
	}
	for in, want := range tests {
		if got := cleanVerseText(in); got != want {
			t.Errorf("cleanVerseText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsPublicDomain(t *testing.T) {
	tests := []struct {
		abbrev, name string
		want         bool
	}{
		{"KJV", "King James Version", true},
		{"kjv", "", true},
		{"RVA", "Reina-Valera Antigua", true},
		{"ASV", "", true},
		{"WEB", "", true},
		{"YLT", "", true},
		{"DARBY", "", true},
		{"BBE", "", true},
		{"XYZ", "American Standard Version", true},
		{"XYZ", "Young's  Literal Translation", true},
		{"HCSB", "Holman Christian Standard Bible", false},
		{"NKJV", "New King James Version", false},
		{"NIV", "New International Version", false},
		{"RVR1960", "Reina-Valera 1960", false},
		{"NASB", "New American Standard Bible", false},
		{"", "", false},
	}
	for _, tc := range tests {
		if got := isPublicDomain(tc.abbrev, tc.name); got != tc.want {
			t.Errorf("isPublicDomain(%q, %q) = %v, want %v", tc.abbrev, tc.name, got, tc.want)
		}
	}
}

func TestReadSyntheticBible(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Test.ewb")
	notice := `{\rtf1\ansi\ansicpg1252{\fonttbl{\f0 Courier New;}}\f0 TST BIBLE TEXT\par \'a9 Somebody\par}`
	writeEWB(t, path, "Test Translation", "TST", "EN", notice, 5, [][]fakeVerse{
		{{1, 1, 1, "In the beginning. "}, {1, 1, 2, "And the earth. "}},
		{{2, 1, 1, "Now these [are] the names. "}},
	})
	bible, verses, err := ReadBible(path)
	if err != nil {
		t.Fatal(err)
	}
	wantBible := models.Bible{Abbrev: "TST", Name: "Test Translation", Language: "en", Copyright: "TST BIBLE TEXT\n© Somebody"}
	if bible != wantBible {
		t.Fatalf("bible = %+v, want %+v", bible, wantBible)
	}
	if len(verses) != 3 || verses[2] != (models.Verse{Book: 2, Chapter: 1, Verse: 1, Text: "Now these are the names."}) {
		t.Fatalf("verses = %+v", verses)
	}
	files, err := FindBibles(dir)
	if err != nil || len(files) != 1 || files[0].Abbrev != "TST" || files[0].PublicDomain || files[0].Path != path {
		t.Fatalf("FindBibles = %+v, %v", files, err)
	}
	info, n, err := ReadBibleInfo(path)
	if err != nil || info != wantBible || n != 3 {
		t.Fatalf("ReadBibleInfo = %+v, %d, %v", info, n, err)
	}
}

func TestReadBibleErrors(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := ReadBible(filepath.Join(dir, "missing.ewb")); err == nil {
		t.Error("missing file: no error")
	}
	// A SQLite file without the expected tables.
	bad := filepath.Join(dir, "bad.ewb")
	db, _ := sql.Open("sqlite", bad)
	db.Exec(`CREATE TABLE x (y)`)
	db.Close()
	if _, _, err := ReadBible(bad); err == nil {
		t.Error("bad schema: no error")
	}
	if _, _, err := ReadBibleInfo(bad); err == nil {
		t.Error("ReadBibleInfo, bad schema: no error")
	}
	files, warnings, err := findBibles(dir)
	if err != nil || len(files) != 0 || len(warnings) != 1 {
		t.Errorf("findBibles = %v, %v, %v", files, warnings, err)
	}
	if files, err := FindBibles(filepath.Join(dir, "nope")); err != nil || len(files) != 0 {
		t.Errorf("FindBibles(missing dir) = %v, %v", files, err)
	}
}
