package uicore

import (
	"fmt"
	"math"
	"testing"
	"time"

	"graphics.gd/variant/Color"
)

func hexOf(c Color.RGBA) string {
	return fmt.Sprintf("#%02X%02X%02X", to8(c.R), to8(c.G), to8(c.B))
}

func TestPaletteMatchesSpec(t *testing.T) {
	dark, light := DarkPalette(), LightPalette()
	tests := []struct {
		token       string
		dark, light Color.RGBA
		wantD       string
		wantL       string
	}{
		{"window.bk0", dark.Bk0, light.Bk0, "#202020", "#F9F9F9"},
		{"window.bk1", dark.Bk1, light.Bk1, "#2C2C2C", "#F9F9F9"},
		{"window.bk2", dark.Bk2, light.Bk2, "#363636", "#F0F0F0"},
		{"window.bk3", dark.Bk3, light.Bk3, "#484848", "#D0D0D0"},
		{"window.border", dark.WindowBorder, light.WindowBorder, "#000000", "#646464"},
		{"text", dark.Text, light.Text, "#B9B9B9", "#2A2A2A"},
		{"text.semibold", dark.TextSemibold, light.TextSemibold, "#F0F0F0", "#000000"},
		{"text.hyperlink", dark.TextHyperlink, light.TextHyperlink, "#699AF9", "#529AEF"},
		{"highlight", dark.Highlight, light.Highlight, "#3A6DCE", "#529AEF"},
		{"highlight.hot", dark.HighlightHot, light.HighlightHot, "#4B8CE9", "#62AAFF"},
		{"highlight.text", dark.HighlightText, light.HighlightText, "#F9F9F9", "#F9F9F9"},
		{"highlight.inactive", dark.HighlightInactive, light.HighlightInactive, "#626262", "#CECECE"},
		{"highlight.inactive.hot", dark.HighlightInactiveHot, light.HighlightInactiveHot, "#707070", "#DCDCDC"},
		{"highlight.text.inactive", dark.HighlightTextInact, light.HighlightTextInact, "#C2C2C2", "#2A2A2A"},
		{"highlight.2", dark.Highlight2, light.Highlight2, "#395792", "#CCE4F7"},
		{"highlight.2.hot", dark.Highlight2Hot, light.Highlight2Hot, "#436AA2", "#E5F1FB"},
		{"highlight.2.text", dark.Highlight2Text, light.Highlight2Text, "#B9B9B9", "#2A2A2A"},
		{"button.bk", dark.ButtonBk, light.ButtonBk, "#2C2C2C", "#E1E1E1"},
		{"splitterbar", dark.SplitterBar, light.SplitterBar, "#000000", "#AAAAAA"},
		{"border", dark.Border, light.Border, "#000000", "#AAAAAA"},
		{"border.bl", dark.BorderBL, light.BorderBL, "#101010", "#D4D4D4"},
		{"border.tl", dark.BorderTL, light.BorderTL, "#000000", "#8C8C8C"},
		{"selectorline", dark.SelectorLine, light.SelectorLine, "#4D4D4D", "#D0D0D0"},
		{"headercolumn.stroke", dark.HeaderColumnStroke, light.HeaderColumnStroke, "#000000", "#D0D0D0"},
		{"libraryselector.bk", dark.LibraryBk, light.LibraryBk, "#202020", "#DEDEE7"},
		{"preseditor", dark.PresEditor, light.PresEditor, "#1F1F1F", "#D0D0D0"},
		{"divider.menu", dark.DividerMenu, light.DividerMenu, "#5D5D5D", "#D7D7D7"},
		{"divider.button.leading", dark.DividerLeading, light.DividerLeading, "#242424", "#A0A0A0"},
		{"divider.button.trailing", dark.DividerTrailing, light.DividerTrailing, "#8F8F8F", "#FFFFFF"},
		{"menuview.fill", dark.MenuFill, light.MenuFill, "#363636", "#F0F0F0"},
		{"menuview.stroke", dark.MenuStroke, light.MenuStroke, "#000000", "#AAAAAA"},
		{"menuitem.fill", dark.MenuItemFill, light.MenuItemFill, "#395792", "#CCE4F7"},
		{"menugutter.fill", dark.MenuGutter, light.MenuGutter, "#303030", "#E6E6E6"},
		{"red", dark.Red, light.Red, "#FF0000", "#FF0000"},
	}
	for _, tt := range tests {
		if got := hexOf(tt.dark); got != tt.wantD {
			t.Errorf("dark %s = %s, want %s", tt.token, got, tt.wantD)
		}
		if got := hexOf(tt.light); got != tt.wantL {
			t.Errorf("light %s = %s, want %s", tt.token, got, tt.wantL)
		}
	}
}

