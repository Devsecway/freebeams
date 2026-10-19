package bibletext

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"freebeams/internal/models"
)

// wholeBibleArray builds a 66-book array-layout document: books maps a book
// number to its chapters (chapter number → verse values); other books and
// chapters are empty. abbrevs overrides a book's "abbrev".
func wholeBibleArray(t *testing.T, books map[int]map[int][]any, abbrevs map[int]string) string {
	t.Helper()
	arr := make([]map[string]any, len(models.Books))
	for i := range arr {
		arr[i] = map[string]any{"abbrev": "xx", "name": "Livro", "chapters": []any{}}
	}
	for book, chapters := range books {
		n := 0
		for c := range chapters {
			if c > n {
				n = c
			}
		}
		list := make([]any, n)
		for i := range list {
			list[i] = []any{}
		}
		for c, verses := range chapters {
			list[c-1] = verses
		}
		arr[book-1]["chapters"] = list
	}
	for book, ab := range abbrevs {
		arr[book-1]["abbrev"] = ab
	}
	data, err := json.Marshal(arr)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestDecodeArrayWholeBible(t *testing.T) {
	doc := wholeBibleArray(t, map[int]map[int][]any{
		1:  {1: {"In the beginning God ( Elohim) created the heavens. [Heb 11:3]", "second ---------- [The earliest manuscripts]"}},
		19: {23: {"The Lordis my Shepherd [to feed me], [Ezek 34:11-31] I shall not want."}},
		32: {1: {"Jonah one one"}},
		43: {3: {"John three one"}},
		65: {1: {"Jude one", 7, "", "  Lordship endures  "}},
		66: {23: {"too many chapters"}},
	}, map[int]string{32: "jn", 43: "jo"}) // the repository's Portuguese abbreviations: position wins
	res, err := DecodeNamed(strings.NewReader("\xef\xbb\xbf"+doc), "en_smk.json")
	if err != nil {
		t.Fatal(err)
	}
	want := models.Bible{Abbrev: "SMK", Language: "en"}
	if res.Bible != want {
		t.Errorf("bible = %+v, want %+v", res.Bible, want)
	}
	wantVerses := []models.Verse{
		{Book: 1, Chapter: 1, Verse: 1, Text: "In the beginning God (Elohim) created the heavens."},
		{Book: 1, Chapter: 1, Verse: 2, Text: "second [The earliest manuscripts]"},
		{Book: 19, Chapter: 23, Verse: 1, Text: "The Lord is my Shepherd [to feed me], I shall not want."},
		{Book: 32, Chapter: 1, Verse: 1, Text: "Jonah one one"},
		{Book: 43, Chapter: 3, Verse: 1, Text: "John three one"},
		{Book: 65, Chapter: 1, Verse: 1, Text: "Jude one"},
		{Book: 65, Chapter: 1, Verse: 4, Text: "Lordship endures"},
	}
	if !reflect.DeepEqual(res.Verses, wantVerses) {
		t.Errorf("verses = %+v\nwant %+v", res.Verses, wantVerses)
	}
	wantWarnings := []string{
		"Jude 1 verse 2: not a string, skipped",
		"Jude 1 verse 3: empty, skipped",
		"Revelation 23: chapter 23 out of range (1-22), skipped",
	}
	if !reflect.DeepEqual(res.Warnings, wantWarnings) {
		t.Errorf("warnings =\n%s\nwant\n%s", strings.Join(res.Warnings, "\n"), strings.Join(wantWarnings, "\n"))
	}
}

func TestDecodeArrayPartialByName(t *testing.T) {
	doc := `[{"abbrev":"gn","name":"Genesis","chapters":[["In the beginning"]]},
	         {"abbrev":"jn","name":"Evangelho","chapters":[["Word"], "not an array"]},
	         {"abbrev":"zz","name":"Foo","chapters":[["x"]]}]`
	res, err := DecodeNamed(strings.NewReader(doc), filepath.Join("some", "dir", "KJV.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := models.Bible{Abbrev: "KJV", Name: "King James Version", Language: "en", Copyright: "Public Domain"}
	if res.Bible != want {
		t.Errorf("bible = %+v, want %+v", res.Bible, want)
	}
	wantVerses := []models.Verse{
		{Book: 1, Chapter: 1, Verse: 1, Text: "In the beginning"},
		{Book: 43, Chapter: 1, Verse: 1, Text: "Word"},
	}
	if !reflect.DeepEqual(res.Verses, wantVerses) {
		t.Errorf("verses = %+v\nwant %+v", res.Verses, wantVerses)
	}
	wantWarnings := []string{"John 2: not an array, skipped", `book 3 "Foo": unknown book, skipped`}
	if !reflect.DeepEqual(res.Warnings, wantWarnings) {
		t.Errorf("warnings = %q, want %q", res.Warnings, wantWarnings)
	}
}

func TestDecodeArrayErrors(t *testing.T) {
	one := `[{"abbrev":"gn","name":"Genesis","chapters":[["x"]]}]`
	tests := []struct {
		name, in, file, want string
	}{
		{"no file name", one, "", `"abbrev"`},
		{"file name without a letter", one, "3.json", `"abbrev"`},
		{"empty array", `[]`, "en_x.json", "no books"},
		{"not books", `[1, 2]`, "en_x.json", ""},
		{"no verses", `[{"abbrev":"gn","name":"Genesis","chapters":[[]]}]`, "en_x.json", "no verses"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecodeNamed(strings.NewReader(tt.in), tt.file)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("DecodeNamed(%q, %q) error = %v, want containing %q", tt.in, tt.file, err, tt.want)
			}
		})
	}
	// Read passes the file name along.
	p := filepath.Join(t.TempDir(), "en_rd.json")
	if err := os.WriteFile(p, []byte(one), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Read(p)
	if err != nil {
		t.Fatal(err)
	}
	if res.Bible.Abbrev != "RD" || res.Bible.Language != "en" || len(res.Verses) != 1 {
		t.Errorf("Read(en_rd.json) = %+v, %d verses", res.Bible, len(res.Verses))
	}
}

func TestTranslationFromFilename(t *testing.T) {
	tests := []struct {
		in, abbrev, language string
	}{
		{"en_esv.json", "ESV", "en"},
		{"/x/y/en_nkjv.json", "NKJV", "en"},
		{"pt-br_nvi.json", "NVI", "pt-br"},
		{"KJV.json", "KJV", ""},
		{"kjv", "KJV", ""},
		{"New Bible.json", "NEWBIBLE", ""},
		{"_kjv.json", "", ""},
		{"en_.json", "", ""},
		{"3.json", "", ""},
		{"", "", ""},
	}
	for _, tt := range tests {
		abbrev, language := TranslationFromFilename(tt.in)
		if abbrev != tt.abbrev || language != tt.language {
			t.Errorf("TranslationFromFilename(%q) = %q, %q; want %q, %q", tt.in, abbrev, language, tt.abbrev, tt.language)
		}
	}
}

func TestCleanArrayText(t *testing.T) {
	tests := []struct {
		name           string
		in             string
		chapter, verse int
		want           string
	}{
		{"cross-reference", "created the heavens and the earth. [Heb 11:3]", 1, 1, "created the heavens and the earth."},
		{"two references", "the rock. [Ex 17:6; Num 20:11] And", 10, 4, "the rock. And"},
		{"ff reference", "a refuge [Num 35:10ff] for", 20, 2, "a refuge for"},
		{"numbered book", "the king [1 Kin 12:28ff] made", 13, 8, "the king made"},
		{"amplification stays", "God so [greatly] loved", 3, 16, "God so [greatly] loved"},
		{"two-letter amplification stays", "they hear [me], they obey", 18, 44, "they hear [me], they obey"},
		{"textual note stays", "afraid. [The earliest manuscripts end here]", 16, 8, "afraid. [The earliest manuscripts end here]"},
		{"parenthesis closes up", "Siddim ( that is, the Salt Sea)", 14, 3, "Siddim (that is, the Salt Sea)"},
		{"glued Lord", "The Lordis my shepherd", 23, 1, "The Lord is my shepherd"},
		{"glued LORD", "the LORDwas with him", 39, 2, "the LORD was with him"},
		{"Lordship stays", "his Lordship endures", 1, 2, "his Lordship endures"},
		{"partly glued", "the Lordwillbe king", 14, 9, "the Lord willbe king"},
		{"dash row", "frightened. ---------- [The most ancient", 16, 8, "frightened. [The most ancient"},
		{"leaked heading", "23 The Lord is my shepherd", 23, 1, "The Lord is my shepherd"},
		{"whitespace", "  Praise  ye\tthe Lord.\n ", 150, 1, "Praise ye the Lord."},
		{"empty", " [Ps 1:1] ", 1, 1, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanArrayText(tt.in, tt.chapter, tt.verse); got != tt.want {
				t.Errorf("CleanArrayText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestRealArrayFiles decodes every *.json in $FREEBEAMS_BIBLE_ARRAYS (a
// folder of array-layout Bibles such as en_kjv.json) and checks that each
// is a whole Bible with John 3:16. It skips without the variable; the files
// are the user's and are never committed.
func TestRealArrayFiles(t *testing.T) {
	dir := os.Getenv("FREEBEAMS_BIBLE_ARRAYS")
	if dir == "" {
		t.Skip("FREEBEAMS_BIBLE_ARRAYS not set")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(files) == 0 {
		t.Skipf("no *.json in %s", dir)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			res, err := Read(f)
			if err != nil {
				t.Fatal(err)
			}
			books := map[int]bool{}
			brackets := 0
			for _, v := range res.Verses {
				books[v.Book] = true
				brackets += strings.Count(v.Text, "[")
			}
			t.Logf("%s (%s, %s): %d books, %d verses, %d bracketed spans, %d notes", res.Bible.Abbrev, res.Bible.Name,
				res.Bible.Language, len(books), len(res.Verses), brackets, len(res.Warnings))
			for _, w := range res.Warnings {
				t.Logf("  note: %s", w)
			}
			if len(books) != 66 || len(res.Verses) < 30000 || len(res.Verses) > 31200 {
				t.Errorf("%d books, %d verses: not a whole Bible", len(books), len(res.Verses))
			}
			if res.Bible.Name == "" {
				t.Errorf("no translation name for %s", res.Bible.Abbrev)
			}
			if got := find(res.Verses, 43, 3, 16); !strings.Contains(got, "loved") {
				t.Errorf("John 3:16 = %q", got)
			}
			for _, v := range res.Verses {
				if gluedLordRE.MatchString(v.Text) && !strings.Contains(v.Text, "Lordship") {
					t.Errorf("%d %d:%d still glued: %q", v.Book, v.Chapter, v.Verse, v.Text)
				}
				if crossRefRE.MatchString(v.Text) {
					t.Errorf("%d %d:%d still has a cross-reference: %q", v.Book, v.Chapter, v.Verse, v.Text)
				}
			}
		})
	}
}
