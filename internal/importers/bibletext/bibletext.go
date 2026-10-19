// Package bibletext reads Bible translations from JSON text files and writes
// FreeBeams's single-file Bible JSON. It is pure Go (no Godot).
//
// Three layouts are read (see docs/BIBLES.md):
//
//   - A translation folder in the layout of
//     github.com/hargarpay/bible-translations:
//     <ABBREV>/{old,new}/<Book_Name>/<chapter>.json, where each chapter file is
//     an object of verse number → text ({"1": "In the beginning…", "2": "…"}).
//     The abbreviation is the folder name. Any chapter file inside such a
//     folder may be given instead of the folder (ReadFile finds the root).
//   - A single-file Bible: {"abbrev": "KJV", "name": "…", "language": "en",
//     "copyright": "Public Domain", "books": {"Genesis": {"1": {"1": "…"}}}}.
//     Book keys are names, abbreviations or canonical numbers ("1".."66").
//     The bundled translations ship in this form, gzip-compressed (Encode
//     writes it).
//   - The array layout of github.com/thiagobodruk/bible (array.go): a JSON
//     array of 66 books, each {"abbrev", "name", "chapters": [["…"], …]},
//     verses positional, no header. The file name gives the abbreviation
//     ("en_esv.json" → ESV) and the text is cleaned by CleanArrayText.
//
// Book names resolve through models.FindBook ("Psalm", "Song_of_Solomon",
// "1 Cor") and verses come out in canonical order. Text is cleaned by
// CleanText. A verse key "N-M" (verses the translation combines) stores the
// text under verse N. Malformed entries become Result.Warnings, never errors
// or panics; only an unreadable file, a folder that is not a translation, or
// a Bible with no verses at all is an error.
package bibletext

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"freebeams/internal/models"
)

// Result is a decoded translation.
type Result struct {
	Bible    models.Bible
	Verses   []models.Verse // canonical order (book, chapter, verse)
	Warnings []string       // malformed entries that were skipped (at most maxWarnings + 1)
}

// maxWarnings bounds Result.Warnings; a final entry counts the rest.
const maxWarnings = 50

// maxFileSize bounds one JSON file (a whole Bible is about 4.5 MB).
const maxFileSize = 256 << 20

// maxRef is the largest chapter or verse number the store encodes (8 bits).
const maxRef = 255

var (
	chapterFileRE = regexp.MustCompile(`^(\d{1,3})\.json$`)
	verseKeyRE    = regexp.MustCompile(`^(\d{1,3})(?:\s*-\s*\d{1,3})?$`)
	footnoteRE    = regexp.MustCompile(`\[[a-z]{1,2}\]`)
	bracketRE     = strings.NewReplacer("[", "", "]", "")
)

// Read reads a translation from path: a translation folder, a chapter file
// inside one, or a single-file Bible JSON.
func Read(path string) (Result, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return Result{}, fmt.Errorf("read bible: %w", err)
	}
	if fi.IsDir() {
		return ReadDir(path)
	}
	return ReadFile(path)
}

// ReadFile reads a single-file Bible JSON, or, for a chapter file inside a
// translation folder (<root>/{old,new}/<Book>/<n>.json), that whole folder.
func ReadFile(path string) (Result, error) {
	if root, ok := translationRoot(path); ok {
		return ReadDir(root)
	}
	f, err := os.Open(path)
	if err != nil {
		return Result{}, fmt.Errorf("read bible: %w", err)
	}
	defer f.Close()
	res, err := DecodeNamed(f, filepath.Base(path))
	if err != nil {
		return Result{}, fmt.Errorf("read bible %s: %w", filepath.Base(path), err)
	}
	return res, nil
}

// translationRoot returns the translation folder of a chapter file path
// (<root>/{old,new}/<Book>/<n>.json).
func translationRoot(path string) (string, bool) {
	if !chapterFileRE.MatchString(filepath.Base(path)) {
		return "", false
	}
	book := filepath.Dir(path)
	testament := filepath.Dir(book)
	switch strings.ToLower(filepath.Base(testament)) {
	case "old", "new":
	default:
		return "", false
	}
	root := filepath.Dir(testament)
	if root == testament || filepath.Base(root) == "" {
		return "", false
	}
	return root, true
}

