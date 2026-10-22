package ui

import (
	"time"

	"freebeams/internal/ui/uicore"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/Theme"
	"graphics.gd/classdb/Tree"
	"graphics.gd/classdb/TreeItem"
	"graphics.gd/classdb/Viewport"
	"graphics.gd/variant/Color"
	"graphics.gd/variant/Float"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Rect2"
	"graphics.gd/variant/Vector2"
)

// Drag and drop into and within the schedule, as in EasyWorship.
//
// The sources (a song row, the selected verses, a media / presentation /
// theme tile, a schedule row) hand Godot a uicore.DragPayload from their
// _get_drag_data (an extension virtual, or set_drag_forwarding on the plain
// Trees) together with a small labelled preview. The schedule list accepts
// them: it shows an insertion line between rows (a highlighted row for a
// theme), scrolls when the pointer nears its top or bottom edge, and on the
// drop reports OnDropInsert / OnDropOnto / OnMove. The widgets stay dumb: the
// app decides what the payload becomes.

// Auto-scroll while a drag hovers near the schedule list's top or bottom.
const (
	dropScrollEdge  = 28  // px: the band at each edge that scrolls
	dropScrollSpeed = 900 // px per second at the very edge
)

// beginDrag starts a drag from src: it sets a labelled preview and returns
// the drag data. Call it from a _get_drag_data (or a drag forwarding func).
func beginDrag(src Control.Instance, p uicore.DragPayload, text string) any {
	pv := &dragPreview{text: text}
	c := pv.AsControl()
	c.SetMouseFilter(Control.MouseFilterIgnore)
	// The preview lives under the viewport, outside the main window's
	// themed tree: give it the theme explicitly.
	if th := themeOf(src); th != (Theme.Instance{}) {
		c.SetTheme(th)
	}
	c.AsCanvasItem().SetModulate(Color.RGBA{R: 1, G: 1, B: 1, A: 0.92})
	src.SetDragPreview(c)
	flushCalls(src) // the preview must land while Godot is still asking for the data
	return p.Encode()
}

// themeOf is the nearest explicitly set theme at or above c.
func themeOf(c Control.Instance) Theme.Instance {
	for c != (Control.Instance{}) {
		if th := c.Theme(); th != (Theme.Instance{}) {
			return th
		}
		c = c.GetParentControl()
	}
	return Theme.Instance{}
}

// dragPreview is what follows the pointer during a drag: the dragged item's
// title on a selected-card chip, below and to the right of the pointer.
type dragPreview struct {
	Control.Extension[dragPreview] `gd:"FwDragPreview"`

	text string
}

func (p *dragPreview) Draw() {
	c := p.AsControl()
	ci := p.AsCanvasItem()
	f := fwFont(c, "semibold")
	tw := f.MoreArgs().GetStringSize(p.text, GUI.HorizontalAlignmentLeft, -1, fontBase, 0,
		TextServer.DirectionAuto, TextServer.OrientationHorizontal).X
	w := min(float32(tw)+20, 280)
	r := rect(14, 12, w, 26)
	fwBox(c, "card_sel").Draw(ci, r)
	drawText(ci, f, fontBase, rect(r.Position.X+10, r.Position.Y+4, w-20, 18), p.text,
		GUI.HorizontalAlignmentLeft, fwColor(c, "highlight_text"))
}

// ---- the schedule list as a drop target ------------------------------------------------

// schedDrop is the schedule list's drop feedback while a drag hovers it.
type schedDrop struct {
	active  bool
	payload uicore.DragPayload
	gap     int // insertion gap (0..len(rows)); -1 for a theme
	row     int // the row a theme goes onto; -1 otherwise
	last    time.Time
	scroll  float64 // fractional auto-scroll carried between frames
}

// initDrop adds the drop overlay (drawn over the rows) and the ticker that
// auto-scrolls and notices the pointer leaving.
func (s *ScheduleList) initDrop() {
	o := Control.New()
	o.SetMouseFilter(Control.MouseFilterIgnore)
	o.SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	o.AsCanvasItem().OnDraw(func() { s.drawDrop(o) })
	add(s, o)
	s.dropOverlay = o
	s.dropTicker = newTicker(s.tickDrop)
	s.AsNode().AddChild(s.dropTicker.AsNode())
}

