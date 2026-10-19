package render

import (
	"math"

	"freebeams/internal/models"
	"freebeams/internal/render/layout"

	"graphics.gd/classdb/Font"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/SystemFont"
	"graphics.gd/classdb/TextParagraph"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/variant/Float"
)

// Fonts and fitted text sizes are shared by every SlideView (all on the main
// thread), so 40 thumbnails of one song resolve fonts and auto-fit once.

type fontKey struct {
	family       string
	bold, italic bool
}

var fontCache = map[fontKey]Font.Instance{}

// defaultFamilies is the fallback chain for Theme.Font.Family == "".
var defaultFamilies = []string{"Helvetica Neue", "Helvetica", "Arial", "sans-serif"}

func fontFor(family string, bold, italic bool) Font.Instance {
	k := fontKey{family, bold, italic}
	if f, ok := fontCache[k]; ok {
		return f
	}
	sf := pin(SystemFont.New()) // cached across frames, see pin

	if family == "" {
		sf.SetFontNames(defaultFamilies)
	} else {
		sf.SetFontNames(append([]string{family}, defaultFamilies...))
	}
	if bold {
		sf.SetFontWeight(700)
	} else {
		sf.SetFontWeight(400)
	}
	sf.SetFontItalic(italic)
	sf.SetAllowSystemFallback(true)
	// Big projected text: light hinting keeps glyph shapes true at any size,
	// and mipmaps keep downscaled preview/thumbnail glyphs smooth.
	sf.SetHinting(TextServer.HintingLight)
	sf.SetGenerateMipmaps(true)
	f := sf.AsFont()
	fontCache[k] = f
	return f
}

// breakFlags are used both to measure and to compute the forced line breaks.
const breakFlags = TextServer.BreakMandatory | TextServer.BreakWordBound | TextServer.BreakAdaptive

// fitKey identifies one auto-fit computation in canvas units.
type fitKey struct {
	text        string
	font        fontKey
	maxSize     int
	minSize     int
	lineSpacing float32
	boxW, boxH  int
}

// fitResult is the fitted font size (canvas units) and the text with the
// line breaks that size produced, so every scale wraps identically.
type fitResult struct {
	size  int
	lines string
}

const fitCacheMax = 4096

var (
	fitCache  = map[fitKey]fitResult{}
	paragraph TextParagraph.Instance
)

func measurer() TextParagraph.Instance {
	if paragraph == TextParagraph.Nil {
		paragraph = pin(TextParagraph.New())
		paragraph.SetBreakFlags(breakFlags)
		paragraph.SetAlignment(GUI.HorizontalAlignmentLeft)
	}
	return paragraph
}

// shape lays text out in the measurement paragraph at size (canvas units).
func shape(text string, font Font.Instance, size int, lineSpacing float32, width float64) TextParagraph.Instance {
	p := measurer()
	p.Clear()
	p.SetWidth(Float.X(width))
	p.SetLineSpacing(Float.X(float64(lineSpacing) * float64(size)))
	p.AddString(text, font, size)
	return p
}

// fitText auto-fits text into a boxW×boxH box (canvas units): the largest size
// in [MinSize, Size] whose wrapped height fits. Results are cached.
func fitText(text string, f models.Font, boxW, boxH float64) fitResult {
	maxSize := int(math.Round(float64(f.Size)))
	if maxSize < 1 {
		maxSize = 1
	}
	minSize := int(math.Round(float64(f.MinSize)))
	if minSize < 1 || minSize > maxSize {
		minSize = maxSize
	}
	k := fitKey{
		text:        text,
		font:        fontKey{f.Family, f.Bold, f.Italic},
		maxSize:     maxSize,
		minSize:     minSize,
		lineSpacing: f.LineSpacing,
		boxW:        int(math.Round(boxW)),
		boxH:        int(math.Round(boxH)),
	}
	if r, ok := fitCache[k]; ok {
		return r
	}
	font := fontFor(f.Family, f.Bold, f.Italic)
	size := layout.FitFontSize(minSize, maxSize, func(s int) bool {
		p := shape(text, font, s, f.LineSpacing, boxW)
		return float64(p.GetSize().Y) <= boxH
	})
	p := shape(text, font, size, f.LineSpacing, boxW)
	n := p.GetLineCount()
	ranges := make([][2]int, n)
	for i := 0; i < n; i++ {
		r := p.GetLineRange(i)
		ranges[i] = [2]int{int(r.X), int(r.Y)}
	}
	res := fitResult{size: size, lines: layout.JoinLines(text, ranges)}
	if len(fitCache) >= fitCacheMax {
		clear(fitCache)
	}
	fitCache[k] = res
	return res
}

// lineHeight returns one line's height in canvas units for the font at size.
func lineHeight(font Font.Instance, size float32) float64 {
	s := int(math.Round(float64(size)))
	if s < 1 {
		return 0
	}
	return float64(font.MoreArgs().GetHeight(s))
}
