// Package biblefolder lists and reads the Bibles folder (appdirs.Bibles):
// every Bible package (.fbbible), Bible JSON file (the layouts
// internal/importers/bibletext reads), EasyWorship .ewb file and
// bible-translations folder in it is a translation the Bible Manager can add.
// It also reads any one such file for "Add from File…". It is pure Go.
package biblefolder

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"freebeams/internal/bibles/biblepkg"
	"freebeams/internal/importers/bibletext"
	"freebeams/internal/importers/easyworship"
	"freebeams/internal/models"
)

// Kind is the format of a Bibles folder entry.
type Kind string

const (
	KindPackage Kind = "package" // .fbbible
	KindJSON    Kind = "json"    // a bibletext single-file Bible (object or array layout)
	KindEWB     Kind = "ewb"     // an EasyWorship .ewb
	KindFolder  Kind = "folder"  // a bible-translations folder (<ABBREV>/{old,new}/…)
)

// kindRank orders the kinds when two entries share an abbreviation: the
// package is what "Save as Package" writes, so it wins.
var kindRank = map[Kind]int{KindPackage: 0, KindEWB: 1, KindJSON: 2, KindFolder: 3}

// Entry is one translation found in the folder.
type Entry struct {
	Path   string
	Kind   Kind
	Bible  models.Bible // no ID
	Verses int
	Size   int64 // bytes on disk (a folder: all its files)
}

// KindOf returns the kind a path would be read as ("" when the folder
// ignores it): by extension for files, a translation folder for folders.
func KindOf(path string, isDir bool) Kind {
	if isDir {
		for _, t := range []string{"old", "new"} {
			if fi, err := os.Stat(filepath.Join(path, t)); err == nil && fi.IsDir() {
				return KindFolder
			}
		}
		return ""
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case biblepkg.Ext:
		return KindPackage
	case ".json":
		return KindJSON
	case ".ewb":
		return KindEWB
	}
	return ""
}

// Scan lists the translations in dir, sorted by abbreviation. Hidden files,
// other file types and folders that are not translations are ignored. A
// file that cannot be read, or that repeats an abbreviation found in a
// better file (a package beats an .ewb beats JSON beats a folder), is
// reported in problems and left out. A missing dir lists nothing.
func Scan(dir string) (entries []Entry, problems []string, err error) {
	des, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []Entry{}, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("scan bibles folder: %w", err)
	}
	byAbbrev := map[string]Entry{}
	for _, de := range des {
		name := de.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		isDir := de.IsDir()
		if de.Type()&fs.ModeSymlink != 0 {
			if fi, err := os.Stat(path); err == nil {
				isDir = fi.IsDir()
			}
		}
		kind := KindOf(path, isDir)
		if kind == "" {
			continue
		}
		e, err := info(path, kind)
		if err != nil {
			msg := err.Error()
			if !strings.HasPrefix(msg, name+": ") {
				msg = name + ": " + msg
			}
			problems = append(problems, msg)
			continue
		}
		key := strings.ToUpper(e.Bible.Abbrev)
		if prev, dup := byAbbrev[key]; dup {
			keep, drop := prev, e
			if kindRank[e.Kind] < kindRank[prev.Kind] {
				keep, drop = e, prev
			}
			byAbbrev[key] = keep
			problems = append(problems, fmt.Sprintf("%s: %s is also in %s, which is used", filepath.Base(drop.Path), keep.Bible.Abbrev, filepath.Base(keep.Path)))
			continue
		}
		byAbbrev[key] = e
	}
	entries = make([]Entry, 0, len(byAbbrev))
	for _, e := range byAbbrev {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToUpper(entries[i].Bible.Abbrev) < strings.ToUpper(entries[j].Bible.Abbrev)
	})
	sort.Strings(problems)
	return entries, problems, nil
}

// info describes one entry: packages and .ewb files from their headers, JSON
// and folders by decoding them.
func info(path string, kind Kind) (Entry, error) {
	e := Entry{Path: path, Kind: kind}
	var err error
	switch kind {
	case KindPackage:
		var h biblepkg.Header
		h, err = biblepkg.ReadHeaderFile(path)
		e.Bible, e.Verses = h.AsBible(), h.Verses
	case KindEWB:
		e.Bible, e.Verses, err = easyworship.ReadBibleInfo(path)
	default:
		var res bibletext.Result
		res, err = bibletext.Read(path)
		e.Bible, e.Verses = res.Bible, len(res.Verses)
	}
	if err != nil {
		return Entry{}, err
	}
	if strings.TrimSpace(e.Bible.Abbrev) == "" {
		return Entry{}, errors.New("no abbreviation")
	}
	e.Size = diskSize(path)
	return e, nil
}

// diskSize is a file's size, or the total of a folder's files.
func diskSize(path string) int64 {
	var n int64
	filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}

// Loaded is a translation read in full.
type Loaded struct {
	Path     string
	Kind     Kind
	Bible    models.Bible
	Verses   []models.Verse
	Warnings []string // entries the reader skipped
}

// Read reads a whole translation from a file or folder: a package, an .ewb
// file, or anything bibletext reads (a translation folder, a chapter file
// inside one, a single-file Bible JSON in either layout).
func Read(path string) (Loaded, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return Loaded{}, fmt.Errorf("read bible: %w", err)
	}
	kind := KindOf(path, fi.IsDir())
	l := Loaded{Path: path, Kind: kind}
	switch kind {
	case KindPackage:
		p, err := biblepkg.ReadFile(path)
		if err != nil {
			return Loaded{}, err
		}
		l.Bible, l.Verses, l.Warnings = p.Bible, p.Verses, p.Warnings
	case KindEWB:
		b, verses, err := easyworship.ReadBible(path)
		if err != nil {
			return Loaded{}, err
		}
		if len(verses) == 0 {
			return Loaded{}, fmt.Errorf("read bible %s: no verses", filepath.Base(path))
		}
		l.Bible, l.Verses = b, verses
	default:
		res, err := bibletext.Read(path)
		if err != nil {
			return Loaded{}, err
		}
		if l.Kind == "" {
			l.Kind = KindJSON
		}
		l.Bible, l.Verses, l.Warnings = res.Bible, res.Verses, res.Warnings
	}
	return l, nil
}

// PackagePath is where "Save as Package" writes abbrev in dir.
func PackagePath(dir, abbrev string) string { return filepath.Join(dir, biblepkg.FileName(abbrev)) }
