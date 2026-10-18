package ui

import (
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/PopupMenu"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Vector2"
)

// Default splitter percentages (UI-SPEC §3.2). PreviewLivePercent deviates from
// EasyWorship's design-time 71.19% so that both columns are usable by default.
const (
	MainPercent        = 0.71
	SchedulePercent    = 0.1944
	PreviewLivePercent = 0.50
	SlideGridPercent   = 0.4638
	ResourcesPercent   = 0.8542
	LibraryPercent     = 0.35
)

// MainWindow is the assembled operator window (UI-SPEC §3): the 58px main
// toolbar, Schedule | Preview | Live on top, and the resource area (tab bar,
// library, item list, resource preview) below. It is a dumb view: the app
// drives it through the parts' setters and callbacks.
type MainWindow struct {
	Control.Extension[MainWindow] `gd:"FwMainWindow"`

	Toolbar         *MainToolbar      `gd:"-"`
	Schedule        *ScheduleList     `gd:"-"`
	PreviewGrid     *SlideGrid        `gd:"-"`
	LiveGrid        *SlideGrid        `gd:"-"`
	PreviewMonitor  *MonitorHost      `gd:"-"` // host slot for the preview render.SlideView
	LiveMonitor     *MonitorHost      `gd:"-"` // host slot for the live render.SlideView
	Resources       *ResourceTabs     `gd:"-"`
	Songs           *SongLibraryPanel `gd:"-"`
	Scriptures      *ScripturePanel   `gd:"-"`
	ResourcePreview *MonitorHost      `gd:"-"` // host slot for the resource preview
	Editor          *SongEditor       `gd:"-"`
	Confirm         *ConfirmDialog    `gd:"-"`

	// Splitters, for persisting their percentages (OnChanged) and restoring
	// them (SetPercent).
	MainSplit        *Splitter `gd:"-"`
	TopSplit         *Splitter `gd:"-"`
	PreviewLiveSplit *Splitter `gd:"-"`
	PreviewSplit     *Splitter `gd:"-"`
	LiveSplit        *Splitter `gd:"-"`
	ResourceSplit    *Splitter `gd:"-"`

	// OnGoBack is the Live header's Go Back button.
	OnGoBack func() `gd:"-"`

	// palette is plain Go data. The Theme is not kept in a field: a RefCounted
	// held by an extension struct is never released (leaks at exit); it lives
	// on the node (AsControl().Theme()).
	palette    Palette
	title      string
	dirty      bool
	resTab     int
	resTitle   string
	resCaption string

	pages      []Control.Instance
	resTitleL  Label.Instance
	resCapL    Label.Instance
	scheduleVS *miniButton
}

// NewMainWindow builds the full layout with the given palette.
func NewMainWindow(p Palette) *MainWindow {
	w := &MainWindow{palette: p, title: "Untitled"}
	c := w.AsControl()
	c.SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	theme := BuildTheme(p)
	c.SetTheme(theme)

	bg := panel(vBk2)
	bg.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	add(w, bg.AsControl())
	col := vbox(0)
	add(bg, col.AsControl())

	w.Toolbar = NewMainToolbar()
	w.MainSplit = NewSplitter(SplitVertical, MainPercent, 300, 0, 0, 150)
	expand(w.MainSplit.AsControl())
	add(col, w.Toolbar.AsControl(), w.MainSplit.AsControl())

	w.TopSplit = NewSplitter(SplitHorizontal, SchedulePercent, 0, 300, 200, 0)
	w.TopSplit.SetPanes(w.buildSchedule(), w.buildPreviewLive())

	w.Resources = NewResourceTabs()
	w.Resources.changed = func(i int) {
		w.resTab = i
		w.render()
	}
	w.ResourceSplit = NewSplitter(SplitHorizontal, ResourcesPercent, 300, 0, 0, 150)
	w.ResourceSplit.SetPanes(w.buildResourcePages(), w.buildResourcePreview())

	w.MainSplit.SetPanes(w.TopSplit.AsControl(), w.ResourceSplit.AsControl())
	w.MainSplit.SetBarControl(w.Resources.AsControl())

	w.Editor = NewSongEditor()
	w.Confirm = NewConfirmDialog()
	w.AsNode().AddChild(w.Editor.AsWindow().AsNode())
	w.AsNode().AddChild(w.Confirm.AsWindow().AsNode())
	w.Editor.SetTheme(theme)
	w.Confirm.SetTheme(theme)
	w.render()
	return w
}

