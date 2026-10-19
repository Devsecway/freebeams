package ui

import (
	"freeworship/internal/ui/uicore"

	"graphics.gd/classdb/DPITexture"
	"graphics.gd/classdb/Texture2D"
	"graphics.gd/variant/Color"
)

// The icon artwork lives in uicore (pure Go, unit-tested): see
// uicore.IconSVG and docs/ICONS.md. This file turns it into textures.
//
// Every icon goes into the theme twice:
//   - FwIcons: rasterised in palette colors. Built-in controls (Tree items)
//     use these, and so do the multicolor icons (Go Live, Live on) that have
//     an accent or red of their own and are drawn untinted.
//   - FwIconMasks: single-color icons rasterised white. The custom-drawn
//     widgets draw them with a modulate color, which gives each button state
//     (normal, hover, active, disabled) its own tint from the palette.

// rasterIcon turns an icon into a DPITexture. Its logical size is the SVG
// size (32, 16 or 8) and Godot re-rasterises it for the drawing viewport's
// oversampling (the window's content scale), so glyphs stay crisp on HiDPI
// screens instead of being 1x bitmaps scaled up.
func rasterIcon(name string, c uicore.IconColors) (Texture2D.Instance, bool) {
	svg, ok := uicore.IconSVG(name, c)
	if !ok {
		return Texture2D.Instance{}, false
	}
	tex := DPITexture.New()
	tex.SetSource(svg)
	return tex.AsTexture2D(), true
}

// paletteIconColors are the colors the FwIcons set is rasterised with.
func paletteIconColors(p Palette) uicore.IconColors {
	return uicore.PaletteIconColors(p)
}

// maskIconColors rasterise a single-color icon white, to be tinted on draw
// (only {c} occurs in such an icon; the rest is set for completeness).
func maskIconColors() uicore.IconColors {
	white := Color.RGBA{R: 1, G: 1, B: 1, A: 1}
	c := uicore.IconColors{Glyph: white, Accent: white, Red: white, Green: white, Blue: white}
	for i := range c.Bars {
		c.Bars[i] = white
	}
	return c
}

// iconIsMulti reports whether an icon carries its own colors (drawn untinted).
func iconIsMulti(name string) bool {
	s, ok := uicore.Icon(name)
	return ok && s.Multi()
}