// DropGapPoint is a point (canvas coordinates) that drops into gap k: just
// inside the top of row k, or below the last row (for the drag hooks).
func (s *ScheduleList) DropGapPoint(k int) (Vector2.XY, bool) {
	n := len(s.views)
	if k < 0 || k > n {
		return Vector2.Zero, false
	}
	lr := s.AsControl().GetGlobalRect()
	x := lr.Position.X + lr.Size.X/2
	if n == 0 {
		return Vector2.New(x, lr.Position.Y+10), true
	}
	if k < n {
		r := s.views[k].AsControl().GetGlobalRect()
		return Vector2.New(x, r.Position.Y+6), true
	}
	r := s.views[n-1].AsControl().GetGlobalRect()
	y := r.Position.Y + r.Size.Y + 10 // the empty space under the rows
	if y > lr.Position.Y+lr.Size.Y-2 {
		y = r.Position.Y + r.Size.Y - 6 // a full list: the lower part of the last row
	}
	return Vector2.New(x, y), true
}

// ScrollRowIntoView scrolls row i into view (for the drag hooks; the
// scroll applies on the next frame).
func (s *ScheduleList) ScrollRowIntoView(i int) {
	if i >= 0 && i < len(s.views) {
		s.scroll.EnsureControlVisible(s.views[i].AsControl())
	}
}

func (s *ScheduleList) rowSpans() (tops, bottoms, mids []float64) {
	for _, v := range s.views {
		r := v.AsControl().GetGlobalRect()
		t, b := float64(r.Position.Y), float64(r.Position.Y+r.Size.Y)
		tops, bottoms, mids = append(tops, t), append(bottoms, b), append(mids, (t+b)/2)
	}
	return
}

// hover works out the drop feedback for payload p at canvas point y.
func (s *ScheduleList) hover(p uicore.DragPayload, y float32) (gap, row int, ok bool) {
	tops, bottoms, mids := s.rowSpans()
	if p.InsertsIntoSchedule() {
		return uicore.DropGap(float64(y), mids), -1, true
	}
	row = uicore.DropRow(float64(y), tops, bottoms)
	return -1, row, row >= 0
}

// dragOver is _can_drop_data for the list and its rows (pos in canvas
// coordinates). It only updates the view's feedback.
func (s *ScheduleList) dragOver(pos Vector2.XY, data any) bool {
	p, ok := uicore.ParseDragPayload(data)
	if !ok {
		s.clearDrop()
		return false
	}
	gap, row, ok := s.hover(p, pos.Y)
	if !ok {
		s.clearDrop()
		return false
	}
	if !s.drop.active {
		s.drop = schedDrop{active: true, gap: -2, last: time.Now()} // gap -2: redraw below
		s.dropTicker.AsNode().SetProcess(true)
	}
	if s.drop.payload != p || s.drop.gap != gap || s.drop.row != row {
		s.drop.payload, s.drop.gap, s.drop.row = p, gap, row
		s.dropOverlay.AsCanvasItem().QueueRedraw()
	}
	return true
}

// dropAt is _drop_data: it reports the drop as the user's intent.
func (s *ScheduleList) dropAt(pos Vector2.XY, data any) {
	s.clearDrop()
	p, ok := uicore.ParseDragPayload(data)
	if !ok {
		return
	}
	gap, row, ok := s.hover(p, pos.Y)
	if !ok {
		return
	}
	s.AsControl().GrabFocus()
	switch {
	case p.Source == uicore.DragSchedule:
		if to, ok := uicore.DropMove(p.Index, gap, len(s.rows)); ok && s.OnMove != nil {
			s.OnMove(p.Index, to)
		}
	case !p.InsertsIntoSchedule():
		if s.OnDropOnto != nil {
			s.OnDropOnto(p.Source, p.Index, row)
		}
	default:
		if s.OnDropInsert != nil {
			s.OnDropInsert(p.Source, p.Index, gap)
		}
	}
}

func (s *ScheduleList) clearDrop() {
	if !s.drop.active {
		return
	}
	s.drop = schedDrop{gap: -1, row: -1}
	s.dropOverlay.AsCanvasItem().QueueRedraw()
	s.dropTicker.AsNode().SetProcess(false)
}

