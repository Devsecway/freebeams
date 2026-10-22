package ui

import (
	"freebeams/internal/ui/uicore"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEvent"
	"graphics.gd/classdb/InputEventKey"
	"graphics.gd/classdb/InputEventMouseButton"
	"graphics.gd/classdb/PopupMenu"
	"graphics.gd/classdb/ScrollContainer"
	"graphics.gd/classdb/VBoxContainer"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2"
)

// ScheduleRow is one schedule item: title (semibold) over an italic subtitle
// on a raised card. Live marks the item that is on the output (red thumbnail
// outline).
type ScheduleRow struct {
	Title    string
	Subtitle string
	Live     bool
	// Theme is the name of the item's theme (checked in the Theme submenu).
	Theme string
}

// Row geometry (TsdScheduleSelector, large view).
const (
	schedRowH      = 46
	schedRowSmallH = 38
	schedThumbW    = 64
	schedThumbH    = 36

	// schedCardInset is the horizontal inset of an item's card in its row
	// (1px above and below); the thumbnail at x = 8 sits 4px inside it.
	schedCardInset = 4
)

// Context menu ids.
const (
	schedMenuUp = iota
	schedMenuDown
	schedMenuRemove
)

// ScheduleList is the schedule panel's item list.
type ScheduleList struct {
	Control.Extension[ScheduleList] `gd:"FwScheduleList"`

	OnSelect   func(i int)        `gd:"-"`
	OnActivate func(i int)        `gd:"-"` // double-click / Enter
	OnRemove   func(i int)        `gd:"-"` // context menu "Remove" or Delete key
	OnMove     func(from, to int) `gd:"-"` // context menu Move Up / Move Down
	// OnSetTheme reports a choice in the context menu's Theme submenu: theme
	// index (into SetThemeNames) for row i.
	OnSetTheme func(i, theme int) `gd:"-"`

	rows       []ScheduleRow
	selected   int
	factory    func(i int) Control.Instance
	thumbnails bool
	themes     []string // Theme submenu entries

	scroll    ScrollContainer.Instance
	list      VBoxContainer.Instance
	views     []*scheduleRowView
	menu      PopupMenu.Instance
	themeMenu PopupMenu.Instance
	menuRow   int
	built     bool

	// Drag and drop (dragdrop.go): a song, the selected verses, a media or
	// presentation tile dropped into gap at (0..len(rows)) is OnDropInsert; a
	// theme dropped onto a row is OnDropOnto; a row dragged to another gap is
	// OnMove. index is the source's row or tile.
	OnDropInsert func(source string, index, at int)  `gd:"-"`
	OnDropOnto   func(source string, index, row int) `gd:"-"`
	drop         schedDrop
	dropOverlay  Control.Instance
	dropTicker   *ticker
}

// NewScheduleList creates an empty schedule list.
func NewScheduleList() *ScheduleList {
	s := &ScheduleList{selected: -1, thumbnails: true}
	c := s.AsControl()
	c.SetFocusMode(Control.FocusAll)
	c.SetClipContents(true)
	bg := panel(vBk1)
	bg.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	bg.AsControl().SetMouseFilter(Control.MouseFilterPass)
	add(s, bg.AsControl())
	s.scroll = ScrollContainer.New()
	s.scroll.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	s.scroll.SetHorizontalScrollMode(ScrollContainer.ScrollModeDisabled)
	s.scroll.AsControl().SetMouseFilter(Control.MouseFilterPass)
	s.list = vbox(0)
	s.list.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	s.list.AsControl().SetMouseFilter(Control.MouseFilterPass)
	add(s.scroll, s.list.AsControl())
	add(s, s.scroll.AsControl())

	s.menu = PopupMenu.New()
	s.menu.MoreArgs().AddItem("Move Item Up", schedMenuUp, 0)
	s.menu.MoreArgs().AddItem("Move Item Down", schedMenuDown, 0)
	s.menu.AddSeparator()
	s.themeMenu = PopupMenu.New()
	s.themeMenu.OnIdPressed(func(id int) {
		if s.OnSetTheme != nil && s.menuRow >= 0 && s.menuRow < len(s.rows) && id >= 0 && id < len(s.themes) {
			s.OnSetTheme(s.menuRow, id)
		}
	})
	// The submenu is named and referenced by name: passing the node itself
	// (add_submenu_node_item) trips graphics.gd's ownership check.
	s.themeMenu.AsNode().SetName("ThemeMenu")
	s.menu.AsNode().AddChild(s.themeMenu.AsNode())
	s.menu.AddSubmenuItem("Theme", "ThemeMenu")
	s.menu.AddSeparator()
	s.menu.MoreArgs().AddItem("Remove Item from Schedule", schedMenuRemove, 0)
	s.menu.OnIdPressed(s.menuPressed)
	s.AsNode().AddChild(s.menu.AsNode())
	s.initDrop()
	return s
}

