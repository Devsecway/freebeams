package models

// This file declares the models API that other packages build against.
// Bodies marked STUB are placeholders; the models work package replaces them
// with real implementations (keeping these exact signatures) plus tests.

// ── Songs ────────────────────────────────────────────────────────────────

// ParseLyrics parses lyric text into sections. Accepted forms:
//   - Tagged:   "[Verse 1]\nline\nline\n\n[Chorus]\nline" (tags case-insensitive,
//     also "Verse 1:" / "Chorus:" header lines, "V1", "C", "Pre-Chorus", "Bridge 2", "Tag", "Intro", "Ending"/"Outro")
//   - Untagged: blank-line separated stanzas become Verse 1, Verse 2, ...
//
// Leading/trailing whitespace is trimmed, CRLF is normalised, empty sections dropped.
func ParseLyrics(text string) []Section { return nil } // STUB

// FormatLyrics is the inverse of ParseLyrics: "[Label]\nlines" blocks joined by a
// blank line. ParseLyrics(FormatLyrics(s)) must round-trip.
func FormatLyrics(sections []Section) string { return "" } // STUB

// SectionLabel returns the display label: (verse,2) -> "Verse 2", (chorus,0) -> "Chorus",
// (prechorus,0) -> "Pre-Chorus".
func SectionLabel(kind SectionKind, number int) string { return "" } // STUB

// Slides splits the song into slides (one per section, or more when a section
// exceeds opt.MaxLines), setting Kind=SlideSong, Label, Group, GroupNumber, Text,
// and the copyright Footer per opt.
func (s Song) Slides(opt SlideOptions) []Slide { return nil } // STUB

// Summary returns the list row for the song.
func (s Song) Summary() SongSummary {
	return SongSummary{ID: s.ID, Title: s.Title, Author: s.Author, Copyright: s.Copyright, CCLI: s.CCLI}
}

// ── Scripture ────────────────────────────────────────────────────────────

// Books is the 66-book Protestant canon in order (Books[0] is Genesis).
var Books []Book // STUB: populated by the models work package

// BookByNumber returns the book with the given 1-based number.
func BookByNumber(n int) (Book, bool) { return Book{}, false } // STUB

// ParseReference parses a human reference such as "jn 3:16", "John 3:16-18,20",
// "1 Cor 13", "1co13:4-7", "Ps 23:1-3", "Song of Solomon 2:1", "rev 22:21",
// "John 3:36-4:2". Book matching is case-insensitive, by name, abbreviation,
// alias or unambiguous prefix. Errors wrap ErrBadReference.
func ParseReference(s string) (Passage, error) { return Passage{}, ErrBadReference } // STUB

// String formats a passage canonically: "John 3:16-18, 20", "1 Corinthians 13", "John 3:36-4:2".
func (p Passage) String() string { return "" } // STUB

// ScriptureSlides makes one slide per verse: Kind=SlideScripture,
// Label "John 3:16", Text = verse text, Footer = "John 3:16 (KJV)".
func ScriptureSlides(p Passage, verses []Verse, bibleAbbrev string, mode ScriptureSlideMode) []Slide {
	return nil // STUB
}

// ── Schedule ─────────────────────────────────────────────────────────────

// NewItemID returns a new unique schedule item id (uuid v4 string).
func NewItemID() string { return "" } // STUB

// NewSchedule returns an empty, clean schedule (Version 1, Title "Untitled").
func NewSchedule() *Schedule { return &Schedule{Version: 1, Title: "Untitled"} }

// Add appends an item (assigning an ID if empty), marks dirty, returns its index.
func (s *Schedule) Add(item ScheduleItem) int { return -1 } // STUB

// Insert inserts at index i (clamped), assigning an ID if empty; marks dirty.
func (s *Schedule) Insert(i int, item ScheduleItem) int { return -1 } // STUB

// Remove deletes the item at i; false if out of range. Marks dirty.
func (s *Schedule) Remove(i int) bool { return false } // STUB

// Move moves the item at from to index to; false if out of range. Marks dirty.
func (s *Schedule) Move(from, to int) bool { return false } // STUB

// IndexOf returns the index of the item with id, or -1.
func (s *Schedule) IndexOf(id string) int { return -1 } // STUB

// Save writes the schedule as indented JSON to path (atomically: temp file +
// rename), sets Path and Title (from the file name when Title is "Untitled"),
// clears Dirty.
func (s *Schedule) Save(path string) error { return nil } // STUB

// LoadSchedule reads a .fws file. Unknown future versions return an error.
func LoadSchedule(path string) (*Schedule, error) { return nil, nil } // STUB

// ── Presenter (see presenter.go for semantics) ───────────────────────────

func (p *Presenter) SetPreview(d *Deck)                         {}               // STUB
func (p *Presenter) SelectPreviewSlide(i int)                   {}               // STUB
func (p *Presenter) GoLive()                                    {}               // STUB
func (p *Presenter) GoLiveSlide(i int)                          {}               // STUB
func (p *Presenter) SelectLiveSlide(i int)                      {}               // STUB
func (p *Presenter) Next() bool                                 { return false } // STUB
func (p *Presenter) Prev() bool                                 { return false } // STUB
func (p *Presenter) First()                                     {}               // STUB
func (p *Presenter) Last()                                      {}               // STUB
func (p *Presenter) JumpToSection(kind SectionKind, n int) bool { return false } // STUB
func (p *Presenter) ToggleBlack()                               {}               // STUB
func (p *Presenter) ToggleClear()                               {}               // STUB
func (p *Presenter) ToggleLogo()                                {}               // STUB
func (p *Presenter) ClearLive()                                 {}               // STUB
func (p *Presenter) SetOutputEnabled(on bool)                   {}               // STUB
func (p *Presenter) SetLogo(bg Background)                      {}               // STUB

// LiveSlide returns the current live slide, or nil.
func (p *Presenter) LiveSlide() *Slide { return nil } // STUB

// LiveFrame returns what the output window should show now (Seq = State.Seq).
func (p *Presenter) LiveFrame() Frame { return Frame{} } // STUB

// PreviewFrame returns the frame for the Preview Output monitor (always ModeNormal).
func (p *Presenter) PreviewFrame() Frame { return Frame{} } // STUB
