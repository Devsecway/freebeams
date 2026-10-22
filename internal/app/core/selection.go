package core

import "slices"

// RowSelection is a multi-row list selection with the usual desktop rules (the
// verse list): a click selects one row, Shift-click selects the range from the
// anchor, Cmd-click (Ctrl elsewhere) toggles a row, and the arrow keys move the
// cursor, extending the range from the anchor with Shift. The selection is
// never emptied by a toggle: the last selected row stays selected.
type RowSelection struct {
	Rows   []int // selected rows, ascending, unique
	Anchor int   // where a Shift range starts; -1 for none
	Cursor int   // the row the keyboard moves from (the last clicked); -1 for none
}

// NewRowSelection selects rows (any order, duplicates and negatives ignored),
// with the anchor and cursor on the first of them.
func NewRowSelection(rows []int) RowSelection {
	s := RowSelection{Anchor: -1, Cursor: -1}
	for _, r := range rows {
		if r >= 0 {
			s.Rows = append(s.Rows, r)
		}
	}
	slices.Sort(s.Rows)
	s.Rows = slices.Compact(s.Rows)
	if len(s.Rows) > 0 {
		s.Anchor, s.Cursor = s.Rows[0], s.Rows[0]
	}
	return s
}

// Has reports whether row is selected.
func (s RowSelection) Has(row int) bool {
	_, ok := slices.BinarySearch(s.Rows, row)
	return ok
}

// Index is row's position among the selected rows (-1 when not selected).
func (s RowSelection) Index(row int) int {
	if i, ok := slices.BinarySearch(s.Rows, row); ok {
		return i
	}
	return -1
}

// Equal reports whether two selections are the same.
func (s RowSelection) Equal(o RowSelection) bool {
	return s.Anchor == o.Anchor && s.Cursor == o.Cursor && slices.Equal(s.Rows, o.Rows)
}

// span returns rows lo..hi (either order).
func span(a, b int) []int {
	lo, hi := min(a, b), max(a, b)
	out := make([]int, 0, hi-lo+1)
	for r := lo; r <= hi; r++ {
		out = append(out, r)
	}
	return out
}

// Click applies a click on row of a list of n rows. extend is Shift, toggle
// Cmd (Ctrl on Windows and Linux); Shift with Cmd adds the range to the
// selection. A row outside the list changes nothing.
func (s RowSelection) Click(row, n int, extend, toggle bool) RowSelection {
	if row < 0 || row >= n {
		return s
	}
	anchor := s.Anchor
	if anchor < 0 || anchor >= n {
		anchor = row
	}
	switch {
	case extend && toggle:
		out := NewRowSelection(append(slices.Clone(s.Rows), span(anchor, row)...))
		out.Anchor, out.Cursor = anchor, row
		return out
	case extend:
		return RowSelection{Rows: span(anchor, row), Anchor: anchor, Cursor: row}
	case toggle:
		if s.Has(row) {
			if len(s.Rows) == 1 {
				return RowSelection{Rows: slices.Clone(s.Rows), Anchor: row, Cursor: row}
			}
			rows := slices.DeleteFunc(slices.Clone(s.Rows), func(r int) bool { return r == row })
			return RowSelection{Rows: rows, Anchor: row, Cursor: row}
		}
		out := NewRowSelection(append(slices.Clone(s.Rows), row))
		out.Anchor, out.Cursor = row, row
		return out
	}
	return RowSelection{Rows: []int{row}, Anchor: row, Cursor: row}
}

// Step moves the cursor by delta rows in a list of n (clamped to the list;
// Home / End pass a delta beyond it). Without extend the cursor row becomes
// the only selected row; with extend (Shift) the selection is the range from
// the anchor to the new cursor. From no cursor, a step down starts at the
// first row and a step up at the last.
func (s RowSelection) Step(delta, n int, extend bool) RowSelection {
	if n <= 0 {
		return s
	}
	var to int
	switch {
	case s.Cursor < 0 || s.Cursor >= n:
		to = 0
		if delta < 0 {
			to = n - 1
		}
	default:
		to = min(max(s.Cursor+delta, 0), n-1)
	}
	if !extend {
		return RowSelection{Rows: []int{to}, Anchor: to, Cursor: to}
	}
	anchor := s.Anchor
	if anchor < 0 || anchor >= n {
		anchor = max(s.Cursor, 0)
		if s.Cursor >= n {
			anchor = to
		}
	}
	return RowSelection{Rows: span(anchor, to), Anchor: anchor, Cursor: to}
}

// All selects every row of a list of n (Cmd+A), keeping the cursor.
func (s RowSelection) All(n int) RowSelection {
	if n <= 0 {
		return s
	}
	cur := s.Cursor
	if cur < 0 || cur >= n {
		cur = 0
	}
	return RowSelection{Rows: span(0, n-1), Anchor: 0, Cursor: cur}
}
