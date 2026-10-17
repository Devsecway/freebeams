package models

import "errors"

// Bible is an installed translation.
type Bible struct {
	ID        int64  `json:"id"`
	Abbrev    string `json:"abbrev"` // "KJV"
	Name      string `json:"name"`   // "King James Version"
	Language  string `json:"language"`
	Copyright string `json:"copyright"`
}

// Book is a canonical Bible book (Protestant 66-book canon, 1-based order).
type Book struct {
	Number  int      // 1 = Genesis ... 66 = Revelation
	Name    string   // "Genesis"
	Abbrev  string   // "Gen"
	Aliases []string // lower-case alternative spellings / abbreviations used by ParseReference
}

// VerseRange is an inclusive range within one chapter. VerseEnd == 0 together
// with VerseStart == 0 means "the whole chapter".
type VerseRange struct {
	Chapter    int `json:"chapter"`
	VerseStart int `json:"verseStart"`
	VerseEnd   int `json:"verseEnd"`
}

// Passage is a parsed scripture reference: one book, one or more ranges.
// "John 3:16-18,20" -> Book 43, Ranges [{3,16,18},{3,20,20}].
// "John 3:36-4:2" spans chapters and is represented as {3,36,0 (= to end of chapter)} + {4,1,2};
// VerseEnd == 0 with VerseStart > 0 means "to the end of the chapter".
type Passage struct {
	Book   int          `json:"book"`
	Ranges []VerseRange `json:"ranges"`
}

// Verse is a single verse of text.
type Verse struct {
	Book    int    `json:"book"`
	Chapter int    `json:"chapter"`
	Verse   int    `json:"verse"`
	Text    string `json:"text"`
}

// ErrBadReference is returned (wrapped) by ParseReference for unparseable input.
var ErrBadReference = errors.New("bad scripture reference")

// ScriptureSlideMode selects how verses are laid out on slides.
type ScriptureSlideMode string

const (
	// OneVersePerSlide puts each verse on its own slide (Phase 1 default).
	OneVersePerSlide ScriptureSlideMode = "verse"
)
