package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"freebeams/internal/models"
	"freebeams/internal/presentations"
)

// Setting key for the one-off registration of EasyWorship's presentation
// files (Resources/Presentations) on first launch with the Presentations tab.
const SettingPresentationsImportDone = "import.easyworship.presentations.done"

// PresentationCaps is what the machine can render.
type PresentationCaps struct {
	PDF    bool // a PDF renderer exists (Core Graphics or pdftoppm)
	Office bool // LibreOffice converts PowerPoint / Keynote / OpenDocument faithfully
}

// PresentationPages lists the page images of a rendered presentation in
// the cache (nil when it has not been rendered).
func PresentationPages(p models.Presentation, cacheDir string) []string {
	if !p.Rendered() || cacheDir == "" {
		return nil
	}
	return presentations.Pages(cacheDir, p.CacheKey, p.Pages)
}

// PresentationSlides turns page images into slides: one full-screen image
// per page, labelled "Slide N".
func PresentationSlides(pages []string) []models.Slide {
	slides := make([]models.Slide, 0, len(pages))
	for i, page := range pages {
		bg := models.Background{Kind: models.BackgroundImage, Color: models.RGBA{A: 1}, Path: page}
		slides = append(slides, models.Slide{Kind: models.SlidePresentation, Label: fmt.Sprintf("Slide %d", i+1), Background: &bg})
	}
	return slides
}

// placeholderSlide is what an unrendered presentation shows while its pages
// are being made.
func placeholderSlide() models.Slide {
	bg := models.Background{Kind: models.BackgroundColor, Color: models.Hex(0x1E1E22)}
	return models.Slide{Kind: models.SlidePresentation, Label: "Rendering…", Text: "Preparing slides…", Background: &bg}
}

// PresentationDeck builds the deck for a library presentation: one slide
// per page image, or a single placeholder slide while it renders. ItemID
// is empty (library content).
func PresentationDeck(p models.Presentation, pages []string) models.Deck {
	d := models.Deck{Title: p.Title, Theme: models.DefaultPresentationTheme(), Slides: PresentationSlides(pages)}
	if len(d.Slides) == 0 {
		d.Slides = []models.Slide{placeholderSlide()}
	}
	return d
}

// PresentationItem makes a schedule item for a rendered presentation: the
// page slides as a snapshot, with the document path and cache key so the
// pages can be found (or re-rendered) again later.
func PresentationItem(p models.Presentation, pages []string) models.ScheduleItem {
	return models.ScheduleItem{
		Kind:           models.ItemPresentation,
		Title:          p.Title,
		Subtitle:       PresentationSubtitle(p),
		PresentationID: p.ID,
		Source:         p.Path,
		CacheKey:       p.CacheKey,
		Theme:          models.DefaultPresentationTheme(),
		Slides:         PresentationSlides(pages),
	}
}

// PresentationSubtitle is the schedule row's second line: "Presentation ·
// 12 slides" (noting embedded-only renders).
func PresentationSubtitle(p models.Presentation) string {
	s := "Presentation"
	if p.Pages > 0 {
		s += " · " + plural(p.Pages, "slide", "slides")
	}
	if p.Partial {
		s += " · embedded images"
	}
	return s
}

// PageLookup finds the current page images of a document by its path (the
// library's render), for ResolvePresentationItem.
type PageLookup func(source string) (pages []string, cacheKey string, ok bool)

// ResolvePresentationItem returns item with its slides rebuilt from the
// library's current render of its source document when the snapshot's pages
// are stale (another cache key) or gone from disk. Other items, and items
// whose pages are present, are returned as is. The schedule itself is not
// changed.
func ResolvePresentationItem(item models.ScheduleItem, lookup PageLookup) models.ScheduleItem {
	if item.Kind != models.ItemPresentation || lookup == nil || item.Source == "" {
		return item
	}
	pages, key, ok := lookup(item.Source)
	if !ok || len(pages) == 0 {
		return item
	}
	if key == item.CacheKey && slidesPresent(item.Slides) {
		return item
	}
	item.CacheKey = key
	item.Slides = PresentationSlides(pages)
	return item
}

