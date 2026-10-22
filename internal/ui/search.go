package ui

import (
	"time"

	"freebeams/internal/ui/uicore"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/LineEdit"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/PanelContainer"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Vector2"
)

// searchDebounce is the quiet period before typing triggers a search.
const searchDebounce = 200 * time.Millisecond

// ticker calls fn every frame while processing is enabled. Widgets enable it
// only while a debounced value is pending, so the idle UI does no work.
type ticker struct {
	Node.Extension[ticker] `gd:"FwTicker"`

	fn func()
}

func newTicker(fn func()) *ticker {
	t := &ticker{fn: fn}
	t.AsNode().SetProcess(false)
	return t
}

func (t *ticker) Process(delta Float.X) {
	if t.fn != nil {
		t.fn()
	}
}

// searchLocator is EasyWorship's embedded TsdSearchLocator: a 24px strip with a
// search glyph, an edit field and a clear button. Typing is debounced; Enter
// fires immediately.
type searchLocator struct {
	onSearch func(text string)

	root PanelContainer.Instance
	edit LineEdit.Instance
	tick *ticker
	deb  uicore.Debouncer

	// onEdit, when set, sees every text change at once (not debounced).
	onEdit func(string)
}

func newSearchLocator(placeholder string, onSearch func(string)) *searchLocator {
	s := &searchLocator{onSearch: onSearch, deb: uicore.Debouncer{Delay: searchDebounce}}
	s.root = panel(vSearchPanel)
	s.root.AsControl().SetCustomMinimumSize(Vector2.New(0, 24))
	row := hbox(4)
	add(s.root, row.AsControl())

	icon := &searchIcon{}
	icon.AsControl().SetCustomMinimumSize(Vector2.New(16, 16))
	icon.AsControl().SetSizeFlagsVertical(Control.SizeShrinkCenter)
	icon.AsControl().SetMouseFilter(Control.MouseFilterIgnore)

	s.edit = LineEdit.New()
	s.edit.AsControl().SetThemeTypeVariation(vSearch)
	s.edit.SetPlaceholderText(placeholder)
	s.edit.SetClearButtonEnabled(true)
	s.edit.SetCaretBlink(true)
	s.edit.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	s.edit.OnTextChanged(func(text string) {
		s.deb.Trigger(time.Now(), text)
		s.tick.AsNode().SetProcess(true)
		if s.onEdit != nil {
			s.onEdit(text)
		}
	})
	s.edit.OnTextSubmitted(func(text string) {
		s.deb.Trigger(time.Now(), text)
		if v, ok := s.deb.Flush(); ok && s.onSearch != nil {
			s.onSearch(v)
		}
		s.tick.AsNode().SetProcess(false)
	})
	add(row, icon.AsControl(), s.edit.AsControl())

	s.tick = newTicker(s.poll)
	s.root.AsNode().AddChild(s.tick.AsNode())
	return s
}

func (s *searchLocator) poll() {
	if v, ok := s.deb.Poll(time.Now()); ok && s.onSearch != nil {
		s.onSearch(v)
	}
	if !s.deb.Pending() {
		s.tick.AsNode().SetProcess(false)
	}
}

// setText replaces the text without triggering a search, with the caret at
// the end (LineEdit.SetText puts it at the start).
func (s *searchLocator) setText(text string) {
	if s.edit.Text() == text {
		return
	}
	s.edit.SetText(text)
	s.edit.SetCaretColumn(len([]rune(text)))
	s.deb.Reset(text)
}

// searchIcon draws the magnifier glyph.
type searchIcon struct {
	Control.Extension[searchIcon] `gd:"FwSearchIcon"`
}

func (i *searchIcon) Draw() {
	c := i.AsControl()
	drawGlyph(i.AsCanvasItem(), c, "search", Vector2.New(0, 0), 16, alpha(fwColor(c, "glyph"), 0.75))
}