func TestGradientsMatchSpec(t *testing.T) {
	dark, light := DarkPalette(), LightPalette()
	tests := []struct {
		name       string
		g          Gradient
		top, botto string
	}{
		{"dark toolbar", dark.Toolbar, "#6E6E6E", "#4C4C4C"},
		{"light toolbar", light.Toolbar, "#F7F3F7", "#CECFCE"},
		{"dark toolbar.mini", dark.ToolbarMini, "#343434", "#292929"},
		{"light toolbar.mini", light.ToolbarMini, "#E5E5E5", "#F4F4F4"},
		{"dark tabbar.1", dark.TabBar1, "#6E6E6E", "#424242"},
		{"light tabbar.1", light.TabBar1, "#F7F7F7", "#CECECE"},
		{"dark hsplitter closed", dark.HSplitterClosed, "#555555", "#2C2C2C"},
	}
	for _, tt := range tests {
		if got := hexOf(tt.g.Sample(0)); got != tt.top {
			t.Errorf("%s top = %s, want %s", tt.name, got, tt.top)
		}
		if got := hexOf(tt.g.Sample(1)); got != tt.botto {
			t.Errorf("%s bottom = %s, want %s", tt.name, got, tt.botto)
		}
	}
	mid := Lin(Hex(0x000000), Hex(0xFFFFFF)).Sample(0.5)
	if math.Abs(float64(mid.R)-0.5) > 1e-6 {
		t.Errorf("midpoint = %v, want 0.5", mid.R)
	}
}

func TestGradientStrip(t *testing.T) {
	g := Lin(Hex(0x000000), Hex(0xFFFFFF))
	px := GradientStrip(g, 10, Edge{1, Hex(0xFF0000)}, Edge{2, Hex(0x00FF00)})
	if len(px) != 40 {
		t.Fatalf("len = %d, want 40", len(px))
	}
	row := func(y int) [4]byte { return [4]byte(px[y*4 : y*4+4]) }
	if row(0) != [4]byte{255, 0, 0, 255} {
		t.Errorf("row 0 = %v, want top edge red", row(0))
	}
	if row(8) != [4]byte{0, 255, 0, 255} || row(9) != [4]byte{0, 255, 0, 255} {
		t.Errorf("rows 8,9 = %v %v, want bottom edge green", row(8), row(9))
	}
	if !(row(1)[0] < row(4)[0] && row(4)[0] < row(7)[0]) {
		t.Errorf("gradient rows not increasing: %v %v %v", row(1), row(4), row(7))
	}
}

func TestGradientBox(t *testing.T) {
	w, px := GradientBox(Flat(Hex(0x808080)), 4, Edge{}, Edge{1, Hex(0x000000)}, Edge{}, Edge{1, Hex(0xFF0000)})
	if w != 2 || len(px) != 2*4*4 {
		t.Fatalf("w=%d len=%d", w, len(px))
	}
	at := func(x, y int) [4]byte { i := (y*w + x) * 4; return [4]byte(px[i : i+4]) }
	if at(0, 0) != [4]byte{128, 128, 128, 255} {
		t.Errorf("center = %v", at(0, 0))
	}
	if at(1, 1) != [4]byte{255, 0, 0, 255} {
		t.Errorf("right edge = %v", at(1, 1))
	}
	if at(1, 3) != [4]byte{0, 0, 0, 255} || at(0, 3) != [4]byte{0, 0, 0, 255} {
		t.Errorf("bottom band = %v %v", at(0, 3), at(1, 3))
	}
}

func TestSplitLayout(t *testing.T) {
	tests := []struct {
		name          string
		s             Split
		first, second float64
	}{
		{"percent", Split{Total: 1001, Bar: 1, Percent: 0.71}, 710, 290},
		{"min1 wins", Split{Total: 401, Bar: 1, Percent: 0.1, Min1: 300}, 300, 100},
		{"min2", Split{Total: 1001, Bar: 1, Percent: 0.9, Min2: 300}, 700, 300},
		{"collapse first", Split{Total: 500, Bar: 1, Closed: 7, Percent: 0.5, Collapse: CollapseFirst}, 0, 493},
		{"collapse second", Split{Total: 500, Bar: 1, Closed: 7, Percent: 0.5, Collapse: CollapseSecond}, 493, 0},
		{"tiny", Split{Total: 100, Bar: 1, Percent: 0.5, Min1: 300}, 99, 0},
	}
	for _, tt := range tests {
		f, s := tt.s.Layout()
		if f != tt.first || s != tt.second {
			t.Errorf("%s: Layout = (%v, %v), want (%v, %v)", tt.name, f, s, tt.first, tt.second)
		}
	}
}