// tickDrop runs every frame while drop feedback shows: it drops the feedback
// once the drag ends or the pointer leaves the list (Godot stops asking the
// list then), and auto-scrolls near the edges.
func (s *ScheduleList) tickDrop() {
	c := s.AsControl()
	if !Viewport.Get(c.AsNode()).GuiIsDragging() {
		s.clearDrop()
		return
	}
	m := c.AsCanvasItem().GetGlobalMousePosition()
	r := c.GetGlobalRect()
	if !Rect2.HasPoint(r, m) {
		s.clearDrop()
		return
	}
	now := time.Now()
	dt := min(now.Sub(s.drop.last).Seconds(), 0.1)
	s.drop.last = now
	speed := uicore.AutoScroll(float64(m.Y), float64(r.Position.Y), float64(r.Position.Y+r.Size.Y), dropScrollEdge, dropScrollSpeed)
	if speed == 0 {
		s.drop.scroll = 0
		return
	}
	s.drop.scroll += speed * dt
	step := int(s.drop.scroll)
	if step == 0 {
		return
	}
	s.drop.scroll -= float64(step)
	s.scroll.SetScrollVertical(s.scroll.ScrollVertical() + step)
	// The rows moved under a still pointer: follow them.
	if gap, row, ok := s.hover(s.drop.payload, m.Y); ok {
		s.drop.gap, s.drop.row = gap, row
	}
	s.dropOverlay.AsCanvasItem().QueueRedraw()
}

// drawDrop draws the insertion line (or a theme's target row) on the overlay.
func (s *ScheduleList) drawDrop(o Control.Instance) {
	ci := o.AsCanvasItem()
	defer func() { _ = ci.Visible() }() // flush the buffered draw calls inside the draw pass
	if !s.drop.active {
		return
	}
	origin := o.GetGlobalRect().Position
	size := o.Size()
	col := fwColor(o, "highlight")
	if row := s.drop.row; row >= 0 && row < len(s.views) {
		r := s.views[row].AsControl().GetGlobalRect()
		card := rect(r.Position.X-origin.X+schedCardInset, r.Position.Y-origin.Y+1, r.Size.X-2*schedCardInset, r.Size.Y-2)
		ci.MoreArgs().DrawRect(card, col, false, 2, false)
		return
	}
	gap := s.drop.gap
	if gap < 0 {
		return
	}
	var y float32
	switch n := len(s.views); {
	case n == 0:
		y = 2
	case gap < n:
		y = s.views[gap].AsControl().GetGlobalRect().Position.Y - origin.Y
	default:
		r := s.views[n-1].AsControl().GetGlobalRect()
		y = r.Position.Y + r.Size.Y - origin.Y
	}
	y = max(2, min(y, size.Y-2))
	x0, x1 := float32(schedCardInset), size.X-schedCardInset
	ci.DrawRect(rect(x0+3, y-1, x1-x0-3, 2), col)
	ci.DrawCircle(Vector2.New(x0+3, y), Float.X(3), col)
}

// CanDropData / DropData: the list itself takes drops on its empty space
// below the rows; the rows forward theirs (scheduleRowView).
func (s *ScheduleList) CanDropData(at Vector2.XY, data any) bool {
	return s.dragOver(toCanvas(s.AsControl(), at), data)
}

func (s *ScheduleList) DropData(at Vector2.XY, data any) {
	s.dropAt(toCanvas(s.AsControl(), at), data)
}

func (s *ScheduleList) Notification(what Object.Notification) {
	if what == Node.NotificationDragEnd {
		s.clearDrop()
	}
}

// toCanvas converts a point local to c into canvas coordinates (c is not
// rotated or scaled).
func toCanvas(c Control.Instance, at Vector2.XY) Vector2.XY {
	g := c.GetGlobalRect().Position
	return Vector2.New(g.X+at.X, g.Y+at.Y)
}

// ---- schedule rows: drag to reorder, and forward drops to the list ---------------------

func (v *scheduleRowView) GetDragData(at Vector2.XY) any {
	if v.list == nil || v.index >= len(v.list.rows) {
		return nil
	}
	return beginDrag(v.AsControl(), uicore.DragPayload{Source: uicore.DragSchedule, Index: v.index}, v.list.rows[v.index].Title)
}

