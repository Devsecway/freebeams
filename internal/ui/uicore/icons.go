package uicore

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"graphics.gd/variant/Color"
)

// FreeBeams's own icon set (see docs/ICONS.md): hand-written SVG bodies on
// two grids, rasterised by the ui package as DPITextures.
//
//   - 32 px toolbar icons: a 24 px live area (4 px padding). Filled shapes
//     whose straight edges and 2 px rounded strokes sit on integer
//     coordinates, so they fill whole pixels at 1x and 2x. Details are cut
//     out of the fills (evenodd holes) and show the button's own background
//     in every state. Corner radius 2.
//   - 16 px mini icons: a 12 px live area (2 px padding), 1.5 px rounded
//     strokes centred on .25 / .75 coordinates (whole device pixels at 2x).
//     Corner radius 1.
//   - 8 px arrows: filled, no stroke (the dropdown ▾).
//
// Colors are placeholders resolved by IconSVG: {c} the glyph color, {a} the
// accent, {r} the live red, {g} green (the Go Live screen, the Web globe,
// the New badge), {b} blue (the Alerts bubble), {s1}..{s7} the SMPTE bars of
// the Live-on screen, and the constants {k} black and {w} white. Icons that
// use anything but {c} and {k} are "multicolor": they are rasterised in
// palette colors and drawn untinted. Every other icon is single-color: it
// can be rasterised white and tinted with a modulate color when drawn ({k}
// stays black under any tint).
const (
	iconHead32 = `<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32" fill="none" stroke="{c}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">`
	iconHead16 = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="{c}" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">`
	iconHead8  = `<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8" viewBox="0 0 8 8" fill="{c}" stroke="none">`
)

// Shared parts of the output icons (Logo, Black, Clear, Live): a 24 x 18
// screen. Its 2 px rim is the glyph color at 55% and its 20 x 14 interior
// the glyph color at 85%, so the rim reads darker than the screen on the
// toolbar, as EasyWorship's screens do. The interior's details are evenodd
// holes that show the toolbar through the screen.
const (
	rim32    = `<rect x="5" y="8" width="22" height="16" rx="2" stroke="{c}" stroke-opacity=".55"/>`
	screenOp = ".85"
)

// screen32 is the rimmed screen with the given hole subpaths cut out of
// its interior (each starting with " M").
func screen32(holes string) string {
	return rim32 + `<path fill="{c}" fill-opacity="` + screenOp + `" fill-rule="evenodd" stroke="none" d="M6 9h20v14H6z` + holes + `"/>`
}

// bars32 is the rimmed screen showing seven colour bars (3, 3, 3, 2, 3, 3,
// 3 px across the 20 px interior) with the given fill attributes, over a
// black bottom band.
func bars32(fills [7]string) string {
	xs := [7][2]int{{6, 3}, {9, 3}, {12, 3}, {15, 2}, {17, 3}, {20, 3}, {23, 3}}
	var b strings.Builder
	b.WriteString(rim32)
	for i, f := range fills {
		fmt.Fprintf(&b, `<rect x="%d" y="9" width="%d" height="11" %s stroke="none"/>`, xs[i][0], xs[i][1], f)
	}
	b.WriteString(`<rect x="6" y="20" width="20" height="3" fill="{k}" stroke="none"/>`)
	return b.String()
}

// greyBars are the Live-off bars: the glyph color fading from 95% to 35%.
func greyBars() [7]string {
	var f [7]string
	for i, o := range [7]string{".95", ".85", ".75", ".65", ".55", ".45", ".35"} {
		f[i] = `fill="{c}" fill-opacity="` + o + `"`
	}
	return f
}

// smpteBars are the Live-on bars in the {s1}..{s7} colours.
func smpteBars() [7]string {
	var f [7]string
	for i := range f {
		f[i] = fmt.Sprintf(`fill="{s%d}"`, i+1)
	}
	return f
}

// IconSpec describes one icon.
type IconSpec struct {
	Size int    // logical size in pixels (32, 16 or 8)
	Body string // SVG elements inside the <svg> head
}

var placeholderRe = regexp.MustCompile(`\{([a-z][a-z0-9]*)\}`)

// Multi reports whether the icon has colors of its own (any placeholder but
// the glyph {c} and black {k}) and must be drawn untinted.
func (s IconSpec) Multi() bool {
	for _, m := range placeholderRe.FindAllStringSubmatch(s.Body, -1) {
		if m[1] != "c" && m[1] != "k" {
			return true
		}
	}
	return false
}

