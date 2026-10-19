package ui

import (
	"math"

	"freebeams/internal/ui/uicore"

	"graphics.gd/classdb/Container"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEvent"
	"graphics.gd/classdb/InputEventMouseButton"
	"graphics.gd/classdb/InputEventMouseMotion"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2"
)

// SplitOrientation follows EasyWorship's TsdSplitterPanel naming.
type SplitOrientation int

const (
	// SplitVertical stacks the panes top / bottom (horizontal bar). EW default.
	SplitVertical SplitOrientation = iota
	// SplitHorizontal puts the panes side by side (vertical bar). EW slHorizontal.
	SplitHorizontal
)

// Collapsed-pane values for Splitter.Collapsed / SetCollapsed.
const (
	CollapseNone   = int(uicore.CollapseNone)
	CollapseFirst  = int(uicore.CollapseFirst)
	CollapseSecond = int(uicore.CollapseSecond)
)

const (
	splitBar      = 1 // visible bar thickness
	splitGrab     = 3 // extra invisible grab area on each side of the bar
	splitClosed   = 8 // snapped-closed grip thickness
	splitDragSlop = 3 // pixels before a press on the bar becomes a drag
)

// Splitter is EasyWorship's TsdSplitterPanel: two panes, a 1px bar, a
// percentage split, hard minimums and snap-to-close thresholds. A snapped-closed
// pane leaves a gradient grip; clicking it reopens the pane. Godot's
// SplitContainer has neither percentages nor snap-close, hence this container.
// It persists nothing itself: OnChanged reports user changes.
type Splitter struct {
	Container.Extension[Splitter] `gd:"FwSplitter"`

	Orientation SplitOrientation
	Percent     float64 // first pane fraction of the space shared by the panes
	Min1, Min2  float64
	Snap1       float64 // first pane snap-closes when dragged below this (0 = never)
	Snap2       float64 // second pane snap-closes when dragged below this (0 = never)

	// OnChanged is called after the user drags the bar or toggles a pane.
	OnChanged func(percent float64)
	// OnCollapsedChanged is called when the user snaps a pane closed or reopens it.
	OnCollapsedChanged func(collapsed int)

	first, second Control.Instance
	bar           Control.Instance // optional custom bar (e.g. the resource tab bar)
	grab          Control.Instance // invisible grab area over the 1px bar
	collapse      uicore.Collapse

	pressed    bool
	moved      bool
	pressAt    float64
	pressFirst float64
}

// NewSplitter creates a splitter with EasyWorship's constraint fields.
func NewSplitter(o SplitOrientation, percent, min1, min2, snap1, snap2 float64) *Splitter {
	s := &Splitter{Orientation: o, Percent: percent, Min1: min1, Min2: min2, Snap1: snap1, Snap2: snap2}
	c := s.AsControl()
	c.SetClipContents(true)
	c.SetMouseFilter(Control.MouseFilterPass)
	s.grab = Control.New()
	s.grab.SetMouseFilter(Control.MouseFilterStop)
	if o == SplitVertical {
		s.grab.SetMouseDefaultCursorShape(Control.CursorVsplit)
	} else {
		s.grab.SetMouseDefaultCursorShape(Control.CursorHsplit)
	}
	s.grab.OnGuiInput(s.barInput)
	c.AsNode().AddChild(s.grab.AsNode())
	return s
}

// SetPanes sets the two pane controls.
func (s *Splitter) SetPanes(first, second Control.Instance) {
	s.first, s.second = first, second
	for _, p := range []Control.Instance{first, second} {
		p.SetClipContents(true)
		s.AsNode().AddChild(p.AsNode())
	}
	s.AsNode().MoveChild(s.grab.AsNode(), -1)
	s.render()
}

// SetBarControl replaces the 1px bar with a control (EasyWorship's
// TsdSplitterBar): its thickness along the split axis is its minimum size,
// dragging its empty area moves the split and clicking it toggles the second
// pane open / closed.
func (s *Splitter) SetBarControl(bar Control.Instance) {
	s.bar = bar
	s.AsNode().AddChild(bar.AsNode())
	bar.SetMouseDefaultCursorShape(Control.CursorPointingHand)
	bar.OnGuiInput(s.barInput)
	s.grab.AsCanvasItem().SetVisible(false)
	s.render()
}

