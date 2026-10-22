package uicore

import "testing"

type stringer string

func (s stringer) String() string { return string(s) }

func TestDragPayloadRoundTrip(t *testing.T) {
	tests := []struct {
		data any
		want DragPayload
		ok   bool
	}{
		{DragPayload{DragSong, 3}.Encode(), DragPayload{DragSong, 3}, true},
		{DragPayload{DragScripture, -1}.Encode(), DragPayload{DragScripture, -1}, true},
		{stringer(DragPayload{DragTheme, 0}.Encode()), DragPayload{DragTheme, 0}, true},
		{"freebeams-drag:bogus:1", DragPayload{}, false},
		{"freebeams-drag:song:x", DragPayload{}, false},
		{"freebeams-drag:song", DragPayload{}, false},
		{"song:1", DragPayload{}, false},
		{42, DragPayload{}, false},
		{nil, DragPayload{}, false},
	}
	for _, tt := range tests {
		got, ok := ParseDragPayload(tt.data)
		if got != tt.want || ok != tt.ok {
			t.Errorf("ParseDragPayload(%v) = %v,%v want %v,%v", tt.data, got, ok, tt.want, tt.ok)
		}
	}
	if (DragPayload{DragTheme, 0}).InsertsIntoSchedule() || !(DragPayload{DragMedia, 0}).InsertsIntoSchedule() {
		t.Error("InsertsIntoSchedule: a theme goes onto a row, the rest insert")
	}
}

func TestDropGap(t *testing.T) {
	mids := []float64{23, 69, 115} // rows of 46 starting at 0
	tests := []struct {
		y    float64
		want int
	}{
		{-5, 0}, {0, 0}, {22, 0}, {24, 1}, {46, 1}, {70, 2}, {114, 2}, {116, 3}, {500, 3},
	}
	for _, tt := range tests {
		if got := DropGap(tt.y, mids); got != tt.want {
			t.Errorf("DropGap(%v) = %d want %d", tt.y, got, tt.want)
		}
	}
	if got := DropGap(10, nil); got != 0 {
		t.Errorf("DropGap on an empty list = %d want 0", got)
	}
}

func TestDropRow(t *testing.T) {
	tops, bottoms := []float64{0, 46, 92}, []float64{46, 92, 138}
	tests := []struct {
		y    float64
		want int
	}{{-1, -1}, {0, 0}, {45.9, 0}, {46, 1}, {137, 2}, {138, -1}}
	for _, tt := range tests {
		if got := DropRow(tt.y, tops, bottoms); got != tt.want {
			t.Errorf("DropRow(%v) = %d want %d", tt.y, got, tt.want)
		}
	}
}

func TestDropMove(t *testing.T) {
	tests := []struct {
		from, gap, n, want int
		ok                 bool
	}{
		{0, 0, 3, 0, false}, // just above itself
		{0, 1, 3, 0, false}, // just below itself
		{0, 2, 3, 1, true},
		{0, 3, 3, 2, true}, // to the end
		{2, 0, 3, 0, true}, // to the top
		{2, 2, 3, 2, false},
		{2, 3, 3, 2, false},
		{1, 0, 3, 0, true},
		{1, 3, 3, 2, true},
		{3, 0, 3, 3, false}, // out of range
		{0, 4, 3, 0, false},
	}
	for _, tt := range tests {
		got, ok := DropMove(tt.from, tt.gap, tt.n)
		if got != tt.want || ok != tt.ok {
			t.Errorf("DropMove(%d,%d,%d) = %d,%v want %d,%v", tt.from, tt.gap, tt.n, got, ok, tt.want, tt.ok)
		}
	}
}

func TestAutoScroll(t *testing.T) {
	tests := []struct {
		y, want float64
	}{
		{100, 0},   // middle
		{0, -600},  // top edge
		{12, -300}, // half way into the top band
		{200, 600}, // bottom edge
		{188, 300},
		{-1, 0}, {201, 0}, // outside the list
	}
	for _, tt := range tests {
		if got := AutoScroll(tt.y, 0, 200, 24, 600); got != tt.want {
			t.Errorf("AutoScroll(%v) = %v want %v", tt.y, got, tt.want)
		}
	}
	if got := AutoScroll(5, 0, 30, 24, 600); got != 0 {
		t.Errorf("a list shorter than both bands does not scroll, got %v", got)
	}
}
