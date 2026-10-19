package uicore

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"graphics.gd/variant/Color"
)

var testColors = IconColors{Glyph: Hex(0xDADADA), Accent: Hex(0x4B8CE9), Red: Hex(0xFF0000), Green: Hex(0x5BB55B), Blue: Hex(0x4B8CE9), Bars: SMPTEBars}

func TestIconSVGWellFormed(t *testing.T) {
	names := IconNames()
	if len(names) < 30 {
		t.Fatalf("only %d icons", len(names))
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			svg, ok := IconSVG(name, testColors)
			if !ok {
				t.Fatal("IconSVG reports an unknown name")
			}
			if strings.ContainsAny(svg, "{}") {
				t.Errorf("unresolved placeholder in %s", svg)
			}
			d := xml.NewDecoder(strings.NewReader(svg))
			for {
				_, err := d.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("not well-formed XML: %v\n%s", err, svg)
				}
			}
			spec, _ := Icon(name)
			switch spec.Size {
			case 32, 16, 8:
			default:
				t.Errorf("size %d is not 32, 16 or 8", spec.Size)
			}
			if !strings.Contains(svg, `width="`+strconv.Itoa(spec.Size)+`"`) {
				t.Errorf("head does not declare width %d", spec.Size)
			}
		})
	}
}

// TestIconGrid checks the pixel-grid rule: straight horizontal and vertical
// segments of 32 px icons (2 px strokes) start on integer coordinates and
// those of 16 px icons (1.5 px strokes) on .25 / .75 coordinates, so lines
// fill whole device pixels at 1x (32 px set) and 2x (both sets).
func TestIconGrid(t *testing.T) {
	// Absolute "M x y" starts followed by an h/v segment (the coordinate
	// across the line must be on the grid; along it the round caps blend
	// anyway), and rect edges.
	move := regexp.MustCompile(`M(-?[\d.]+) (-?[\d.]+)([hvHV])`)
	rectAttr := regexp.MustCompile(`<rect x="([\d.]+)" y="([\d.]+)" width="([\d.]+)" height="([\d.]+)"`)
	onGrid := func(size int, f float64) bool {
		frac := f - float64(int(f))
		if size == 32 {
			return frac == 0
		}
		return frac == 0.25 || frac == 0.75
	}
	atof := func(s string) float64 {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
		return f
	}
	for _, name := range IconNames() {
		spec, _ := Icon(name)
		if spec.Size == 8 || name == "gear" {
			continue
		}
		for _, m := range move.FindAllStringSubmatch(spec.Body, -1) {
			across := m[2] // y of a horizontal segment
			if m[3] == "v" || m[3] == "V" {
				across = m[1] // x of a vertical one
			}
			if !onGrid(spec.Size, atof(across)) {
				t.Errorf("%s: segment %q is off the %d px grid", name, m[0], spec.Size)
			}
		}
		for _, m := range rectAttr.FindAllStringSubmatch(spec.Body, -1) {
			x, y, w, h := atof(m[1]), atof(m[2]), atof(m[3]), atof(m[4])
			for _, v := range []float64{x, y, x + w, y + h} {
				if !onGrid(spec.Size, v) {
					t.Errorf("%s: rect edge %v is off the %d px grid (%s)", name, v, spec.Size, m[0])
				}
			}
		}
	}
}

func TestIconMulti(t *testing.T) {
	tests := map[string]bool{
		"golive": true, "golive16": true, "live_on": true, "alerts": true, "new": true, "web": true,
		"live": false, "black": false, "logo": false, "clear": false, "open": false, "save": false,
		"remote": false, "plus": false, "gear": false,
	}
	for name, want := range tests {
		spec, ok := Icon(name)
		if !ok {
			t.Fatalf("%s missing", name)
		}
		if spec.Multi() != want {
			t.Errorf("%s: Multi = %v, want %v", name, spec.Multi(), want)
		}
	}
	if _, ok := IconSVG("nope", testColors); ok {
		t.Error("unknown icon reported ok")
	}
}

// TestLiveBars checks that the Live screens' seven bars tile the 20 px
// interior (x 6..26) edge to edge, and that live_on uses the SMPTE colours.
func TestLiveBars(t *testing.T) {
	bar := regexp.MustCompile(`<rect x="(\d+)" y="9" width="(\d+)" height="11" fill="([^"]+)"`)
	for _, name := range []string{"live", "live_on"} {
		svg, _ := IconSVG(name, testColors)
		bars := bar.FindAllStringSubmatch(svg, -1)
		if len(bars) != 7 {
			t.Fatalf("%s: %d bars, want 7", name, len(bars))
		}
		x := 6
		for i, b := range bars {
			bx, _ := strconv.Atoi(b[1])
			w, _ := strconv.Atoi(b[2])
			if bx != x {
				t.Errorf("%s: bar %d starts at %d, want %d", name, i, bx, x)
			}
			x += w
			if name == "live_on" && b[3] != HexColor(SMPTEBars[i]) {
				t.Errorf("live_on: bar %d is %s, want %s", i, b[3], HexColor(SMPTEBars[i]))
			}
		}
		if x != 26 {
			t.Errorf("%s: bars end at %d, want 26", name, x)
		}
	}
}

func TestIconColors(t *testing.T) {
	tests := []struct {
		c    Color.RGBA
		want string
	}{
		{Hex(0xDADADA), "#dadada"},
		{Hex(0x000000), "#000000"},
		{Color.RGBA{R: 1.5, G: -1, B: 0.5, A: 0.3}, "#ff0080"},
	}
	for _, tt := range tests {
		if got := HexColor(tt.c); got != tt.want {
			t.Errorf("HexColor(%v) = %s, want %s", tt.c, got, tt.want)
		}
	}
	if got := Mix(Hex(0x000000), Hex(0xFFFFFF), 0.5); HexColor(got) != "#808080" {
		t.Errorf("Mix = %s, want #808080", HexColor(got))
	}
	if got := HexColor(GlyphColor(DarkPalette())); got != "#dadada" {
		t.Errorf("dark glyph = %s, want #dadada", got)
	}
	if got := HexColor(GlyphColor(LightPalette())); got != "#2a2a2a" {
		t.Errorf("light glyph = %s, want #2a2a2a", got)
	}
	if AccentColor(DarkPalette()) != DarkPalette().HighlightHot || AccentColor(LightPalette()) != LightPalette().Highlight {
		t.Error("accent colors")
	}
}

// TestDumpIcons writes every icon as an SVG file into $FW_ICON_DUMP (dark
// colors) for previewing the set outside the app; it is skipped otherwise.
func TestDumpIcons(t *testing.T) {
	dir := os.Getenv("FW_ICON_DUMP")
	if dir == "" {
		t.Skip("FW_ICON_DUMP not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	c := PaletteIconColors(DarkPalette())
	for _, name := range IconNames() {
		svg, _ := IconSVG(name, c)
		if err := os.WriteFile(filepath.Join(dir, name+".svg"), []byte(svg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
