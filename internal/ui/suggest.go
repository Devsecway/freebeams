package ui

import (
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEvent"
	"graphics.gd/classdb/InputEventMouseButton"
	"graphics.gd/classdb/InputEventMouseMotion"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2"
)

const (
	suggestRowH    = 22 // logical px per row
	suggestMaxRows = 8  // rows shown before the list scrolls with the highlight
	suggestPad     = 4  // the PopupMenu panel's content margin
)

// suggestionList is the reference field's dropdown: the books a partly typed
// reference may name (models.BookCompletions), drawn over the translations
// list right under the field. It is a dumb view: the app owns the items and
// the highlighted row; a click reports OnPick. It never takes keyboard focus,
// so the field keeps the caret while the operator picks.
type suggestionList struct {
	Control.Extension[suggestionList] `gd:"FwSuggestionList"`

	OnPick func(i int) `gd:"-"`

	items []string
	sel   int // highlighted (app state)
	hover int // under the pointer (view only)
	first int // first visible row (scrolled to keep sel in view)
}

func newSuggestionList() *suggestionList {
	l := &suggestionList{sel: -1, hover: -1}
	c := l.AsControl()
	c.SetFocusMode(Control.FocusNone)
	c.SetMouseFilter(Control.MouseFilterStop)
	c.SetAnchorsAndOffsetsPreset(Control.PresetTopWide)
	c.AsCanvasItem().SetVisible(false)
	return l
}

// set projects the items and the highlighted row; no items hides the list.
func (l *suggestionList) set(items []string, sel int) {
	l.items = append(l.items[:0], items...)
	l.sel = sel
	l.hover = -1
	n := len(items)
	switch {
	case sel < l.first:
		l.first = max(sel, 0)
	case sel >= l.first+suggestMaxRows:
		l.first = sel - suggestMaxRows + 1
	}
	l.first = min(l.first, max(n-suggestMaxRows, 0))
	c := l.AsControl()
	c.SetOffsetBottom(Float.X(min(n, suggestMaxRows)*suggestRowH + 2*suggestPad))
	c.AsCanvasItem().SetVisible(n > 0)
	c.AsCanvasItem().QueueRedraw()
}

// rowAt is the item index under local position p (-1 when none).
func (l *suggestionList) rowAt(p Vector2.XY) int {
	y := int(p.Y) - suggestPad
	if y < 0 {
		return -1
	}
	i := l.first + y/suggestRowH
	if i >= len(l.items) || i >= l.first+suggestMaxRows {
		return -1
	}
	return i
}

// rowCenter is item i's centre in local coordinates (false when scrolled out).
func (l *suggestionList) rowCenter(i int) (Vector2.XY, bool) {
	if i < l.first || i >= len(l.items) || i >= l.first+suggestMaxRows {
		return Vector2.XY{}, false
	}
	y := suggestPad + (i-l.first)*suggestRowH + suggestRowH/2
	return Vector2.New(l.AsControl().Size().X/2, float32(y)), true
}

func (l *suggestionList) GuiInput(ev InputEvent.Instance) {
	c := l.AsControl()
	if mm, ok := Object.As[InputEventMouseMotion.Instance](ev); ok {
		if h := l.rowAt(mm.AsInputEventMouse().Position()); h != l.hover {
			l.hover = h
			c.AsCanvasItem().QueueRedraw()
		}
		return
	}
	mb, ok := Object.As[InputEventMouseButton.Instance](ev)
	if !ok {
		return
	}
	if mb.ButtonIndex() == Input.MouseButtonLeft && mb.AsInputEvent().IsPressed() {
		if i := l.rowAt(mb.AsInputEventMouse().Position()); i >= 0 && l.OnPick != nil {
			l.OnPick(i)
		}
	}
	acceptEvent(c) // every click on the list is the list's (none reaches the Bibles under it)
}

func (l *suggestionList) Notification(what Object.Notification) {
	if what == Control.NotificationMouseExit && l.hover >= 0 {
		l.hover = -1
		l.AsControl().AsCanvasItem().QueueRedraw()
	}
}

func (l *suggestionList) Draw() {
	c := l.AsControl()
	ci := c.AsCanvasItem()
	size := c.Size()
	c.MoreArgs().GetThemeStylebox("panel", "PopupMenu").Draw(ci, rect(0, 0, size.X, size.Y))
	font := fwFont(c, "regular")
	for i := l.first; i < len(l.items) && i < l.first+suggestMaxRows; i++ {
		y := float32(suggestPad + (i-l.first)*suggestRowH)
		row := rect(suggestPad, y, size.X-2*suggestPad, suggestRowH)
		color := fwColor(c, "text")
		switch {
		case i == l.sel:
			fwBox(c, "toolbutton_sel").Draw(ci, row)
			color = fwColor(c, "highlight_text")
		case i == l.hover:
			fwBox(c, "toolbutton_hot").Draw(ci, row)
		}
		drawText(ci, font, fontBase, rect(row.Position.X+8, y, row.Size.X-16, suggestRowH), l.items[i], GUI.HorizontalAlignmentLeft, color)
	}
}
