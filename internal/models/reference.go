package models

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Scripture reference grammar
//
//	reference := book [ items ]
//	items     := item { (',' | ';') item }
//	item      := point [ '-' point ] [ "ff" ]
//	point     := N [ ':' N ]
//
// Input is case-insensitive and whitespace-tolerant. "." and the words "v",
// "vv", "ver", "verse", "verses" are read as ':' ("John 3.16", "John 3 v 16"),
// and a space between two numbers is read as ':' ("John 3 16"). En/em dashes
// are read as '-'. Periods after book abbreviations are ignored ("Gen. 1:1").
//
// A bare number N is a verse when a previous item in the same ','-list named a
// verse ("John 3:16, 18"), and a chapter otherwise ("Ps 23", "Ps 1-3"). ';'
// starts a new chapter context ("John 3:16; 4" = 3:16 plus all of chapter 4).
// In single-chapter books (Obadiah, Philemon, 2 John, 3 John, Jude) bare
// numbers are verses ("Jude 3" = Jude 1:3) and the book alone means the whole
// book; every other book needs a chapter. "ff" means "to the end of the
// chapter" ("John 3:16ff").
//
// Chapters are checked against the book's chapter count; verse numbers must be
// 1..MaxVerse (verse counts per chapter are not known here).

// MaxVerse is the highest verse number ParseReference accepts (Psalm 119:176).
const MaxVerse = 176

// refToken is one lexical token of the chapter/verse part of a reference.
type refToken struct {
	kind byte // 'n' number, ':' '-' ',' ';', 'f' ("ff")
	n    int
}

// badRef returns an error wrapping ErrBadReference.
func badRef(input, format string, args ...any) error {
	return fmt.Errorf("%w %q: %s", ErrBadReference, input, fmt.Sprintf(format, args...))
}

// ParseReference parses a human reference such as "jn 3:16", "John 3:16-18,20",
// "1 Cor 13", "1co13:4-7", "Ps 23:1-3", "Song of Solomon 2:1", "rev 22:21",
// "John 3:36-4:2". Book matching is case-insensitive, by name, abbreviation,
// alias or unambiguous prefix. Errors wrap ErrBadReference.
//
// See the "Scripture reference grammar" comment in reference.go for details.
// Ranges are returned in input order and are not merged.
func ParseReference(s string) (Passage, error) {
	input := s
	s = strings.TrimSpace(normaliseDashes(s))
	s = strings.TrimRight(s, ". ")
	if s == "" {
		return Passage{}, badRef(input, "empty reference")
	}

	bookPart, rest := splitBook(s)
	book, err := FindBook(bookPart)
	if err != nil {
		return Passage{}, fmt.Errorf("parse reference %q: %w", input, err)
	}
	p := Passage{Book: book.Number}
	single := ChapterCount(book.Number) == 1

	toks, err := lexReference(input, rest)
	if err != nil {
		return Passage{}, err
	}
	if len(toks) == 0 {
		if single {
			p.Ranges = []VerseRange{{Chapter: 1}}
			return p, nil
		}
		return Passage{}, badRef(input, "missing chapter")
	}

	ps := refParser{input: input, toks: toks, book: book.Number, single: single}
	ranges, err := ps.parse()
	if err != nil {
		return Passage{}, err
	}
	p.Ranges = ranges
	return p, nil
}

// splitBook splits "1 Cor 13:4" / "1co13:4" into the book part and the rest:
// an optional leading ordinal digit, then everything up to the first digit.
func splitBook(s string) (book, rest string) {
	i := 0
	if len(s) > 1 && s[0] >= '1' && s[0] <= '3' && !isDigit(s[1]) {
		i = 1
	}
	for i < len(s) && !isDigit(s[i]) {
		i++
	}
	return s[:i], s[i:]
}