var icons = map[string]IconSpec{
	// ---- main toolbar (32) ----
	// New: a page with a folded corner, a text line and a green plus badge
	// in a cut-out ring. Open: a folder, its back at 55%. Save: a floppy with
	// its shutter and label cut out. Web: a green globe with white meridians.
	// Remote: signal arcs around a dot.
	"new":    {32, `<path fill="{c}" fill-opacity=".85" fill-rule="evenodd" stroke="none" d="M8 4h10v6h6v18H8z M11 12h10v2H11z M12 21a6 6 0 1 0 12 0a6 6 0 1 0-12 0z"/><path fill="{c}" fill-opacity=".5" stroke="none" d="M18 4l6 6h-6z"/><circle cx="18" cy="21" r="4.5" fill="{g}" stroke="none"/><path d="M18 18v6M15 21h6" stroke="{w}"/>`},
	"open":   {32, `<path fill="{c}" fill-opacity=".55" stroke="none" d="M4 7h8l3 3h13v4H4z"/><path fill="{c}" fill-opacity=".85" stroke="none" d="M4 26l3-12h21l-3 12z"/>`},
	"save":   {32, `<path fill="{c}" fill-opacity=".85" fill-rule="evenodd" stroke="none" d="M6 4h16l4 4v20H6z M11 6h9v5h-9z M17 7h2v3h-2z M10 18h12v8H10z M12 21h8v2h-8z"/>`},
	"web":    {32, `<circle cx="16" cy="16" r="12" fill="{g}" stroke="none"/><path d="M4 16h24" stroke="{w}" stroke-opacity=".75"/><ellipse cx="16" cy="16" rx="5" ry="12" stroke="{w}" stroke-opacity=".75"/><circle cx="16" cy="16" r="11" stroke="{w}" stroke-opacity=".3"/>`},
	"remote": {32, `<path d="M8.93 8.93A10 10 0 0 0 8.93 23.07M23.07 8.93A10 10 0 0 1 23.07 23.07"/><path d="M11.76 11.76A6 6 0 0 0 11.76 20.24M20.24 11.76A6 6 0 0 1 20.24 20.24"/><circle cx="16" cy="16" r="2" fill="{c}" stroke="none"/>`},
	// Output icons (docs/ICONS.md): Go Live is a green screen with a white
	// arrow and dot; Alerts a blue speech bubble with a signal glyph; Logo a
	// screen showing a picture (hills and sun); Black a screen whose lower-left
	// half is black; Clear a screen whose content lines are being peeled off;
	// Live a screen of colour bars, grey while the output is off (live) and in
	// SMPTE colours while it is on (live_on).
	"golive":  {32, `<rect x="4" y="7" width="24" height="18" rx="2" fill="{g}" stroke="none"/><rect x="5" y="8" width="22" height="16" rx="2" stroke="{w}" stroke-opacity=".3"/><path fill="{w}" stroke="none" d="M12 14h6v-3l7 5-7 5v-3h-6z"/><circle cx="8.5" cy="16" r="1.75" fill="{w}" stroke="none"/>`},
	"alerts":  {32, `<path fill="{b}" stroke="none" d="M7 5h18a3 3 0 0 1 3 3v11a3 3 0 0 1-3 3H14l-5 5v-5H7a3 3 0 0 1-3-3V8a3 3 0 0 1 3-3z"/><circle cx="11" cy="17" r="1.75" fill="{w}" stroke="none"/><path d="M11 12.5a4.5 4.5 0 0 1 4.5 4.5M11 9a8 8 0 0 1 8 8" stroke="{w}"/>`},
	"logo":    {32, screen32(` M7 23l6-8 4 4 3-3 5 7z M18 12.5a2 2 0 1 0 4 0a2 2 0 1 0-4 0z`)},
	"black":   {32, screen32(``) + `<path fill="{k}" stroke="none" d="M6 9v14h20z"/>`},
	"clear":   {32, screen32(` M9 12h9v2H9z M9 16h6v2H9z M17 14h9v9h-9z`) + `<path fill="{c}" fill-opacity=".5" stroke="none" d="M26 14l-9 9v-9z"/>`},
	"live":    {32, bars32(greyBars())},
	"live_on": {32, bars32(smpteBars())},

	// ---- mini buttons, tabs, lists (16) ----
	"plus":         {16, `<path d="M7.75 2.5v10.5M2.5 7.75h10.5"/>`},
	"gear":         {16, gearPath()},
	"view_large":   {16, `<rect x="2.75" y="2.75" width="4.5" height="4.5" rx="1"/><rect x="8.75" y="2.75" width="4.5" height="4.5" rx="1"/><rect x="2.75" y="8.75" width="4.5" height="4.5" rx="1"/><rect x="8.75" y="8.75" width="4.5" height="4.5" rx="1"/>`},
	"view_details": {16, `<path d="M2.5 3.75h11M2.5 7.75h11M2.5 11.75h11"/>`},
	"search":       {16, `<circle cx="7.25" cy="7.25" r="4.5"/><path d="M10.5 10.5l3.25 3.25"/>`},
	"clear_field":  {16, `<path d="M4.5 4.5l7 7M11.5 4.5l-7 7"/>`},
	"golive16":     {16, `<rect x="1.75" y="3.75" width="12.5" height="8.5" rx="1" fill="{g}" stroke="{g}"/><path fill="{w}" stroke="none" d="M6.25 7.25h2.5V5.5L12.25 8 8.75 10.5V8.75h-2.5z"/><circle cx="4.25" cy="8" r="1" fill="{w}" stroke="none"/>`},
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

	// ---- audio playlist (16) ----
	// The playlist transport under the schedule (ui/playlist.go): stop,
	// shuffle and repeat-one (repeat all is "loop"; previous / next track are
	// skip_back / skip_fwd).
	"stop":       {16, `<rect x="3.75" y="3.75" width="8.5" height="8.5" rx="1" fill="{c}" stroke="none"/>`},
	"shuffle":    {16, `<path d="M2.75 4.75h2.25l5 6.5h2.75"/><path d="M2.75 11.25h2.25l1.75-2.25"/><path d="M9.25 6.75l1-2h2.75"/><path d="M11.25 2.75l2 2-2 2"/><path d="M11.25 9.25l2 2-2 2"/>`},
	"repeat_one": {16, `<path d="M3.25 9.25V7.25a2.5 2.5 0 0 1 2.5-2.5h6"/><path d="M9.75 2.75l2 2-2 2"/><path d="M12.75 6.75v2a2.5 2.5 0 0 1-2.5 2.5h-6"/><path d="M6.25 13.25l-2-2 2-2"/><path d="M7.25 7l1-0.75v3.5"/>`},

	// ---- song editor (16) ----
	// The Lyrics header's SpellCheck button (ui/spelling.go): an "a" with a check mark.
	"spellcheck": {16, `<path d="M2.25 10.75L5 3.25l2.75 7.5M3.25 8h3.5"/><path d="M8 10.25l2.25 2.5 3.75-5.5"/>`},
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
	Accent Color.RGBA // {a}: accent
	Red    Color.RGBA // {r}: the live red

	Green Color.RGBA    // {g}: the Go Live screen, the Web globe, the New badge
	Blue  Color.RGBA    // {b}: the Alerts bubble
	Bars  [7]Color.RGBA // {s1}..{s7}: the Live-on colour bars
}