// HandleCenter returns the global position of the drag handle's centre: the
// custom bar when one is set (its left end, clear of the tab buttons), else
// the grab area over the 1px bar. Used to drive real drags in tests.
func (s *Splitter) HandleCenter() (Vector2.XY, bool) {
	h := s.grab
	if s.bar != (Control.Instance{}) {
		h = s.bar
	}
	if !h.AsCanvasItem().IsVisibleInTree() {
		return Vector2.Zero, false
	}
	r := h.GetGlobalRect()
	if s.bar != (Control.Instance{}) {
		// The tab buttons sit at the left; the empty area is to the right.
		return Vector2.New(r.Position.X+r.Size.X*0.6, r.Position.Y+r.Size.Y/2), true
	}
	return Vector2.New(r.Position.X+r.Size.X/2, r.Position.Y+r.Size.Y/2), true
}

// SetPercent sets the split programmatically (does not call OnChanged).
func (s *Splitter) SetPercent(p float64) {
	s.Percent = math.Min(1, math.Max(0, p))
	s.render()
}

// Collapsed returns CollapseNone, CollapseFirst or CollapseSecond.
func (s *Splitter) Collapsed() int { return int(s.collapse) }

// SetCollapsed snaps a pane closed (or reopens with CollapseNone) without
// calling the callbacks.
func (s *Splitter) SetCollapsed(c int) {
	s.collapse = uicore.Collapse(c)
	s.render()
}

func (s *Splitter) render() {
	s.AsContainer().QueueSort()
	s.AsCanvasItem().QueueRedraw()
}

func (s *Splitter) spec() uicore.Split {
	size := s.AsControl().Size()
	total := float64(size.Y)
	if s.Orientation == SplitHorizontal {
		total = float64(size.X)
	}
	bar, closed := float64(splitBar), float64(splitClosed)
	if s.bar != (Control.Instance{}) {
		m := s.bar.GetCombinedMinimumSize()
		bar = float64(m.Y)
		if s.Orientation == SplitHorizontal {
			bar = float64(m.X)
		}
		closed = bar
	}
	return uicore.Split{Total: total, Bar: bar, Closed: closed, Percent: s.Percent,
		Min1: s.Min1, Min2: s.Min2, Snap1: s.Snap1, Snap2: s.Snap2, Collapse: s.collapse}
}

// axisRect builds a rect from an offset/length along the split axis.
func (s *Splitter) axisRect(off, length float64) (x, y, w, h float32) {
	size := s.AsControl().Size()
	if s.Orientation == SplitHorizontal {
		return float32(off), 0, float32(length), size.Y
	}
	return 0, float32(off), size.X, float32(length)
}

func (s *Splitter) Notification(what Object.Notification) {
	if what != Container.NotificationSortChildren {
		return
	}
	defer flushCalls(s.AsControl()) // apply the layout now (see flushCalls)
	sp := s.spec()
	first, second := sp.Layout()
	barSize := sp.BarSize()
	fit := func(c Control.Instance, off, length float64) {
		if c == (Control.Instance{}) {
			return
		}
		c.AsCanvasItem().SetVisible(length > 0)
		x, y, w, h := s.axisRect(off, length)
		s.AsContainer().FitChildInRect(c, rect(x, y, w, h))
	}
	fit(s.first, 0, first)
	fit(s.second, first+barSize, second)
	if s.bar != (Control.Instance{}) {
		fit(s.bar, first, barSize)
		return
	}
	grabOff, grabLen := first-splitGrab, barSize+2*splitGrab
	if s.collapse != uicore.CollapseNone {
		grabOff, grabLen = first, barSize
	}
	x, y, w, h := s.axisRect(grabOff, grabLen)
	s.AsContainer().FitChildInRect(s.grab, rect(x, y, w, h))
}

