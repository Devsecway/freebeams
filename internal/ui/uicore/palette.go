// Package uicore holds the pure-Go parts of the operator UI: the EasyWorship
// palette, gradient rasterisation, splitter / grid / letterbox geometry and the
// search debouncer. It imports no graphics.gd class packages (only the pure-Go
// variant/Color type) so it is unit-tested with plain `go test`.
package uicore

import "graphics.gd/variant/Color"

// Hex converts 0xRRGGBB to an opaque color.
func Hex(rgb uint32) Color.RGBA {
	return Color.RGBA{
		R: float32((rgb>>16)&0xFF) / 255,
		G: float32((rgb>>8)&0xFF) / 255,
		B: float32(rgb&0xFF) / 255,
		A: 1,
	}
}

// HexA converts 0xAARRGGBB (the FireMonkey `xAARRGGBB` notation) to a color.
func HexA(argb uint32) Color.RGBA {
	c := Hex(argb & 0xFFFFFF)
	c.A = float32((argb>>24)&0xFF) / 255
	return c
}

// WithAlpha returns c with its alpha replaced.
func WithAlpha(c Color.RGBA, a float32) Color.RGBA {
	c.A = a
	return c
}

// Stop is one gradient stop (offset 0..1, top to bottom).
type Stop struct {
	Offset float32
	Color  Color.RGBA
}

// Gradient is a vertical (top → bottom) gradient.
type Gradient []Stop

// Flat returns a one-color gradient.
func Flat(c Color.RGBA) Gradient { return Gradient{{0, c}, {1, c}} }

// Lin returns an evenly spaced gradient through the given colors.
func Lin(colors ...Color.RGBA) Gradient {
	if len(colors) == 1 {
		return Flat(colors[0])
	}
	g := make(Gradient, len(colors))
	for i, c := range colors {
		g[i] = Stop{Offset: float32(i) / float32(len(colors)-1), Color: c}
	}
	return g
}

// Palette is the full set of EasyWorship interface colors (docs/UI-SPEC.md §1).
// Field names follow the spec's token names.
type Palette struct {
	Name string // "dark" or "light"

	Bk0, Bk1, Bk2, Bk3   Color.RGBA // window.bk0..3
	WindowBorder         Color.RGBA // window.border
	Text                 Color.RGBA // text
	TextSemibold         Color.RGBA // text.semibold (panel titles)
	TextHyperlink        Color.RGBA // text.hyperlink
	Highlight            Color.RGBA // highlight (active selection)
	HighlightHot         Color.RGBA // highlight.hot
	HighlightText        Color.RGBA // highlight.text
	HighlightInactive    Color.RGBA // highlight.inactive
	HighlightInactiveHot Color.RGBA // highlight.inactive.hot
	HighlightTextInact   Color.RGBA // highlight.text.inactive
	Highlight2           Color.RGBA // highlight.2 (secondary selection)
	Highlight2Hot        Color.RGBA // highlight.2.hot
	Highlight2Text       Color.RGBA // highlight.2.text
	ButtonBk             Color.RGBA // button.bk
	SplitterBar          Color.RGBA // splitterbar
	Border               Color.RGBA // border
	BorderBL             Color.RGBA // border.bl
	BorderTL             Color.RGBA // border.tl
	SelectorLine         Color.RGBA // selectorline
	HeaderColumnStroke   Color.RGBA // headercolumn.stroke
	LibraryBk            Color.RGBA // libraryselector.bk
	PresEditor           Color.RGBA // preseditor
	DividerMenu          Color.RGBA // divider.menu
	DividerLeading       Color.RGBA // divider.button.leading
	DividerTrailing      Color.RGBA // divider.button.trailing
	DividerMiniLeading   Color.RGBA // divider.button.mini.leading
	DividerMiniTrailing  Color.RGBA // divider.button.mini.trailing
	MenuFill             Color.RGBA // menuview.fill
	MenuStroke           Color.RGBA // menuview.stroke
	MenuItemFill         Color.RGBA // menuitem.fill
	MenuGutter           Color.RGBA // menugutter.fill
	TabDividerStroke     Color.RGBA // tabbardivider.stroke
	TabDividerFill       Color.RGBA // tabbardivider.fill
	ScrollThumb          Color.RGBA // scrollbar.thumb.fill
	InactiveOverlay      Color.RGBA // inactive selection overlay
	Red                  Color.RGBA // live indicator / errors

	Toolbar            Gradient // toolbar (main 58px toolbar)
	ToolbarMini        Gradient // toolbar.mini (28px panel headers/footers)
	ToolbarMini2       Gradient // toolbar.mini2
	TabBar1            Gradient // tabbar.1 (resource tab bar)
	TabBar2            Gradient // tabbar.2 (dialog tab bars)
	HSplitterClosed    Gradient // hsplitterbar.closed
	VSplitterClosed    Gradient // vsplitterbar.closed (left → right)
	GutterFill         Gradient // gutter.fill (trackbar)
	GutterProgress     Gradient // gutter.progress.highlight
	PlayerButton       Gradient // "mini button normal" in the spec (player buttons)
	PlayerButtonHot    Gradient
	PlayerButtonSelect Gradient
}

