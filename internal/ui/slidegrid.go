package ui

import (
	"strconv"

	"freeworship/internal/ui/uicore"

	"graphics.gd/classdb/Container"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEvent"
	"graphics.gd/classdb/InputEventKey"
	"graphics.gd/classdb/InputEventMouseButton"
	"graphics.gd/classdb/ScrollContainer"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2"
)

// SlideGridItem is one tile of a slide grid: "Verse 1" (left) and the slide
// number (right) under the thumbnail.
type SlideGridItem struct {
	Label  string
	Number int
	// Badge is drawn where the number goes when Number is 0 (e.g. a media
	// item's duration or status).
	Badge string
}

// Default tile geometry (TsdSlideSelector).
const (
	tileGap    = 6
	tilePad    = 6
	tileInset  = 3
	tileLabelH = 18
	// DefaultTileWidth is the default minimum tile width (view sizer "Medium").
	DefaultTileWidth = 150
)

// TileWidths are the view-sizer presets: Small, Medium, Large.
var TileWidths = []float64{110, DefaultTileWidth, 210}

// SlideGrid is the Preview / Live slide selector: a wrapping grid of 16:9
// thumbnails with a label row. The selected tile is highlighted; the Live
// grid's selection is the live slide. Thumbnails come from an injected factory.
type SlideGrid struct {
	Control.Extension[SlideGrid] `gd:"FwSlideGrid"`

	// OnSelect reports a user click / arrow-key selection (not SetSelected).
	OnSelect func(i int) `gd:"-"`
	// OnActivate reports a double-click or Enter on a tile.
	OnActivate func(i int) `gd:"-"`
	// OnContextMenu reports a right-click on a tile (after selecting it).
	OnContextMenu func(i int) `gd:"-"`

	items    []SlideGridItem
	selected int
	marked   int
	tileW    float64
	factory  func(i int) Control.Instance

	scroll ScrollContainer.Instance
	flow   *tileFlow
	tiles  []*slideTile
	built  bool // tiles reflect items/factory
}

// NewSlideGrid creates an empty grid.
func NewSlideGrid() *SlideGrid {
	g := &SlideGrid{selected: -1, marked: -1, tileW: DefaultTileWidth}
	c := g.AsControl()
	c.SetFocusMode(Control.FocusAll)
	c.SetClipContents(true)
	g.scroll = ScrollContainer.New()
	g.scroll.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	g.scroll.SetHorizontalScrollMode(ScrollContainer.ScrollModeDisabled)
	g.scroll.AsControl().SetMouseFilter(Control.MouseFilterPass)
	g.flow = &tileFlow{grid: g}
	expand(g.flow.AsControl())
	g.flow.AsControl().SetMouseFilter(Control.MouseFilterPass)
	add(g.scroll, g.flow.AsControl())
	add(g, g.scroll.AsControl())
	return g
}

// SetItems replaces the tiles. The selection is cleared.
func (g *SlideGrid) SetItems(items []SlideGridItem) {
	g.items = append([]SlideGridItem(nil), items...)
	g.selected, g.marked = -1, -1
	g.built = false
	g.render()
}

// SetSelected selects tile i (-1 for none) without calling OnSelect.
func (g *SlideGrid) SetSelected(i int) {
	g.selected = i
	g.render()
}

// Selected returns the selected tile (-1 for none).
func (g *SlideGrid) Selected() int { return g.selected }

// SetMarked draws a red outline around tile i's thumbnail (-1 for none),
// e.g. the slide that is currently live.
func (g *SlideGrid) SetMarked(i int) {
	g.marked = i
	g.render()
}

// SetThumbnailFactory sets the thumbnail factory and rebuilds the tiles.
func (g *SlideGrid) SetThumbnailFactory(f func(i int) Control.Instance) {
	g.factory = f
	g.built = false
	g.render()
}

// RefreshThumbnails rebuilds every thumbnail from the factory.
func (g *SlideGrid) RefreshThumbnails() {
	g.built = false
	g.render()
}