func (s *Splitter) Draw() {
	if s.bar != (Control.Instance{}) {
		return
	}
	c := s.AsControl()
	ci := s.AsCanvasItem()
	sp := s.spec()
	first, _ := sp.Layout()
	x, y, w, h := s.axisRect(first, sp.BarSize())
	if s.collapse == uicore.CollapseNone {
		ci.DrawRect(rect(x, y, w, h), fwColor(c, "splitter"))
		return
	}
	// Snapped closed: gradient grip (hsplitterbar.closed / vsplitterbar.closed)
	// framed by splitter lines, with a few grip dots in the middle.
	var cols []Color.RGBA
	if s.Orientation == SplitVertical {
		a, b := fwColor(c, "grip_a"), fwColor(c, "grip_b")
		cols = []Color.RGBA{a, a, b, b} // top → bottom
	} else {
		a, b := fwColor(c, "vgrip_a"), fwColor(c, "vgrip_b")
		cols = []Color.RGBA{a, b, b, a} // left → right
	}
	ci.DrawPolygon([]Vector2.XY{Vector2.New(x, y), Vector2.New(x+w, y), Vector2.New(x+w, y+h), Vector2.New(x, y+h)}, cols)
	line := fwColor(c, "splitter")
	dot := alpha(fwColor(c, "text"), 0.6)
	if s.Orientation == SplitVertical {
		ci.DrawRect(rect(x, y, w, 1), line)
		ci.DrawRect(rect(x, y+h-1, w, 1), line)
		for i := -2; i <= 2; i++ {
			ci.DrawRect(rect(x+w/2+float32(i)*5-1, y+h/2-1, 2, 2), dot)
		}
	} else {
		ci.DrawRect(rect(x, y, 1, h), line)
		ci.DrawRect(rect(x+w-1, y, 1, h), line)
		for i := -2; i <= 2; i++ {
			ci.DrawRect(rect(x+w/2-1, y+h/2+float32(i)*5-1, 2, 2), dot)
		}
	}
}

func (s *Splitter) axis(v Vector2.XY) float64 {
	if s.Orientation == SplitHorizontal {
		return float64(v.X)
	}
	return float64(v.Y)
}

// barInput turns presses / drags on the bar into split changes.
func (s *Splitter) barInput(ev InputEvent.Instance) {
	if mb, ok := Object.As[InputEventMouseButton.Instance](ev); ok {
		if mb.ButtonIndex() != Input.MouseButtonLeft {
			return
		}
		if mb.AsInputEvent().IsPressed() {
			s.pressed, s.moved = true, false
			s.pressAt = s.axis(mb.AsInputEventMouse().GlobalPosition())
			s.pressFirst, _ = s.spec().Layout()
			return
		}
		if !s.pressed {
			return
		}
		s.pressed = false
		if !s.moved {
			s.click()
		}
		return
	}
	if mm, ok := Object.As[InputEventMouseMotion.Instance](ev); ok && s.pressed {
		d := s.axis(mm.AsInputEventMouse().GlobalPosition()) - s.pressAt
		if !s.moved && math.Abs(d) < splitDragSlop {
			return
		}
		s.moved = true
		s.apply(s.spec().Drag(s.pressFirst + d))
	}
}

// click reopens a snapped pane; on a custom bar it toggles the second pane.
func (s *Splitter) click() {
	sp := s.spec()
	switch {
	case sp.Collapse != uicore.CollapseNone:
		s.apply(sp.Toggle(uicore.CollapseNone))
	case s.bar != (Control.Instance{}):
		s.apply(sp.Toggle(uicore.CollapseSecond))
	}
}

// apply commits a user change and reports it.
func (s *Splitter) apply(next uicore.Split) {
	percentChanged := next.Percent != s.Percent
	collapseChanged := next.Collapse != s.collapse
	s.Percent, s.collapse = next.Percent, next.Collapse
	s.render()
	if collapseChanged && s.OnCollapsedChanged != nil {
		s.OnCollapsedChanged(int(s.collapse))
	}
	if (percentChanged || collapseChanged) && s.OnChanged != nil {
		s.OnChanged(s.Percent)
	}
}
