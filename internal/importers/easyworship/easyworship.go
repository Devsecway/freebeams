// Package easyworship imports songs and Bibles from a local EasyWorship
// installation. It must not import graphics.gd.
//
// The EasyWorship databases are only ever opened read-only
// (SQLite URI mode=ro&immutable=1). See docs/formats/easyworship-songs.md and
// docs/formats/ewb.md for the file formats.
package easyworship

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"freeworship/internal/importers/rtf"
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
	// SongsDir is the EasyWorship Databases/Data dir (Songs.db, SongWords.db
	// and Media.db); "" = DefaultSongsDir().
	SongsDir string
	// BiblesDir is the EasyWorship Plugins/Bibles dir; "" = DefaultBiblesDir().
	BiblesDir string
	// ResourcesDir is the EasyWorship Resources dir (Images, Videos);
	// "" = DefaultResourcesDir().
	ResourcesDir string
	// IncludeCopyrightedBibles imports non-public-domain Bibles too.
	IncludeCopyrightedBibles bool
	// BiblesOnly skips the songs (an on-request Bible import after the first run).
	BiblesOnly bool
	// SkipMedia leaves the media library alone (ImportMedia runs it on its own).
	SkipMedia bool

	// BundledBibles are the abbreviations of the translations FreeWorship
	// ships (internal/bibles/bundled). Their EasyWorship files are reported
	// as skipped "(bundled)" (case-insensitive), so an EasyWorship KJV never
	// duplicates or races the bundled one.
	BundledBibles []string
}

// Report summarises an import.
type Report struct {
	SongsImported  int
	SongsSkipped   int // already present (same title+author) or unreadable
	BiblesImported []string
	BiblesSkipped  []string // with reason, e.g. "HCSB (copyrighted)"
	MediaImported  int
	MediaSkipped   int // already present (same path)
	Warnings       []string

	// Presentation files under Resources/Presentations (ImportPresentations).
	PresentationsImported int
	PresentationsSkipped  int // already present (same path)
}

const (
	songsDB     = "Songs.db"
	songWordsDB = "SongWords.db"
)

// publicDocuments returns the Windows public documents folder.
func publicDocuments() string {
	if p := os.Getenv("PUBLIC"); p != "" {
		return filepath.Join(p, "Documents")
	}
	return `C:\Users\Public\Documents`
}

// DefaultSongsDir returns the platform's EasyWorship song DB directory
// (macOS: /Users/Shared/Softouch/Easyworship/Default/v6.1/Databases/Data,
// Windows: C:\Users\Public\Documents\Softouch\Easyworship\Default\v6.1\Databases\Data).
// Other platforms return "".
func DefaultSongsDir() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Users/Shared/Softouch/Easyworship/Default/v6.1/Databases/Data"
	case "windows":
		return filepath.Join(publicDocuments(), "Softouch", "Easyworship", "Default", "v6.1", "Databases", "Data")
	}
	return ""
}

// DefaultBiblesDir returns the platform's EasyWorship Bibles directory
// (macOS: /Users/Shared/Softouch/Easyworship.v8/Plugins/Bibles; Windows: the
// same layout under C:\Users\Public\Documents\Softouch). Other platforms return "".
func DefaultBiblesDir() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Users/Shared/Softouch/Easyworship.v8/Plugins/Bibles"
	case "windows":
		return filepath.Join(publicDocuments(), "Softouch", "Easyworship.v8", "Plugins", "Bibles")
	}
	return ""
}

// Installed reports whether an EasyWorship song database exists at DefaultSongsDir().
func Installed() bool {
	dir := DefaultSongsDir()
	if dir == "" {
		return false
	}
	fi, err := os.Stat(filepath.Join(dir, songsDB))
	return err == nil && fi.Mode().IsRegular()
}

