package core

import (
	"reflect"
	"testing"
)

func TestNewRowSelection(t *testing.T) {
	s := NewRowSelection([]int{5, -1, 3, 5, 4})
	want := RowSelection{Rows: []int{3, 4, 5}, Anchor: 3, Cursor: 3}
	if !reflect.DeepEqual(s, want) {
		t.Errorf("NewRowSelection = %+v, want %+v", s, want)
	}
	if e := NewRowSelection(nil); e.Anchor != -1 || e.Cursor != -1 || len(e.Rows) != 0 {
		t.Errorf("empty selection = %+v", e)
	}
	if !s.Has(4) || s.Has(6) || s.Index(5) != 2 || s.Index(0) != -1 {
		t.Errorf("Has / Index wrong for %+v", s)
	}
}

func TestRowSelectionClick(t *testing.T) {
	const n = 36
	one := func(r int) RowSelection { return RowSelection{Rows: []int{r}, Anchor: r, Cursor: r} }
	tests := []struct {
		name           string
		from           RowSelection
		row            int
		extend, toggle bool
		want           RowSelection
	}{
		{"plain click", one(3), 15, false, false, one(15)},
		{"plain click collapses", RowSelection{Rows: []int{15, 16, 17}, Anchor: 15, Cursor: 17}, 16, false, false, one(16)},
		{"shift-click down", one(15), 17, true, false, RowSelection{Rows: []int{15, 16, 17}, Anchor: 15, Cursor: 17}},
		{"shift-click up", one(15), 13, true, false, RowSelection{Rows: []int{13, 14, 15}, Anchor: 15, Cursor: 13}},
		{"shift-click again keeps the anchor", RowSelection{Rows: []int{15, 16, 17}, Anchor: 15, Cursor: 17}, 16, true, false, RowSelection{Rows: []int{15, 16}, Anchor: 15, Cursor: 16}},
		{"shift-click with no anchor", RowSelection{Anchor: -1, Cursor: -1}, 4, true, false, one(4)},
		{"cmd-click adds", RowSelection{Rows: []int{15, 16, 17}, Anchor: 15, Cursor: 17}, 19, false, true, RowSelection{Rows: []int{15, 16, 17, 19}, Anchor: 19, Cursor: 19}},
		{"cmd-click removes", RowSelection{Rows: []int{15, 16, 17}, Anchor: 15, Cursor: 17}, 16, false, true, RowSelection{Rows: []int{15, 17}, Anchor: 16, Cursor: 16}},
		{"cmd-click keeps the last row", one(15), 15, false, true, one(15)},
		{"shift-cmd-click adds a range", RowSelection{Rows: []int{2, 15}, Anchor: 15, Cursor: 15}, 17, true, true, RowSelection{Rows: []int{2, 15, 16, 17}, Anchor: 15, Cursor: 17}},
		{"a row outside the list", one(3), 36, false, false, one(3)},
		{"a negative row", one(3), -1, true, false, one(3)},
	}
	for _, tt := range tests {
		if got := tt.from.Click(tt.row, n, tt.extend, tt.toggle); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: Click(%d) = %+v, want %+v", tt.name, tt.row, got, tt.want)
		}
	}
}

func TestRowSelectionStep(t *testing.T) {
	const n = 10
	one := func(r int) RowSelection { return RowSelection{Rows: []int{r}, Anchor: r, Cursor: r} }
	tests := []struct {
		name   string
		from   RowSelection
		delta  int
		extend bool
		want   RowSelection
	}{
		{"down", one(3), 1, false, one(4)},
		{"up", one(3), -1, false, one(2)},
		{"down at the end", one(9), 1, false, one(9)},
		{"up at the top", one(0), -1, false, one(0)},
		{"down collapses a range", RowSelection{Rows: []int{3, 4, 5}, Anchor: 3, Cursor: 5}, 1, false, one(6)},
		{"shift-down", one(3), 1, true, RowSelection{Rows: []int{3, 4}, Anchor: 3, Cursor: 4}},
		{"shift-down twice", RowSelection{Rows: []int{3, 4}, Anchor: 3, Cursor: 4}, 1, true, RowSelection{Rows: []int{3, 4, 5}, Anchor: 3, Cursor: 5}},
		{"shift-up shrinks", RowSelection{Rows: []int{3, 4, 5}, Anchor: 3, Cursor: 5}, -1, true, RowSelection{Rows: []int{3, 4}, Anchor: 3, Cursor: 4}},
		{"shift-up past the anchor", one(3), -2, true, RowSelection{Rows: []int{1, 2, 3}, Anchor: 3, Cursor: 1}},
		{"shift-end", one(7), 1 << 20, true, RowSelection{Rows: []int{7, 8, 9}, Anchor: 7, Cursor: 9}},
		{"home", one(7), -(1 << 20), false, one(0)},
		{"down from nothing", RowSelection{Anchor: -1, Cursor: -1}, 1, false, one(0)},
		{"up from nothing", RowSelection{Anchor: -1, Cursor: -1}, -1, false, one(9)},
		{"shift-down from nothing", RowSelection{Anchor: -1, Cursor: -1}, 1, true, RowSelection{Rows: []int{0}, Anchor: 0, Cursor: 0}},
	}
	for _, tt := range tests {
		if got := tt.from.Step(tt.delta, n, tt.extend); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: Step(%d, %v) = %+v, want %+v", tt.name, tt.delta, tt.extend, got, tt.want)
		}
	}
	if got := one(3).Step(1, 0, false); !reflect.DeepEqual(got, one(3)) {
		t.Errorf("Step in an empty list = %+v", got)
	}
}

func TestRowSelectionAll(t *testing.T) {
	got := RowSelection{Rows: []int{4}, Anchor: 4, Cursor: 4}.All(5)
	want := RowSelection{Rows: []int{0, 1, 2, 3, 4}, Anchor: 0, Cursor: 4}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("All = %+v, want %+v", got, want)
	}
	if !got.Equal(want) || got.Equal(RowSelection{}) {
		t.Errorf("Equal is wrong")
	}
}
