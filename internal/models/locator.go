package models

import (
	"errors"
	"fmt"
	"strings"
)

// The scripture locator (the Scriptures tab's reference field) is more
// forgiving than ParseReference: while the operator types, the book part is
// completed from a list of the books it may name (BookCompletions,
// CompleteBook), a book alone shows its first chapter (ParseLocator), and the
// verse list always lists whole chapters (Passage.Chapters) with the verses the
// reference names selected (Passage.Contains). Selecting other verses there
// turns the selection back into a passage (SelectionPassage).

// locatorParts splits typed locator text into the book part and the
// chapter/verse rest, as ParseReference does. ordinal is true for text that is
// only a book ordinal ("1", "2 "), which ParseReference reads as a missing book.
func locatorParts(text string) (book, rest string, ordinal bool) {
	s := strings.TrimLeft(normaliseDashes(text), " ")
	book, rest = splitBook(s)
	if strings.TrimSpace(book) == "" {
		if r := strings.TrimSpace(rest); len(r) == 1 && r[0] >= '1' && r[0] <= '3' {
			return r, "", true
		}
	}
	return book, rest, false
}

// BookCompletions returns the books the locator offers for typed text, in
// canonical order (BooksMatching): every book the book part may be the start
// of while no chapter is typed yet ("joh" → John, 1 John, 2 John, 3 John; "1"
// → the eight books that start with 1), or while the book part is ambiguous
// even though a chapter follows ("jo 3"). None for an empty book part, a book
// completed with a trailing space ("John "), a chapter after an unambiguous
// book ("john 3") or text that names no book ("xyz").
func BookCompletions(text string) []Book {
	book, rest, ordinal := locatorParts(text)
	if ordinal {
		return BooksMatching(book)
	}
	if bookKey(book) == "" {
		return nil
	}
	if strings.TrimSpace(rest) != "" {
		var amb *AmbiguousBookError
		if _, err := FindBook(book); !errors.As(err, &amb) {
			return nil
		}
		return BooksMatching(book)
	}
	books := BooksMatching(book)
	if strings.HasSuffix(text, " ") {
		k := bookKey(book)
		for _, b := range books {
			if bookKey(b.Name) == k {
				return nil // completed: "John "
			}
		}
	}
	return books
}

// CompleteBook replaces the book part of typed text with b's name and keeps
// the rest: "joh" → "John " (ready for the chapter), "jo 3:16" → "John 3:16".
func CompleteBook(text string, b Book) string {
	_, rest, _ := locatorParts(text)
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return b.Name + " "
	}
	return b.Name + " " + rest
}

// ParseLocator parses the locator's text. It is ParseReference, except that a
// book alone means its first chapter ("gen" → Genesis 1, "John " → John 1; a
// single-chapter book is the whole book either way), and an ordinal alone
// ("1") is an *AmbiguousBookError like any other ambiguous book part. Errors
// wrap ErrBadReference.
func ParseLocator(text string) (Passage, error) {
	s := strings.TrimRight(strings.TrimSpace(normaliseDashes(text)), ". ")
	book, rest, ordinal := locatorParts(s)
	if ordinal {
		return Passage{}, fmt.Errorf("parse reference %q: %w", text, &AmbiguousBookError{Name: book, Books: BooksMatching(book)})
	}
	if s != "" && strings.TrimSpace(rest) == "" {
		b, err := FindBook(book)
		if err != nil {
			return Passage{}, fmt.Errorf("parse reference %q: %w", text, err)
		}
		return Passage{Book: b.Number, Ranges: []VerseRange{{Chapter: 1}}}, nil
	}
	return ParseReference(text)
}

// Contains reports whether the passage includes verse chapter:verse.
func (p Passage) Contains(chapter, verse int) bool {
	for _, r := range p.Ranges {
		if r.Chapter != chapter {
			continue
		}
		lo, hi := r.VerseStart, r.VerseEnd
		switch {
		case lo <= 0: // the whole chapter
			return true
		case hi == 0: // to the end of the chapter
			hi = MaxVerse
		case hi < lo:
			lo, hi = hi, lo
		}
		if verse >= lo && verse <= hi {
			return true
		}
	}
	return false
}

// Chapters widens the passage to whole chapters: one whole-chapter range for
// every chapter it touches, in chapter order ("John 3:16; 4:2" → John 3-4).
func (p Passage) Chapters() Passage {
	out := Passage{Book: p.Book}
	seen := map[int]bool{}
	for _, r := range p.Ranges {
		if !seen[r.Chapter] {
			seen[r.Chapter] = true
			out.Ranges = append(out.Ranges, VerseRange{Chapter: r.Chapter})
		}
	}
	// Chapter order (a reference may name them out of order: "John 4; 3").
	for i := 1; i < len(out.Ranges); i++ {
		for j := i; j > 0 && out.Ranges[j].Chapter < out.Ranges[j-1].Chapter; j-- {
			out.Ranges[j], out.Ranges[j-1] = out.Ranges[j-1], out.Ranges[j]
		}
	}
	return out
}

// SelectionPassage is the passage of the selected rows of a verse list:
// listed holds the verses of whole chapters in order (one row each), sel the
// selected row indices in ascending order. Adjacent rows make one range, so a
// verse the translation leaves out does not split it; a run that covers a
// whole listed chapter is that chapter, and a run that continues into the next
// chapter runs to the end of the first one, so the passage prints as
// "John 3:36-4:2", "Psalms 1-2" or "John 3:16-18, 20" and parses back to the
// same verses.
func SelectionPassage(book int, listed []Verse, sel []int) Passage {
	p := Passage{Book: book}
	first, last := map[int]int{}, map[int]int{} // chapter -> first / last row
	for i, v := range listed {
		if _, ok := first[v.Chapter]; !ok {
			first[v.Chapter] = i
		}
		last[v.Chapter] = i
	}
	valid := make([]int, 0, len(sel))
	for _, i := range sel {
		if i >= 0 && i < len(listed) && (len(valid) == 0 || i > valid[len(valid)-1]) {
			valid = append(valid, i)
		}
	}
	for k := 0; k < len(valid); {
		// A segment is a run of adjacent rows within one chapter.
		a := valid[k]
		b := a
		for k+1 < len(valid) && valid[k+1] == b+1 && listed[valid[k+1]].Chapter == listed[a].Chapter {
			k++
			b = valid[k]
		}
		k++
		ch := listed[a].Chapter
		continues := k < len(valid) && valid[k] == b+1 // the run goes on into the next chapter
		switch {
		case a == first[ch] && b == last[ch]:
			p.Ranges = append(p.Ranges, VerseRange{Chapter: ch})
		case continues:
			p.Ranges = append(p.Ranges, VerseRange{Chapter: ch, VerseStart: listed[a].Verse})
		default:
			p.Ranges = append(p.Ranges, VerseRange{Chapter: ch, VerseStart: listed[a].Verse, VerseEnd: listed[b].Verse})
		}
	}
	return p
}
