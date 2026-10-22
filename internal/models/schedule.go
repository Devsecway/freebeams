package models

import (
	"path/filepath"
	"strings"
)

// ItemKind is the type of a schedule item.
type ItemKind string

const (
	ItemSong      ItemKind = "song"
	ItemScripture ItemKind = "scripture"
	ItemMedia     ItemKind = "media"
	// ItemPresentation is a PDF / PowerPoint deck shown as page images.
	ItemPresentation ItemKind = "presentation"
)

// ScheduleItem is one entry in a service schedule. It references library
// content (SongID, Passage) and also stores a snapshot of the resolved slides
// so a saved schedule still opens when the library has changed.
type ScheduleItem struct {
	ID       string   `json:"id"` // uuid, stable for the item's lifetime
	Kind     ItemKind `json:"kind"`
	Title    string   `json:"title"`    // "Amazing Grace" / "John 3:16-18"
	Subtitle string   `json:"subtitle"` // author / translation
	SongID   int64    `json:"songId,omitempty"`
	Passage  *Passage `json:"passage,omitempty"`
	BibleID  int64    `json:"bibleId,omitempty"`
	MediaID  int64    `json:"mediaId,omitempty"`
	// PresentationID, Source and CacheKey identify a presentation item: the
	// library row, the document it was rendered from and the cache folder its
	// page images (the Slides' backgrounds) live in. The app re-renders the
	// pages from Source when the cache is gone (core.ResolvePresentations).
	PresentationID int64  `json:"presentationId,omitempty"`
	Source         string `json:"source,omitempty"`
	CacheKey       string `json:"cacheKey,omitempty"`
	// Theme is the theme to render with (resolved when the item was added).
	Theme  Theme   `json:"theme"`
	Slides []Slide `json:"slides"`
}

// Schedule is an ordered list of items, saved as a .fbs file (JSON).
type Schedule struct {
	// Version of the file format; currently 1.
	Version int            `json:"version"`
	Title   string         `json:"title"`
	Items   []ScheduleItem `json:"items"`

	// Playlist is the background audio (playlist.go). It is left out of the
	// file while empty, and a file without it (every schedule before it
	// existed) opens with an empty playlist; the format version stays 1.
	Playlist Playlist `json:"playlist,omitzero"`

	// Path is where the schedule was loaded from / saved to (not serialized).
	Path string `json:"-"`
	// Dirty is set by mutations and cleared by save/load (not serialized).
	Dirty bool `json:"-"`
}

// ScheduleFileExt is the extension of saved schedules. LegacyScheduleFileExt
// is the extension the app wrote under its former name (FreeWorship); such
// files still open and keep their name when saved.
const (
	ScheduleFileExt       = ".fbs"
	LegacyScheduleFileExt = ".fws"
)

// IsScheduleFile reports whether path has a schedule extension (either one).
func IsScheduleFile(path string) bool {
	ext := filepath.Ext(path)
	return strings.EqualFold(ext, ScheduleFileExt) || strings.EqualFold(ext, LegacyScheduleFileExt)
}
