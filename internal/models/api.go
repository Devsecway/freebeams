package models

// The models API that other packages build against. Implementations live in:
//
//	lyrics.go           ParseLyrics, FormatLyrics, SectionLabel, Song.Slides, Song.CopyrightFooter
//	scripture_books.go  Books, BookByNumber, FindBook, ChapterCount
//	reference.go        ParseReference, Passage.String, ScriptureSlides
//	schedule_ops.go     NewItemID, Schedule.Add/Insert/Remove/Move/IndexOf/Save, LoadSchedule
//	presenter_impl.go   Presenter methods, LiveSlide, LiveFrame, PreviewFrame

// Summary returns the list row for the song.
func (s Song) Summary() SongSummary {
	return SongSummary{ID: s.ID, Title: s.Title, Author: s.Author, Copyright: s.Copyright, CCLI: s.CCLI}
}

// NewSchedule returns an empty, clean schedule (Version 1, Title "Untitled").
func NewSchedule() *Schedule { return &Schedule{Version: 1, Title: "Untitled"} }
