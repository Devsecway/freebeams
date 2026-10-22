package core

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"freebeams/internal/models"
)

// Bible Manager sources (BibleEntry.Source).
const (
	BibleSourceBuiltin = "builtin" // embedded in the app (bundled)
	BibleSourceFolder  = "folder"  // a file in the Bibles folder
	BibleSourceLibrary = "library" // only in the library (imported, no package)
)

// BibleAvailable is a translation the Bible Manager can add: a built-in or
// a file in the Bibles folder.
type BibleAvailable struct {
	Bible   models.Bible // no ID
	Verses  int
	Size    int64  // bytes: the embedded package, or the file(s) on disk
	Source  string // BibleSourceBuiltin or BibleSourceFolder
	Path    string // the folder file (BibleSourceFolder)
	Package bool   // the folder file is a Bible package (.fbbible)
}

// BibleEntry is one row of the Bible Manager: a translation from any source,
// merged with what the library has (/state.bibleManager.entries).
type BibleEntry struct {
	Abbrev       string `json:"abbrev"`
	Name         string `json:"name"`
	Language     string `json:"language"`
	Copyright    string `json:"copyright,omitempty"`
	Source       string `json:"source"` // builtin | folder | library
	Installed    bool   `json:"installed"`
	Verses       int    `json:"verses"` // the library's count when installed, else the source's
	Size         int64  `json:"size"`   // the package or file size; 0 for library only
	Path         string `json:"path,omitempty"`
	Builtin      bool   `json:"builtin"`    // also embedded in the app
	HasPackage   bool   `json:"hasPackage"` // a .fbbible for it is in the Bibles folder
	PublicDomain bool   `json:"publicDomain"`
}

// CanAdd reports whether Add applies: not installed, and there is a source
// to install from.
func (e BibleEntry) CanAdd() bool { return !e.Installed && (e.Builtin || e.Path != "") }

// CanSavePackage reports whether "Save as Package" applies: installed, and
// neither built in nor already a package in the Bibles folder.
func (e BibleEntry) CanSavePackage() bool { return e.Installed && !e.Builtin && !e.HasPackage }

// MergeBibleEntries merges the built-ins, the Bibles folder and the library
// into the manager's rows, one per abbreviation (case-insensitive), sorted by
// abbreviation. The source is the best one to add from: a built-in, else the
// folder file, else "library" (installed only). Installed rows take the
// library's name and verse count (counts, by bible ID).
func MergeBibleEntries(builtin, folder []BibleAvailable, installed []models.Bible, counts map[int64]int) []BibleEntry {
	rows := map[string]*BibleEntry{}
	get := func(b models.Bible) *BibleEntry {
		key := strings.ToUpper(strings.TrimSpace(b.Abbrev))
		e := rows[key]
		if e == nil {
			e = &BibleEntry{Abbrev: strings.TrimSpace(b.Abbrev), Source: BibleSourceLibrary}
			rows[key] = e
		}
		if e.Name == "" {
			e.Name = b.Name
		}
		if e.Language == "" {
			e.Language = b.Language
		}
		if e.Copyright == "" {
			e.Copyright = b.Copyright
		}
		return e
	}
	for _, b := range installed {
		e := get(b)
		e.Abbrev, e.Name, e.Installed = b.Abbrev, b.Name, true
		e.Verses = counts[b.ID]
	}
	for _, f := range folder {
		e := get(f.Bible)
		e.Path, e.HasPackage = f.Path, f.Package
		e.Source, e.Size = BibleSourceFolder, f.Size
		if !e.Installed {
			e.Verses = f.Verses
		}
	}
	for _, b := range builtin {
		e := get(b.Bible)
		e.Builtin, e.Source, e.Size = true, BibleSourceBuiltin, b.Size
		if !e.Installed {
			e.Verses = b.Verses
		}
	}
	out := make([]BibleEntry, 0, len(rows))
	for _, e := range rows {
		e.PublicDomain = models.IsPublicDomainBible(e.Abbrev, e.Name)
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToUpper(out[i].Abbrev) < strings.ToUpper(out[j].Abbrev) })
	return out
}

// FindBibleEntry returns the index of abbrev in entries (case-insensitive), or -1.
func FindBibleEntry(entries []BibleEntry, abbrev string) int {
	abbrev = strings.TrimSpace(abbrev)
	for i, e := range entries {
		if abbrev != "" && strings.EqualFold(e.Abbrev, abbrev) {
			return i
		}
	}
	return -1
}

// BibleDisplayName is "King James Version (KJV)", or the abbreviation alone.
func BibleDisplayName(abbrev, name string) string {
	if name = strings.TrimSpace(name); name != "" && !strings.EqualFold(name, abbrev) {
		return name + " (" + abbrev + ")"
	}
	return abbrev
}

