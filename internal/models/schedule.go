package models

// ItemKind is the type of a schedule item.
type ItemKind string

const (
	ItemSong      ItemKind = "song"
	ItemScripture ItemKind = "scripture"
	ItemMedia     ItemKind = "media"
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
	// Theme is the theme to render with (resolved when the item was added).
	Theme  Theme   `json:"theme"`
	Slides []Slide `json:"slides"`
}

// Schedule is an ordered list of items, saved as a .fws file (JSON).
type Schedule struct {
	// Version of the file format; currently 1.
	Version int            `json:"version"`
	Title   string         `json:"title"`
	Items   []ScheduleItem `json:"items"`

	// Path is where the schedule was loaded from / saved to (not serialized).
	Path string `json:"-"`
	// Dirty is set by mutations and cleared by save/load (not serialized).
	Dirty bool `json:"-"`
}

// ScheduleFileExt is the extension of saved schedules.
const ScheduleFileExt = ".fws"
