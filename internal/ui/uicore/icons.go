package uicore

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"graphics.gd/variant/Color"
)

// FreeWorship's own icon set (see docs/ICONS.md): hand-written SVG bodies on
// two grids, rasterised by the ui package as DPITextures.
//
//   - 32 px toolbar icons: a 24 px live area (4 px padding), 2 px rounded
//     strokes on integer coordinates, so straight lines fill whole pixels at
//     1x and 2x. Corner radius 2.
//   - 16 px mini icons: a 12 px live area (2 px padding), 1.5 px rounded
//     strokes centred on .25 / .75 coordinates (whole device pixels at 2x).
//     Corner radius 1.
//   - 8 px arrows: filled, no stroke (the dropdown ▾).
//
// Colors are placeholders resolved by IconSVG: {c} the glyph color, {a} the
// accent (Go Live), {r} the live red, {k} black (the Black screen's interior).
// Icons that use {a} or {r} are "multicolor": they are rasterised in palette
// colors and drawn untinted. Every other icon is single-color: it can be
// rasterised white and tinted with a modulate color when drawn.
const (
	iconHead32 = `<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32" fill="none" stroke="{c}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">`
	iconHead16 = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="{c}" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">`
	iconHead8  = `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8" viewBox="0 0 8 8" fill="{c}" stroke="none">`
)

// Shared parts of the output icons: a monitor on a stand (Go Live, Live) and
// a plain screen (Logo, Black, Clear).
const (
	monitor32 = `<rect x="4" y="5" width="24" height="16" rx="2"/><path d="M11 28h10M16 21v7"/>`
	screen32  = `<rect x="4" y="7" width="24" height="18" rx="2"/>`
)

// IconSpec describes one icon.
type IconSpec struct {
	Size int    // logical size in pixels (32, 16 or 8)
	Body string // SVG elements inside the <svg> head
}

// Multi reports whether the icon has colors of its own ({a} or {r}) and must
// be drawn untinted.
func (s IconSpec) Multi() bool {
	return strings.Contains(s.Body, "{a}") || strings.Contains(s.Body, "{r}")
}