func (w *MainWindow) Ready() { w.render() }

// SetPalette switches the interface color (View → Interface Color).
func (w *MainWindow) SetPalette(p Palette) {
	w.palette = p
	theme := BuildTheme(p)
	w.AsControl().SetTheme(theme)
	w.Editor.SetTheme(theme)
	w.Confirm.SetTheme(theme)
}

// CurrentPalette returns the active palette.
func (w *MainWindow) CurrentPalette() Palette { return w.palette }

// SetScheduleTitle sets the window title: "FreeWorship — <title>" with a "•"
// dirty marker.
func (w *MainWindow) SetScheduleTitle(title string, dirty bool) {
	w.title, w.dirty = title, dirty
	w.render()
}

// WindowTitle is the title the window shows for the current state.
func (w *MainWindow) WindowTitle() string {
	t := "FreeWorship — " + w.title
	if w.dirty {
		t += " •"
	}
	return t
}

// SetResourcePreviewInfo sets the resource preview's title and footer caption.
func (w *MainWindow) SetResourcePreviewInfo(title, caption string) {
	w.resTitle, w.resCaption = title, caption
	w.render()
}

func (w *MainWindow) render() {
	for i, p := range w.pages {
		p.AsCanvasItem().SetVisible(i == w.resTab)
	}
	w.resTitleL.SetText(w.resTitle)
	w.resCapL.SetText(w.resCaption)
	if w.AsNode().IsInsideTree() {
		if win := Window.Get(w.AsNode()); win != Window.Nil {
			win.SetTitle(w.WindowTitle())
		}
	}
}

// ---- builders -------------------------------------------------------------

// panelHeader is a 28px panel header mini toolbar with a title.
func panelHeader(title string, left, right []Control.Instance) Control.Instance {
	bar := panel(vMiniBar)
	bar.AsControl().SetCustomMinimumSize(Vector2.New(0, 28))
	row := hbox(0)
	add(bar, row.AsControl())
	add(row, left...)
	t := label(title, vTitle)
	t.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	add(row, t.AsControl())
	add(row, right...)
	return bar.AsControl()
}

// gridSizer wires a 24+16 view-sizer split button to a slide grid's tile width.
func gridSizer(g *SlideGrid) []Control.Instance {
	l, r := viewSizer("view_large")
	menu := PopupMenu.New()
	names := []string{"Small", "Medium", "Large"}
	for i, n := range names {
		menu.MoreArgs().AddRadioCheckItem(n, i, 0)
	}
	menu.OnIdPressed(func(id int) { g.SetTileWidth(TileWidths[id]) })
	r.AsNode().AddChild(menu.AsNode())
	l.AsBaseButton().OnPressed(func() {
		next := 0
		for i, tw := range TileWidths {
			if tw == g.TileWidth() {
				next = (i + 1) % len(TileWidths)
			}
		}
		g.SetTileWidth(TileWidths[next])
	})
	r.AsBaseButton().OnPressed(func() {
		for i, tw := range TileWidths {
			menu.SetItemChecked(i, tw == g.TileWidth())
		}
		popupBelow(menu, r.AsControl())
	})
	return []Control.Instance{l.AsControl(), r.AsControl()}
}

func (w *MainWindow) buildSchedule() Control.Instance {
	w.Schedule = NewScheduleList()
	l, r := viewSizer("view_large")
	w.scheduleVS = l
	l.AsBaseButton().OnPressed(func() { w.Schedule.SetThumbnailsVisible(!w.Schedule.ThumbnailsVisible()) })
	r.AsBaseButton().OnPressed(func() { w.Schedule.SetThumbnailsVisible(!w.Schedule.ThumbnailsVisible()) })
	gear := newMiniButton(miniDropdown, "gear", 40, 24, "Schedule Actions")
	header := panelHeader("Schedule", nil, []Control.Instance{
		l.AsControl(), r.AsControl(), withMargins(newDivider(true).AsControl(), 2, 2), gear.AsControl(),
	})
	col := vbox(0)
	expand(w.Schedule.AsControl())
	add(col, header, w.Schedule.AsControl())
	return col.AsControl()
}

