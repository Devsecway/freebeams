package models

import "time"

// SectionKind classifies a song section. The values match the EasyWorship
// section jump hotkeys (V, C, B, R, T, I, E).
type SectionKind string

const (
	SectionVerse     SectionKind = "verse"
	SectionChorus    SectionKind = "chorus"
	SectionPreChorus SectionKind = "prechorus"
	SectionBridge    SectionKind = "bridge"
	SectionTag       SectionKind = "tag"
	SectionIntro     SectionKind = "intro"
	SectionEnding    SectionKind = "ending"
	SectionOther     SectionKind = "other"
)

// Section is one labelled block of lyrics.
type Section struct {
	Kind   SectionKind `json:"kind"`
	Number int         `json:"number,omitempty"` // Verse 2 -> 2; 0 when unnumbered
	// Label is the display label ("Verse 2", "Chorus", or a custom label).
	Label string   `json:"label"`
	Lines []string `json:"lines"`
}

// Song is a song in the library.
type Song struct {
	ID            int64     `json:"id"`
	UUID          string    `json:"uuid"`
	Title         string    `json:"title"`
	Author        string    `json:"author"`
	Copyright     string    `json:"copyright"`
	Administrator string    `json:"administrator"`
	CCLI          string    `json:"ccli"`
	Tags          string    `json:"tags"`
	Sections      []Section `json:"sections"`
	ThemeID       int64     `json:"themeId,omitempty"` // 0 = default song theme
	Created       time.Time `json:"created"`
	Updated       time.Time `json:"updated"`
}

// SongSummary is a lightweight row for library lists and search results.
type SongSummary struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	Copyright string `json:"copyright"`
	CCLI      string `json:"ccli"`

	// Matched says where a search found the song (MatchedTitle, MatchedAuthor
	// or MatchedLyrics); "" when the list is not a search result. Snippet is
	// the lyric excerpt around the match when Matched is MatchedLyrics.
	Matched string `json:"matched,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

// Where a song search matched (SongSummary.Matched).
const (
	MatchedTitle  = "title"  // every term is in the title
	MatchedAuthor = "author" // every term is in the title or the author, not the title alone
	MatchedLyrics = "lyrics" // the lyrics were needed for at least one term
)

// SlideOptions controls how sections are split into slides.
type SlideOptions struct {
	// MaxLines is the maximum number of lyric lines per slide. A longer section
	// is split into several slides, as evenly as possible. 0 means no limit.
	MaxLines int
	// IncludeCopyright adds the "Title / Author / © Copyright / CCLI #" footer to
	// the last slide (EasyWorship's default) or to every slide when
	// CopyrightEverySlide is set.
	IncludeCopyright    bool
	CopyrightEverySlide bool
}

// DefaultSlideOptions matches EasyWorship's defaults.
func DefaultSlideOptions() SlideOptions {
	return SlideOptions{MaxLines: 0, IncludeCopyright: true}
}