// SetThemeNames sets the themes the context menu's Theme submenu offers (the
// per-item theme override), in the order OnSetTheme reports them.
func (s *ScheduleList) SetThemeNames(names []string) {
	s.themes = append([]string(nil), names...)
}

// SetRows replaces the rows. The selection is kept when still in range.
func (s *ScheduleList) SetRows(rows []ScheduleRow) {
	s.rows = append([]ScheduleRow(nil), rows...)
	if s.selected >= len(s.rows) {
		s.selected = -1
	}
	s.built = false
	s.render()
}

// SetSelected selects row i (-1 for none) without calling OnSelect.
func (s *ScheduleList) SetSelected(i int) {
	s.selected = i
	s.render()
}

// Selected returns the selected row (-1 for none).
func (s *ScheduleList) Selected() int { return s.selected }

// RowCenter returns the centre of row i in its viewport's coordinates (for
// automated clicks); false when there is no such row.
func (s *ScheduleList) RowCenter(i int) (Vector2.XY, bool) {
	if i < 0 || i >= len(s.views) {
		return Vector2.Zero, false
	}
	r := s.views[i].AsControl().GetGlobalRect()
	return Vector2.New(r.Position.X+r.Size.X/2, r.Position.Y+r.Size.Y/2), true
}

// SetThumbnailFactory sets the thumbnail factory (one small 16:9 thumbnail per row).
func (s *ScheduleList) SetThumbnailFactory(f func(i int) Control.Instance) {
	s.factory = f
	s.built = false
	s.render()
}

// SetThumbnailsVisible switches between the large (thumbnail) and small rows
// (the schedule's view sizer).
func (s *ScheduleList) SetThumbnailsVisible(on bool) {
	s.thumbnails = on
	s.built = false
	s.render()
}

// ThumbnailsVisible reports the view-sizer state.
func (s *ScheduleList) ThumbnailsVisible() bool { return s.thumbnails }

func (s *ScheduleList) render() {
	if !s.built {
		s.rebuild()
	}
	for _, v := range s.views {
		v.AsCanvasItem().QueueRedraw()
	}
	if s.selected >= 0 && s.selected < len(s.views) {
		s.scroll.EnsureControlVisible(s.views[s.selected].AsControl())
	}
}

func (s *ScheduleList) rebuild() {
	s.built = true
	for _, v := range s.views {
		s.list.AsNode().RemoveChild(v.AsNode())
		v.AsNode().QueueFree()
	}
	s.views = s.views[:0]
	h := float32(schedRowSmallH)
	if s.thumbnails {
		h = schedRowH
	}
	for i := range s.rows {
		v := &scheduleRowView{list: s, index: i}
		vc := v.AsControl()
		vc.SetCustomMinimumSize(Vector2.New(0, h))
		vc.SetMouseFilter(Control.MouseFilterStop)
		if s.thumbnails {
			v.thumb = newThumbHost()
			tc := v.thumb.AsControl()
			tc.SetMouseFilter(Control.MouseFilterIgnore)
			tc.SetPosition(Vector2.New(8, (h-schedThumbH)/2-0.5))
			tc.SetSize(Vector2.New(schedThumbW, schedThumbH))
			if s.factory != nil {
				if content := s.factory(i); content != (Control.Instance{}) {
					content.SetMouseFilter(Control.MouseFilterIgnore)
					v.thumb.SetContent(content)
				}
			}
			add(v, tc)
		}
		add(s.list, vc)
		s.views = append(s.views, v)
	}
}

func (s *ScheduleList) userSelect(i int) {
	if i < 0 || i >= len(s.rows) {
		return
	}
	changed := i != s.selected
	s.selected = i
	s.render()
	if changed && s.OnSelect != nil {
		s.OnSelect(i)
	}
}

func (s *ScheduleList) move(i, delta int) {
	to, ok := uicore.MoveTarget(i, len(s.rows), delta)
	if ok && s.OnMove != nil {
		s.OnMove(i, to)
	}
}

func (s *ScheduleList) remove(i int) {
	if i >= 0 && i < len(s.rows) && s.OnRemove != nil {
		s.OnRemove(i)
	}
}

func (s *ScheduleList) menuPressed(id int) {
	switch id {
	case schedMenuUp:
		s.move(s.menuRow, -1)
	case schedMenuDown:
		s.move(s.menuRow, +1)
	case schedMenuRemove:
		s.remove(s.menuRow)
	}
}