func (w *MainWindow) buildPreviewLive() Control.Instance {
	// Preview column.
	w.PreviewGrid = NewSlideGrid()
	w.PreviewMonitor = NewMonitorHost()
	w.PreviewSplit = NewSplitter(SplitVertical, SlideGridPercent, 150, 0, 0, 200)
	w.PreviewSplit.SetPanes(
		column(panelHeader("Preview", nil, gridSizer(w.PreviewGrid)), w.PreviewGrid.AsControl()),
		columnFooter(w.PreviewMonitor.AsControl(), outputBar(hexpand(label("Preview Output", "").AsControl()))),
	)

	// Live column: Go Back | Live header.
	w.LiveGrid = NewSlideGrid()
	w.LiveMonitor = NewMonitorHost()
	goBack := newMiniButton(miniText, "", 64, 24, "Go Back")
	goBack.text = "Go Back"
	goBack.AsBaseButton().OnPressed(func() {
		if w.OnGoBack != nil {
			w.OnGoBack()
		}
	})
	liveHeader := panelHeader("Live", []Control.Instance{
		goBack.AsControl(), withMargins(newDivider(true).AsControl(), 0, 6),
	}, gridSizer(w.LiveGrid))
	swap := newMiniButton(miniPlain, "swap", 24, 24, "Output: Live / Foldback")
	w.LiveSplit = NewSplitter(SplitVertical, SlideGridPercent, 150, 0, 0, 200)
	w.LiveSplit.SetPanes(
		column(liveHeader, w.LiveGrid.AsControl()),
		columnFooter(w.LiveMonitor.AsControl(), outputBar(swap.AsControl(), fixed(4, 0), hexpand(label("Live Output", "").AsControl()))),
	)

	w.PreviewLiveSplit = NewSplitter(SplitHorizontal, PreviewLivePercent, 50, 50, 0, 0)
	w.PreviewLiveSplit.SetPanes(w.PreviewSplit.AsControl(), w.LiveSplit.AsControl())
	return w.PreviewLiveSplit.AsControl()
}

// column stacks a fixed header over an expanding body.
func column(header, body Control.Instance) Control.Instance {
	col := vbox(0)
	expand(body)
	add(col, header, body)
	return col.AsControl()
}

// columnFooter stacks an expanding body over a fixed footer.
func columnFooter(body, footer Control.Instance) Control.Instance {
	col := vbox(0)
	expand(body)
	add(col, body, footer)
	return col.AsControl()
}

// outputBar is the 28px bar under an output monitor.
func outputBar(items ...Control.Instance) Control.Instance {
	bar := panel(vMiniBar)
	bar.AsControl().SetCustomMinimumSize(Vector2.New(0, 28))
	row := hbox(0)
	add(bar, row.AsControl())
	add(row, items...)
	return bar.AsControl()
}

func (w *MainWindow) buildResourcePages() Control.Instance {
	host := Control.New()
	host.SetClipContents(true)
	w.Songs = NewSongLibraryPanel()
	w.Scriptures = NewScripturePanel()
	w.pages = []Control.Instance{
		w.Songs.AsControl(),
		w.Scriptures.AsControl(),
		placeholderPage(),
		placeholderPage(),
		placeholderPage(),
	}
	for _, p := range w.pages {
		p.SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
		host.AsNode().AddChild(p.AsNode())
	}
	return host
}

func placeholderPage() Control.Instance {
	bg := panel(vBk1)
	l := label("Coming in Phase 2", vPlaceholder)
	l.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
	add(bg, l.AsControl())
	return bg.AsControl()
}

func (w *MainWindow) buildResourcePreview() Control.Instance {
	bg := panel(vBk1)
	col := vbox(0)
	add(bg, col.AsControl())
	w.ResourcePreview = NewMonitorHost()
	w.ResourcePreview.FitWidth = true
	w.ResourcePreview.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	grow := Control.New()
	grow.SetSizeFlagsVertical(Control.SizeExpandFill)
	w.resTitleL = label("", "")
	w.resTitleL.AsControl().SetThemeTypeVariation(vResTitle)
	footer := panel(vMiniBarTL)
	footer.AsControl().SetCustomMinimumSize(Vector2.New(0, 28))
	w.resCapL = label("", vCaption)
	add(footer, w.resCapL.AsControl())
	add(col, w.ResourcePreview.AsControl(), w.resTitleL.AsControl(), grow, footer.AsControl())
	return bg.AsControl()
}