// normaliseDashes maps Unicode dashes and spaces to '-' and ' '.
func normaliseDashes(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2015', '\u2212', '\uFE58', '\uFE63', '\uFF0D':
			return '-'
		}
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, s)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// lexReference tokenises the chapter/verse part.
func lexReference(input, s string) ([]refToken, error) {
	var toks []refToken
	lastNum := false   // previous token is a number
	pendingSp := false // whitespace seen since the previous token
	emit := func(t refToken) {
		// Collapse "3: v 16" -> "3:16".
		if t.kind == ':' && len(toks) > 0 && toks[len(toks)-1].kind == ':' {
			return
		}
		toks = append(toks, t)
		lastNum = t.kind == 'n'
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ':
			pendingSp = true
			i++
			continue
		case isDigit(c):
			j := i
			for j < len(s) && isDigit(s[j]) {
				j++
			}
			if j-i > 4 {
				return nil, badRef(input, "number too large")
			}
			n, _ := strconv.Atoi(s[i:j])
			if lastNum && pendingSp {
				emit(refToken{kind: ':'})
			}
			emit(refToken{kind: 'n', n: n})
			i = j
		case c == ':' || c == '.':
			emit(refToken{kind: ':'})
			i++
		case c == '-' || c == ',' || c == ';':
			emit(refToken{kind: c})
			i++
		case c < 0x80 && unicode.IsLetter(rune(c)):
			j := i
			for j < len(s) && s[j] < 0x80 && unicode.IsLetter(rune(s[j])) {
				j++
			}
			switch strings.ToLower(s[i:j]) {
			case "v", "vv", "ver", "vs", "verse", "verses":
				emit(refToken{kind: ':'})
			case "ff":
				emit(refToken{kind: 'f'})
			default:
				return nil, badRef(input, "unexpected %q", s[i:j])
			}
			i = j
		default:
			return nil, badRef(input, "unexpected character %q", s[i:])
		}
		pendingSp = false
	}
	return toks, nil
}

// refParser turns tokens into verse ranges.
type refParser struct {
	input  string
	toks   []refToken
	pos    int
	book   int
	single bool // single-chapter book

	verseCtx bool // a bare number is a verse in chapter cur
	cur      int
}

// point is a resolved position: a verse, or a whole chapter when v == 0.
type point struct {
	ch, v int
}

func (ps *refParser) peek() byte {
	if ps.pos < len(ps.toks) {
		return ps.toks[ps.pos].kind
	}
	return 0
}

func (ps *refParser) number() (int, error) {
	if ps.peek() != 'n' {
		return 0, badRef(ps.input, "expected a number")
	}
	n := ps.toks[ps.pos].n
	ps.pos++
	return n, nil
}

func (ps *refParser) parse() ([]VerseRange, error) {
	var out []VerseRange
	for {
		rs, err := ps.item()
		if err != nil {
			return nil, err
		}
		out = append(out, rs...)
		switch ps.peek() {
		case 0:
			return out, nil
		case ',':
		case ';':
			ps.verseCtx = false
		default:
			return nil, badRef(ps.input, "unexpected %q", string(ps.peek()))
		}
		ps.pos++
	}
}

// rawPoint reads "N" or "N:M"; explicit reports whether a ':' was present.
func (ps *refParser) rawPoint() (a, b int, explicit bool, err error) {
	if a, err = ps.number(); err != nil {
		return
	}
	if ps.peek() == ':' {
		ps.pos++
		if b, err = ps.number(); err != nil {
			return
		}
		explicit = true
	}
	return
}

