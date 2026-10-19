package bibletext

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"freeworship/internal/models"
)

func TestCleanText(t *testing.T) {
	tests := []struct {
		name           string
		in             string
		chapter, verse int
		want           string
	}{
		{"plain", "In the beginning God created the heaven and the earth.", 1, 1, "In the beginning God created the heaven and the earth."},
		{"trailing space", "For John was not yet cast into prison. ", 3, 24, "For John was not yet cast into prison."},
		{"footnote marker", "Except one be born [a]anew, he cannot see", 3, 3, "Except one be born anew, he cannot see"},
		{"two-letter marker", "and [ab]his [zz]name", 1, 2, "and his name"},
		{"nested marker in italics", "Saul was [[a]forty] years old", 13, 1, "Saul was forty years old"},
		{"italics keep the word", "darkness [was] upon the face", 1, 2, "darkness was upon the face"},
		{"leaked psalm heading", "[a]36 The transgression of the wicked", 36, 1, "The transgression of the wicked"},
		{"heading only on verse 1", "36 The transgression", 36, 2, "36 The transgression"},
		{"heading must match the chapter", "35 The transgression", 36, 1, "35 The transgression"},
		{"whitespace collapsed", "  Praise  ye\tJehovah.\n Praise ", 150, 1, "Praise ye Jehovah. Praise"},
		{"uppercase brackets keep words", "[A] man", 1, 1, "A man"},
		{"empty", " [a] ", 1, 1, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanText(tt.in, tt.chapter, tt.verse); got != tt.want {
				t.Errorf("CleanText(%q, %d, %d) = %q, want %q", tt.in, tt.chapter, tt.verse, got, tt.want)
			}
		})
	}
}

const sampleDoc = `{
  "abbrev": " SMP ", "name": "Sample  Bible", "language": "EN", "copyright": " Public Domain ",
  "books": {
    "John": {"3": {"17": "For God sent not the Son into the world to judge the world.", "16": "For God so loved the world. ", "16-17": "combined"}},
    "Psalm": {"23": {"1": "[a]23 Jehovah is my shepherd; I shall not want."}, "x": {"1": "bad chapter"}, "151": {"1": "no such chapter"}},
    "1_Samuel": {"13": {"1": "Saul was [[a]forty] years old when he began to reign"}},
    "66": {"22": {"21": "The grace of the Lord Jesus be with the saints. Amen.", "a": "bad key", "5": 7, "6": "   [b] "}},
    "Foo": {"1": {"1": "unknown book"}},
    "Jn": {"3": {"16": "duplicate of John 3:16"}},
    "Acts": "not an object",
    "Romans": {"1": "not an object"}
  }
}`

func TestDecode(t *testing.T) {
	res, err := Decode(strings.NewReader(sampleDoc))
	if err != nil {
		t.Fatal(err)
	}
	want := models.Bible{Abbrev: "SMP", Name: "Sample Bible", Language: "en", Copyright: "Public Domain"}
	if res.Bible != want {
		t.Errorf("bible = %+v, want %+v", res.Bible, want)
	}
	wantVerses := []models.Verse{
		{Book: 9, Chapter: 13, Verse: 1, Text: "Saul was forty years old when he began to reign"},
		{Book: 19, Chapter: 23, Verse: 1, Text: "Jehovah is my shepherd; I shall not want."},
		{Book: 43, Chapter: 3, Verse: 16, Text: "For God so loved the world."},
		{Book: 43, Chapter: 3, Verse: 17, Text: "For God sent not the Son into the world to judge the world."},
		{Book: 66, Chapter: 22, Verse: 21, Text: "The grace of the Lord Jesus be with the saints. Amen."},
	}
	// The file's own order decides which duplicate wins ("John" before "Jn").
	if !reflect.DeepEqual(res.Verses, wantVerses) {
		t.Errorf("verses = %+v\nwant %+v", res.Verses, wantVerses)
	}
	wantWarnings := []string{
		`John 3 verse 16-17: duplicate of 43 3:16, skipped`,
		`Psalm: bad chapter "x", skipped`,
		`Psalm 151: chapter 151 out of range (1-150), skipped`,
		`66 22: bad verse key "a", skipped`,
		`66 22 verse 5: not a string, skipped`,
		`66 22 verse 6: empty, skipped`,
		`book "Foo": unknown book, skipped`,
		`Jn 3 verse 16: duplicate of 43 3:16, skipped`,
		`book "Acts": not an object`,
		`Romans 1: not an object`,
	}
	if !reflect.DeepEqual(res.Warnings, wantWarnings) {
		t.Errorf("warnings =\n%s\nwant\n%s", strings.Join(res.Warnings, "\n"), strings.Join(wantWarnings, "\n"))
	}
}