func TestSplitDrag(t *testing.T) {
	base := Split{Total: 1001, Bar: 1, Closed: 7, Percent: 0.5, Min1: 300, Snap2: 150}
	tests := []struct {
		name     string
		pos      float64
		percent  float64
		collapse Collapse
	}{
		{"normal", 600, 0.6, CollapseNone},
		{"clamped by min1", 100, 0.3, CollapseNone},
		{"snap detent", 900, 0.85, CollapseNone},
		{"snap close", 960, 0.5, CollapseSecond},
	}
	for _, tt := range tests {
		got := base.Drag(tt.pos)
		if math.Abs(got.Percent-tt.percent) > 1e-9 || got.Collapse != tt.collapse {
			t.Errorf("%s: Drag(%v) = (%v, %v), want (%v, %v)", tt.name, tt.pos, got.Percent, got.Collapse, tt.percent, tt.collapse)
		}
	}
	left := Split{Total: 1000, Bar: 1, Percent: 0.2, Snap1: 200, Min2: 300}
	if got := left.Drag(50); got.Collapse != CollapseFirst || got.Percent != 0.2 {
		t.Errorf("snap first: %+v", got)
	}
	if got := left.Drag(50).Toggle(CollapseFirst); got.Collapse != CollapseNone || got.Percent != 0.2 {
		t.Errorf("reopen keeps percent: %+v", got)
	}
	if got := left.Toggle(CollapseSecond); got.Collapse != CollapseSecond {
		t.Errorf("toggle closes: %+v", got)
	}
}

func TestTileGrid(t *testing.T) {
	g := TileGrid{Width: 600, MinTileW: 150, Gap: 6, Pad: 6, Inset: 3, LabelH: 18, Aspect: 16.0 / 9.0}
	if c := g.Columns(); c != 3 {
		t.Fatalf("Columns = %d, want 3", c)
	}
	w, h := g.TileSize()
	if w != 192 {
		t.Errorf("tile width = %v, want 192", w)
	}
	// thumb 186 wide -> 105 high (rounded), + 2*3 inset + 18 label
	if h != 129 {
		t.Errorf("tile height = %v, want 129", h)
	}
	if r := g.Tile(4); r.X != 6+192+6 || r.Y != 6+129+6 {
		t.Errorf("Tile(4) = %+v", r)
	}
	if th := g.Thumb(w, h); th.W != 186 || th.H != 105 || th.X != 3 {
		t.Errorf("Thumb = %+v", th)
	}
	if ch := g.ContentHeight(7); ch != 6*2+3*129+2*6 {
		t.Errorf("ContentHeight(7) = %v", ch)
	}
	narrow := TileGrid{Width: 100, MinTileW: 150, Pad: 6}
	if c := narrow.Columns(); c != 1 {
		t.Errorf("narrow Columns = %d, want 1", c)
	}
	if lr := g.Label(w, h); lr.X != 4 || lr.Y != 3+105 || lr.W != 184 || lr.H != 18 {
		t.Errorf("Label = %+v", lr)
	}
}

func TestTileGridRows(t *testing.T) {
	// The Details view: one row per item, a 64 px thumbnail at the left and
	// the label beside it, whatever the width.
	g := TileGrid{Width: 600, MinTileW: 150, Gap: 6, Pad: 6, Inset: 3, LabelH: 18, Aspect: 16.0 / 9.0, Rows: true, RowThumbW: 64}
	if c := g.Columns(); c != 1 {
		t.Fatalf("Columns = %d, want 1", c)
	}
	w, h := g.TileSize()
	if w != 588 || h != 36+6 { // 64 wide -> 36 high, + 2*3 inset
		t.Errorf("row size = %v x %v, want 588 x 42", w, h)
	}
	if th := g.Thumb(w, h); th != (Rect{X: 3, Y: 3, W: 64, H: 36}) {
		t.Errorf("Thumb = %+v", th)
	}
	if lr := g.Label(w, h); lr != (Rect{X: 3 + 64 + 8, Y: 3, W: 588 - 75 - 3, H: 36}) {
		t.Errorf("Label = %+v", lr)
	}
	if r := g.Tile(2); r != (Rect{X: 6, Y: 6 + 2*(42+6), W: 588, H: 42}) {
		t.Errorf("Tile(2) = %+v", r)
	}
	if ch := g.ContentHeight(3); ch != 12+3*42+2*6 {
		t.Errorf("ContentHeight(3) = %v", ch)
	}
	// The height never depends on the width (no scrollbar feedback loop).
	wide := g
	wide.Width = 1400
	if _, hw := wide.TileSize(); hw != h || wide.ContentHeight(3) != g.ContentHeight(3) {
		t.Errorf("row height follows the width: %v", hw)
	}
	// A row narrower than the thumbnail clips the thumbnail, not the layout.
	tiny := g
	tiny.Width = 40
	if th := tiny.Thumb(tiny.TileSize()); th.W != 40-12-6 || tiny.Label(tiny.TileSize()).W != 0 {
		t.Errorf("tiny row: thumb %+v label %+v", th, tiny.Label(tiny.TileSize()))
	}
}