// ReadSongs reads all songs (metadata + lyrics parsed from RTF into sections),
// in EasyWorship's row order. Songs without lyrics are returned with no
// sections. songsDir "" means DefaultSongsDir().
func ReadSongs(songsDir string) ([]models.Song, error) {
	if songsDir == "" {
		songsDir = DefaultSongsDir()
	}
	if songsDir == "" {
		return nil, errors.New("read EasyWorship songs: no songs directory on this platform")
	}
	db, err := openReadOnly(filepath.Join(songsDir, songsDB))
	if err != nil {
		return nil, fmt.Errorf("read EasyWorship songs: %w", err)
	}
	defer db.Close()

	// No ORDER BY / WHERE on text columns: they use EW's UTF8_U_CI collation.
	rows, err := db.Query(`SELECT rowid, title, author, copyright, administrator, reference_number, tags FROM song`)
	if err != nil {
		return nil, fmt.Errorf("read EasyWorship songs: %w", err)
	}
	type row struct {
		id   int64
		song models.Song
	}
	var list []row
	for rows.Next() {
		var (
			id                                          int64
			title, author, copyright, admin, ccli, tags sql.NullString
		)
		if err := rows.Scan(&id, &title, &author, &copyright, &admin, &ccli, &tags); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read EasyWorship songs: %w", err)
		}
		list = append(list, row{id: id, song: models.Song{
			Title:         clean(title.String),
			Author:        clean(author.String),
			Copyright:     clean(copyright.String),
			Administrator: clean(admin.String),
			CCLI:          clean(ccli.String),
			Tags:          clean(tags.String),
		}})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("read EasyWorship songs: %w", err)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].id < list[j].id })

	words, err := readWords(filepath.Join(songsDir, songWordsDB))
	if err != nil {
		return nil, fmt.Errorf("read EasyWorship songs: %w", err)
	}
	out := make([]models.Song, 0, len(list))
	for _, r := range list {
		if w, ok := words[r.id]; ok {
			r.song.Sections = parseLyrics(lyricsText(w))
		}
		out = append(out, r.song)
	}
	return out, nil
}

