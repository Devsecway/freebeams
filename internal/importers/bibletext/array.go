package bibletext

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"freeworship/internal/models"
)

// The array layout is the form of github.com/thiagobodruk/bible and the
// many "en_kjv.json" files derived from it: a JSON array of books, each
// {"abbrev": "gn", "name": "Gênesis", "chapters": [["verse 1", "verse 2"], …]},
// in canonical order, verses positional (no numbers). The file has no
// header: the abbreviation and language come from the file name
// ("en_esv.json" → ESV, "en") and the full name from KnownTranslations. The
// book abbreviations and names are Portuguese in the original repository
// ("jn" is Jonah there and "jo" John), so a whole Bible (66 entries)
// resolves its books by position; a partial file by name, then abbreviation.

// arrayBook is one book of the array layout.
type arrayBook struct {
	Abbrev   string            `json:"abbrev"`
	Name     string            `json:"name"`
	Chapters []json.RawMessage `json:"chapters"`
}

var utf8BOM = []byte("\xef\xbb\xbf")

// isArrayJSON reports whether data is a JSON array.
func isArrayJSON(data []byte) bool {
	data = bytes.TrimLeft(data, " \t\r\n")
	return len(data) > 0 && data[0] == '['
}

// decodeArray reads the array layout; filename names the translation.
func decodeArray(data []byte, filename string) (Result, error) {
	var books []arrayBook
	if err := json.Unmarshal(data, &books); err != nil {
		return Result{}, fmt.Errorf("decode bible: %w", err)
	}
	if len(books) == 0 {
		return Result{}, errors.New("decode bible: no books")
	}
	abbrev, language := TranslationFromFilename(filename)
	if abbrev == "" {
		return Result{}, errors.New(`decode bible: the file has no "abbrev"; name it <ABBREV>.json or <language>_<abbrev>.json (en_kjv.json)`)
	}
	r := newReader()
	r.clean = CleanArrayText
	positional := len(books) == len(models.Books)
	for i, ab := range books {
		book, ok := resolveArrayBook(ab, i, positional)
		if !ok {
			r.warn("book %d %q: unknown book, skipped", i+1, arrayBookLabel(ab))
			continue
		}
		name, _ := models.BookByNumber(book)
		for ci, raw := range ab.Chapters {
			var verses []json.RawMessage
			if err := json.Unmarshal(raw, &verses); err != nil {
				r.warn("%s %d: not an array, skipped", name.Name, ci+1)
				continue
			}
			members := make([]member, len(verses))
			for vi, v := range verses {
				members[vi] = member{strconv.Itoa(vi + 1), v}
			}
			r.addChapter(members, book, ci+1, name.Name+" "+strconv.Itoa(ci+1))
		}
	}
	b := models.Bible{Abbrev: abbrev, Language: language}
	if k, ok := KnownTranslations[abbrev]; ok {
		b.Name = k.Name
		if b.Language == "" {
			b.Language = k.Language
		}
	}
	if models.IsPublicDomainBible(abbrev, b.Name) {
		b.Copyright = "Public Domain"
	}
	return r.result(b)
}

// resolveArrayBook maps the i-th book of the array to its canonical number:
// by position in a whole Bible, otherwise by its name, then its abbreviation.
func resolveArrayBook(ab arrayBook, i int, positional bool) (int, bool) {
	if positional {
		return i + 1, true
	}
	for _, key := range []string{ab.Name, ab.Abbrev} {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if b, err := models.FindBook(key); err == nil {
			return b.Number, true
		}
	}
	return 0, false
}

func arrayBookLabel(ab arrayBook) string {
	if n := strings.TrimSpace(ab.Name); n != "" {
		return n
	}
	return strings.TrimSpace(ab.Abbrev)
}

var (
	languageRE = regexp.MustCompile(`^[a-z]{2,3}(?:-[A-Za-z]{2,4})?$`)
	abbrevRE   = regexp.MustCompile(`^[A-Z0-9-]*[A-Z][A-Z0-9-]*$`)
)

// TranslationFromFilename derives a header-less file's translation from its
// name: "en_kjv.json" → ("KJV", "en"), "pt-br_nvi.json" → ("NVI", "pt-br"),
// "NKJV.json" → ("NKJV", ""). The abbreviation is upper-cased; a name that
// gives none (empty, or without a letter) returns "".
func TranslationFromFilename(name string) (abbrev, language string) {
	if strings.TrimSpace(name) == "" {
		return "", ""
	}
	stem := filepath.Base(name)
	stem = strings.TrimSuffix(stem, filepath.Ext(stem))
	if lang, rest, ok := strings.Cut(stem, "_"); ok && languageRE.MatchString(lang) && rest != "" {
		language, stem = strings.ToLower(lang), rest
	}
	abbrev = strings.ToUpper(strings.Join(strings.Fields(stem), ""))
	if !abbrevRE.MatchString(abbrev) {
		return "", ""
	}
	return abbrev, language
}

var (
	// crossRefRE is a bracketed scripture reference ("[Heb 11:3]",
	// "[Ex 17:6; Num 20:11]", "[Num 35:10ff]"): the Amplified Bible's inline
	// cross-references, which print as footnotes.
	crossRefRE = regexp.MustCompile(`\s*\[(?:[1-3] ?)?[A-Z][a-z]{1,6}\.? ?\d+[^\[\]]*\]`)
	// gluedLordRE is "Lord" run into the next word ("Lordis"): the scrape
	// dropped the space after a small-caps LORD.
	gluedLordRE  = regexp.MustCompile(`\b(?:Lord|LORD)[a-z]+`)
	parenSpaceRE = regexp.MustCompile(`\(\s+`)
	dashRunRE    = regexp.MustCompile(`\s*-{3,}\s*`)
)

// lordWords are real words gluedLordRE must leave alone.
var lordWords = map[string]bool{"Lordship": true, "Lordly": true, "Lords": true}

// CleanArrayText tidies a verse of the array layout, whose files are web
// scrapes: bracketed cross-references are removed (other brackets stay; the
// Amplified Bible's amplifications are its text), "( Elohim)" closes up,
// "Lordis" becomes "Lord is", rows of dashes go, whitespace is collapsed and
// a chapter heading leaked into verse 1 is dropped.
func CleanArrayText(s string, chapter, verse int) string {
	s = crossRefRE.ReplaceAllString(s, "")
	s = parenSpaceRE.ReplaceAllString(s, "(")
	s = dashRunRE.ReplaceAllString(s, " ")
	s = gluedLordRE.ReplaceAllStringFunc(s, func(m string) string {
		if lordWords[m] {
			return m
		}
		return m[:4] + " " + m[4:]
	})
	s = strings.Join(strings.Fields(s), " ")
	return dropLeakedHeading(s, chapter, verse)
}