// SMPTEBars are the seven colour bars of the Live-on screen (white, yellow,
// cyan, green, magenta, red, blue), a little below full saturation so they
// sit on the toolbar without glowing.
var SMPTEBars = [7]Color.RGBA{Hex(0xF2F2F2), Hex(0xF2E600), Hex(0x00E0E0), Hex(0x00D200), Hex(0xE600E6), Hex(0xE60000), Hex(0x1E1EE6)}

// PaletteIconColors are the colors the palette-colored icon set is
// rasterised with for a palette.
func PaletteIconColors(p Palette) IconColors {
	return IconColors{Glyph: GlyphColor(p), Accent: AccentColor(p), Red: p.Red, Green: p.IconGreen, Blue: p.IconBlue, Bars: SMPTEBars}
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
	pairs := []string{
		"{c}", HexColor(c.Glyph), "{a}", HexColor(c.Accent), "{r}", HexColor(c.Red),
		"{g}", HexColor(c.Green), "{b}", HexColor(c.Blue),
		"{k}", "#000000", "{w}", "#ffffff",
	}
	for i, bar := range c.Bars {
		pairs = append(pairs, fmt.Sprintf("{s%d}", i+1), HexColor(bar))
	}
	return strings.NewReplacer(pairs...).Replace(head + s.Body + `</svg>`), true
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