func (v *scheduleRowView) CanDropData(at Vector2.XY, data any) bool {
	return v.list != nil && v.list.dragOver(toCanvas(v.AsControl(), at), data)
}

func (v *scheduleRowView) DropData(at Vector2.XY, data any) {
	if v.list != nil {
		v.list.dropAt(toCanvas(v.AsControl(), at), data)
	}
}

// ---- tiles ------------------------------------------------------------------------------

func (t *slideTile) GetDragData(at Vector2.XY) any {
	if t.grid == nil || t.grid.DragSource == "" || t.index >= len(t.grid.items) {
		return nil
	}
	return beginDrag(t.AsControl(), uicore.DragPayload{Source: t.grid.DragSource, Index: t.index}, t.grid.items[t.index].Label)
}

// noDrop is the can_drop / drop half of a drag forwarding that only drags.
func noDrop(Vector2.XY, any) bool { return false }
func ignoreDrop(Vector2.XY, any)  {}

// ---- the song list and the verse list (plain Trees: drag forwarding) ---------------

// initDrag makes a song row draggable into the schedule.
func (s *SongLibraryPanel) initDrag() {
	s.list.AsControl().SetDragForwarding(func(at Vector2.XY) any {
		i := treeIndex(s.list.GetItemAtPosition(at))
		if i < 0 || i >= len(s.songs) {
			return nil
		}
		return beginDrag(s.list.AsControl(), uicore.DragPayload{Source: uicore.DragSong, Index: i}, s.songs[i].Title)
	}, noDrop, ignoreDrop)
}

// RowCenter is song row i's centre in canvas coordinates, scrolled into view
// first (for the drag hooks); false when there is no such row.
func (s *SongLibraryPanel) RowCenter(i int) (Vector2.XY, bool) {
	return treeRowCenter(s.list, i)
}

// initDrag makes the selected verses draggable into the schedule. A press on
// a row of a multi-selection waits to narrow it (verseCollapseDelay); a drag
// carries the whole selection instead.
func (s *ScripturePanel) initDrag() {
	s.verses.AsControl().SetDragForwarding(func(at Vector2.XY) any {
		row := treeIndex(s.verses.GetItemAtPosition(at))
		if row < 0 || row >= len(s.verseRows) {
			return nil
		}
		s.pendingRow = -1
		text := s.locator.edit.Text()
		if text == "" {
			text = s.verseRows[row]
		}
		return beginDrag(s.verses.AsControl(), uicore.DragPayload{Source: uicore.DragScripture, Index: row}, text)
	}, noDrop, ignoreDrop)
}

// treeRowCenter scrolls top-level row i of t into view and returns its
// centre in canvas coordinates.
func treeRowCenter(t Tree.Instance, i int) (Vector2.XY, bool) {
	root := t.GetRoot()
	if root == (TreeItem.Instance{}) || i < 0 || i >= root.GetChildCount() {
		return Vector2.XY{}, false
	}
	it := root.GetChild(i)
	// Twice: see ScrollVerseIntoView (the cached scroll offset).
	t.MoreArgs().ScrollToItem(it, true)
	t.MoreArgs().ScrollToItem(it, true)
	r := t.GetItemAreaRect(it)
	c := t.AsControl()
	y := r.Position.Y + r.Size.Y/2
	if y < 0 || y > c.Size().Y {
		return Vector2.XY{}, false
	}
	g := c.GetGlobalRect().Position
	return Vector2.New(g.X+min(c.Size().X/2, 120), g.Y+y), true
}

// TileCenter is tile i's centre in canvas coordinates, scrolled into view
// first (for the drag hooks); false when there is no such tile.
func (g *SlideGrid) TileCenter(i int) (Vector2.XY, bool) {
	if i < 0 || i >= len(g.tiles) {
		return Vector2.XY{}, false
	}
	t := g.tiles[i].AsControl()
	g.scroll.EnsureControlVisible(t)
	r := t.GetGlobalRect()
	return Vector2.New(r.Position.X+r.Size.X/2, r.Position.Y+r.Size.Y/2), r.Size.X > 0
}
