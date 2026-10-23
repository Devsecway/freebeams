package core

import (
	"path/filepath"
	"strings"

	"freebeams/internal/bibles/biblepkg"
	"freebeams/internal/models"
)

// OpenFiles is what a launch with file arguments (a double-clicked .fbs /
// .fws schedule or .fbbible package, "Open with FreeBeams", a drop on the
// app icon) or the `open-files` control action asks the app to open.
type OpenFiles struct {
	Schedule string   // the schedule to open (the first one given)
	Bibles   []string // Bible packages, for the Bible Manager to add in turn
	Ignored  []string // arguments that are neither (other files, flags, missing paths)
}

// Empty reports whether there is nothing to open.
func (o OpenFiles) Empty() bool { return o.Schedule == "" && len(o.Bibles) == 0 }

// Paths lists the files to open, the schedule first.
func (o OpenFiles) Paths() []string {
	var p []string
	if o.Schedule != "" {
		p = append(p, o.Schedule)
	}
	return append(p, o.Bibles...)
}

// ClassifyOpenFiles sorts command-line arguments (or control paths) into the
// files the app opens. Flags (a leading "-", Godot's own and the "--"
// separator) are skipped silently; a path is kept only when exists says it
// is a regular file, and made absolute against cwd when it is relative.
func ClassifyOpenFiles(args []string, cwd string, exists func(string) bool) OpenFiles {
	var o OpenFiles
	seen := map[string]bool{}
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg == "" || strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "+") {
			continue
		}
		path := arg
		if !filepath.IsAbs(path) && cwd != "" {
			path = filepath.Join(cwd, path)
		}
		path = filepath.Clean(path)
		if seen[path] {
			continue
		}
		seen[path] = true
		switch {
		case !IsOpenableFile(path) || !exists(path):
			o.Ignored = append(o.Ignored, arg)
		case models.IsScheduleFile(path):
			if o.Schedule == "" {
				o.Schedule = path
			} else {
				o.Ignored = append(o.Ignored, arg) // one schedule is open at a time
			}
		default:
			o.Bibles = append(o.Bibles, path)
		}
	}
	return o
}

// IsOpenableFile reports whether path has an extension the app registers
// with the system: schedules (.fbs, legacy .fws) and Bible packages (.fbbible).
func IsOpenableFile(path string) bool {
	return models.IsScheduleFile(path) || strings.EqualFold(filepath.Ext(path), biblepkg.Ext)
}
