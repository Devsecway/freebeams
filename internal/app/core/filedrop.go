package core

import (
	"fmt"
	"path/filepath"
	"strings"

	"freebeams/internal/bibles/biblepkg"
	"freebeams/internal/models"
)

// Files dropped from the Finder (or Explorer) onto the operator window. The
// window's files_dropped signal carries only the paths; the app finds what
// is under the pointer (the schedule, with an insertion gap, or the Media /
// Presentations tab) and plans the drop here: which files go to which
// library, which become schedule items (in drop order), which open as File ▸
// Open or as a Bible package would, and which are skipped, with one message.

// Where a file drop lands (FileDropPlan target, the drop-files action).
const (
	FileDropSchedule      = "schedule"
	FileDropMedia         = "media"
	FileDropPresentations = "presentations"
)

// DroppedKind is what a dropped path is.
type DroppedKind string

const (
	DroppedMedia        DroppedKind = "media"        // an image, video or audio file
	DroppedPresentation DroppedKind = "presentation" // PDF, PowerPoint, Keynote, OpenDocument
	DroppedSchedule     DroppedKind = "schedule"     // .fbs (or the legacy .fws)
	DroppedBible        DroppedKind = "bible"        // a .fbbible package
	DroppedFolder       DroppedKind = "folder"
	DroppedOther        DroppedKind = "other" // anything else
)

// DroppedKindOf classifies path (isDir: it is a folder).
func DroppedKindOf(path string, isDir bool) DroppedKind {
	switch {
	case isDir:
		return DroppedFolder
	case models.IsScheduleFile(path):
		return DroppedSchedule
	case strings.EqualFold(filepath.Ext(path), biblepkg.Ext):
		return DroppedBible
	case models.IsPresentationFile(path):
		return DroppedPresentation
	}
	if _, ok := models.MediaKindOf(path); ok {
		return DroppedMedia
	}
	return DroppedOther
}

// DroppedFile is one file the drop inserts into the schedule.
type DroppedFile struct {
	Path string
	Kind DroppedKind // DroppedMedia or DroppedPresentation
}

// Why a dropped path was skipped (SkippedFile.Reason).
const (
	SkipMissing      = "missing"       // the path does not exist
	SkipUnsupported  = "unsupported"   // not a file FreeBeams uses (for this target)
	SkipFolder       = "folder"        // a folder dropped on the schedule
	SkipOneSchedule  = "one-schedule"  // a second schedule in the same drop
	SkipWithSchedule = "with-schedule" // a resource dropped together with a schedule, which replaces it
	SkipOutside      = "outside"       // dropped outside the schedule and the Media / Presentations tabs
)

// SkippedFile is a dropped path the drop does not use.
type SkippedFile struct {
	Path   string
	Reason string
}

// FileDropPlan is what a drop does.
type FileDropPlan struct {
	Target        string
	Media         []string      // to register in the media library (files or folders, drop order)
	Presentations []string      // to register in the presentation library
	Insert        []DroppedFile // schedule drops: the items to insert at the gap, in drop order
	Schedule      string        // a schedule to open (File ▸ Open, asking about unsaved changes)
	Bibles        []string      // Bible packages for the Bible Manager's Add
	Skipped       []SkippedFile
}

// Empty reports whether the drop does nothing (everything was skipped).
func (p FileDropPlan) Empty() bool {
	return len(p.Media) == 0 && len(p.Presentations) == 0 && p.Schedule == "" && len(p.Bibles) == 0
}

// PlanFileDrop plans a drop of paths on target. stat reports whether a path
// exists and is a folder. Blank and repeated paths are ignored.
//
//   - The schedule takes media and presentation files: they are added to
//     their libraries and inserted in drop order. A schedule file opens as
//     File ▸ Open would (one per drop, and the other resources of that drop
//     are skipped: the opened schedule replaces the one they would go into);
//     a Bible package goes to the Bible Manager. Folders are skipped (an
//     accidental drop of a whole folder would flood the schedule).
//   - The Media tab takes media files and folders (scanned as Add Folder
//     does); the Presentations tab presentation files and folders. Anything
//     else is skipped.
//   - Any other target skips everything.
func PlanFileDrop(target string, paths []string, stat func(path string) (exists, isDir bool)) FileDropPlan {
	plan := FileDropPlan{Target: target}
	seen := map[string]bool{}
	var withSchedule []string // resources that a schedule in the same drop pre-empts
	for _, raw := range paths {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		path := filepath.Clean(raw)
		if seen[path] {
			continue
		}
		seen[path] = true
		skip := func(reason string) { plan.Skipped = append(plan.Skipped, SkippedFile{Path: path, Reason: reason}) }
		exists, isDir := stat(path)
		if !exists {
			skip(SkipMissing)
			continue
		}
		kind := DroppedKindOf(path, isDir)
		switch target {
		case FileDropSchedule:
			switch kind {
			case DroppedMedia, DroppedPresentation:
				if plan.Schedule != "" {
					skip(SkipWithSchedule)
					continue
				}
				withSchedule = append(withSchedule, path)
				plan.Insert = append(plan.Insert, DroppedFile{Path: path, Kind: kind})
				if kind == DroppedMedia {
					plan.Media = append(plan.Media, path)
				} else {
					plan.Presentations = append(plan.Presentations, path)
				}
			case DroppedSchedule:
				if plan.Schedule != "" {
					skip(SkipOneSchedule)
					continue
				}
				plan.Schedule = path
			case DroppedBible:
				plan.Bibles = append(plan.Bibles, path)
			case DroppedFolder:
				skip(SkipFolder)
			default:
				skip(SkipUnsupported)
			}
		case FileDropMedia:
			if kind == DroppedMedia || kind == DroppedFolder {
				plan.Media = append(plan.Media, path)
			} else {
				skip(SkipUnsupported)
			}
		case FileDropPresentations:
			if kind == DroppedPresentation || kind == DroppedFolder {
				plan.Presentations = append(plan.Presentations, path)
			} else {
				skip(SkipUnsupported)
			}
		default:
			skip(SkipOutside)
		}
	}
	if plan.Schedule != "" && len(withSchedule) > 0 {
		// The schedule came after resources: they are pre-empted as well.
		var kept []SkippedFile
		for _, p := range withSchedule {
			kept = append(kept, SkippedFile{Path: p, Reason: SkipWithSchedule})
		}
		plan.Skipped = append(kept, plan.Skipped...)
		plan.Insert, plan.Media, plan.Presentations = nil, nil, nil
	}
	return plan
}