// SetTileWidth sets the minimum tile width (the view sizer).
func (g *SlideGrid) SetTileWidth(px float64) {
	g.tileW = px
	g.render()
}

// TileWidth returns the minimum tile width.
func (g *SlideGrid) TileWidth() float64 { return g.tileW }

func (g *SlideGrid) layout(width float64) uicore.TileGrid {
	return uicore.TileGrid{Width: width, MinTileW: g.tileW, Gap: tileGap, Pad: tilePad,
		Inset: tileInset, LabelH: tileLabelH, Aspect: 16.0 / 9.0}
}

func (g *SlideGrid) render() {
	if !g.built {
		g.rebuild()
	}
	for _, t := range g.tiles {
		t.AsCanvasItem().QueueRedraw()
	}
	g.flow.AsContainer().QueueSort()
	g.flow.AsControl().UpdateMinimumSize()
	if g.selected >= 0 && g.selected < len(g.tiles) {
		g.scroll.EnsureControlVisible(g.tiles[g.selected].AsControl())
	}
}

func (g *SlideGrid) rebuild() {
	g.built = true
	for _, t := range g.tiles {
		g.flow.AsNode().RemoveChild(t.AsNode())
		t.AsNode().QueueFree()
	}
	g.tiles = g.tiles[:0]
	for i := range g.items {
		t := &slideTile{grid: g, index: i}
		t.AsControl().SetMouseFilter(Control.MouseFilterStop)
		t.thumb = newThumbHost()
		t.thumb.AsControl().SetMouseFilter(Control.MouseFilterIgnore)
		if g.factory != nil {
			if content := g.factory(i); content != (Control.Instance{}) {
				content.SetMouseFilter(Control.MouseFilterIgnore)
				t.thumb.SetContent(content)
			}
		}
		add(t, t.thumb.AsControl())
		add(g.flow, t.AsControl())
		g.tiles = append(g.tiles, t)
	}
}

func (g *SlideGrid) userSelect(i int) {
	if i < 0 || i >= len(g.items) {
		return
	}
	changed := i != g.selected
	g.selected = i
	g.render()
	if changed && g.OnSelect != nil {
		g.OnSelect(i)
	}
}

func (g *SlideGrid) userActivate(i int) {
	if i < 0 || i >= len(g.items) {
		return
	}
	if g.OnActivate != nil {
		g.OnActivate(i)
	}
}

func (g *SlideGrid) GuiInput(ev InputEvent.Instance) {
	k, ok := Object.As[InputEventKey.Instance](ev)
	if !ok || !k.AsInputEvent().IsPressed() {
		return
	}
	cols := g.layout(float64(g.flow.AsControl().Size().X)).Columns()
	nav := map[Input.Key]uicore.NavKey{
		Input.KeyLeft: uicore.NavLeft, Input.KeyRight: uicore.NavRight,
		Input.KeyUp: uicore.NavUp, Input.KeyDown: uicore.NavDown,
		Input.KeyHome: uicore.NavHome, Input.KeyEnd: uicore.NavEnd,
	}
	key := k.Keycode()
	if n, ok := nav[key]; ok {
		g.userSelect(uicore.Navigate(g.selected, len(g.items), cols, n))
		acceptEvent(g.AsControl())
		return
	}
	if key == Input.KeyEnter || key == Input.KeyKpEnter {
		g.userActivate(g.selected)
		acceptEvent(g.AsControl())
	}
}

// tileFlow lays the tiles out in rows (the grid's scrollable content).
type tileFlow struct {
	Container.Extension[tileFlow] `gd:"FwTileFlow"`

	grid *SlideGrid
}

func (f *tileFlow) GetMinimumSize() Vector2.XY {
	if f.grid == nil {
		return Vector2.Zero
	}
	l := f.grid.layout(float64(f.AsControl().Size().X))
	return Vector2.New(0, float32(l.ContentHeight(len(f.grid.items))))
}