// ReadDir reads a translation folder (<ABBREV>/{old,new}/<Book>/<n>.json). The
// abbreviation is the folder's name (upper-cased). Known translations get
// their full name (KnownTranslations) and public-domain ones the copyright
// "Public Domain".
func ReadDir(dir string) (Result, error) {
	dir = filepath.Clean(dir)
	abbrev := strings.ToUpper(filepath.Base(dir))
	r := newReader()
	found := 0
	for _, testament := range []string{"old", "new"} {
		tdir := filepath.Join(dir, testament)
		entries, err := os.ReadDir(tdir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return Result{}, fmt.Errorf("read bible folder %s: %w", dir, err)
		}
		found++
		for _, e := range entries {
			if hidden(e.Name()) {
				continue
			}
			if !e.IsDir() {
				r.warn("%s/%s: not a book folder, skipped", testament, e.Name())
				continue
			}
			book, err := models.FindBook(strings.ReplaceAll(e.Name(), "_", " "))
			if err != nil {
				r.warn("%s/%s: unknown book, skipped", testament, e.Name())
				continue
			}
			if err := r.readBookDir(filepath.Join(tdir, e.Name()), book.Number, testament+"/"+e.Name()); err != nil {
				return Result{}, fmt.Errorf("read bible folder %s: %w", dir, err)
			}
		}
	}
	if found == 0 {
		return Result{}, fmt.Errorf("read bible folder %s: not a translation folder (no old/ or new/ subfolder)", dir)
	}
	b := models.Bible{Abbrev: abbrev}
	if k, ok := KnownTranslations[abbrev]; ok {
		b.Name, b.Language = k.Name, k.Language
	}
	if models.IsPublicDomainBible(abbrev, b.Name) {
		b.Copyright = "Public Domain"
	}
	return r.result(b)
}

func hidden(name string) bool { return strings.HasPrefix(name, ".") }

// readBookDir reads the chapter files of one book folder.
func (r *reader) readBookDir(dir string, book int, where string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if hidden(e.Name()) || e.IsDir() {
			continue
		}
		m := chapterFileRE.FindStringSubmatch(e.Name())
		if m == nil {
			r.warn("%s/%s: not a chapter file, skipped", where, e.Name())
			continue
		}
		chapter, _ := strconv.Atoi(m[1])
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			r.warn("%s/%s: %v", where, e.Name(), err)
			continue
		}
		raw, err := objectMembers(data)
		if err != nil {
			r.warn("%s/%s: %v", where, e.Name(), err)
			continue
		}
		r.addChapter(raw, book, chapter, where+"/"+e.Name())
	}
	return nil
}

// document is the single-file Bible JSON.
type document struct {
	Abbrev    string          `json:"abbrev"`
	Name      string          `json:"name,omitempty"`
	Language  string          `json:"language,omitempty"`
	Copyright string          `json:"copyright,omitempty"`
	Books     json.RawMessage `json:"books"`
}

// Decode reads a single-file Bible JSON in the object layout (the array
// layout needs the file name: DecodeNamed).
func Decode(rd io.Reader) (Result, error) { return DecodeNamed(rd, "") }

// DecodeNamed reads a single-file Bible JSON in either layout. filename is
// the file's base name ("" when unknown); the array layout takes its
// abbreviation and language from it (TranslationFromFilename).
func DecodeNamed(rd io.Reader, filename string) (Result, error) {
	data, err := io.ReadAll(io.LimitReader(rd, maxFileSize+1))
	if err != nil {
		return Result{}, fmt.Errorf("decode bible: %w", err)
	}
	if len(data) > maxFileSize {
		return Result{}, errors.New("decode bible: file too large")
	}
	data = bytes.TrimPrefix(data, utf8BOM)
	if isArrayJSON(data) {
		return decodeArray(data, filename)
	}
	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		return Result{}, fmt.Errorf("decode bible: %w", err)
	}
	abbrev := strings.TrimSpace(doc.Abbrev)
	if abbrev == "" {
		return Result{}, errors.New(`decode bible: "abbrev" is required`)
	}
	if len(doc.Books) == 0 || string(doc.Books) == "null" {
		return Result{}, errors.New(`decode bible: no "books"`)
	}
	books, err := objectMembers(doc.Books)
	if err != nil {
		return Result{}, fmt.Errorf(`decode bible: "books": %w`, err)
	}
	r := newReader()
	for _, bm := range books {
		book, ok := resolveBook(bm.key)
		if !ok {
			r.warn("book %q: unknown book, skipped", bm.key)
			continue
		}
		chapters, err := objectMembers(bm.val)
		if err != nil {
			r.warn("book %q: %v", bm.key, err)
			continue
		}
		for _, cm := range chapters {
			chapter, err := strconv.Atoi(strings.TrimSpace(cm.key))
			if err != nil || chapter < 1 {
				r.warn("%s: bad chapter %q, skipped", bm.key, cm.key)
				continue
			}
			verses, err := objectMembers(cm.val)
			if err != nil {
				r.warn("%s %s: %v", bm.key, cm.key, err)
				continue
			}
			r.addChapter(verses, book, chapter, bm.key+" "+cm.key)
		}
	}
	b := models.Bible{
		Abbrev:    abbrev,
		Name:      strings.Join(strings.Fields(doc.Name), " "),
		Language:  strings.ToLower(strings.TrimSpace(doc.Language)),
		Copyright: strings.TrimSpace(doc.Copyright),
	}
	if b.Copyright == "" && models.IsPublicDomainBible(abbrev, b.Name) {
		b.Copyright = "Public Domain"
	}
	return r.result(b)
}

