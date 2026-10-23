package models

// HAlign / VAlign position text inside the theme's text box.
type HAlign string
type VAlign string

const (
	AlignLeft   HAlign = "left"
	AlignCenter HAlign = "center"
	AlignRight  HAlign = "right"

	AlignTop    VAlign = "top"
	AlignMiddle VAlign = "middle"
	AlignBottom VAlign = "bottom"
)

// Margins in canvas units (see CanvasWidth/CanvasHeight).
type Margins struct {
	Left, Top, Right, Bottom float32
}

// Font describes the main slide text. Size is the maximum size; the renderer
// shrinks text down to MinSize to make it fit the text box (auto-fit).
type Font struct {
	Family  string  `json:"family"` // empty = platform default
	Size    float32 `json:"size"`
	MinSize float32 `json:"minSize"`
	Color   RGBA    `json:"color"`
	Bold    bool    `json:"bold"`
	Italic  bool    `json:"italic"`
	// LineSpacing is extra spacing between lines as a fraction of Size.
	LineSpacing float32 `json:"lineSpacing"`
}

// Outline around glyphs.
type Outline struct {
	Enabled bool    `json:"enabled"`
	Size    float32 `json:"size"`
	Color   RGBA    `json:"color"`
}

// Shadow behind glyphs.
type Shadow struct {
	Enabled bool    `json:"enabled"`
	OffsetX float32 `json:"offsetX"`
	OffsetY float32 `json:"offsetY"`
	Blur    float32 `json:"blur"`
	Color   RGBA    `json:"color"`
}

// Theme is the look applied to slides.
type Theme struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Background Background `json:"background"`
	Font       Font       `json:"font"`
	Outline    Outline    `json:"outline"`
	Shadow     Shadow     `json:"shadow"`
	HAlign     HAlign     `json:"hAlign"`
	VAlign     VAlign     `json:"vAlign"`
	// Margins define the text box inside the canvas.
	Margins Margins `json:"margins"`
	// FooterSize / FooterColor style Slide.Footer (bottom of the canvas, inside margins).
	FooterSize  float32 `json:"footerSize"`
	FooterColor RGBA    `json:"footerColor"`

	// Custom marks an item's own copy that was restyled from the saved theme
	// it is based on (ID, Name): the Preview Output menu's Background ▸ /
	// Text ▸ changes. A custom copy keeps its look when the saved theme is
	// edited; library themes never have it (store.SaveTheme clears it).
	Custom bool `json:"custom,omitempty"`

	// Box fills the text box (the Margins rect) with a translucent band
	// behind the words: a lower third over a video, or legibility over a
	// busy image. The zero value draws nothing.
	Box TextBox `json:"box,omitzero"`
}

// TextBox is a theme's band behind the text. Padding (canvas units) grows
// the band beyond the margins rect on every side; FullWidth runs it from
// the left edge of the canvas to the right one.
type TextBox struct {
	Enabled   bool    `json:"enabled"`
	Color     RGBA    `json:"color"`
	Padding   float32 `json:"padding"`
	FullWidth bool    `json:"fullWidth,omitempty"`
}

// SameLook reports whether a and b draw the same: every field but the ID,
// the Name and Custom.
func (t Theme) SameLook(o Theme) bool {
	t.ID, t.Name, t.Custom = 0, "", false
	o.ID, o.Name, o.Custom = 0, "", false
	return t == o
}

// DefaultSongTheme returns the built-in theme for songs.
func DefaultSongTheme() Theme {
	return Theme{
		Name: "Default Song",
		Background: Background{
			Kind:   BackgroundGradient,
			Color:  Hex(0x0B1D3A),
			Color2: Hex(0x16222A),
		},
		Font: Font{
			Size:        96,
			MinSize:     36,
			Color:       Hex(0xFFFFFF),
			Bold:        true,
			LineSpacing: 0.1,
		},
		Outline:     Outline{Enabled: true, Size: 4, Color: RGBA{0, 0, 0, 1}},
		Shadow:      Shadow{Enabled: true, OffsetX: 4, OffsetY: 4, Blur: 6, Color: RGBA{0, 0, 0, 0.8}},
		HAlign:      AlignCenter,
		VAlign:      AlignMiddle,
		Margins:     Margins{Left: 96, Top: 72, Right: 96, Bottom: 96},
		FooterSize:  28,
		FooterColor: RGBA{1, 1, 1, 0.85},
	}
}

// DefaultScriptureTheme returns the built-in theme for scripture.
func DefaultScriptureTheme() Theme {
	t := DefaultSongTheme()
	t.Name = "Default Scripture"
	t.Background = Background{Kind: BackgroundGradient, Color: Hex(0x1E1E1E), Color2: Hex(0x3A3A3A)}
	t.Font.Size = 80
	t.Font.Bold = false
	t.HAlign = AlignLeft
	t.FooterSize = 36
	return t
}