// IsDark reports whether this is the dark interface.
func (p Palette) IsDark() bool { return p.Name != "light" }

// DarkPalette is EasyWorship's default (icDark) interface color.
func DarkPalette() Palette {
	return Palette{
		Name:                 "dark",
		Bk0:                  Hex(0x202020),
		Bk1:                  Hex(0x2C2C2C),
		Bk2:                  Hex(0x363636),
		Bk3:                  Hex(0x484848),
		WindowBorder:         Hex(0x000000),
		Text:                 Hex(0xB9B9B9),
		TextSemibold:         Hex(0xF0F0F0),
		TextHyperlink:        Hex(0x699AF9),
		Highlight:            Hex(0x3A6DCE),
		HighlightHot:         Hex(0x4B8CE9),
		HighlightText:        Hex(0xF9F9F9),
		HighlightInactive:    Hex(0x626262),
		HighlightInactiveHot: Hex(0x707070),
		HighlightTextInact:   Hex(0xC2C2C2),
		Highlight2:           Hex(0x395792),
		Highlight2Hot:        Hex(0x436AA2),
		Highlight2Text:       Hex(0xB9B9B9),
		ButtonBk:             Hex(0x2C2C2C),
		SplitterBar:          Hex(0x000000),
		Border:               Hex(0x000000),
		BorderBL:             Hex(0x101010),
		BorderTL:             Hex(0x000000),
		SelectorLine:         Hex(0x4D4D4D),
		HeaderColumnStroke:   Hex(0x000000),
		LibraryBk:            Hex(0x202020),
		PresEditor:           Hex(0x1F1F1F),
		DividerMenu:          Hex(0x5D5D5D),
		DividerLeading:       Hex(0x242424),
		DividerTrailing:      Hex(0x8F8F8F),
		DividerMiniLeading:   Hex(0x000000),
		DividerMiniTrailing:  Hex(0x363636),
		MenuFill:             Hex(0x363636),
		MenuStroke:           Hex(0x000000),
		MenuItemFill:         Hex(0x395792),
		MenuGutter:           Hex(0x303030),
		TabDividerStroke:     Hex(0x000000),
		TabDividerFill:       Hex(0x606060),
		ScrollThumb:          HexA(0xE6828282),
		InactiveOverlay:      WithAlpha(Hex(0x828282), 0.9),
		Red:                  Hex(0xFF0000),

		Toolbar:            Gradient{{0, Hex(0x6E6E6E)}, {0.9938, Hex(0x4C4C4C)}},
		ToolbarMini:        Gradient{{0.0031, Hex(0x343434)}, {0.9938, Hex(0x292929)}},
		ToolbarMini2:       Flat(Hex(0x363636)),
		TabBar1:            Lin(Hex(0x6E6E6E), Hex(0x424242)),
		TabBar2:            Lin(Hex(0x2C2C2C), Hex(0x4C4C4C), Hex(0x4C4C4C), Hex(0x404040)),
		HSplitterClosed:    Lin(Hex(0x555555), Hex(0x2C2C2C)),
		VSplitterClosed:    Lin(Hex(0x342F2F), Hex(0x2C2C2C)),
		GutterFill:         Gradient{{0, Hex(0x262626)}, {0.1491, Hex(0x262626)}, {0.3012, Hex(0x343434)}, {0.9814, Hex(0x4C4C4C)}},
		GutterProgress:     Lin(Hex(0x66ACFF), Hex(0x2B5CAF)),
		PlayerButton:       Gradient{{0, Hex(0x515151)}, {0.5466, Hex(0x3D3D3D)}, {1, Hex(0x424242)}},
		PlayerButtonHot:    Gradient{{0, Hex(0x606060)}, {0.4845, Hex(0x4A4A4A)}, {1, Hex(0x505050)}},
		PlayerButtonSelect: Gradient{{0, Hex(0x363636)}, {0.4752, Hex(0x303030)}, {0.9969, Hex(0x363636)}},
	}
}

