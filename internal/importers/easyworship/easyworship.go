// Package easyworship imports songs and Bibles from a local EasyWorship
// installation. It must not import graphics.gd.
//
// This file fixes the public API used by internal/app. Bodies marked STUB are
// replaced by the data work package (keeping these signatures) with tests.
package easyworship

import (
	"context"

	"freeworship/internal/models"
	"freeworship/internal/store"
)

// BibleFile describes an EasyWorship .ewb Bible found on disk.
type BibleFile struct {
	Path         string
	Abbrev       string
	Name         string
	Copyright    string
	PublicDomain bool // true for known public-domain texts (KJV, ASV, WEB, RVA 1909, ...)
}

// Options controls ImportInto.
type Options struct {
	// SongsDir is the EasyWorship Databases/Data dir; "" = DefaultSongsDir().
	SongsDir string
	// BiblesDir is the EasyWorship Plugins/Bibles dir; "" = DefaultBiblesDir().
	BiblesDir string
	// IncludeCopyrightedBibles imports non-public-domain Bibles too.
	IncludeCopyrightedBibles bool
}

// Report summarises an import.
type Report struct {
	SongsImported  int
	SongsSkipped   int // already present (same title+author) or unreadable
	BiblesImported []string
	BiblesSkipped  []string // with reason, e.g. "HCSB (copyrighted)"
	Warnings       []string
}

// DefaultSongsDir returns the platform's EasyWorship song DB directory
// (macOS: /Users/Shared/Softouch/Easyworship/Default/v6.1/Databases/Data,
// Windows: C:\Users\Public\Documents\Softouch\Easyworship\Default\v6.1\Databases\Data).
func DefaultSongsDir() string { return "" } // STUB

// DefaultBiblesDir returns the platform's EasyWorship Bibles directory
// (macOS: /Users/Shared/Softouch/Easyworship.v8/Plugins/Bibles).
func DefaultBiblesDir() string { return "" } // STUB

// Installed reports whether an EasyWorship song database exists at DefaultSongsDir().
func Installed() bool { return false } // STUB

// ReadSongs reads all songs (metadata + lyrics parsed from RTF into sections).
func ReadSongs(songsDir string) ([]models.Song, error) { return nil, nil } // STUB

// FindBibles lists the .ewb files in biblesDir.
func FindBibles(biblesDir string) ([]BibleFile, error) { return nil, nil } // STUB

// ReadBible decodes a .ewb file into its translation and all verses.
func ReadBible(path string) (models.Bible, []models.Verse, error) { return models.Bible{}, nil, nil } // STUB

// ImportInto imports songs and Bibles into st. It is idempotent: running it
// twice does not duplicate songs or Bibles.
func ImportInto(ctx context.Context, st *store.Store, opt Options) (Report, error) {
	return Report{}, nil // STUB
}
