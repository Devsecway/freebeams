// Package store persists the FreeWorship library in SQLite (modernc.org/sqlite,
// pure Go). It must not import graphics.gd.
//
// This file fixes the public API used by internal/app. Bodies marked STUB are
// replaced by the store work package (keeping these signatures) with tests.
package store

import (
	"context"
	"errors"

	"freeworship/internal/models"
)

// ErrNotFound is returned (wrapped) when a row does not exist.
var ErrNotFound = errors.New("not found")

// Store is a handle to the library database. Safe for concurrent use.
type Store struct {
	// unexported fields added by the implementation
}

// Open opens (creating if needed) the database at path and runs migrations.
func Open(path string) (*Store, error) { return &Store{}, nil } // STUB

// DefaultPath returns the library path in the user's config dir:
// <os.UserConfigDir()>/FreeWorship/library.db (directory created).
func DefaultPath() (string, error) { return "", nil } // STUB

func (s *Store) Close() error { return nil } // STUB

// ── Songs ────────────────────────────────────────────────────────────────

// SearchSongs returns songs matching query (FTS over title, author, lyrics;
// prefix matching per word), ordered by title. Empty query lists all songs.
// limit <= 0 means no limit.
func (s *Store) SearchSongs(ctx context.Context, query string, limit int) ([]models.SongSummary, error) {
	return nil, nil // STUB
}

// GetSong loads a song with its sections.
func (s *Store) GetSong(ctx context.Context, id int64) (models.Song, error) {
	return models.Song{}, ErrNotFound // STUB
}

// SaveSong inserts (ID == 0) or updates a song, assigning ID/UUID/Created/Updated.
func (s *Store) SaveSong(ctx context.Context, song *models.Song) error { return nil } // STUB

// DeleteSong deletes a song.
func (s *Store) DeleteSong(ctx context.Context, id int64) error { return nil } // STUB

// CountSongs returns the number of songs.
func (s *Store) CountSongs(ctx context.Context) (int, error) { return 0, nil } // STUB

// ── Bibles ───────────────────────────────────────────────────────────────

// ListBibles returns installed translations ordered by abbrev.
func (s *Store) ListBibles(ctx context.Context) ([]models.Bible, error) { return nil, nil } // STUB

// CreateBible inserts a translation and returns it with its ID. Abbrev is unique.
func (s *Store) CreateBible(ctx context.Context, b models.Bible) (models.Bible, error) {
	return b, nil // STUB
}

// InsertVerses bulk-inserts verses for a bible in one transaction.
func (s *Store) InsertVerses(ctx context.Context, bibleID int64, verses []models.Verse) error {
	return nil // STUB
}

// Verses returns the verses of a passage in canonical order. Whole-chapter and
// to-end-of-chapter ranges are expanded. Missing verses are skipped.
func (s *Store) Verses(ctx context.Context, bibleID int64, p models.Passage) ([]models.Verse, error) {
	return nil, nil // STUB
}

// ChapterCount / VerseCount support reference validation and pickers.
func (s *Store) ChapterCount(ctx context.Context, bibleID int64, book int) (int, error) {
	return 0, nil
} // STUB
func (s *Store) VerseCount(ctx context.Context, bibleID int64, book, chapter int) (int, error) {
	return 0, nil // STUB
}

// SearchVerses does a full-text search within a bible (limit <= 0: 100).
func (s *Store) SearchVerses(ctx context.Context, bibleID int64, text string, limit int) ([]models.Verse, error) {
	return nil, nil // STUB
}

// ── Settings (small key/value store, JSON values) ────────────────────────

// GetSetting unmarshals the JSON value for key into dst; returns false if unset.
func (s *Store) GetSetting(ctx context.Context, key string, dst any) (bool, error) { return false, nil } // STUB

// SetSetting stores v (JSON-marshalled) under key.
func (s *Store) SetSetting(ctx context.Context, key string, v any) error { return nil } // STUB
