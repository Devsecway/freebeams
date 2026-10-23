package models

import (
	"math"
	"testing"
)


func TestBuiltinThemes(t *testing.T) {
	seen := map[string]bool{DefaultSongTheme().Name: true, DefaultScriptureTheme().Name: true}
	for _, th := range BuiltinThemes() {
		if th.Name == "" || seen[th.Name] {
			t.Errorf("built-in theme name %q is empty or taken", th.Name)
		}
		seen[th.Name] = true
		if th.ID != 0 || th.Custom {
			t.Errorf("%s: ID %d custom %v", th.Name, th.ID, th.Custom)
		}
		if th.Font.Size <= 0 || th.Font.MinSize <= 0 || th.Font.MinSize > th.Font.Size {
			t.Errorf("%s: font sizes %v..%v", th.Name, th.Font.MinSize, th.Font.Size)
		}
		if th.Margins.Left+th.Margins.Right >= CanvasWidth || th.Margins.Top+th.Margins.Bottom >= CanvasHeight {
			t.Errorf("%s: margins leave no text box: %+v", th.Name, th.Margins)
		}
		if c := contrast(th.Font.Color, th.Background); c < 7 {
			t.Errorf("%s: text contrast %.1f:1 is under 7:1", th.Name, c)
		}
	}
	if !LowerThirdTheme().Box.Enabled {
		t.Error("the lower third has no band")
	}
}

// contrast is the WCAG contrast ratio of text over the background's
// lighter (worst case) colour; a band, when enabled, is what the text sits on.
func contrast(text RGBA, bg Background) float64 {
	lum := func(c RGBA) float64 {
		ch := func(v float32) float64 {
			x := float64(v)
			if x <= 0.03928 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
	}
	lt := lum(text)
	worst := 100.0
	cols := []RGBA{bg.Color}
	if bg.Kind == BackgroundGradient {
		cols = append(cols, bg.Color2)
	}
	for _, c := range cols {
		lb := lum(c)
		hi, lo := max(lt, lb), min(lt, lb)
		worst = min(worst, (hi+0.05)/(lo+0.05))
	}
	return worst
}

