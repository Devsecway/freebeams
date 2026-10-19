package ui

import (
	"fmt"
	"math"
	"strings"

	"graphics.gd/classdb/DPITexture"
	"graphics.gd/classdb/Texture2D"
	"graphics.gd/variant/Color"
)

// FreeWorship's own line icons (not EasyWorship's artwork): SVG templates
// (32px for the main toolbar, 16px for mini buttons) loaded as DPITextures.
// {c} is the glyph color, {r} the live red, {bg} the knock-out color.
const svgHead32 = `<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32" fill="none" stroke="{c}" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">`
const svgHead16 = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="{c}" stroke-width="1.35" stroke-linecap="round" stroke-linejoin="round">`

// screen32 is the projector-screen outline shared by the output icons.
const screen32 = `<rect x="3.5" y="6.5" width="25" height="16.5" rx="1.5"/>`

var icons32 = map[string]string{
	"new":    `<path d="M8.5 4.5h10l5 5v18h-15z"/><path d="M18.5 4.5v5h5"/><path d="M16 14v9M11.5 18.5h9"/>`,
	"open":   `<path d="M3.5 25.5v-17h8l2.5 3h11v4"/><path d="M3.5 25.5l4-10h21l-4 10z"/>`,
	"save":   `<path d="M5.5 5.5h16l5 5v16h-21z"/><path d="M10 5.5v6h10v-6"/><rect x="9.5" y="17" width="13" height="9.5"/>`,
	"web":    `<circle cx="16" cy="16" r="11"/><path d="M5 16h22M16 5c-5 6-5 16 0 22M16 5c5 6 5 16 0 22"/>`,
	"remote": `<rect x="10.5" y="3.5" width="11" height="25" rx="2.5"/><path d="M14.5 24.5h3"/>`,
	"golive": screen32 + `<path d="M13.5 10.5l7 4.25-7 4.25z" fill="{c}"/><path d="M11 28h10M16 23v5"/>`,
	"alerts": `<path d="M16 4.5a7 7 0 0 1 7 7v6l3 4H6l3-4v-6a7 7 0 0 1 7-7z"/><path d="M13 25a3 3 0 0 0 6 0"/>`,
	"logo":   screen32 + `<path d="M16 9.5v11M12 13.5h8"/>`,
	"black":  `<rect x="3.5" y="6.5" width="25" height="16.5" rx="1.5" fill="{c}" fill-opacity=".85"/>`,
	"clear":  screen32 + `<path d="M11.5 11l9 7.5M20.5 11l-9 7.5"/>`,
	"live":   screen32 + `<circle cx="16" cy="14.75" r="3.6" fill="{r}" stroke="none"/><path d="M11 28h10M16 23v5"/>`,
}

var icons16 = map[string]string{
	"plus":         `<path d="M8 3v10M3 8h10"/>`,
	"view_large":   `<rect x="2.5" y="2.5" width="4.5" height="4.5" rx=".5"/><rect x="9" y="2.5" width="4.5" height="4.5" rx=".5"/><rect x="2.5" y="9" width="4.5" height="4.5" rx=".5"/><rect x="9" y="9" width="4.5" height="4.5" rx=".5"/>`,
	"view_details": `<path d="M2.5 4h11M2.5 8h11M2.5 12h11"/>`,
	"search":       `<circle cx="6.75" cy="6.75" r="4"/><path d="M9.75 9.75l3.5 3.5"/>`,
	"clear_field":  `<path d="M5 5l6 6M11 5l-6 6" stroke-width="1.5"/>`,
	"golive":       `<rect x="1.5" y="3" width="13" height="9" rx="1"/><path d="M6.5 5.5l3.5 2-3.5 2z" fill="{c}"/>`,
	"addschedule":  `<path d="M2 3.5h7M2 7h7M2 10.5h4"/><path d="M11.5 8.5v6M8.5 11.5h6"/>`,
	"goback":       `<path d="M6 4L2.5 7.5 6 11"/><path d="M2.5 7.5h7a4 4 0 0 1 0 8h-1"/>`,
	"swap":         `<path d="M3 5.5h10l-2.5-2.5M13 10.5H3l2.5 2.5"/>`,
	"chev_right":   `<path d="M6 4l4 4-4 4" stroke-width="1.5"/>`,
	"chev_down":    `<path d="M4 6l4 4 4-4" stroke-width="1.5"/>`,
	"play":         `<path d="M4.5 3l8.5 5-8.5 5z" fill="{c}"/>`,
	"library":      `<path d="M6 12V3.5l7-1.5v8.5"/><circle cx="4.25" cy="12" r="1.9"/><circle cx="11.25" cy="10.5" r="1.9"/>`,
	"bible":        `<path d="M3.5 2.5h8.5v11H3.5z"/><path d="M5.5 13.5v-11M8.75 5v4M7.25 6.5h3"/>`,
	"video":        `<rect x="1.5" y="4" width="9" height="8" rx="1"/><path d="M10.5 7l4-2.5v7l-4-2.5z"/>`,
	"image":        `<rect x="2" y="3" width="12" height="10" rx="1"/><circle cx="5.5" cy="6.5" r="1.25"/><path d="M2.5 12l3.5-3.5 2.5 2.5 2-2 3 3"/>`,
	"presentation": `<rect x="1.5" y="2.5" width="13" height="8.5" rx="1"/><path d="M8 11v3M5.5 14h5M4.5 8.5l2.5-2 2 1.5 2.5-3"/>`,
}

// gearPath builds a cog outline for the 16px gear glyph.
func gearPath() string {
	const teeth = 8
	const ro, ri = 6.9, 5.1
	var b strings.Builder
	for k := 0; k < teeth; k++ {
		base := float64(k) * 2 * math.Pi / teeth
		pts := []struct{ a, r float64 }{
			{base - 0.30, ri}, {base - 0.17, ro}, {base + 0.17, ro}, {base + 0.30, ri},
		}
		for i, p := range pts {
			x, y := 8+p.r*math.Cos(p.a), 8+p.r*math.Sin(p.a)
			cmd := "L"
			if k == 0 && i == 0 {
				cmd = "M"
			}
			fmt.Fprintf(&b, "%s%.2f %.2f", cmd, x, y)
		}
	}
	b.WriteString("Z")
	return `<path d="` + b.String() + `"/><circle cx="8" cy="8" r="2.2"/>`
}

func colorHex(c Color.RGBA) string {
	to := func(v float32) int { return int(math.Round(float64(min(max(v, 0), 1)) * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", to(c.R), to(c.G), to(c.B))
}

// iconSVG returns the SVG text for a named icon in the given colors, and its
// logical size. ok is false for unknown names.
func iconSVG(name string, glyph, red, bg Color.RGBA) (svg string, size int, ok bool) {
	var head, body string
	if b, found := icons32[name]; found {
		head, body, size = svgHead32, b, 32
	} else if name == "gear" {
		head, body, size = svgHead16, gearPath(), 16
	} else if b, found := icons16[name]; found {
		head, body, size = svgHead16, b, 16
	} else {
		return "", 0, false
	}
	r := strings.NewReplacer("{c}", colorHex(glyph), "{r}", colorHex(red), "{bg}", colorHex(bg))
	return r.Replace(head + body + `</svg>`), size, true
}

// iconNames lists every icon put into the theme.
func iconNames() []string {
	names := []string{"gear"}
	for n := range icons32 {
		names = append(names, n)
	}
	for n := range icons16 {
		names = append(names, n)
	}
	return names
}

// rasterIcon turns an icon into a DPITexture: its logical size is the SVG
// size (32 or 16) and Godot re-rasterises it for the window's content scale /
// oversampling, so glyphs stay crisp on HiDPI screens. Built-in controls
// (LineEdit, Tree) therefore get correctly sized icons too.
func rasterIcon(name string, glyph, red, bg Color.RGBA) (Texture2D.Instance, bool) {
	svg, _, ok := iconSVG(name, glyph, red, bg)
	if !ok {
		return Texture2D.Instance{}, false
	}
	tex := DPITexture.New()
	tex.SetSource(svg)
	return tex.AsTexture2D(), true
}
