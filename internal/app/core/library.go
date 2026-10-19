package core

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"freebeams/internal/importers/easyworship"
	"freebeams/internal/models"
)

// Setting keys used by the app.
const (
	SettingImportDone = "import.easyworship.done" // ImportRecord
	SettingLogo       = "output.logo"             // models.Background
	SettingRecent     = "schedule.recent"         // []string, newest first
	SettingLayout     = "ui.layout"               // Layout
)

// LibraryEnv overrides the library database path (tests use a temp DB).
const LibraryEnv = "FREEBEAMS_LIBRARY"

// ImportRecord is stored under SettingImportDone after the first-run import.
type ImportRecord struct {
	Done    bool   `json:"done"`
	Summary string `json:"summary"`
}

// ImportSummary is a one-line summary of an EasyWorship import report.
func ImportSummary(r easyworship.Report) string {
	parts := []string{fmt.Sprintf("%d songs imported", r.SongsImported)}
	if r.SongsSkipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", r.SongsSkipped))
	}
	if len(r.BiblesImported) > 0 {
		parts = append(parts, "Bibles: "+strings.Join(r.BiblesImported, ", "))
	}
	if len(r.BiblesSkipped) > 0 {
		parts = append(parts, "skipped "+strings.Join(r.BiblesSkipped, ", "))
	}
	if r.MediaImported > 0 || r.MediaSkipped > 0 {
		media := fmt.Sprintf("%d media items", r.MediaImported)
		if r.MediaSkipped > 0 {
			media += fmt.Sprintf(" (%d already present)", r.MediaSkipped)
		}
		parts = append(parts, media)
	}
	if len(r.Warnings) > 0 {
		parts = append(parts, fmt.Sprintf("%d warnings", len(r.Warnings)))
	}
	return strings.Join(parts, "; ")
}

// LogoGradient is the default logo background: a dark gradient.
func LogoGradient() models.Background {
	return models.Background{Kind: models.BackgroundGradient, Color: models.Hex(0x1B2735), Color2: models.Hex(0x090A0F)}
}

// LogoBackground returns the logo background: the first image (by name) in
// dir, else LogoGradient. The image falls back to the gradient's top color if
// it fails to load.
func LogoBackground(dir string) models.Background {
	if dir != "" {
		var imgs []string
		for _, pat := range []string{"*.jpg", "*.jpeg", "*.png", "*.JPG", "*.JPEG", "*.PNG"} {
			m, _ := filepath.Glob(filepath.Join(dir, pat))
			imgs = append(imgs, m...)
		}
		sort.Strings(imgs)
		for _, p := range imgs {
			if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
				return models.Background{Kind: models.BackgroundImage, Path: p, Color: LogoGradient().Color}
			}
		}
	}
	return LogoGradient()
}

// AddRecent puts path at the front of a recent-files list (deduplicated,
// at most max entries).
func AddRecent(list []string, path string, max int) []string {
	out := []string{path}
	for _, p := range list {
		if p != path && len(out) < max {
			out = append(out, p)
		}
	}
	return out
}

// IsSaveAs reports whether saving to target is a Save As: a file other than
// the schedule's current one (cur, "" for a never-saved schedule). A Save As
// renames the schedule after the new file.
func IsSaveAs(cur, target string) bool {
	target = WithScheduleExt(target)
	if target == "" {
		return false
	}
	if cur == "" {
		return true
	}
	a, errA := filepath.Abs(cur)
	b, errB := filepath.Abs(target)
	if errA != nil || errB != nil {
		return filepath.Clean(cur) != filepath.Clean(target)
	}
	return a != b
}

// WithScheduleExt appends ".fbs" unless path already has a schedule
// extension (".fbs", or the former ".fws", which a schedule keeps).
func WithScheduleExt(path string) string {
	if path == "" || models.IsScheduleFile(path) {
		return path
	}
	return path + models.ScheduleFileExt
}