func TestNavigate(t *testing.T) {
	tests := []struct {
		sel, n, cols int
		key          NavKey
		want         int
	}{
		{-1, 6, 3, NavRight, 0},
		{0, 6, 3, NavRight, 1},
		{0, 6, 3, NavLeft, 0},
		{5, 6, 3, NavRight, 5},
		{1, 6, 3, NavDown, 4},
		{4, 6, 3, NavUp, 1},
		{1, 6, 3, NavUp, 1},
		{2, 5, 3, NavDown, 4}, // partial last row
		{3, 6, 3, NavHome, 0},
		{0, 6, 3, NavEnd, 5},
		{0, 0, 3, NavDown, -1},
	}
	for _, tt := range tests {
		if got := Navigate(tt.sel, tt.n, tt.cols, tt.key); got != tt.want {
			t.Errorf("Navigate(%d,%d,%d,%d) = %d, want %d", tt.sel, tt.n, tt.cols, tt.key, got, tt.want)
		}
	}
}

func TestLetterbox(t *testing.T) {
	tests := []struct {
		w, h, margin float64
		want         Rect
	}{
		{1928, 1088, 4, Rect{4, 4, 1920, 1080}},
		{400, 400, 0, Rect{0, 87, 400, 225}},
		{800, 225, 0, Rect{200, 0, 400, 225}},
		{2, 2, 4, Rect{1, 1, 0, 0}},
	}
	for _, tt := range tests {
		if got := Letterbox(tt.w, tt.h, 16.0/9.0, tt.margin); got != tt.want {
			t.Errorf("Letterbox(%v,%v,%v) = %+v, want %+v", tt.w, tt.h, tt.margin, got, tt.want)
		}
	}
}

func TestMoveTarget(t *testing.T) {
	tests := []struct {
		i, n, d, want int
		ok            bool
	}{
		{0, 3, -1, 0, false},
		{1, 3, -1, 0, true},
		{2, 3, 1, 2, false},
		{0, 3, 1, 1, true},
		{5, 3, 1, 5, false},
	}
	for _, tt := range tests {
		got, ok := MoveTarget(tt.i, tt.n, tt.d)
		if got != tt.want || ok != tt.ok {
			t.Errorf("MoveTarget(%d,%d,%d) = %d,%v want %d,%v", tt.i, tt.n, tt.d, got, ok, tt.want, tt.ok)
		}
	}
}

func TestDebouncer(t *testing.T) {
	t0 := time.Unix(0, 0)
	ms := func(n int) time.Time { return t0.Add(time.Duration(n) * time.Millisecond) }
	d := Debouncer{Delay: 200 * time.Millisecond}

	d.Trigger(ms(0), "a")
	d.Trigger(ms(100), "am")
	if _, ok := d.Poll(ms(250)); ok {
		t.Fatal("fired before quiet period after last keystroke")
	}
	if v, ok := d.Poll(ms(300)); !ok || v != "am" {
		t.Fatalf("Poll = %q,%v want am,true", v, ok)
	}
	if _, ok := d.Poll(ms(400)); ok {
		t.Fatal("fired twice")
	}
	d.Trigger(ms(500), "am")
	if _, ok := d.Poll(ms(800)); ok {
		t.Fatal("repeated value should be swallowed")
	}
	d.Trigger(ms(900), "am")
	if v, ok := d.Flush(); !ok || v != "am" {
		t.Fatalf("Flush = %q,%v", v, ok)
	}
	if _, ok := d.Flush(); ok {
		t.Fatal("Flush with nothing pending")
	}
	d.Reset("")
	d.Trigger(ms(1000), "")
	if _, ok := d.Poll(ms(1300)); ok {
		t.Fatal("value equal to Reset value should be swallowed")
	}
}