// resolveBook maps a book key (canonical number or a name models.FindBook
// knows) to its number.
func resolveBook(key string) (int, bool) {
	key = strings.TrimSpace(key)
	if n, err := strconv.Atoi(key); err == nil {
		_, ok := models.BookByNumber(n)
		return n, ok
	}
	b, err := models.FindBook(strings.ReplaceAll(key, "_", " "))
	if err != nil {
		return 0, false
	}
	return b.Number, true
}

// member is one key/value pair of a JSON object, in document order.
type member struct {
	key string
	val json.RawMessage
}

// objectMembers decodes a JSON object into its members in document order
// (encoding/json maps would lose it, and "first wins" for duplicates should
// mean first in the file).
func objectMembers(raw []byte) ([]member, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if tok != json.Delim('{') {
		return nil, errors.New("not an object")
	}
	var out []member
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := kt.(string)
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return nil, err
		}
		out = append(out, member{key, val})
	}
	if _, err := dec.Token(); err != nil { // the closing brace
		return nil, err
	}
	return out, nil
}

// reader accumulates verses and warnings.
type reader struct {
	verses   map[[3]int]models.Verse
	warnings []string
	dropped  int

	clean func(s string, chapter, verse int) string // the layout's text cleaning
}

func newReader() *reader { return &reader{verses: map[[3]int]models.Verse{}, clean: CleanText} }

func (r *reader) warn(format string, args ...any) {
	if len(r.warnings) >= maxWarnings {
		r.dropped++
		return
	}
	r.warnings = append(r.warnings, fmt.Sprintf(format, args...))
}

// addChapter adds the verses of one chapter object (verse key → JSON string).
func (r *reader) addChapter(raw []member, book, chapter int, where string) {
	if n := models.ChapterCount(book); chapter < 1 || chapter > n || chapter > maxRef {
		r.warn("%s: chapter %d out of range (1-%d), skipped", where, chapter, n)
		return
	}
	for _, vm := range raw {
		k := vm.key
		m := verseKeyRE.FindStringSubmatch(strings.TrimSpace(k))
		if m == nil {
			r.warn("%s: bad verse key %q, skipped", where, k)
			continue
		}
		verse, _ := strconv.Atoi(m[1])
		if verse < 1 || verse > maxRef {
			r.warn("%s: verse %d out of range (1-%d), skipped", where, verse, maxRef)
			continue
		}
		var text string
		if err := json.Unmarshal(vm.val, &text); err != nil {
			r.warn("%s verse %s: not a string, skipped", where, k)
			continue
		}
		text = r.clean(text, chapter, verse)
		if text == "" {
			r.warn("%s verse %s: empty, skipped", where, k)
			continue
		}
		key := [3]int{book, chapter, verse}
		if _, dup := r.verses[key]; dup {
			r.warn("%s verse %s: duplicate of %d %d:%d, skipped", where, k, book, chapter, verse)
			continue
		}
		r.verses[key] = models.Verse{Book: book, Chapter: chapter, Verse: verse, Text: text}
	}
}

