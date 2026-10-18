package models

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// ScheduleVersion is the .fws file format version written by Save and the
// newest version LoadSchedule accepts.
const ScheduleVersion = 1

// untitled is the title of a schedule that has never been named.
const untitled = "Untitled"

// NewItemID returns a new unique schedule item id (uuid v4 string).
func NewItemID() string { return uuid.NewString() }

// Add appends an item (assigning an ID if empty), marks dirty, returns its index.
func (s *Schedule) Add(item ScheduleItem) int {
	return s.Insert(len(s.Items), item)
}

// Insert inserts at index i (clamped), assigning an ID if empty; marks dirty.
// It returns the index the item ended up at.
func (s *Schedule) Insert(i int, item ScheduleItem) int {
	if item.ID == "" {
		item.ID = NewItemID()
	}
	i = max(0, min(i, len(s.Items)))
	s.Items = append(s.Items, ScheduleItem{})
	copy(s.Items[i+1:], s.Items[i:])
	s.Items[i] = item
	s.Dirty = true
	return i
}

// Remove deletes the item at i; false if out of range. Marks dirty.
func (s *Schedule) Remove(i int) bool {
	if i < 0 || i >= len(s.Items) {
		return false
	}
	s.Items = append(s.Items[:i], s.Items[i+1:]...)
	s.Dirty = true
	return true
}

// Move moves the item at from to index to; false if out of range. Marks dirty
// (moving an item onto itself succeeds without marking dirty).
func (s *Schedule) Move(from, to int) bool {
	n := len(s.Items)
	if from < 0 || from >= n || to < 0 || to >= n {
		return false
	}
	if from == to {
		return true
	}
	item := s.Items[from]
	if from < to {
		copy(s.Items[from:to], s.Items[from+1:to+1])
	} else {
		copy(s.Items[to+1:from+1], s.Items[to:from])
	}
	s.Items[to] = item
	s.Dirty = true
	return true
}

// IndexOf returns the index of the item with id, or -1.
func (s *Schedule) IndexOf(id string) int {
	for i := range s.Items {
		if s.Items[i].ID == id {
			return i
		}
	}
	return -1
}

// titleFromPath returns the file name of path without its extension.
func titleFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// Save writes the schedule as indented JSON to path (atomically: temp file +
// rename), sets Path and Title (from the file name when Title is "Untitled"),
// clears Dirty.
//
// An empty Title is treated like "Untitled", and Version 0 is written as
// ScheduleVersion. On failure the schedule is left unchanged and the temp file
// removed.
func (s *Schedule) Save(path string) error {
	out := *s
	if out.Version == 0 {
		out.Version = ScheduleVersion
	}
	if t := strings.TrimSpace(out.Title); t == "" || t == untitled {
		out.Title = titleFromPath(path)
	}
	if out.Items == nil {
		out.Items = []ScheduleItem{}
	}
	data, err := json.MarshalIndent(&out, "", "  ")
	if err != nil {
		return fmt.Errorf("save schedule %s: %w", path, err)
	}
	data = append(data, '\n')
	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("save schedule %s: %w", path, err)
	}
	s.Version, s.Title, s.Path, s.Dirty = out.Version, out.Title, path, false
	return nil
}

// SaveAs writes the schedule to a new file and names the schedule after it
// (Title becomes the file name without its extension), like File → Save As.
// Otherwise it is Save. On failure the schedule is left unchanged.
func (s *Schedule) SaveAs(path string) error {
	c := *s
	c.Title = titleFromPath(path)
	if err := c.Save(path); err != nil {
		return err
	}
	*s = c
	return nil
}

// repairIDs gives every item a unique, non-empty ID. Hand-made or merged
// files can lack ids or repeat them, and the app identifies items (the live
// one, the selected one) by ID. The first item keeps a repeated id; later
// ones get new ids. It reports whether any id changed.
func (s *Schedule) repairIDs() bool {
	seen := make(map[string]bool, len(s.Items))
	changed := false
	for i := range s.Items {
		id := strings.TrimSpace(s.Items[i].ID)
		if id == "" || seen[id] {
			id = NewItemID()
		}
		if id != s.Items[i].ID {
			s.Items[i].ID = id
			changed = true
		}
		seen[id] = true
	}
	return changed
}

// writeFileAtomic writes data to a temp file next to path and renames it into
// place, so readers never see a partially written file.
func writeFileAtomic(path string, data []byte) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Chmod(0o644); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadSchedule reads a .fws file. Unknown future versions return an error.
//
// A missing or zero version is read as version 1. The returned schedule has
// Path set and Dirty cleared; an empty Title is taken from the file name.
// Items without an id, or with an id an earlier item already uses, get a new
// id (the file on disk is only rewritten by the next save).
func LoadSchedule(path string) (*Schedule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load schedule: %w", err)
	}
	var s Schedule
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("load schedule %s: %w", path, err)
	}
	switch {
	case s.Version == 0:
		s.Version = ScheduleVersion
	case s.Version < 0 || s.Version > ScheduleVersion:
		return nil, fmt.Errorf("load schedule %s: unsupported file version %d (this version of FreeWorship reads up to %d)",
			path, s.Version, ScheduleVersion)
	}
	if strings.TrimSpace(s.Title) == "" {
		s.Title = titleFromPath(path)
	}
	s.repairIDs()
	s.Path, s.Dirty = path, false
	return &s, nil
}
