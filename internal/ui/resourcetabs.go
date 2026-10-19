package ui

import (
	"graphics.gd/classdb/BaseButton"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/HBoxContainer"
	"graphics.gd/variant/Vector2"
)

// Resource tabs, in EasyWorship order.
const (
	TabSongs = iota
	TabScriptures
	TabMedia
	TabPresentations
	TabThemes
)

// resourceTabSpecs are the tab captions and exact widths (Resource_TabBar).
var resourceTabSpecs = []struct {
	text  string
	width float32
}{
	{"Songs", 66}, {"Scriptures", 80}, {"Media", 60}, {"Presentations", 100}, {"Themes", 70},
}

// Resource tab bar geometry (SplitterBar_Resources).
const (
	resBarH     = 37 // whole bar
	resTabBarH  = 32 // gradient part
	resTabTop   = 3  // padding-top 2 + margin-top 1
	resTabH     = 33 // tab button height (overlaps the divider by 4)
	resDividerH = 5
)

// ResourceTabs is the 37px resource tab bar: Songs, Scriptures, Media,
// Presentations, Themes plus the Add-to-schedule and Go-live mini buttons. In
// the main window it is also the main splitter's bar (drag to resize, click
// its empty area to toggle the library).
type ResourceTabs struct {
	OnTab           func(i int)
	OnAddToSchedule func()
	OnGoLive        func()

	changed func(i int) // MainWindow page switching (user and programmatic)

	root    *tabStrip
	tabs    []*tabButton
	current int
}

// NewResourceTabs builds the tab bar.
func NewResourceTabs() *ResourceTabs {
	r := &ResourceTabs{root: &tabStrip{}}
	rc := r.root.AsControl()
	rc.SetCustomMinimumSize(Vector2.New(0, resBarH))
	rc.SetMouseFilter(Control.MouseFilterStop)

	row := hbox(0)
	row.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	row.AsControl().SetOffsetLeft(1)
	row.AsControl().SetOffsetTop(resTabTop)
	row.AsControl().SetOffsetBottom(resTabTop + resTabH - resBarH)
	row.AsControl().SetMouseFilter(Control.MouseFilterPass)
	add(r.root, row.AsControl())

	for i, spec := range resourceTabSpecs {
		t := &tabButton{text: spec.text}
		tc := t.AsControl()
		tc.SetCustomMinimumSize(Vector2.New(spec.width, resTabH))
		tc.SetFocusMode(Control.FocusNone)
		t.AsBaseButton().SetToggleMode(true)
		idx := i
		t.AsBaseButton().OnPressed(func() { r.userTab(idx) })
		r.tabs = append(r.tabs, t)
		add(row, tc)
	}
	add(row, spacer())

	// ToolBar_ResourceButtons (MostRight): [+ 28x27] | [▶ 28x27], padding 4.
	btns := HBoxContainer.New()
	btns.AsControl().AddThemeConstantOverride("separation", 0)
	btns.AsControl().SetSizeFlagsVertical(Control.SizeShrinkBegin)
	addSched := newMiniButton(miniPlain, "addschedule", 28, 27, "Add to Schedule")
	goLive := newMiniButton(miniPlain, "golive16", 28, 27, "Go Live")
	addSched.AsBaseButton().OnPressed(func() {
		if r.OnAddToSchedule != nil {
			r.OnAddToSchedule()
		}
	})
	goLive.AsBaseButton().OnPressed(func() {
		if r.OnGoLive != nil {
			r.OnGoLive()
		}
	})
	add(btns, addSched.AsControl(), withMargins(newDivider(true).AsControl(), 2, 2), goLive.AsControl())
	add(row, btns.AsControl(), fixed(4, 0))
	r.render()
	return r
}

// AsControl returns the bar's root control.
func (r *ResourceTabs) AsControl() Control.Instance { return r.root.AsControl() }

// SetTab selects tab i without calling OnTab.
func (r *ResourceTabs) SetTab(i int) {
	if i < 0 || i >= len(r.tabs) {
		return
	}
	r.current = i
	r.render()
	if r.changed != nil {
		r.changed(i)
	}
}

// Tab returns the selected tab.
func (r *ResourceTabs) Tab() int { return r.current }

func (r *ResourceTabs) render() {
	for i, t := range r.tabs {
		t.AsBaseButton().SetPressedNoSignal(i == r.current)
		t.AsCanvasItem().QueueRedraw()
	}
}

func (r *ResourceTabs) userTab(i int) {
	if i == r.current {
		r.render() // undo Godot's toggle-off of the pressed tab
		return
	}
	r.current = i
	r.render()
	if r.changed != nil {
		r.changed(i)
	}
	if r.OnTab != nil {
		r.OnTab(i)
	}
}

// fixed returns an empty control of a fixed size (spacing).
func fixed(w, h float32) Control.Instance {
	c := Control.New()
	c.SetCustomMinimumSize(Vector2.New(w, h))
	c.SetMouseFilter(Control.MouseFilterIgnore)
	return c
}

// tabStrip draws the bar background: the tabbar.1 gradient (32px) and the 5px
// tab-bar divider (tabbardivider stroke + fill) along the bottom.
type tabStrip struct {
	Control.Extension[tabStrip] `gd:"FwTabStrip"`
}

func (s *tabStrip) Draw() {
	c := s.AsControl()
	ci := s.AsCanvasItem()
	w := c.Size().X
	fwBox(c, "tabbar1").Draw(ci, rect(0, 0, w, resTabBarH))
	ci.DrawRect(rect(0, resTabBarH, w, resDividerH), fwColor(c, "tab_div_fill"))
	ci.DrawRect(rect(0, resTabBarH, w, 1), fwColor(c, "tab_div_stroke"))
	ci.DrawRect(rect(0, resBarH-1, w, 1), fwColor(c, "splitter"))
}

// tabButton is a resource tab (sd.tabbarbutton.1.style): transparent at rest;
// selected = highlight.inactive body with a 4px highlight strip at the bottom.
type tabButton struct {
	BaseButton.Extension[tabButton] `gd:"FwTabButton"`

	text string
}

func (t *tabButton) Draw() {
	c := t.AsControl()
	ci := t.AsCanvasItem()
	size := c.Size()
	if t.AsBaseButton().ButtonPressed() {
		fwBox(c, "tab_sel").Draw(ci, rect(0, 0, size.X, size.Y-4))
		ci.DrawRect(rect(0, size.Y-4, size.X, 4), fwColor(c, "highlight"))
		ci.DrawRect(rect(0, size.Y-4, size.X, 1), alpha(fwColor(c, "window_border"), 0.5))
	}
	col := fwColor(c, "tab_text")
	if t.AsBaseButton().ButtonPressed() {
		col = fwColor(c, "tab_text_sel")
	}
	drawText(ci, fwFont(c, "regular"), fontBase, rect(5, 2, size.X-10, size.Y-8), t.text, GUI.HorizontalAlignmentCenter, col)
}