var icons = map[string]IconSpec{
	// ---- main toolbar (32) ----
	"new":     {32, `<path d="M8 4h10l6 6v18H8z"/><path d="M18 4v6h6"/><path d="M16 14v8M12 18h8"/>`},
	"open":    {32, `<path d="M4 26V7h8l3 3h10v6"/><path d="M4 26l3-10h21l-3 10z"/>`},
	"save":    {32, `<path d="M6 4h16l4 4v20H6z"/><path d="M11 4v7h9V4"/><path d="M10 28V17h12v11"/>`},
	"web":     {32, `<circle cx="16" cy="16" r="12"/><path d="M4 16h24"/><ellipse cx="16" cy="16" rx="5" ry="12"/>`},
	"remote":  {32, `<rect x="10" y="4" width="12" height="24" rx="2.5"/><path d="M14 24h4"/>`},
	"golive":  {32, monitor32 + `<path d="M13.5 9.5l6 3.5-6 3.5z" fill="{a}" stroke="{a}"/>`},
	"alerts":  {32, `<path d="M16 5a7 7 0 0 1 7 7v5l3 4H6l3-4v-5a7 7 0 0 1 7-7z"/><path d="M13 25a3 3 0 0 0 6 0"/>`},
	"logo":    {32, screen32 + `<path d="M8 21l5-6 4 4 3-3 4 5"/><circle cx="20.5" cy="12" r="1.75" fill="{c}" stroke="none"/>`},
	"black":   {32, `<rect x="4" y="7" width="24" height="18" rx="2" fill="{k}"/>`},
	"clear":   {32, screen32 + `<path d="M12 12l8 8M20 12l-8 8"/>`},
	"live":    {32, monitor32 + `<circle cx="16" cy="13" r="3"/>`},
	"live_on": {32, monitor32 + `<circle cx="16" cy="13" r="4" fill="{r}" stroke="none"/>`},

	// ---- mini buttons, tabs, lists (16) ----
	"plus":         {16, `<path d="M7.75 2.5v10.5M2.5 7.75h10.5"/>`},
	"gear":         {16, gearPath()},
	"view_large":   {16, `<rect x="2.75" y="2.75" width="4.5" height="4.5" rx="1"/><rect x="8.75" y="2.75" width="4.5" height="4.5" rx="1"/><rect x="2.75" y="8.75" width="4.5" height="4.5" rx="1"/><rect x="8.75" y="8.75" width="4.5" height="4.5" rx="1"/>`},
	"view_details": {16, `<path d="M2.5 3.75h11M2.5 7.75h11M2.5 11.75h11"/>`},
	"search":       {16, `<circle cx="7.25" cy="7.25" r="4.5"/><path d="M10.5 10.5l3.25 3.25"/>`},
	"clear_field":  {16, `<path d="M4.5 4.5l7 7M11.5 4.5l-7 7"/>`},
	"golive16":     {16, `<rect x="1.75" y="2.75" width="12.5" height="8.5" rx="1"/><path d="M5.5 13.75h4.5M7.75 11.25v2.5"/><path d="M6.75 5.25l3 1.75-3 1.75z" fill="{a}" stroke="{a}"/>`},
	"addschedule":  {16, `<path d="M2.5 3.75h8M2.5 7.75h8M2.5 11.75h4"/><path d="M11.75 8.75v5M9.25 11.25h5"/>`},
	"goback":       {16, `<path d="M6.25 3.5L2.5 7.25l3.75 3.75"/><path d="M2.5 7.25h7.75a3 3 0 0 1 0 6H8.5"/>`},
	"swap":         {16, `<path d="M13.5 5.25H2.5M10.75 2.5l2.75 2.75-2.75 2.75"/><path d="M2.5 10.75h11M5.25 8l-2.75 2.75 2.75 2.75"/>`},
	"chev_right":   {16, `<path d="M6.25 4l3.75 4-3.75 4"/>`},
	"chev_down":    {16, `<path d="M4 6.25l4 3.75 4-3.75"/>`},
	"play":         {16, `<path d="M4.75 3.25l7.5 4.75-7.5 4.75z" fill="{c}" stroke="{c}"/>`},
	"library":      {16, `<path d="M5.75 12.25V4.25l7.5-1.75v8.25"/><circle cx="3.9" cy="12.25" r="1.85"/><circle cx="11.4" cy="10.75" r="1.85"/>`},
	"bible":        {16, `<rect x="3.25" y="2.75" width="9.5" height="10.5" rx="0.75"/><path d="M5.25 2.75v10.5"/><path d="M9.25 5.25v4.5M7.25 7.25h4"/>`},
	"video":        {16, `<rect x="1.75" y="4.25" width="8.5" height="7.5" rx="1"/><path d="M10.25 7.25l4-2.5v6.5l-4-2.5"/>`},
	"image":        {16, `<rect x="2.25" y="3.25" width="11.5" height="9.5" rx="1"/><circle cx="5.75" cy="6.25" r="1.25" fill="{c}" stroke="none"/><path d="M3 11.75l3.25-3.25 2.5 2.5 2-2 2.25 2.25"/>`},
	"presentation": {16, `<rect x="1.75" y="2.75" width="12.5" height="8.5" rx="1"/><path d="M7.75 11.25v2.5M5.5 13.75h4.5"/><path d="M4.75 5.75h6.5M4.75 8.25h4"/>`},
	"theme":        {16, `<circle cx="8" cy="8" r="5.75"/><circle cx="5.5" cy="6" r="1.1" fill="{c}" stroke="none"/><circle cx="8.5" cy="4.75" r="1.1" fill="{c}" stroke="none"/><circle cx="11" cy="6.75" r="1.1" fill="{c}" stroke="none"/><circle cx="9.5" cy="10.25" r="1.5"/>`},

	// ---- arrows (8) ----
	"arrow_down": {8, `<path d="M0.75 2.25h6.5L4 6z"/>`},

	// ---- transport & audio (16) ----
	// The library's Audio collection (an eighth note), the transport bar
	// under the output monitors (pause, skip to start / end, loop, volume;
	// play is the existing "play") and the sliders' round grabber.
	"audio":     {16, `<path d="M6.75 12.25V3.25l5.5 1.5v2.5"/><circle cx="4.9" cy="12.25" r="1.85"/>`},
	"pause":     {16, `<rect x="3.75" y="3.25" width="2.5" height="9.5" rx="0.5" fill="{c}" stroke="none"/><rect x="9.75" y="3.25" width="2.5" height="9.5" rx="0.5" fill="{c}" stroke="none"/>`},
	"skip_back": {16, `<path d="M3.75 3.75v8.5"/><path d="M13.25 3.75L7.25 8l6 4.25z" fill="{c}" stroke="{c}"/>`},
	"skip_fwd":  {16, `<path d="M12.25 3.75v8.5"/><path d="M2.75 3.75L8.75 8l-6 4.25z" fill="{c}" stroke="{c}"/>`},
	"loop":      {16, `<path d="M3.25 9.25V7.25a2.5 2.5 0 0 1 2.5-2.5h6"/><path d="M9.75 2.75l2 2-2 2"/><path d="M12.75 6.75v2a2.5 2.5 0 0 1-2.5 2.5h-6"/><path d="M6.25 13.25l-2-2 2-2"/>`},
	"volume":    {16, `<path d="M2.75 6.25h2.5l3.5-3v9.5l-3.5-3h-2.5z" fill="{c}" stroke="{c}" stroke-linejoin="round"/><path d="M11.25 5.5a3.5 3.5 0 0 1 0 5"/><path d="M13.25 3.75a6 6 0 0 1 0 8.5"/>`},
	"knob":      {16, `<circle cx="8" cy="8" r="5.5" fill="{c}" stroke="none"/>`},
}