// languageNames are the languages of the translations FreeBeams meets, by
// ISO 639-1 code.
var languageNames = map[string]string{
	"en": "English", "es": "Spanish", "pt": "Portuguese", "fr": "French", "de": "German",
	"it": "Italian", "nl": "Dutch", "ru": "Russian", "zh": "Chinese", "ko": "Korean", "ja": "Japanese",
	"ar": "Arabic", "he": "Hebrew", "el": "Greek", "la": "Latin", "sw": "Swahili", "tl": "Tagalog",
	"ro": "Romanian", "pl": "Polish", "uk": "Ukrainian", "hi": "Hindi", "id": "Indonesian",
	"vi": "Vietnamese", "tw": "Twi", "ak": "Akan", "ee": "Ewe", "yo": "Yoruba", "ig": "Igbo",
	"ha": "Hausa", "af": "Afrikaans", "zu": "Zulu", "no": "Norwegian", "sv": "Swedish", "da": "Danish",
	"fi": "Finnish", "hu": "Hungarian", "cs": "Czech",
}

// LanguageName names a language code ("es" → "Spanish"); an unknown code is
// shown upper-cased, none as "—".
func LanguageName(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return "—"
	}
	if n, ok := languageNames[code]; ok {
		return n
	}
	return strings.ToUpper(code)
}

// BibleSourceLabel is the Source column ("Built-in", "Bibles folder", "Library only").
func BibleSourceLabel(source string) string {
	switch source {
	case BibleSourceBuiltin:
		return "Built-in"
	case BibleSourceFolder:
		return "Bibles folder"
	}
	return "Library only"
}

// BibleStatusLabel is the Status column.
func BibleStatusLabel(installed bool) string {
	if installed {
		return "Installed"
	}
	return "Not installed"
}

// SizeText formats a byte count ("1.3 MB", "820 KB", "512 B"; 0 is "—").
func SizeText(n int64) string {
	switch {
	case n <= 0:
		return "—"
	case n < 1000:
		return fmt.Sprintf("%d B", n)
	case n < 1000*1000:
		return fmt.Sprintf("%d KB", (n+500)/1000)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/1e6)
}

// BibleEntryDetail is the line under the list for the selected row: the
// verse count, the copyright (or public domain) and where it comes from.
func BibleEntryDetail(e BibleEntry) string {
	parts := []string{VerseCountText(e.Verses)}
	switch notice := firstLine(e.Copyright, 70); {
	case e.PublicDomain || strings.EqualFold(notice, "public domain"):
		parts = append(parts, "Public domain")
	case notice != "":
		parts = append(parts, notice)
	default:
		parts = append(parts, "Copyright not stated")
	}
	switch {
	case e.Builtin:
		parts = append(parts, "ships with FreeBeams")
	case e.Path != "":
		parts = append(parts, filepath.Base(e.Path))
	case e.Installed:
		parts = append(parts, "no package: Save as Package keeps a copy")
	}
	return strings.Join(parts, " · ")
}

// BibleRemoveQuestion is the confirmation before a translation is removed
// from the library (ui.ConfirmDialog: a heading line and one sentence).
func BibleRemoveQuestion(e BibleEntry) string {
	q := "Remove " + BibleDisplayName(e.Abbrev, e.Name) + " from the library? Schedule items keep their slides."
	if e.Builtin || e.Path != "" {
		return q + " You can add it again in the Bible Manager."
	}
	return q + " It has no package, so it cannot be added again from the Bible Manager."
}

// BibleAddQuestion is the licence confirmation before a copyrighted
// translation is added from the Bibles folder (BibleFileQuestion's wording).
func BibleAddQuestion(e BibleEntry) string {
	return BibleFileQuestion(models.Bible{Abbrev: e.Abbrev, Name: e.Name, Copyright: e.Copyright}, e.Verses, e.Path)
}

// BibleManagerState is /state.bibleManager.
type BibleManagerState struct {
	Open      bool         `json:"open"`
	Folder    string       `json:"folder"`
	Scanning  bool         `json:"scanning"`
	Scanned   bool         `json:"scanned"` // the folder has been listed at least once
	Task      string       `json:"task"`    // "" | add | remove | save
	TaskBible string       `json:"taskBible"`
	Progress  float64      `json:"progress"` // 0..1 while a task runs
	Status    string       `json:"status"`
	Error     bool         `json:"error"`
	Selected  string       `json:"selected"` // the selected row's abbreviation
	Problems  []string     `json:"problems"` // Bibles folder files that could not be listed
	Entries   []BibleEntry `json:"entries"`
}

// OpenFolderCommand opens a folder in the platform's file manager (the
// Bible Manager's "Open Bibles Folder").
func OpenFolderCommand(dir, goos string) (name string, args []string) {
	switch goos {
	case "darwin":
		return "open", []string{dir}
	case "windows":
		return "explorer", []string{dir}
	}
	return "xdg-open", []string{dir}
}