// item parses one comma/semicolon separated element.
func (ps *refParser) item() ([]VerseRange, error) {
	a, b, explicit, err := ps.rawPoint()
	if err != nil {
		return nil, err
	}
	var start point
	switch {
	case explicit:
		start, err = ps.verse(a, b)
	case ps.verseCtx:
		start, err = ps.verse(ps.cur, a)
	case ps.single:
		start, err = ps.verse(1, a)
	default:
		start, err = ps.chapter(a)
	}
	if err != nil {
		return nil, err
	}

	end := start
	hasEnd := false
	if ps.peek() == '-' {
		ps.pos++
		a, b, explicit, err := ps.rawPoint()
		if err != nil {
			return nil, err
		}
		switch {
		case explicit:
			end, err = ps.verse(a, b)
		case start.v > 0:
			end, err = ps.verse(start.ch, a)
		default:
			end, err = ps.chapter(a)
		}
		if err != nil {
			return nil, err
		}
		hasEnd = true
	}
	ff := false
	if ps.peek() == 'f' {
		if hasEnd || start.v == 0 {
			return nil, badRef(ps.input, `"ff" must follow a single verse`)
		}
		ps.pos++
		ff = true
	}

	var out []VerseRange
	switch {
	case ff:
		out = []VerseRange{{start.ch, start.v, 0}}
	case !hasEnd:
		out = []VerseRange{{start.ch, start.v, start.v}}
	case end.ch < start.ch || (end.ch == start.ch && start.v > 0 && end.v < start.v):
		return nil, badRef(ps.input, "range ends before it starts")
	case start.v == 0 && end.v == 0: // chapters c-d
		for c := start.ch; c <= end.ch; c++ {
			out = append(out, VerseRange{Chapter: c})
		}
	case end.ch == start.ch: // within one chapter ("3:16-18"; "3-3:5" starts at verse 1)
		out = []VerseRange{{start.ch, max(start.v, 1), end.v}}
	default: // across chapters
		if start.v == 0 {
			out = append(out, VerseRange{Chapter: start.ch})
		} else {
			out = append(out, VerseRange{start.ch, start.v, 0})
		}
		for c := start.ch + 1; c < end.ch; c++ {
			out = append(out, VerseRange{Chapter: c})
		}
		// end is a verse here: chapter-to-chapter ranges were handled above.
		out = append(out, VerseRange{end.ch, 1, end.v})
	}

	ps.verseCtx = end.v > 0
	ps.cur = end.ch
	return out, nil
}

// chapter validates a whole-chapter position.
func (ps *refParser) chapter(c int) (point, error) {
	if n := ChapterCount(ps.book); c < 1 || c > n {
		return point{}, badRef(ps.input, "chapter %d out of range (1-%d)", c, n)
	}
	return point{c, 0}, nil
}

// verse validates a verse position.
func (ps *refParser) verse(c, v int) (point, error) {
	if _, err := ps.chapter(c); err != nil {
		return point{}, err
	}
	if v < 1 || v > MaxVerse {
		return point{}, badRef(ps.input, "verse %d out of range (1-%d)", v, MaxVerse)
	}
	return point{c, v}, nil
}

// span is a contiguous run from (c1, v1) to (c2, v2); v1 == 0 is the start of
// chapter c1 and v2 == 0 the end of chapter c2.
type span struct{ c1, v1, c2, v2 int }