func TestDecodeCombinedVerseKey(t *testing.T) {
	res, err := Decode(strings.NewReader(`{"abbrev":"X","books":{"Mark":{"1":{"1-2": "two verses in one", "3": "three"}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []models.Verse{{Book: 41, Chapter: 1, Verse: 1, Text: "two verses in one"}, {Book: 41, Chapter: 1, Verse: 3, Text: "three"}}
	if !reflect.DeepEqual(res.Verses, want) || len(res.Warnings) != 0 {
		t.Errorf("got %+v, %q", res.Verses, res.Warnings)
	}
	if res.Bible.Copyright != "" {
		t.Errorf("unknown translation got copyright %q", res.Bible.Copyright)
	}
}

func TestDecodePublicDomainByName(t *testing.T) {
	res, err := Decode(strings.NewReader(`{"abbrev":"KJV","books":{"John":{"3":{"16":"For God so loved the world"}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Bible.Copyright != "Public Domain" {
		t.Errorf("KJV copyright = %q, want Public Domain", res.Bible.Copyright)
	}
}

func TestDecodeErrors(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"not json", `garbage`, "invalid character"},
		{"no abbrev", `{"books":{"John":{"3":{"16":"x"}}}}`, `"abbrev" is required`},
		{"no books", `{"abbrev":"X"}`, `no "books"`},
		{"no verses", `{"abbrev":"X","books":{"Foo":{"1":{"1":"x"}}}}`, "no verses"},
		{"books not an object", `{"abbrev":"X","books":[]}`, "not an object"},
		{"books null", `{"abbrev":"X","books":null}`, `no "books"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode(strings.NewReader(tt.in))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Decode(%q) error = %v, want containing %q", tt.in, err, tt.want)
			}
		})
	}
}

func TestWarningsAreBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"abbrev":"X","books":{"John":{"3":{"16":"ok"`)
	for i := 0; i < maxWarnings+10; i++ {
		b.WriteString(`,"k` + strings.Repeat("x", i) + `":"bad"`)
	}
	b.WriteString(`}}}}`)
	res, err := Decode(strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != maxWarnings+1 || res.Warnings[maxWarnings] != "… and 10 more" {
		t.Errorf("warnings: %d, last %q", len(res.Warnings), res.Warnings[len(res.Warnings)-1])
	}
}

// writeTree writes a translation folder in the bible-translations layout.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "kjv")
	writeTree(t, root, map[string]string{
		"old/Psalm/23.json":          `{"1": "The Lord is my shepherd; I shall not want."}`,
		"old/Song_of_Solomon/1.json": `{"1": "The song of songs, which is Solomon's."}`,
		"new/John/3.json":            `{"16": "For God so loved the world", "17": "For God sent not his Son"}`,
		"new/John/4.json":            `{"1": "When therefore the Lord knew"}`,
		"new/John/notes.txt":         `not a chapter`,
		"new/John/.DS_Store":         `ignored`,
		"new/John/999.json":          `{"1": "chapter out of range"}`,
		"new/John/5.json":            `not json`,
		"new/Bogus/1.json":           `{"1": "unknown book"}`,
		"new/README.md":              `not a book`,
		"new/1_Corinthians/13.json":  `{"13": "And now abideth faith, hope, charity"}`,
	})
	want := []models.Verse{
		{Book: 19, Chapter: 23, Verse: 1, Text: "The Lord is my shepherd; I shall not want."},
		{Book: 22, Chapter: 1, Verse: 1, Text: "The song of songs, which is Solomon's."},
		{Book: 43, Chapter: 3, Verse: 16, Text: "For God so loved the world"},
		{Book: 43, Chapter: 3, Verse: 17, Text: "For God sent not his Son"},
		{Book: 43, Chapter: 4, Verse: 1, Text: "When therefore the Lord knew"},
		{Book: 46, Chapter: 13, Verse: 13, Text: "And now abideth faith, hope, charity"},
	}
	wantWarnings := []string{
		"new/Bogus: unknown book, skipped",
		"new/John/5.json: invalid character 'o' in literal null (expecting 'u')",
		"new/John/999.json: chapter 999 out of range (1-21), skipped",
		"new/John/notes.txt: not a chapter file, skipped",
		"new/README.md: not a book folder, skipped",
	}
	wantBible := models.Bible{Abbrev: "KJV", Name: "King James Version", Language: "en", Copyright: "Public Domain"}

	for _, path := range []string{root, filepath.Join(root, "new", "John", "3.json"), filepath.Join(root, "old", "Psalm", "23.json")} {
		res, err := Read(path)
		if err != nil {
			t.Fatalf("Read(%s): %v", path, err)
		}
		if res.Bible != wantBible {
			t.Errorf("Read(%s) bible = %+v, want %+v", path, res.Bible, wantBible)
		}
		if !reflect.DeepEqual(res.Verses, want) {
			t.Errorf("Read(%s) verses = %+v\nwant %+v", path, res.Verses, want)
		}
		if !reflect.DeepEqual(res.Warnings, wantWarnings) {
			t.Errorf("Read(%s) warnings =\n%s\nwant\n%s", path, strings.Join(res.Warnings, "\n"), strings.Join(wantWarnings, "\n"))
		}
	}

	// An unknown abbreviation: no name, no copyright claim.
	other := filepath.Join(t.TempDir(), "smk")
	writeTree(t, other, map[string]string{"new/John/3.json": `{"16": "x"}`})
	res, err := ReadDir(other)
	if err != nil || res.Bible != (models.Bible{Abbrev: "SMK"}) || len(res.Verses) != 1 {
		t.Errorf("ReadDir(smk) = %+v, %v", res, err)
	}
}

func TestReadErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := Read(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("missing file: no error")
	}
	if _, err := ReadDir(dir); err == nil || !strings.Contains(err.Error(), "not a translation folder") {
		t.Errorf("empty dir: %v", err)
	}
	// A chapter file outside a translation folder is a single-file Bible, and not a valid one.
	p := filepath.Join(dir, "3.json")
	os.WriteFile(p, []byte(`{"16": "For God so loved the world"}`), 0o644)
	if _, err := Read(p); err == nil || !strings.Contains(err.Error(), `"abbrev" is required`) {
		t.Errorf("bare chapter file: %v", err)
	}
	empty := filepath.Join(dir, "empty")
	writeTree(t, empty, map[string]string{"new/John/3.json": `{}`})
	if _, err := ReadDir(empty); err == nil || !strings.Contains(err.Error(), "no verses") {
		t.Errorf("no verses: %v", err)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	bible := models.Bible{Abbrev: "RT", Name: "Round Trip", Language: "en", Copyright: "Public Domain"}
	verses := []models.Verse{
		{Book: 43, Chapter: 3, Verse: 17, Text: "second \"quoted\" <b>&</b>"},
		{Book: 1, Chapter: 1, Verse: 1, Text: "In the beginning"},
		{Book: 43, Chapter: 3, Verse: 16, Text: "For God so loved the world — unicode ’"},
		{Book: 19, Chapter: 119, Verse: 176, Text: "I have gone astray"},
	}
	var buf bytes.Buffer
	if err := Encode(&buf, bible, verses); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"Psalms":{"119":{"176":`) || !strings.Contains(buf.String(), `<b>&</b>`) {
		t.Errorf("encoded:\n%s", buf.String())
	}
	res, err := Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	want := []models.Verse{verses[1], verses[3], verses[2], verses[0]}
	if res.Bible != bible || !reflect.DeepEqual(res.Verses, want) || len(res.Warnings) != 0 {
		t.Errorf("round trip = %+v, %+v, %q", res.Bible, res.Verses, res.Warnings)
	}
	if err := Encode(&buf, bible, []models.Verse{{Book: 67, Chapter: 1, Verse: 1, Text: "x"}}); err == nil {
		t.Error("invalid reference encoded")
	}
	if err := Encode(&buf, models.Bible{}, nil); err == nil {
		t.Error("empty abbrev encoded")
	}
}

// realRepo returns the bible-translations clone named by
// FREEWORSHIP_BIBLE_TRANSLATIONS (its json/ folder), or skips.
func realRepo(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("FREEWORSHIP_BIBLE_TRANSLATIONS")
	if dir == "" {
		t.Skip("FREEWORSHIP_BIBLE_TRANSLATIONS is not set (a clone of github.com/hargarpay/bible-translations)")
	}
	if fi, err := os.Stat(filepath.Join(dir, "json", "KJV")); err != nil || !fi.IsDir() {
		t.Skipf("%s has no json/KJV folder", dir)
	}
	return filepath.Join(dir, "json")
}

// TestRealTranslations decodes the KJV and ASV folders of the source
// repository. The KJV has the full 31,102 verses of the Protestant canon. The
// ASV has 31,086: it follows the critical Greek text and omits 16 New
// Testament verses the KJV has (Matthew 17:21, 18:11, 23:14; Mark 7:16, 9:44,
// 9:46, 11:26, 15:28; Luke 17:36, 23:17; John 5:4; Acts 8:37, 15:34, 24:7,
// 28:29; Romans 16:24), which is the expected ASV versification, not a
// decoding loss.
func TestRealTranslations(t *testing.T) {
	root := realRepo(t)
	tests := []struct {
		abbrev string
		verses int
		john16 string
	}{
		{"KJV", 31102, "For God so loved the world, that he gave his only begotten Son, that whosoever believeth in him should not perish, but have everlasting life."},
		{"ASV", 31086, "For God so loved the world, that he gave his only begotten Son, that whosoever believeth on him should not perish, but have eternal life."},
	}
	for _, tt := range tests {
		t.Run(tt.abbrev, func(t *testing.T) {
			res, err := ReadDir(filepath.Join(root, tt.abbrev))
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Warnings) != 0 {
				t.Errorf("warnings: %q", res.Warnings)
			}
			if len(res.Verses) != tt.verses {
				t.Errorf("%d verses, want %d", len(res.Verses), tt.verses)
			}
			books := map[int]bool{}
			for _, v := range res.Verses {
				books[v.Book] = true
			}
			if len(books) != 66 {
				t.Errorf("%d books, want 66", len(books))
			}
			if got := find(res.Verses, 43, 3, 16); got != tt.john16 {
				t.Errorf("John 3:16 = %q, want %q", got, tt.john16)
			}
			if res.Bible.Copyright != "Public Domain" || res.Bible.Language != "en" || res.Bible.Name == "" {
				t.Errorf("bible = %+v", res.Bible)
			}
			for _, v := range res.Verses {
				if strings.ContainsAny(v.Text, "[] ") {
					t.Errorf("%d %d:%d not cleaned: %q", v.Book, v.Chapter, v.Verse, v.Text)
				}
			}
		})
	}
	// The ASV source leaks the psalm number into verse 1 of 18 psalms.
	res, _ := ReadDir(filepath.Join(root, "ASV"))
	if got := find(res.Verses, 19, 36, 1); !strings.HasPrefix(got, "The transgression of the wicked") {
		t.Errorf("ASV Psalm 36:1 = %q", got)
	}
	if got := find(res.Verses, 9, 13, 1); !strings.HasPrefix(got, "Saul was forty years old") {
		t.Errorf("ASV 1 Samuel 13:1 = %q", got)
	}
}

func find(verses []models.Verse, book, chapter, verse int) string {
	for _, v := range verses {
		if v.Book == book && v.Chapter == chapter && v.Verse == verse {
			return v.Text
		}
	}
	return ""
}
