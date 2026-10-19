package core

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"freebeams/internal/models"
)

// The library tile menus (the Media and Presentations tabs' right-click and
// gear menus, UI-SPEC §3.6): the View ▸ and Sort by ▸ preferences and their
// setting keys, the item sorts, the logo backgrounds items provide, the
// Refresh file check, Update items in Schedule and Open File Location. The
// events live in app/menus_items.go.

// Setting keys of the tile menus' View ▸ and Sort by ▸ choices.
const (
	SettingMediaView         = "ui.media.view"         // ItemView
	SettingMediaSort         = "ui.media.sort"         // ItemSort
	SettingPresentationsView = "ui.presentations.view" // ItemView
	SettingPresentationsSort = "ui.presentations.sort" // ItemSort
)

// ItemView is a library grid's View ▸ choice: a thumbnail size, or the
// Details list (one row per item with a small thumbnail).
type ItemView string

const (
	ViewLarge   ItemView = "large"
	ViewMedium  ItemView = "medium" // the default
	ViewSmall   ItemView = "small"
	ViewDetails ItemView = "details"
)

// ItemViews lists the views in menu order.
var ItemViews = []ItemView{ViewLarge, ViewMedium, ViewSmall, ViewDetails}

// Valid reports whether v is a known view.
func (v ItemView) Valid() bool {
	return v == ViewLarge || v == ViewMedium || v == ViewSmall || v == ViewDetails
}

// Normalized returns v, or ViewMedium for "" and unknown values.
func (v ItemView) Normalized() ItemView {
	if v.Valid() {
		return v
	}
	return ViewMedium
}

// ParseItemView reads "large" / "medium" / "small" / "details" (case-insensitive).
func ParseItemView(s string) (ItemView, error) {
	v := ItemView(strings.ToLower(strings.TrimSpace(s)))
	if !v.Valid() {
		return "", fmt.Errorf("unknown view %q (large|medium|small|details)", s)
	}
	return v, nil
}

// ItemSort is a library grid's Sort by ▸ choice.
type ItemSort string

const (
	SortName ItemSort = "name" // title, A to Z (the default; the store's order)
	SortDate ItemSort = "date" // date added, newest first
	SortKind ItemSort = "kind" // kind (images before videos; PDF, PowerPoint, …), then title
)

// ItemSorts lists the sorts in menu order.
var ItemSorts = []ItemSort{SortName, SortDate, SortKind}

// Valid reports whether s is a known sort.
func (s ItemSort) Valid() bool { return s == SortName || s == SortDate || s == SortKind }

// Normalized returns s, or SortName for "" and unknown values.
func (s ItemSort) Normalized() ItemSort {
	if s.Valid() {
		return s
	}
	return SortName
}

// ParseItemSort reads "name" / "date" / "kind" (case-insensitive).
func ParseItemSort(s string) (ItemSort, error) {
	v := ItemSort(strings.ToLower(strings.TrimSpace(s)))
	if !v.Valid() {
		return "", fmt.Errorf("unknown sort %q (name|date|kind)", s)
	}
	return v, nil
}

// lessTitle orders titles case-insensitively, then by id (the store's order).
func lessTitle(a, b string, idA, idB int64) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	if la != lb {
		return la < lb
	}
	return idA < idB
}

// SortMedia orders list in place by sort (stable) and returns it: by title,
// by date added (newest first), or by kind (images before videos) then title.
func SortMedia(list []models.Media, by ItemSort) []models.Media {
	by = by.Normalized()
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		switch by {
		case SortDate:
			if !a.Added.Equal(b.Added) {
				return a.Added.After(b.Added)
			}
		case SortKind:
			if a.Kind != b.Kind {
				return a.Kind < b.Kind
			}
		}
		return lessTitle(a.Title, b.Title, a.ID, b.ID)
	})
	return list
}

// PresentationKind names a document's type by its extension ("PDF",
// "PowerPoint", "Keynote", "OpenDocument"; the upper-cased extension
// otherwise), for Sort by ▸ Kind.
func PresentationKind(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".pdf":
		return "PDF"
	case ".pptx", ".ppt":
		return "PowerPoint"
	case ".key":
		return "Keynote"
	case ".odp":
		return "OpenDocument"
	}
	return strings.ToUpper(strings.TrimPrefix(ext, "."))
}