// FileDropInsertAt is the schedule index a drop inserts at: the insertion
// gap under the pointer (gap >= 0, clamped to 0..n), else after the selected
// row, else at the end.
func FileDropInsertAt(gap, selected, n int) int {
	switch {
	case gap >= 0:
		return min(gap, n)
	case selected >= 0 && selected < n:
		return selected + 1
	}
	return n
}

// maxNamedSkips is how many file names a skip message names per reason;
// the rest are counted ("and 3 more"). The message dialog is a fixed size.
const maxNamedSkips = 3

// FileDropMessage is the one message about the skipped files of a drop on
// target ("" when nothing was skipped): a line per reason, naming the files.
func FileDropMessage(target string, skipped []SkippedFile) string {
	if len(skipped) == 0 {
		return ""
	}
	order := []string{SkipOutside, SkipUnsupported, SkipFolder, SkipMissing, SkipOneSchedule, SkipWithSchedule}
	byReason := map[string][]string{}
	for _, s := range skipped {
		byReason[s.Reason] = append(byReason[s.Reason], filepath.Base(s.Path))
	}
	var lines []string
	for _, reason := range order {
		names := byReason[reason]
		if len(names) == 0 {
			continue
		}
		lines = append(lines, namesList(names)+": "+skipReasonText(target, reason, len(names))+".")
	}
	return strings.Join(lines, "\n")
}

// namesList joins up to maxNamedSkips names and counts the rest.
func namesList(names []string) string {
	if len(names) <= maxNamedSkips {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:maxNamedSkips], ", "), len(names)-maxNamedSkips)
}

func skipReasonText(target, reason string, n int) string {
	switch reason {
	case SkipOutside:
		return "drop files onto the schedule or the Media or Presentations tab"
	case SkipMissing:
		return "not found"
	case SkipFolder:
		return "folders go onto the Media or Presentations tab"
	case SkipOneSchedule:
		return "one schedule opens at a time"
	case SkipWithSchedule:
		return "not added, the dropped schedule replaces the current one"
	}
	what := "not"
	if n > 1 {
		what = "none of these is"
	}
	switch target {
	case FileDropMedia:
		return what + " an image, video or audio file"
	case FileDropPresentations:
		return what + " a PDF, PowerPoint or Keynote file"
	}
	return what + " a media file, presentation, schedule or Bible package"
}

// FileDropSummary is the log line (and state) for a planned drop.
func FileDropSummary(p FileDropPlan) string {
	return fmt.Sprintf("%s: %d media, %d presentations, %d to insert, schedule %q, %d Bible packages, %d skipped",
		p.Target, len(p.Media), len(p.Presentations), len(p.Insert), p.Schedule, len(p.Bibles), len(p.Skipped))
}

// FileDropState is the last file drop, for GET /state.
type FileDropState struct {
	Target   string   `json:"target"`   // schedule | media | presentations | "" (outside)
	At       int      `json:"at"`       // schedule drops: the insert index; -1 otherwise
	Inserted []string `json:"inserted"` // the schedule items it inserted, in order
	Skipped  []string `json:"skipped"`  // file names it skipped
	Done     bool     `json:"done"`     // the libraries were updated and the items inserted
}

// PendingPresentationItem is the schedule item for a dropped presentation
// whose pages are not rendered yet: a "Preparing slides…" placeholder and
// no cache key, which FillPendingPresentation recognises.
func PendingPresentationItem(p models.Presentation) models.ScheduleItem {
	item := PresentationItem(p, nil)
	item.CacheKey = ""
	item.Slides = []models.Slide{placeholderSlide()}
	return item
}

// FillPendingPresentation gives the pending schedule items of document p
// (PendingPresentationItem: same source, no cache key) the rendered pages.
// It returns the indices it changed. An item's id, title and theme stay.
func FillPendingPresentation(items []models.ScheduleItem, p models.Presentation, pages []string) []int {
	if len(pages) == 0 || p.Path == "" {
		return nil
	}
	var changed []int
	for i := range items {
		it := &items[i]
		if it.Kind != models.ItemPresentation || it.Source != p.Path || it.CacheKey != "" {
			continue
		}
		full := PresentationItem(p, pages)
		it.PresentationID, it.CacheKey, it.Subtitle, it.Slides = full.PresentationID, full.CacheKey, full.Subtitle, full.Slides
		changed = append(changed, i)
	}
	return changed
}
