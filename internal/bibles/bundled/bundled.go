// Package bundled holds the Bible translations that ship inside FreeWorship:
// the King James Version and the American Standard Version, both public
// domain. Nothing else is ever bundled: copyrighted translations (NIV, NLT,
// ESV, NKJV, MSG, …) only enter a library through the user's own import of a
// file they hold a licence for (see docs/BIBLES.md).
//
// The data is embedded as gzip-compressed single-file Bible JSON (the form
// internal/importers/bibletext.Encode writes), generated from a clone of
// github.com/hargarpay/bible-translations by gen/main.go. Install adds the
// translations a library lacks on first launch; it is idempotent and records
// Version so a later bundle can add more.
package bundled

import (
	"compress/gzip"
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"freeworship/internal/importers/bibletext"
	"freeworship/internal/store"
)

//go:embed data/*.json.gz
var data embed.FS

// Version is the bundle version recorded in the library (SettingVersion).
// Raise it when a translation is added or its data changes: Install then
// runs again on libraries that recorded an older version, skipping the
// abbreviations they already have.
const Version = 1

// SettingVersion is the setting key that records the installed bundle version.
const SettingVersion = "bibles.bundled.version"

// Source is where the data comes from.
const (
	SourceURL    = "https://github.com/hargarpay/bible-translations"
	SourceCommit = "98b32dfcc20cfe58e0e6c5d5bd126b226f40e465" // 2021-01-21
	SourceFormat = "json/<ABBREV>/{old,new}/<Book>/<chapter>.json, scraped from BibleGateway by the repository's app.py"
)

// Translation is one bundled translation (the manifest).
type Translation struct {
	Abbrev      string
	Name        string
	Language    string // ISO 639-1
	License     string // always "Public Domain"
	Attribution string
	File        string // embedded file
	Books       int    // expected book count (TestBundledTranslations checks the data)
	Verses      int    // expected verse count
}

// Translations is the manifest, in installation order.
var Translations = []Translation{
	{
		Abbrev: "KJV", Name: "King James Version", Language: "en", License: "Public Domain",
		Attribution: "King James Version (Authorized Version), as published on BibleGateway and collected in " +
			SourceURL + " (json/KJV, commit " + SourceCommit[:7] + "). The text is in the public domain " +
			"(in the United Kingdom it remains under perpetual Crown letters patent).",
		File: "data/kjv.json.gz", Books: 66, Verses: 31102,
	},
	{
		Abbrev: "ASV", Name: "American Standard Version", Language: "en", License: "Public Domain",
		Attribution: "American Standard Version (1901), as published on BibleGateway and collected in " +
			SourceURL + " (json/ASV, commit " + SourceCommit[:7] + "). Public domain: its copyright has expired. " +
			"The ASV follows the critical Greek text and omits 16 New Testament verses the KJV has, so it " +
			"counts 31,086 verses to the KJV's 31,102.",
		File: "data/asv.json.gz", Books: 66, Verses: 31086,
	},
}

// Patch is a correction gen/main.go applies to a verse of the source data
// before bundling it (the source is a web scrape with a few defects).
type Patch struct {
	Abbrev               string
	Book, Chapter, Verse int
	Old, New             string // Old is replaced by New, once
	Reason               string
}

// Patches are the corrections applied to the bundled data.
var Patches = []Patch{
	{
		Abbrev: "ASV", Book: 40, Chapter: 18, Verse: 24,
		Old:    "ten thousand This talent was probably worth about £200, or $1000.talents.",
		New:    "ten thousand talents.",
		Reason: "the source spliced the BibleGateway footnote body into the verse text",
	},
}

// Lookup returns the manifest entry for abbrev (case-insensitive).
func Lookup(abbrev string) (Translation, bool) {
	for _, t := range Translations {
		if strings.EqualFold(t.Abbrev, abbrev) {
			return t, true
		}
	}
	return Translation{}, false
}

// Abbrevs lists the bundled abbreviations.
func Abbrevs() []string {
	out := make([]string, len(Translations))
	for i, t := range Translations {
		out[i] = t.Abbrev
	}
	return out
}

// Size returns the embedded (compressed) size of all translations in bytes.
func Size() int64 {
	var n int64
	for _, t := range Translations {
		if fi, err := fs.Stat(data, t.File); err == nil {
			n += fi.Size()
		}
	}
	return n
}

// Load decodes a bundled translation.
func Load(abbrev string) (bibletext.Result, error) {
	t, ok := Lookup(abbrev)
	if !ok {
		return bibletext.Result{}, fmt.Errorf("bundled bible %q: not bundled", abbrev)
	}
	f, err := data.Open(t.File)
	if err != nil {
		return bibletext.Result{}, fmt.Errorf("bundled bible %s: %w", t.Abbrev, err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return bibletext.Result{}, fmt.Errorf("bundled bible %s: %w", t.Abbrev, err)
	}
	defer zr.Close()
	res, err := bibletext.Decode(zr)
	if err != nil {
		return bibletext.Result{}, fmt.Errorf("bundled bible %s: %w", t.Abbrev, err)
	}
	return res, nil
}

// Report summarises Install.
type Report struct {
	Installed []string // abbreviations added
	Skipped   []string // with reason, e.g. "KJV (already installed)"
	Version   int      // the bundle version recorded
}

// Needed reports whether Install has work to do: the library recorded no
// bundle version, or one older than Version.
func Needed(ctx context.Context, st *store.Store) (bool, error) {
	if st == nil {
		return false, errors.New("bundled bibles: nil store")
	}
	var v int
	ok, err := st.GetSetting(ctx, SettingVersion, &v)
	if err != nil {
		return false, fmt.Errorf("bundled bibles: %w", err)
	}
	return !ok || v < Version, nil
}

// Install adds the bundled translations the library lacks, matching
// abbreviations case-insensitively (a KJV imported from EasyWorship is left
// alone), and records Version. It is idempotent: a second run adds nothing.
// Each translation is written in one transaction, so a cancelled install
// leaves no half Bible.
func Install(ctx context.Context, st *store.Store) (Report, error) {
	rep := Report{Installed: []string{}, Skipped: []string{}}
	if st == nil {
		return rep, errors.New("install bundled bibles: nil store")
	}
	installed, err := st.ListBibles(ctx)
	if err != nil {
		return rep, fmt.Errorf("install bundled bibles: %w", err)
	}
	have := make(map[string]bool, len(installed))
	for _, b := range installed {
		have[strings.ToUpper(b.Abbrev)] = true
	}
	for _, t := range Translations {
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		if have[strings.ToUpper(t.Abbrev)] {
			rep.Skipped = append(rep.Skipped, t.Abbrev+" (already installed)")
			continue
		}
		res, err := Load(t.Abbrev)
		if err != nil {
			return rep, fmt.Errorf("install bundled bibles: %w", err)
		}
		if _, err := st.CreateBibleWithVerses(ctx, res.Bible, res.Verses); err != nil {
			if errors.Is(err, store.ErrExists) {
				rep.Skipped = append(rep.Skipped, t.Abbrev+" (already installed)")
				continue
			}
			return rep, fmt.Errorf("install bundled bible %s: %w", t.Abbrev, err)
		}
		rep.Installed = append(rep.Installed, t.Abbrev)
	}
	if err := st.SetSetting(ctx, SettingVersion, Version); err != nil {
		return rep, fmt.Errorf("install bundled bibles: %w", err)
	}
	rep.Version = Version
	return rep, nil
}