func (s *ScheduleList) showMenu(i int) {
	s.menuRow = i
	s.menu.SetItemDisabled(0, i <= 0)
	s.menu.SetItemDisabled(1, i >= len(s.rows)-1)
	// The Theme submenu: one radio entry per theme, the row's own checked.
	s.themeMenu.Clear()
	current := ""
	if i >= 0 && i < len(s.rows) {
		current = s.rows[i].Theme
	}
	for k, name := range s.themes {
		s.themeMenu.MoreArgs().AddRadioCheckItem(name, k, 0)
		s.themeMenu.SetItemChecked(k, name == current)
	}
	if len(s.themes) == 0 {
		s.themeMenu.MoreArgs().AddItem("No themes", -1, 0)
		s.themeMenu.SetItemDisabled(0, true)
	}
	popupAtMouse(s.menu)
}

func (s *ScheduleList) GuiInput(ev InputEvent.Instance) {
	k, ok := Object.As[InputEventKey.Instance](ev)
	if !ok || !k.AsInputEvent().IsPressed() {
		return
	}
	switch k.Keycode() {
	case Input.KeyUp:
		s.userSelect(uicore.Navigate(s.selected, len(s.rows), 1, uicore.NavUp))
	case Input.KeyDown:
		s.userSelect(uicore.Navigate(s.selected, len(s.rows), 1, uicore.NavDown))
	case Input.KeyEnter, Input.KeyKpEnter:
		if s.selected >= 0 && s.OnActivate != nil {
			s.OnActivate(s.selected)
		}
	case Input.KeyDelete, Input.KeyBackspace:
		s.remove(s.selected)
	default:
		return
	}
	acceptEvent(s.AsControl())
}

// scheduleRowView draws one schedule row.
type scheduleRowView struct {
	Control.Extension[scheduleRowView] `gd:"FwScheduleRow"`

	list  *ScheduleList
	index int
	thumb *MonitorHost
	hover bool
}

func (v *scheduleRowView) Draw() {
	if v.list == nil || v.index >= len(v.list.rows) {
		return
	}
	c := v.AsControl()
	ci := v.AsCanvasItem()
	size := c.Size()
	row := v.list.rows[v.index]
	selected := v.index == v.list.selected
	// Each item is a raised card (card.bk, EasyWorship's Mac look) inset in
	// its row; the row background shows in the gaps between cards.
	card := rect(schedCardInset, 1, size.X-2*schedCardInset, size.Y-2)
	title, sub := fwColor(c, "text_semibold"), alpha(fwColor(c, "text"), 0.8)
	if selected {
		fwBox(c, "card_sel").Draw(ci, card)
		title, sub = fwColor(c, "highlight_text"), alpha(fwColor(c, "highlight_text"), 0.8)
	} else {
		fwBox(c, "card").Draw(ci, card)
		if v.hover {
			fwBox(c, "card_hot").Draw(ci, card)
		}
	}
	x := float32(10)
	if v.thumb != nil {
		tr := rect(8, (size.Y-schedThumbH)/2-0.5, schedThumbW, schedThumbH)
		ci.DrawRect(tr, fwColor(c, "thumb_bg"))
		if row.Live {
			ci.MoreArgs().DrawRect(rect(tr.Position.X-1, tr.Position.Y-1, tr.Size.X+2, tr.Size.Y+2), fwColor(c, "red"), false, 2, false)
		}
		x = 8 + schedThumbW + 9
	} else if row.Live {
		ci.DrawRect(rect(card.Position.X, card.Position.Y+2, 3, card.Size.Y-4), fwColor(c, "red"))
	}
	w := size.X - x - 6 - schedCardInset
	mid := (size.Y - 1) / 2
	drawText(ci, fwFont(c, "semibold"), fontBase, rect(x, mid-17, w, 18), row.Title, GUI.HorizontalAlignmentLeft, title)
	drawText(ci, fwFont(c, "italic"), fontCaption, rect(x, mid+1, w, 15), row.Subtitle, GUI.HorizontalAlignmentLeft, sub)
}

func (v *scheduleRowView) GuiInput(ev InputEvent.Instance) {
	mb, ok := Object.As[InputEventMouseButton.Instance](ev)
	if !ok || !mb.AsInputEvent().IsPressed() {
		return
	}
	switch mb.ButtonIndex() {
	case Input.MouseButtonLeft:
		v.list.AsControl().GrabFocus()
		v.list.userSelect(v.index)
		if mb.DoubleClick() && v.list.OnActivate != nil {
			v.list.OnActivate(v.index)
		}
	case Input.MouseButtonRight:
		v.list.AsControl().GrabFocus()
		v.list.userSelect(v.index)
		v.list.showMenu(v.index)
	default:
		return
	}
	acceptEvent(v.AsControl())
}

func (v *scheduleRowView) Notification(what Object.Notification) {
	switch what {
	case Control.NotificationMouseEnter:
		v.hover = true
		v.AsCanvasItem().QueueRedraw()
	case Control.NotificationMouseExit:
		v.hover = false
		v.AsCanvasItem().QueueRedraw()
	}
}