// gearPath builds the 16 px cog: 8 teeth between radii 4.9 and 6.5 and a hub.
func gearPath() string {
	const teeth = 8
	const ro, ri = 6.5, 4.9
	var b strings.Builder
	for k := 0; k < teeth; k++ {
		base := float64(k) * 2 * math.Pi / teeth
		for i, p := range []struct{ a, r float64 }{
			{base - 0.30, ri}, {base - 0.19, ro}, {base + 0.19, ro}, {base + 0.30, ri},
		} {
			cmd := "L"
			if k == 0 && i == 0 {
				cmd = "M"
			}
			fmt.Fprintf(&b, "%s%.2f %.2f", cmd, 8+p.r*math.Cos(p.a), 8+p.r*math.Sin(p.a))
		}
	}
	b.WriteString("Z")
	return `<path d="` + b.String() + `"/><circle cx="8" cy="8" r="2.1"/>`
}

// IconColors are the colors an icon is rasterised with.
type IconColors struct {
	Glyph  Color.RGBA // {c}: strokes and single-color fills
	Accent Color.RGBA // {a}: the Go Live play glyph
	Red    Color.RGBA // {r}: the Live-on indicator
}

// Icon returns the spec of a named icon.
func Icon(name string) (IconSpec, bool) {
	s, ok := icons[name]
	return s, ok
}

// IconNames lists every icon, sorted.
func IconNames() []string {
	names := make([]string, 0, len(icons))
	for n := range icons {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// IconSVG returns the complete SVG document for a named icon in the given
// colors. ok is false for unknown names.
func IconSVG(name string, c IconColors) (svg string, ok bool) {
	s, found := icons[name]
	if !found {
		return "", false
	}
	var head string
	switch s.Size {
	case 32:
		head = iconHead32
	case 16:
		head = iconHead16
	default:
		head = iconHead8
	}
	r := strings.NewReplacer("{c}", HexColor(c.Glyph), "{a}", HexColor(c.Accent), "{r}", HexColor(c.Red), "{k}", "#000000")
	return r.Replace(head + s.Body + `</svg>`), true
}

// HexColor formats an opaque #rrggbb color (alpha is ignored).
func HexColor(c Color.RGBA) string {
	to := func(v float32) int { return int(math.Round(float64(min(max(v, 0), 1)) * 255)) }
	return fmt.Sprintf("#%02x%02x%02x", to(c.R), to(c.G), to(c.B))
}

// Mix blends a toward b by t (0 = a, 1 = b), alpha included.
func Mix(a, b Color.RGBA, t float32) Color.RGBA {
	l := func(x, y float32) float32 { return x + (y-x)*t }
	return Color.RGBA{R: l(a.R, b.R), G: l(a.G, b.G), B: l(a.B, b.B), A: l(a.A, b.A)}
}

// GlyphColor is the icon color for a palette: in the dark interface body
// text (#B9B9B9) is too dim for 2 px strokes on the toolbar gradient, so
// glyphs sit 60% of the way from text to text.semibold (#DADADA); in the
// light interface text (#2A2A2A) already reads well.
func GlyphColor(p Palette) Color.RGBA {
	if p.IsDark() {
		return Mix(p.Text, p.TextSemibold, 0.6)
	}
	return p.Text
}

// AccentColor is the Go Live accent: highlight.hot on the dark toolbar,
// where the plain highlight is too close to the gradient, and highlight on
// the light one.
func AccentColor(p Palette) Color.RGBA {
	if p.IsDark() {
		return p.HighlightHot
	}
	return p.Highlight
}