// slidesPresent reports whether every slide's background image exists.
func slidesPresent(slides []models.Slide) bool {
	if len(slides) == 0 {
		return false
	}
	for _, s := range slides {
		if s.Background == nil || s.Background.Kind != models.BackgroundImage {
			continue
		}
		if fi, err := os.Stat(s.Background.Path); err != nil || !fi.Mode().IsRegular() {
			return false
		}
	}
	return true
}

// PresentationThumb is what the grid draws for a presentation: its page-1
// thumbnail when rendered, else a plain dark tile.
func PresentationThumb(p models.Presentation, cacheDir string) models.Background {
	bg := models.Background{Kind: models.BackgroundColor, Color: models.Hex(0x2A2A2A)}
	if p.Rendered() && cacheDir != "" {
		bg.Kind, bg.Path = models.BackgroundImage, presentations.ThumbPath(cacheDir, p.CacheKey)
	}
	return bg
}

// RenderProgress is a render in progress (Active): done pages of total (0
// while the total is unknown).
type RenderProgress struct {
	Active      bool
	Done, Total int
}

// PresentationBadge is the short text on a tile's right: the slide count
// when rendered (noting embedded-only renders), the progress while
// rendering, or why it cannot render yet (failed: its render failed).
func PresentationBadge(p models.Presentation, prog RenderProgress, failed bool, caps PresentationCaps) string {
	switch {
	case prog.Active && prog.Total > 0 && !p.Rendered():
		return fmt.Sprintf("Rendering %d/%d", prog.Done, prog.Total)
	case prog.Active && !p.Rendered():
		return "Rendering…"
	case p.Rendered():
		s := plural(p.Pages, "slide", "slides")
		if p.Partial {
			s += " · embedded"
		}
		return s
	case failed:
		return "Unavailable"
	}
	ext := strings.ToLower(filepath.Ext(p.Path))
	switch {
	case ext == ".pdf" && !caps.PDF:
		return "Needs poppler"
	case ext != ".pdf" && ext != ".pptx" && ext != ".key" && !caps.Office:
		return "Needs LibreOffice"
	}
	return "Queued"
}

// PresentationsFooter is the tab's footer text: the count, what is being
// done, and what is holding documents back (office is how many listed
// documents are not PDFs).
func PresentationsFooter(count, office int, caps PresentationCaps, working bool) string {
	parts := []string{plural(count, "presentation", "presentations")}
	switch {
	case working:
		parts = append(parts, "rendering slides…")
	case count > 0 && !caps.PDF:
		parts = append(parts, "no PDF renderer: install poppler (pdftoppm)")
	case office > 0 && !caps.Office:
		parts = append(parts, "LibreOffice not found: PowerPoint shows its embedded images only")
	}
	return strings.Join(parts, " · ")
}

// OfficeCount counts the documents in list that are not PDFs.
func OfficeCount(list []models.Presentation) int {
	n := 0
	for _, p := range list {
		if !strings.EqualFold(filepath.Ext(p.Path), ".pdf") {
			n++
		}
	}
	return n
}

// FindPresentation returns the index of the item in list whose title
// matches title (exact, then prefix, then substring, case-insensitively),
// else the one whose path ends with title; -1 when none.
func FindPresentation(list []models.Presentation, title string) int {
	t := strings.ToLower(strings.TrimSpace(title))
	if t == "" {
		return -1
	}
	for _, match := range []func(string) bool{
		func(s string) bool { return s == t },
		func(s string) bool { return strings.HasPrefix(s, t) },
		func(s string) bool { return strings.Contains(s, t) },
	} {
		for i, p := range list {
			if match(strings.ToLower(p.Title)) {
				return i
			}
		}
	}
	for i, p := range list {
		if strings.HasSuffix(strings.ToLower(p.Path), t) {
			return i
		}
	}
	return -1
}

// PresentationIndexOf returns the index of the item with id in list, or -1.
func PresentationIndexOf(list []models.Presentation, id int64) int {
	for i, p := range list {
		if p.ID == id {
			return i
		}
	}
	return -1
}