// readWords returns song rowid -> lyrics (RTF) from SongWords.db. A missing
// file yields an empty map (songs then have no lyrics).
func readWords(path string) (map[int64]string, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return map[int64]string{}, nil
	}
	db, err := openReadOnly(path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT song_id, words FROM word`)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var (
			id    sql.NullInt64
			words sql.NullString
		)
		if err := rows.Scan(&id, &words); err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		if id.Valid {
			out[id.Int64] = words.String
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return out, nil
}

// lyricsText converts a word.words value (normally RTF) to plain text.
func lyricsText(words string) string {
	if t, err := rtf.ToText(words); err == nil {
		return t
	}
	return words // some imports store plain text
}

func clean(s string) string { return strings.Join(strings.Fields(s), " ") }

// FindBibles lists the readable .ewb files in biblesDir ("" = DefaultBiblesDir()),
// ordered by abbreviation. Unreadable files are left out.
func FindBibles(biblesDir string) ([]BibleFile, error) {
	files, _, err := findBibles(biblesDir)
	return files, err
}

func findBibles(biblesDir string) ([]BibleFile, []string, error) {
	if biblesDir == "" {
		biblesDir = DefaultBiblesDir()
	}
	if biblesDir == "" {
		return nil, nil, nil
	}
	entries, err := os.ReadDir(biblesDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("find bibles in %s: %w", biblesDir, err)
	}
	var (
		out      []BibleFile
		warnings []string
	)
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".ewb") {
			continue
		}
		path := filepath.Join(biblesDir, e.Name())
		bf, err := readBibleFile(path)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", e.Name(), err))
			continue
		}
		out = append(out, bf)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToUpper(out[i].Abbrev) < strings.ToUpper(out[j].Abbrev) })
	return out, warnings, nil
}

func readBibleFile(path string) (BibleFile, error) {
	db, err := openReadOnly(path)
	if err != nil {
		return BibleFile{}, err
	}
	defer db.Close()
	h, err := readEWBHeader(db)
	if err != nil {
		return BibleFile{}, err
	}
	return BibleFile{
		Path:         path,
		Abbrev:       h.Abbrev,
		Name:         h.Name,
		Copyright:    h.Copyright,
		PublicDomain: isPublicDomain(h.Abbrev, h.Name),
	}, nil
}

// ReadBible decodes a .ewb file into its translation and all verses, in
// canonical order (book 1..66 numbering). Bracketed italics markers are
// stripped ("[was]" -> "was"). Public-domain translations get Copyright
// "Public Domain"; others keep EasyWorship's notice as plain text.
func ReadBible(path string) (models.Bible, []models.Verse, error) {
	return readEWB(path)
}

// ImportInto imports songs, Bibles and media into st (opt.BiblesOnly limits it to
// the Bibles; opt.SkipMedia leaves the media library alone). It is idempotent:
// running it twice does not duplicate anything. Songs already in the library
// with the same title and author (case-insensitive) are skipped, as are
// Bibles whose abbreviation exists and media files already registered.
// Problems reading EasyWorship data become Warnings; only store failures
// (and cancellation) return an error.
func ImportInto(ctx context.Context, st *store.Store, opt Options) (Report, error) {
	var rep Report
	if st == nil {
		return rep, errors.New("import EasyWorship: nil store")
	}
	if !opt.BiblesOnly {
		if err := importSongs(ctx, st, opt, &rep); err != nil {
			return rep, err
		}
	}
	if err := importBibles(ctx, st, opt, &rep); err != nil {
		return rep, err
	}
	if !opt.SkipMedia && !opt.BiblesOnly {
		if err := importMedia(ctx, st, opt, &rep); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

func songKey(title, author string) string {
	return strings.ToLower(clean(title)) + "\x00" + strings.ToLower(clean(author))
}

func importSongs(ctx context.Context, st *store.Store, opt Options, rep *Report) error {
	dir := opt.SongsDir
	if dir == "" {
		dir = DefaultSongsDir()
	}
	if dir == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir, songsDB)); err != nil {
		if opt.SongsDir != "" {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("no EasyWorship songs: %v", err))
		}
		return nil
	}
	songs, err := ReadSongs(dir)
	if err != nil {
		rep.Warnings = append(rep.Warnings, err.Error())
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	existing, err := st.SearchSongs(ctx, "", 0)
	if err != nil {
		return fmt.Errorf("import EasyWorship songs: %w", err)
	}
	have := make(map[string]bool, len(existing))
	for _, s := range existing {
		have[songKey(s.Title, s.Author)] = true
	}
	var batch []*models.Song
	for i := range songs {
		s := &songs[i]
		key := songKey(s.Title, s.Author)
		if s.Title == "" || have[key] {
			rep.SongsSkipped++
			continue
		}
		have[key] = true
		if len(s.Sections) == 0 {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("song %q has no lyrics", s.Title))
		}
		batch = append(batch, s)
	}
	if len(batch) == 0 {
		return nil
	}
	if err := st.SaveSongs(ctx, batch); err != nil {
		return fmt.Errorf("import EasyWorship songs: %w", err)
	}
	rep.SongsImported = len(batch)
	return nil
}

func importBibles(ctx context.Context, st *store.Store, opt Options, rep *Report) error {
	files, warnings, err := findBibles(opt.BiblesDir)
	rep.Warnings = append(rep.Warnings, warnings...)
	if err != nil {
		rep.Warnings = append(rep.Warnings, err.Error())
		return nil
	}
	if len(files) == 0 {
		return nil
	}
	installed, err := st.ListBibles(ctx)
	if err != nil {
		return fmt.Errorf("import EasyWorship bibles: %w", err)
	}
	have := map[string]bool{}
	for _, b := range installed {
		have[strings.ToUpper(b.Abbrev)] = true
	}
	bundled := map[string]bool{}
	for _, a := range opt.BundledBibles {
		bundled[strings.ToUpper(strings.TrimSpace(a))] = true
	}
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		abbrev := strings.ToUpper(f.Abbrev)
		switch {
		case bundled[abbrev]:
			rep.BiblesSkipped = append(rep.BiblesSkipped, f.Abbrev+" (bundled)")
			continue
		case have[abbrev]:
			rep.BiblesSkipped = append(rep.BiblesSkipped, f.Abbrev+" (already installed)")
			continue
		case !f.PublicDomain && !opt.IncludeCopyrightedBibles:
			rep.BiblesSkipped = append(rep.BiblesSkipped, f.Abbrev+" (copyrighted)")
			continue
		}
		bible, verses, err := ReadBible(f.Path)
		if err != nil {
			rep.BiblesSkipped = append(rep.BiblesSkipped, f.Abbrev+" (unreadable)")
			rep.Warnings = append(rep.Warnings, err.Error())
			continue
		}
		if len(verses) == 0 {
			rep.BiblesSkipped = append(rep.BiblesSkipped, f.Abbrev+" (no verses)")
			continue
		}
		if _, err := st.CreateBibleWithVerses(ctx, bible, verses); err != nil {
			if errors.Is(err, store.ErrExists) {
				rep.BiblesSkipped = append(rep.BiblesSkipped, f.Abbrev+" (already installed)")
				continue
			}
			return fmt.Errorf("import EasyWorship bible %s: %w", f.Abbrev, err)
		}
		have[abbrev] = true
		rep.BiblesImported = append(rep.BiblesImported, f.Abbrev)
	}
	return nil
}
