package uicore

import "math"

// Rect is a plain rectangle (position + size).
type Rect struct{ X, Y, W, H float64 }

// TileGrid lays out slide-selector tiles (TsdSlideSelector): tiles wrap left to
// right, are at least MinTileW wide and stretch to fill the row (like CSS
// `repeat(auto-fill, minmax(MinTileW, 1fr))`). A tile is a 16:9 thumbnail plus
// a label row, surrounded by Inset (the selection highlight padding).
type TileGrid struct {
	Width    float64 // available width
	MinTileW float64 // view sizer: minimum tile width
	Gap      float64 // spacing between tiles
	Pad      float64 // padding around the grid
	Inset    float64 // highlight padding inside a tile, around the thumbnail
	LabelH   float64 // label row height
	Aspect   float64 // thumbnail aspect (16/9)
}

// Columns is the number of tiles per row (at least 1).
func (g TileGrid) Columns() int {
	inner := g.Width - 2*g.Pad
	if g.MinTileW <= 0 || inner <= g.MinTileW {
		return 1
	}
	return max(1, int(math.Floor((inner+g.Gap)/(g.MinTileW+g.Gap))))
}

// TileSize is the size of one tile.
func (g TileGrid) TileSize() (w, h float64) {
	cols := float64(g.Columns())
	w = math.Floor((g.Width - 2*g.Pad - (cols-1)*g.Gap) / cols)
	if w < 1 {
		w = 1
	}
	aspect := g.Aspect
	if aspect <= 0 {
		aspect = 16.0 / 9.0
	}
	thumbW := w - 2*g.Inset
	h = math.Round(thumbW/aspect) + 2*g.Inset + g.LabelH
	return w, h
}

// Tile returns the rect of tile i.
func (g TileGrid) Tile(i int) Rect {
	cols := g.Columns()
	w, h := g.TileSize()
	col, row := i%cols, i/cols
	return Rect{
		X: g.Pad + float64(col)*(w+g.Gap),
		Y: g.Pad + float64(row)*(h+g.Gap),
		W: w,
		H: h,
	}
}

// Thumb returns the thumbnail rect inside a tile of size (w, h), relative to the tile.
func (g TileGrid) Thumb(w, h float64) Rect {
	return Rect{X: g.Inset, Y: g.Inset, W: w - 2*g.Inset, H: h - 2*g.Inset - g.LabelH}
}

// ContentHeight is the total height needed for n tiles.
func (g TileGrid) ContentHeight(n int) float64 {
	if n <= 0 {
		return 2 * g.Pad
	}
	cols := g.Columns()
	rows := (n + cols - 1) / cols
	_, h := g.TileSize()
	return 2*g.Pad + float64(rows)*h + float64(rows-1)*g.Gap
}

// NavKey is a keyboard navigation direction in a grid or list.
type NavKey int

const (
	NavLeft NavKey = iota
	NavRight
	NavUp
	NavDown
	NavHome
	NavEnd
)

// Navigate moves a selection index in a grid of n items with cols columns.
// A selection of -1 (nothing selected) moves to the first item.
func Navigate(sel, n, cols int, key NavKey) int {
	if n <= 0 {
		return -1
	}
	if cols < 1 {
		cols = 1
	}
	if sel < 0 || sel >= n {
		if key == NavEnd {
			return n - 1
		}
		return 0
	}
	switch key {
	case NavLeft:
		sel--
	case NavRight:
		sel++
	case NavUp:
		if sel-cols >= 0 {
			sel -= cols
		}
	case NavDown:
		if sel+cols < n {
			sel += cols
		} else if sel/cols < (n-1)/cols {
			sel = n - 1 // partial last row: land on the last tile
		}
	case NavHome:
		sel = 0
	case NavEnd:
		sel = n - 1
	}
	return min(max(sel, 0), n-1)
}

// Letterbox fits a rect with the given aspect ratio, centered, inside
// (0,0,w,h) minus margin on every side.
func Letterbox(w, h, aspect, margin float64) Rect {
	if aspect <= 0 {
		aspect = 16.0 / 9.0
	}
	aw, ah := math.Max(0, w-2*margin), math.Max(0, h-2*margin)
	rw, rh := aw, aw/aspect
	if rh > ah {
		rh = ah
		rw = ah * aspect
	}
	rw, rh = math.Floor(rw), math.Floor(rh)
	return Rect{X: math.Floor((w - rw) / 2), Y: math.Floor((h - rh) / 2), W: rw, H: rh}
}

// MoveTarget returns the destination of a "Move Up" (delta -1) or "Move Down"
// (delta +1) on item i of n, and false when the move is not possible.
func MoveTarget(i, n, delta int) (int, bool) {
	if i < 0 || i >= n {
		return i, false
	}
	to := i + delta
	if to < 0 || to >= n {
		return i, false
	}
	return to, true
}
