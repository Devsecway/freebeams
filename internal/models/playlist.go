package models

import (
	"fmt"
	"math"
	"strings"
)

// Background audio: the schedule's audio playlist.
//
// A playlist is an ordered list of audio files from the media library that
// plays independently of the Live slide (walk-in music under the
// announcements loop, quiet music under a prayer slide). It is saved with
// the schedule (Schedule.Playlist, the "playlist" key of a .fbs file); a
// schedule without one has an empty playlist. What plays now (the current
// track, playing / paused, the volume) is app state, not part of the file:
// see core.AudioPlayer.

// RepeatMode is what happens at the end of a track.
type RepeatMode string

const (
	RepeatOff RepeatMode = "off" // the next track; the list stops after the last one (the default)
	RepeatAll RepeatMode = "all" // the next track; the first one again after the last
	RepeatOne RepeatMode = "one" // the same track again
)

// RepeatModes lists the modes in the order the transport's button cycles them.
var RepeatModes = []RepeatMode{RepeatOff, RepeatAll, RepeatOne}

// Normalized returns m, or RepeatOff for "" and unknown values.
func (m RepeatMode) Normalized() RepeatMode {
	switch m {
	case RepeatAll, RepeatOne:
		return m
	}
	return RepeatOff
}

// Next is the mode after m in the transport button's cycle (off → all → one → off).
func (m RepeatMode) Next() RepeatMode {
	switch m.Normalized() {
	case RepeatOff:
		return RepeatAll
	case RepeatAll:
		return RepeatOne
	}
	return RepeatOff
}

// ParseRepeatMode reads "off" / "all" / "one" (case-insensitive; "" is off).
func ParseRepeatMode(s string) (RepeatMode, error) {
	m := RepeatMode(strings.ToLower(strings.TrimSpace(s)))
	switch m {
	case "":
		return RepeatOff, nil
	case RepeatOff, RepeatAll, RepeatOne:
		return m, nil
	}
	return "", fmt.Errorf("unknown repeat mode %q (off|all|one)", s)
}

// MaxFadeOut bounds the fade-out on stop, in seconds.
const MaxFadeOut = 30

// ClampFadeOut keeps a fade-out length inside 0..MaxFadeOut seconds (NaN is 0).
func ClampFadeOut(secs float64) float64 {
	if math.IsNaN(secs) || secs < 0 {
		return 0
	}
	return min(secs, MaxFadeOut)
}

// PlaylistTrack is one audio file in a playlist. ID is stable for the
// track's lifetime (the player follows the current track by ID while the
// list is edited); the same file may be in the list twice.
type PlaylistTrack struct {
	ID      string  `json:"id"`
	MediaID int64   `json:"mediaId,omitempty"` // the library row it was added from (0: a file only)
	Path    string  `json:"path"`              // absolute path of the audio file
	Title   string  `json:"title"`
	Seconds float64 `json:"seconds,omitempty"` // duration as the library knew it (0: unknown)
}

// Playlist is the schedule's background audio.
type Playlist struct {
	Tracks  []PlaylistTrack `json:"tracks"`
	Repeat  RepeatMode      `json:"repeat,omitempty"`  // "" is RepeatOff
	Shuffle bool            `json:"shuffle,omitempty"` // play the tracks in a random order, each once per pass
	// FadeOut is how long Stop fades the music out, in seconds (0 stops at once).
	FadeOut float64 `json:"fadeOut,omitempty"`
}

// IsZero reports an empty playlist with the default settings: such a
// playlist is left out of a saved schedule (omitzero), so schedules without
// background audio are written exactly as before.
func (p Playlist) IsZero() bool {
	return len(p.Tracks) == 0 && p.Repeat.Normalized() == RepeatOff && !p.Shuffle && p.FadeOut == 0
}

// TrackFromMedia makes a playlist track of a library audio item (a new ID).
func TrackFromMedia(m Media) PlaylistTrack {
	title := m.Title
	if strings.TrimSpace(title) == "" {
		title = MediaTitle(m.Path)
	}
	return PlaylistTrack{ID: NewItemID(), MediaID: m.ID, Path: m.Path, Title: title, Seconds: m.Duration.Seconds()}
}

// IndexOf returns the index of the track with id, or -1.
func (p *Playlist) IndexOf(id string) int {
	if id == "" {
		return -1
	}
	for i := range p.Tracks {
		if p.Tracks[i].ID == id {
			return i
		}
	}
	return -1
}

// Track returns the track with id.
func (p *Playlist) Track(id string) (PlaylistTrack, bool) {
	if i := p.IndexOf(id); i >= 0 {
		return p.Tracks[i], true
	}
	return PlaylistTrack{}, false
}

// Insert inserts t at index i (clamped; i < 0 appends), giving it an ID
// when it has none, and returns the index it ended up at.
func (p *Playlist) Insert(i int, t PlaylistTrack) int {
	if strings.TrimSpace(t.ID) == "" || p.IndexOf(t.ID) >= 0 {
		t.ID = NewItemID()
	}
	if t.Title == "" {
		t.Title = MediaTitle(t.Path)
	}
	if i < 0 || i > len(p.Tracks) {
		i = len(p.Tracks)
	}
	p.Tracks = append(p.Tracks, PlaylistTrack{})
	copy(p.Tracks[i+1:], p.Tracks[i:])
	p.Tracks[i] = t
	return i
}

// Remove deletes the track at i; false when out of range.
func (p *Playlist) Remove(i int) bool {
	if i < 0 || i >= len(p.Tracks) {
		return false
	}
	p.Tracks = append(p.Tracks[:i], p.Tracks[i+1:]...)
	return true
}

// Move moves the track at from to index to; false when out of range.
// Moving a track onto itself succeeds and changes nothing.
func (p *Playlist) Move(from, to int) bool {
	n := len(p.Tracks)
	if from < 0 || from >= n || to < 0 || to >= n {
		return false
	}
	if from == to {
		return true
	}
	t := p.Tracks[from]
	if from < to {
		copy(p.Tracks[from:to], p.Tracks[from+1:to+1])
	} else {
		copy(p.Tracks[to+1:from+1], p.Tracks[to:from])
	}
	p.Tracks[to] = t
	return true
}

// Normalize repairs a playlist read from a file: unknown repeat modes are
// off, the fade-out is clamped into 0..MaxFadeOut, tracks without a path are
// dropped, and missing or repeated IDs are replaced. It reports whether
// anything changed.
func (p *Playlist) Normalize() bool {
	changed := false
	if r := p.Repeat.Normalized(); r != p.Repeat && p.Repeat != "" {
		p.Repeat = r
		changed = true
	}
	if f := ClampFadeOut(p.FadeOut); f != p.FadeOut {
		p.FadeOut = f
		changed = true
	}
	seen := make(map[string]bool, len(p.Tracks))
	keep := p.Tracks[:0]
	for _, t := range p.Tracks {
		if strings.TrimSpace(t.Path) == "" {
			changed = true
			continue
		}
		if id := strings.TrimSpace(t.ID); id == "" || seen[id] {
			t.ID = NewItemID()
			changed = true
		}
		if t.Title == "" {
			t.Title = MediaTitle(t.Path)
			changed = true
		}
		seen[t.ID] = true
		keep = append(keep, t)
	}
	p.Tracks = keep
	return changed
}

// EditPlaylist runs edit on the schedule's playlist and marks the schedule
// dirty when edit reports a change.
func (s *Schedule) EditPlaylist(edit func(p *Playlist) bool) bool {
	if !edit(&s.Playlist) {
		return false
	}
	s.Dirty = true
	return true
}
