// Package models holds FreeWorship's domain types and pure logic. It must not
// import graphics.gd; everything here is unit-testable with plain `go test`.
package models

// CanvasWidth and CanvasHeight define the logical slide canvas. Theme sizes
// (font size, margins, outline, shadow offsets) are expressed in these units
// and scaled by the renderer to the actual output resolution.
const (
	CanvasWidth  = 1920
	CanvasHeight = 1080
)

// RGBA is a color with components in [0,1].
type RGBA struct {
	R, G, B, A float32
}

// Hex builds an opaque RGBA from 0xRRGGBB.
func Hex(rgb uint32) RGBA {
	return RGBA{
		R: float32((rgb>>16)&0xFF) / 255,
		G: float32((rgb>>8)&0xFF) / 255,
		B: float32(rgb&0xFF) / 255,
		A: 1,
	}
}

// BackgroundKind selects how a slide background is drawn.
type BackgroundKind string

const (
	BackgroundColor    BackgroundKind = "color"
	BackgroundGradient BackgroundKind = "gradient" // vertical, Color (top) -> Color2 (bottom)
	BackgroundImage    BackgroundKind = "image"    // Path, scaled by Fit (cover by default)
	BackgroundVideo    BackgroundKind = "video"    // Path plays where the platform can (else Poster, else Color), scaled by Fit
	BackgroundAudio    BackgroundKind = "audio"    // Path plays (sound only) over the Color
)

// Plays reports whether the background kind is media that plays over time.
func (k BackgroundKind) Plays() bool { return k == BackgroundVideo || k == BackgroundAudio }

// Background describes a slide or theme background.
type Background struct {
	Kind   BackgroundKind `json:"kind"`
	Color  RGBA           `json:"color"`
	Color2 RGBA           `json:"color2,omitempty"`
	Path   string         `json:"path,omitempty"`
	// Poster is a still shown in place of a video that cannot play (no
	// transcode yet, or the file is missing).
	Poster string `json:"poster,omitempty"`

	// Fit is how an image or video is scaled to the canvas (the media item's
	// Aspect Ratio); "" is AspectFill. Loop makes a video or audio background
	// start again when it ends (the media item's Repeating). It is the item's
	// default: the operator can switch looping on and off while it plays (the
	// transport bar). A snapshot saved without it does not loop.
	Fit  MediaAspect `json:"fit,omitempty"`
	Loop bool        `json:"loop,omitempty"`
}

// SlideKind tells the renderer and UI what produced a slide.
type SlideKind string

const (
	SlideSong      SlideKind = "song"
	SlideScripture SlideKind = "scripture"
	SlideBlank     SlideKind = "blank"
	SlideMedia     SlideKind = "media" // a full-screen image or video, no text
	// SlidePresentation is one page of a PDF / PowerPoint deck, rendered to
	// an image (internal/presentations); no text.
	SlidePresentation SlideKind = "presentation"
)

// Slide is the unit of display. It is plain data: the renderer turns a Slide
// plus a Theme into nodes, and the same code draws thumbnails, preview and the
// output so what you preview is exactly what goes live.
type Slide struct {
	Kind SlideKind `json:"kind"`
	// Label is the short tag shown under thumbnails ("Verse 1", "Chorus", "John 3:16").
	Label string `json:"label"`
	// Group identifies the section the slide belongs to, used for section jump
	// hotkeys. For songs it is the section's Kind; empty otherwise.
	Group SectionKind `json:"group,omitempty"`
	// GroupNumber is the section number (Verse 2 -> 2); 0 when not numbered.
	GroupNumber int `json:"groupNumber,omitempty"`
	// Text is the main body; lines are separated by "\n".
	Text string `json:"text"`
	// Footer is small text at the bottom (copyright line, scripture reference).
	Footer string `json:"footer,omitempty"`
	// Background, when non-nil, overrides the theme background for this slide.
	Background *Background `json:"background,omitempty"`
}