// LightPalette is EasyWorship's Light interface color.
func LightPalette() Palette {
	return Palette{
		Name:                 "light",
		Bk0:                  Hex(0xF9F9F9),
		Bk1:                  Hex(0xF9F9F9),
		Bk2:                  Hex(0xF0F0F0),
		Bk3:                  Hex(0xD0D0D0),
		WindowBorder:         Hex(0x646464),
		Text:                 Hex(0x2A2A2A),
		TextSemibold:         Hex(0x000000),
		TextHyperlink:        Hex(0x529AEF),
		Highlight:            Hex(0x529AEF),
		HighlightHot:         Hex(0x62AAFF),
		HighlightText:        Hex(0xF9F9F9),
		HighlightInactive:    Hex(0xCECECE),
		HighlightInactiveHot: Hex(0xDCDCDC),
		HighlightTextInact:   Hex(0x2A2A2A),
		Highlight2:           Hex(0xCCE4F7),
		Highlight2Hot:        Hex(0xE5F1FB),
		Highlight2Text:       Hex(0x2A2A2A),
		ButtonBk:             Hex(0xE1E1E1),
		SplitterBar:          Hex(0xAAAAAA),
		Border:               Hex(0xAAAAAA),
		BorderBL:             Hex(0xD4D4D4),
		BorderTL:             Hex(0x8C8C8C),
		SelectorLine:         Hex(0xD0D0D0),
		HeaderColumnStroke:   Hex(0xD0D0D0),
		LibraryBk:            Hex(0xDEDEE7),
		PresEditor:           Hex(0xD0D0D0),
		DividerMenu:          Hex(0xD7D7D7),
		DividerLeading:       Hex(0xA0A0A0),
		DividerTrailing:      Hex(0xFFFFFF),
		DividerMiniLeading:   Hex(0xA0A0A0),
		DividerMiniTrailing:  Hex(0xFFFFFF),
		MenuFill:             Hex(0xF0F0F0),
		MenuStroke:           Hex(0xAAAAAA),
		MenuItemFill:         Hex(0xCCE4F7),
		MenuGutter:           Hex(0xE6E6E6),
		TabDividerStroke:     Hex(0x8C8C8C),
		TabDividerFill:       Hex(0xE3E3E3),
		ScrollThumb:          HexA(0x80000000),
		InactiveOverlay:      WithAlpha(Hex(0x000000), 0.5),
		Red:                  Hex(0xFF0000),

		Toolbar:            Lin(Hex(0xF7F3F7), Hex(0xCECFCE)),
		ToolbarMini:        Gradient{{0, Hex(0xE5E5E5)}, {0.9938, Hex(0xF4F4F4)}},
		ToolbarMini2:       Lin(Hex(0xE5E5E5), Hex(0xF4F4F4)),
		TabBar1:            Lin(Hex(0xF7F7F7), Hex(0xCECECE)),
		TabBar2:            Lin(Hex(0xCCCCCC), Hex(0xECECEC), Hex(0xECECEC), Hex(0xE0E0E0)),
		HSplitterClosed:    Lin(Hex(0xF9F9F9), Hex(0xC8C8C8)),
		VSplitterClosed:    Lin(Hex(0xF9F9F9), Hex(0xC8C8C8)),
		GutterFill:         Lin(Hex(0xBDBDBD), Hex(0xE7E7E7)),
		GutterProgress:     Lin(Hex(0x66ACFF), Hex(0x2B5CAF)),
		PlayerButton:       Gradient{{0.0093, Hex(0xE6E6E6)}, {0.5217, Hex(0xD2D2D2)}, {1, Hex(0xE6E6E6)}},
		PlayerButtonHot:    Gradient{{0, Hex(0xF0F0F0)}, {0.472, Hex(0xE6E6E6)}, {0.9938, Hex(0xF0F0F0)}},
		PlayerButtonSelect: Gradient{{0.0031, Hex(0xDCDCDC)}, {0.4845, Hex(0xC8C8C8)}, {1, Hex(0xDCDCDC)}},
	}
}
