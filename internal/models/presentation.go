package models

import (
	"path/filepath"
	"strings"
	"time"
)

// presentationExts are the document types the Presentations tab accepts
// (lower case, with the dot). They are rendered to page images by
// internal/presentations; which of them can be rendered depends on the
// tools installed (Core Graphics / pdftoppm for PDF, LibreOffice for the
// others, with an embedded-image fallback for PowerPoint and Keynote).
var presentationExts = map[string]bool{".pdf": true, ".pptx": true, ".ppt": true, ".key": true, ".odp": true}

// IsPresentationFile reports whether path has a presentation extension.
func IsPresentationFile(path string) bool {
	return presentationExts[strings.ToLower(filepath.Ext(path))]
}

// Presentation is one item of the presentation library: a document on disk
// and what the app rendered from it (its pages as images in the cache).
type Presentation struct {
	ID    int64  `json:"id"`
	Path  string `json:"path"`  // absolute path of the document
	Title string `json:"title"` // defaults to MediaTitle(Path)
	// Pages is the number of rendered page images; 0 until rendered.
	Pages int `json:"pages"`
	// CacheKey names the cache folder holding the page images (see
	// presentations.Key); "" until rendered.
	CacheKey string `json:"cacheKey,omitempty"`
	// Renderer is what made the pages ("coregraphics", "pdftoppm",
	// "libreoffice", "embedded"); "" until rendered.
	Renderer string `json:"renderer,omitempty"`
	// Partial is set when the pages are the document's embedded images only
	// (no renderer could draw the slides), so text and shapes are missing.
	Partial bool      `json:"partial,omitempty"`
	Added   time.Time `json:"added"`
}

// Rendered reports whether the presentation has page images.
func (p Presentation) Rendered() bool { return p.Pages > 0 && p.CacheKey != "" }

// DefaultPresentationTheme is the theme presentation pages are shown with: a
// plain black background that the page image is letterboxed on.
func DefaultPresentationTheme() Theme {
	t := DefaultSongTheme()
	t.Name = "Presentation"
	t.Background = Background{Kind: BackgroundColor, Color: RGBA{A: 1}}
	return t
}
