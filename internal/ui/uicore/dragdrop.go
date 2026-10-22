package uicore

import (
	"fmt"
	"strconv"
	"strings"
)

// Drag sources: what a drag into the schedule carries (DragPayload.Source).
const (
	DragSong         = "song"         // a song list row
	DragScripture    = "scripture"    // the selected verses (Index is the pressed row)
	DragMedia        = "media"        // a media tile
	DragPresentation = "presentation" // a presentation tile
	DragTheme        = "theme"        // a theme tile: dropped onto a schedule row
	DragSchedule     = "schedule"     // a schedule row being reordered
)

// dragPrefix marks FreeBeams' own drag data, so a drop target ignores
// anything else Godot may hand it.
const dragPrefix = "freebeams-drag:"

// DragPayload is what a drag carries: the source and the dragged row or
// tile. It travels through Godot as a string (Encode / ParseDragPayload).
type DragPayload struct {
	Source string
	Index  int
}

// Encode is the payload as Godot drag data.
func (p DragPayload) Encode() string {
	return dragPrefix + p.Source + ":" + strconv.Itoa(p.Index)
}

// ParseDragPayload decodes drag data made by Encode; false for anything else
// (another kind of drag, an unknown source).
func ParseDragPayload(data any) (DragPayload, bool) {
	var s string
	switch v := data.(type) {
	case string:
		s = v
	case fmt.Stringer:
		s = v.String()
	default:
		return DragPayload{}, false
	}
	rest, ok := strings.CutPrefix(s, dragPrefix)
	if !ok {
		return DragPayload{}, false
	}
	src, idx, ok := strings.Cut(rest, ":")
	if !ok {
		return DragPayload{}, false
	}
	i, err := strconv.Atoi(idx)
	if err != nil {
		return DragPayload{}, false
	}
	switch src {
	case DragSong, DragScripture, DragMedia, DragPresentation, DragTheme, DragSchedule:
		return DragPayload{Source: src, Index: i}, true
	}
	return DragPayload{}, false
}

// InsertsIntoSchedule reports whether a drop of this payload inserts an item
// at a gap between rows (a theme is dropped onto a row instead).
func (p DragPayload) InsertsIntoSchedule() bool { return p.Source != DragTheme }

// DropGap is the insertion gap (0 before the first row … n after the last)
// for a pointer at y over rows whose vertical centres are mids (ascending):
// the upper half of a row inserts before it, the lower half after it.
func DropGap(y float64, mids []float64) int {
	for i, m := range mids {
		if y < m {
			return i
		}
	}
	return len(mids)
}

// DropRow is the row under y, given each row's top and bottom (ascending,
// adjacent), or -1 outside every row.
func DropRow(y float64, tops, bottoms []float64) int {
	for i := range tops {
		if i < len(bottoms) && y >= tops[i] && y < bottoms[i] {
			return i
		}
	}
	return -1
}

// DropMove turns a reorder drop of row from into gap (DropGap) into the move
// target index of Schedule.Move. False when the drop leaves the order as it
// is (the gap just above or below the row) or is out of range.
func DropMove(from, gap, n int) (int, bool) {
	if from < 0 || from >= n || gap < 0 || gap > n {
		return from, false
	}
	to := gap
	if gap > from {
		to = gap - 1
	}
	if to == from {
		return from, false
	}
	return to, true
}

// AutoScroll is the scroll speed (pixels per second, negative upwards) while
// a drag hovers at y over a list spanning top..bottom: zero outside the edge
// bands, rising to maxSpeed at the very edge.
func AutoScroll(y, top, bottom, edge, maxSpeed float64) float64 {
	if edge <= 0 || bottom-top < 2*edge || y < top || y > bottom {
		return 0
	}
	switch {
	case y < top+edge:
		return -maxSpeed * (top + edge - y) / edge
	case y > bottom-edge:
		return maxSpeed * (y - (bottom - edge)) / edge
	}
	return 0
}