// SortPresentations orders list in place by sort (stable) and returns it:
// by title, by date added (newest first), or by kind (PresentationKind) then title.
func SortPresentations(list []models.Presentation, by ItemSort) []models.Presentation {
	by = by.Normalized()
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		switch by {
		case SortDate:
			if !a.Added.Equal(b.Added) {
				return a.Added.After(b.Added)
			}
		case SortKind:
			if ka, kb := PresentationKind(a.Path), PresentationKind(b.Path); ka != kb {
				return ka < kb
			}
		}
		return lessTitle(a.Title, b.Title, a.ID, b.ID)
	})
	return list
}

// MediaLogo is the logo background a media item provides (Use As Logo
// Background): the item's own background (image or video, with its aspect
// ratio) over the logo gradient's color.
func MediaLogo(m models.Media) models.Background {
	bg := m.Background()
	bg.Color = LogoGradient().Color
	return bg
}

// PresentationLogo is the logo background a presentation provides: its first
// page image. ok is false until the presentation has been rendered.
func PresentationLogo(p models.Presentation, cacheDir string) (bg models.Background, ok bool) {
	pages := PresentationPages(p, cacheDir)
	if len(pages) == 0 {
		return models.Background{}, false
	}
	return models.Background{Kind: models.BackgroundImage, Path: pages[0], Color: LogoGradient().Color}, true
}

// FileMissing reports whether path is not a regular file any more.
func FileMissing(path string) bool {
	fi, err := os.Stat(path)
	return err != nil || !fi.Mode().IsRegular()
}

// MissingMedia returns the items whose source files are gone (Refresh).
func MissingMedia(list []models.Media) []models.Media {
	var out []models.Media
	for _, m := range list {
		if FileMissing(m.Path) {
			out = append(out, m)
		}
	}
	return out
}

// MissingPresentations returns the presentations whose documents are gone (Refresh).
func MissingPresentations(list []models.Presentation) []models.Presentation {
	var out []models.Presentation
	for _, p := range list {
		if FileMissing(p.Path) {
			out = append(out, p)
		}
	}
	return out
}

// RefreshQuestion words the Refresh confirmation for n missing files. It is
// short enough for the confirm dialog (a heading line plus one sentence).
func RefreshQuestion(n int, what string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s file is missing from disk.\nRemove it from the library?", what)
	}
	return fmt.Sprintf("%d %s files are missing from disk.\nRemove them from the library?", n, what)
}

// UpdateMediaItems rewrites the schedule's items for media m (matched by
// MediaID) from the library's current row (Update items in Schedule): the
// title, subtitle and slide (aspect ratio, repeat, poster). It returns how
// many changed and marks the schedule dirty when any did.
func UpdateMediaItems(s *models.Schedule, m models.Media) int {
	if s == nil || m.ID == 0 {
		return 0
	}
	fresh := MediaItem(m)
	n := 0
	for i := range s.Items {
		it := &s.Items[i]
		if it.Kind != models.ItemMedia || it.MediaID != m.ID {
			continue
		}
		if it.Title == fresh.Title && it.Subtitle == fresh.Subtitle && reflect.DeepEqual(it.Slides, fresh.Slides) {
			continue
		}
		it.Title, it.Subtitle, it.Slides = fresh.Title, fresh.Subtitle, cloneSlides(fresh.Slides)
		n++
	}
	if n > 0 {
		s.Dirty = true
	}
	return n
}

// RevealCommand is the command that shows a file in the platform's file
// manager (Open File Location): Finder on macOS (`open -R`), Explorer on
// Windows (`explorer /select,`), the folder through xdg-open elsewhere.
func RevealCommand(path, goos string) (name string, args []string) {
	switch goos {
	case "darwin":
		return "open", []string{"-R", path}
	case "windows":
		return "explorer", []string{"/select," + path}
	}
	return "xdg-open", []string{filepath.Dir(path)}
}