// result sorts the verses canonically and finishes the warnings.
func (r *reader) result(b models.Bible) (Result, error) {
	if len(r.verses) == 0 {
		return Result{}, fmt.Errorf("bible %s: no verses", b.Abbrev)
	}
	verses := make([]models.Verse, 0, len(r.verses))
	for _, v := range r.verses {
		verses = append(verses, v)
	}
	sort.Slice(verses, func(i, j int) bool {
		a, b := verses[i], verses[j]
		if a.Book != b.Book {
			return a.Book < b.Book
		}
		if a.Chapter != b.Chapter {
			return a.Chapter < b.Chapter
		}
		return a.Verse < b.Verse
	})
	warnings := r.warnings
	if r.dropped > 0 {
		warnings = append(warnings, fmt.Sprintf("… and %d more", r.dropped))
	}
	return Result{Bible: b, Verses: verses, Warnings: warnings}, nil
}

// CleanText tidies one verse's text: footnote markers ("[a]".."[zz]") are
// removed, other brackets lose their brackets and keep their words ("[was]"
// -> "was"), whitespace (including no-break spaces) is collapsed to single
// spaces, and a chapter heading leaked into the start of verse 1 ("36 The
// transgression…" in Psalm 36) is dropped.
func CleanText(s string, chapter, verse int) string {
	s = footnoteRE.ReplaceAllString(s, "")
	s = bracketRE.Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	return dropLeakedHeading(s, chapter, verse)
}

// dropLeakedHeading removes a chapter number leaked into the start of verse
// 1 ("36 The transgression…" in Psalm 36).
func dropLeakedHeading(s string, chapter, verse int) string {
	if verse == 1 && chapter > 0 {
		if p := strconv.Itoa(chapter) + " "; strings.HasPrefix(s, p) {
			s = s[len(p):]
		}
	}
	return s
}

// Encode writes b and verses as a single-file Bible JSON (the form Decode
// reads and the bundled translations use): books keyed by their canonical
// English name, chapters and verses in numeric order, one chapter per line.
func Encode(w io.Writer, b models.Bible, verses []models.Verse) error {
	if strings.TrimSpace(b.Abbrev) == "" {
		return errors.New("encode bible: abbrev is required")
	}
	sorted := append([]models.Verse(nil), verses...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Book != b.Book {
			return a.Book < b.Book
		}
		if a.Chapter != b.Chapter {
			return a.Chapter < b.Chapter
		}
		return a.Verse < b.Verse
	})
	for _, v := range sorted {
		if _, ok := models.BookByNumber(v.Book); !ok || v.Chapter < 1 || v.Chapter > maxRef || v.Verse < 1 || v.Verse > maxRef {
			return fmt.Errorf("encode bible %s: invalid reference %d %d:%d", b.Abbrev, v.Book, v.Chapter, v.Verse)
		}
	}

	bw := bufio.NewWriter(w)
	hdr, err := json.Marshal(struct {
		Abbrev    string `json:"abbrev"`
		Name      string `json:"name,omitempty"`
		Language  string `json:"language,omitempty"`
		Copyright string `json:"copyright,omitempty"`
	}{strings.TrimSpace(b.Abbrev), b.Name, b.Language, b.Copyright})
	if err != nil {
		return fmt.Errorf("encode bible %s: %w", b.Abbrev, err)
	}
	bw.Write(hdr[:len(hdr)-1]) // reopen the object for "books"
	bw.WriteString(",\n\"books\":{")
	book, chapter := 0, 0
	for i, v := range sorted {
		if v.Book != book {
			if book != 0 {
				bw.WriteString("}}")
			}
			if i > 0 {
				bw.WriteString(",\n")
			}
			name, _ := models.BookByNumber(v.Book)
			writeJSONString(bw, name.Name)
			bw.WriteString(":{")
			book, chapter = v.Book, 0
		}
		if v.Chapter != chapter {
			if chapter != 0 {
				bw.WriteString("},\n")
			}
			bw.WriteString(`"` + strconv.Itoa(v.Chapter) + `":{`)
			chapter = v.Chapter
		} else {
			bw.WriteString(",")
		}
		bw.WriteString(`"` + strconv.Itoa(v.Verse) + `":`)
		writeJSONString(bw, v.Text)
	}
	if book != 0 {
		bw.WriteString("}}")
	}
	bw.WriteString("}}\n")
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("encode bible %s: %w", b.Abbrev, err)
	}
	return nil
}

// writeJSONString writes s as a JSON string without HTML escaping.
func writeJSONString(w *bufio.Writer, s string) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(s) // never fails for a string
	w.Write(bytes.TrimRight(buf.Bytes(), "\n"))
}
