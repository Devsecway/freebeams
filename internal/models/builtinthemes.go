package models

// The professional built-in themes FreeBeams adds to a library next to
// "Default Song" and "Default Scripture" (store.EnsureBuiltinThemes). They
// are made of our own gradients and colours only (no licensed media), and
// each is tuned for legibility at the back of a room: high contrast between
// text and background, generous margins, a modest outline or none.

// BuiltinThemesVersion is bumped when a theme is added to BuiltinThemes, so
// an existing library is offered the new ones once.
const BuiltinThemesVersion = 1

// BuiltinThemes returns the built-in themes (ID 0), in display order.
func BuiltinThemes() []Theme {
	return []Theme{CleanDarkTheme(), WarmLightTheme(), HighContrastTheme(), LowerThirdTheme()}
}

// CleanDarkTheme is a quiet blue-black gradient with white text, a soft
// shadow and no outline: the modern default look for lyrics.
func CleanDarkTheme() Theme {
	return Theme{
		Name:       "Clean Dark",
		Background: Background{Kind: BackgroundGradient, Color: Hex(0x141B2D), Color2: Hex(0x070A12)},
		Font: Font{
			Size:        92,
			MinSize:     40,
			Color:       Hex(0xF5F7FA),
			Bold:        true,
			LineSpacing: 0.12,
		},
		Shadow:      Shadow{Enabled: true, OffsetX: 0, OffsetY: 4, Blur: 10, Color: RGBA{0, 0, 0, 0.6}},
		Outline:     Outline{Size: 3, Color: RGBA{0, 0, 0, 1}},
		HAlign:      AlignCenter,
		VAlign:      AlignMiddle,
		Margins:     Margins{Left: 140, Top: 110, Right: 140, Bottom: 130},
		FooterSize:  28,
		FooterColor: RGBA{0.86, 0.89, 0.94, 0.7},
	}
}

// WarmLightTheme is a cream-to-sand gradient with deep brown text: for
// bright rooms and daytime services, where a dark screen looks like a hole.
func WarmLightTheme() Theme {
	return Theme{
		Name:       "Warm Light",
		Background: Background{Kind: BackgroundGradient, Color: Hex(0xFBF5EA), Color2: Hex(0xEADBC2)},
		Font: Font{
			Size:        88,
			MinSize:     40,
			Color:       Hex(0x2E2117),
			Bold:        true,
			LineSpacing: 0.12,
		},
		Outline:     Outline{Size: 3, Color: Hex(0xFFFFFF)},
		Shadow:      Shadow{OffsetX: 2, OffsetY: 2, Blur: 4, Color: RGBA{0, 0, 0, 0.3}},
		HAlign:      AlignCenter,
		VAlign:      AlignMiddle,
		Margins:     Margins{Left: 140, Top: 110, Right: 140, Bottom: 130},
		FooterSize:  28,
		FooterColor: RGBA{0.36, 0.27, 0.19, 0.85},
	}
}

// HighContrastTheme is white bold text with a black outline on pure black:
// the most legible choice for low vision and washed-out projectors.
func HighContrastTheme() Theme {
	return Theme{
		Name:       "High Contrast",
		Background: Background{Kind: BackgroundColor, Color: Hex(0x000000)},
		Font: Font{
			Size:        112,
			MinSize:     48,
			Color:       Hex(0xFFFFFF),
			Bold:        true,
			LineSpacing: 0.08,
		},
		Outline:     Outline{Enabled: true, Size: 4, Color: Hex(0x000000)},
		HAlign:      AlignCenter,
		VAlign:      AlignMiddle,
		Margins:     Margins{Left: 72, Top: 64, Right: 72, Bottom: 96},
		FooterSize:  32,
		FooterColor: Hex(0xFFD400),
	}
}

// LowerThirdTheme puts scripture in a translucent band across the bottom
// third, left-aligned with the reference under it, over a dark gradient.
// Give it a video or camera background and the picture stays visible above
// the words.
func LowerThirdTheme() Theme {
	return Theme{
		Name:       "Scripture Lower Third",
		Background: Background{Kind: BackgroundGradient, Color: Hex(0x1F2A3A), Color2: Hex(0x0B0F16)},
		Font: Font{
			Size:        58,
			MinSize:     30,
			Color:       Hex(0xFFFFFF),
			LineSpacing: 0.1,
		},
		Shadow:      Shadow{Enabled: true, OffsetX: 0, OffsetY: 2, Blur: 4, Color: RGBA{0, 0, 0, 0.7}},
		Outline:     Outline{Size: 2, Color: Hex(0x000000)},
		HAlign:      AlignLeft,
		VAlign:      AlignMiddle,
		Margins:     Margins{Left: 120, Top: 750, Right: 120, Bottom: 70},
		FooterSize:  34,
		FooterColor: Hex(0xF2C14E),
		Box:         TextBox{Enabled: true, Color: RGBA{0.03, 0.05, 0.09, 0.78}, Padding: 36, FullWidth: true},
	}
}