// spans converts ranges into spans, merging a range that runs to the end of a
// chapter with a following range that starts at the beginning of the next.
func spans(ranges []VerseRange) []span {
	var out []span
	for _, r := range ranges {
		s := span{r.Chapter, r.VerseStart, r.Chapter, r.VerseEnd}
		if s.v1 == 0 && s.v2 > 0 {
			s.v1 = 1
		}
		if n := len(out); n > 0 {
			last := &out[n-1]
			// B must start the chapter: a whole chapter, or 1-N ({c,1,0} "1ff"
			// is kept separate so it prints back the way it parsed).
			if last.v2 == 0 && s.c1 == last.c2+1 && (s.v1 == 0 || (s.v1 == 1 && s.v2 > 0)) {
				last.c2, last.v2 = s.c2, s.v2
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

// String formats a passage canonically: "John 3:16-18, 20", "1 Corinthians 13", "John 3:36-4:2".
//
// Items in the same chapter are joined with ", ", a new chapter starts with
// "; " ("John 3:16; 4:2"). Consecutive whole chapters print as a range
// ("Psalms 1-3"), a range to the end of a chapter as "ff" ("John 3:16ff"), and a
// single-chapter book with no ranges or only its whole chapter prints as the
// book name alone ("Jude"). ParseReference(p.String()) returns p for any
// passage ParseReference produced.
func (p Passage) String() string {
	name := fmt.Sprintf("Book %d", p.Book)
	if b, ok := BookByNumber(p.Book); ok {
		name = b.Name
	}
	ss := spans(p.Ranges)
	if len(ss) == 0 || (ChapterCount(p.Book) == 1 && len(ss) == 1 && ss[0] == span{1, 0, 1, 0}) {
		return name
	}

	var b strings.Builder
	b.WriteString(name)
	verseCtx, cur := false, 0
	for i, s := range ss {
		sameChapter := verseCtx && s.v1 > 0 && s.c1 == cur
		switch {
		case i == 0:
			b.WriteString(" ")
		case sameChapter:
			b.WriteString(", ")
		default:
			b.WriteString("; ")
		}
		switch {
		case s.v1 == 0 && s.v2 == 0: // whole chapters
			b.WriteString(strconv.Itoa(s.c1))
			if s.c2 != s.c1 {
				b.WriteString("-" + strconv.Itoa(s.c2))
			}
			verseCtx = false
		case s.v1 == 0: // chapter start to c2:v2
			fmt.Fprintf(&b, "%d-%d:%d", s.c1, s.c2, s.v2)
			verseCtx = true
		default:
			if !sameChapter {
				b.WriteString(strconv.Itoa(s.c1) + ":")
			}
			b.WriteString(strconv.Itoa(s.v1))
			switch {
			case s.c2 == s.c1 && s.v2 == s.v1:
				verseCtx = true
			case s.c2 == s.c1 && s.v2 == 0:
				b.WriteString("ff")
				verseCtx = true
			case s.c2 == s.c1:
				b.WriteString("-" + strconv.Itoa(s.v2))
				verseCtx = true
			case s.v2 > 0:
				fmt.Fprintf(&b, "-%d:%d", s.c2, s.v2)
				verseCtx = true
			default: // c1:v1 to the end of chapter c2: "3:16ff; 4-5"
				b.WriteString("ff; " + strconv.Itoa(s.c1+1))
				if s.c2 > s.c1+1 {
					b.WriteString("-" + strconv.Itoa(s.c2))
				}
				verseCtx = false
			}
		}
		cur = s.c2
	}
	return b.String()
}

// verseRef formats "John 3:16" for a single verse.
func verseRef(book, chapter, verse int) string {
	name := fmt.Sprintf("Book %d", book)
	if b, ok := BookByNumber(book); ok {
		name = b.Name
	}
	return fmt.Sprintf("%s %d:%d", name, chapter, verse)
}

// ScriptureSlides makes one slide per verse: Kind=SlideScripture,
// Label "John 3:16", Text = verse text, Footer = "John 3:16 (KJV)".
//
// Verses are used in the given order (the caller looks them up for p). A
// verse's Book defaults to p.Book when 0. Whitespace in the text is collapsed
// to single spaces, verses with no text are skipped, and the footer omits
// " (abbrev)" when bibleAbbrev is empty. OneVersePerSlide is the only mode;
// any other value is treated as OneVersePerSlide.
func ScriptureSlides(p Passage, verses []Verse, bibleAbbrev string, mode ScriptureSlideMode) []Slide {
	_ = mode // only OneVersePerSlide exists so far
	abbrev := strings.TrimSpace(bibleAbbrev)
	var slides []Slide
	for _, v := range verses {
		text := strings.Join(strings.Fields(v.Text), " ")
		if text == "" {
			continue
		}
		book := v.Book
		if book == 0 {
			book = p.Book
		}
		label := verseRef(book, v.Chapter, v.Verse)
		footer := label
		if abbrev != "" {
			footer += " (" + abbrev + ")"
		}
		slides = append(slides, Slide{Kind: SlideScripture, Label: label, Text: text, Footer: footer})
	}
	return slides
}