func (f *tileFlow) Notification(what Object.Notification) {
	if what != Container.NotificationSortChildren || f.grid == nil {
		return
	}
	l := f.grid.layout(float64(f.AsControl().Size().X))
	for i, t := range f.grid.tiles {
		r := l.Tile(i)
		f.AsContainer().FitChildInRect(t.AsControl(), rect(float32(r.X), float32(r.Y), float32(r.W), float32(r.H)))
		th := l.Thumb(r.W, r.H)
		t.thumb.AsControl().SetPosition(Vector2.New(float32(th.X), float32(th.Y)))
		t.thumb.AsControl().SetSize(Vector2.New(float32(th.W), float32(th.H)))
	}
	f.AsControl().UpdateMinimumSize()
	// Apply the layout before any tile draws (see flushCalls): a tile moved
	// from inside its own Draw stays drawn at its old position.
	flushCalls(f.AsControl())
}

// slideTile is one thumbnail + label tile.
type slideTile struct {
	Control.Extension[slideTile] `gd:"FwSlideTile"`

	grid  *SlideGrid
	index int
	thumb *MonitorHost
	hover bool // view-only hover feedback
}

func (t *slideTile) Draw() {
	if t.grid == nil || t.index >= len(t.grid.items) {
		return
	}
	c := t.AsControl()
	ci := t.AsCanvasItem()
	size := c.Size()
	item := t.grid.items[t.index]
	selected := t.index == t.grid.selected
	textColor := fwColor(c, "text")
	switch {
	case selected:
		fwBox(c, "tile_sel").Draw(ci, rect(0, 0, size.X, size.Y))
		textColor = fwColor(c, "highlight_text")
	case t.hover:
		fwBox(c, "tile_hot").Draw(ci, rect(0, 0, size.X, size.Y))
	}
	th := t.grid.layout(float64(size.X)).Thumb(float64(size.X), float64(size.Y))
	ci.DrawRect(rect(float32(th.X), float32(th.Y), float32(th.W), float32(th.H)), fwColor(c, "thumb_bg"))
	if t.index == t.grid.marked {
		ci.MoreArgs().DrawRect(rect(float32(th.X)-1, float32(th.Y)-1, float32(th.W)+2, float32(th.H)+2), fwColor(c, "red"), false, 2, false)
	}
	f := fwFont(c, "regular")
	ly := float32(th.Y + th.H)
	lr := rect(float32(th.X)+1, ly, float32(th.W)-2, tileLabelH)
	num := item.Badge
	if item.Number > 0 {
		num = strconv.Itoa(item.Number)
	}
	badgeW := float32(22)
	if item.Number == 0 && num != "" {
		badgeW = min(f.MoreArgs().GetStringSize(num, GUI.HorizontalAlignmentLeft, -1, fontCaption, 0,
			TextServer.DirectionAuto, TextServer.OrientationHorizontal).X+6, lr.Size.X/2)
	}
	drawText(ci, f, fontCaption, lr, num, GUI.HorizontalAlignmentRight, textColor)
	lr.Size.X -= badgeW
	drawText(ci, f, fontCaption, lr, item.Label, GUI.HorizontalAlignmentLeft, textColor)
}

func (t *slideTile) GuiInput(ev InputEvent.Instance) {
	mb, ok := Object.As[InputEventMouseButton.Instance](ev)
	if !ok || !mb.AsInputEvent().IsPressed() {
		return
	}
	switch mb.ButtonIndex() {
	case Input.MouseButtonLeft:
		t.grid.AsControl().GrabFocus()
		t.grid.userSelect(t.index)
		if mb.DoubleClick() {
			t.grid.userActivate(t.index)
		}
	case Input.MouseButtonRight:
		if t.grid.OnContextMenu == nil {
			return
		}
		t.grid.AsControl().GrabFocus()
		t.grid.userSelect(t.index)
		t.grid.OnContextMenu(t.index)
	default:
		return
	}
	acceptEvent(t.AsControl())
}

func (t *slideTile) Notification(what Object.Notification) {
	switch what {
	case Control.NotificationMouseEnter:
		t.hover = true
		t.AsCanvasItem().QueueRedraw()
	case Control.NotificationMouseExit:
		t.hover = false
		t.AsCanvasItem().QueueRedraw()
	}
}
