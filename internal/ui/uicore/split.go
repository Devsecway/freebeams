package uicore

import "math"

// Collapse says which pane of a splitter is snapped closed.
type Collapse int

const (
	CollapseNone   Collapse = 0
	CollapseFirst  Collapse = 1
	CollapseSecond Collapse = 2
)

// Split describes a TsdSplitterPanel along its main axis.
//
// Percent is the first pane's share of the space available to the panes
// (Total minus the bar). Min1/Min2 are hard minimums; Snap1/Snap2 are snap
// thresholds: dragging a pane below its snap size closes it (0 disables).
type Split struct {
	Total    float64
	Bar      float64 // bar thickness while open
	Closed   float64 // bar (grip) thickness while a pane is snapped closed
	Percent  float64
	Min1     float64
	Min2     float64
	Snap1    float64
	Snap2    float64
	Collapse Collapse
}

// BarSize is the bar thickness in the current state.
func (s Split) BarSize() float64 {
	if s.Collapse != CollapseNone && s.Closed > 0 {
		return s.Closed
	}
	return s.Bar
}

// Available is the space shared by the two panes.
func (s Split) Available() float64 {
	return math.Max(0, s.Total-s.BarSize())
}

// Layout returns the first pane's size and the bar's offset (they are equal:
// the bar starts where the first pane ends) and the second pane's size.
func (s Split) Layout() (first, second float64) {
	avail := s.Available()
	switch s.Collapse {
	case CollapseFirst:
		return 0, avail
	case CollapseSecond:
		return avail, 0
	}
	first = math.Round(s.Percent * avail)
	first = s.clampFirst(first, avail)
	return first, avail - first
}

// clampFirst applies Min1/Min2. When the space cannot satisfy both minimums
// the first pane's minimum wins (EasyWorship keeps the top/left pane usable).
func (s Split) clampFirst(first, avail float64) float64 {
	if hi := avail - s.Min2; first > hi {
		first = hi
	}
	if first < s.Min1 {
		first = s.Min1
	}
	if first > avail {
		first = avail
	}
	if first < 0 {
		first = 0
	}
	return first
}

// Drag computes the new state when the user drags the bar so the first pane
// would be `pos` long. Dragging into a snap zone collapses that pane and keeps
// the previous Percent so a later reopen restores it.
func (s Split) Drag(pos float64) Split {
	open := s
	open.Collapse = CollapseNone
	avail := open.Available()
	if avail <= 0 {
		return s
	}
	if s.Snap1 > 0 && pos < s.Snap1/2 {
		s.Collapse = CollapseFirst
		return s
	}
	if s.Snap2 > 0 && avail-pos < s.Snap2/2 {
		s.Collapse = CollapseSecond
		return s
	}
	// Between half the snap size and the snap size the pane holds at its snap
	// size, so the snap feels like a detent rather than a jump.
	if s.Snap1 > 0 && pos < s.Snap1 {
		pos = s.Snap1
	}
	if s.Snap2 > 0 && avail-pos < s.Snap2 {
		pos = avail - s.Snap2
	}
	pos = open.clampFirst(pos, avail)
	s.Collapse = CollapseNone
	s.Percent = pos / avail
	return s
}

// Toggle reopens a collapsed splitter, or collapses the given pane.
func (s Split) Toggle(pane Collapse) Split {
	if s.Collapse != CollapseNone {
		s.Collapse = CollapseNone
	} else {
		s.Collapse = pane
	}
	return s
}
