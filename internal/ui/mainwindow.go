package ui

import (
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/Node"
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

	Menus           *Menus            `gd:"-"` // File / Edit / View / Help (menus.go)
	Toolbar         *MainToolbar      `gd:"-"`
	Schedule        *ScheduleList     `gd:"-"`
	PreviewGrid     *SlideGrid        `gd:"-"`
	LiveGrid        *SlideGrid        `gd:"-"`
	PreviewMonitor  *MonitorHost      `gd:"-"` // host slot for the preview render.SlideView
	LiveMonitor     *MonitorHost      `gd:"-"` // host slot for the live render.SlideView
	Resources       *ResourceTabs     `gd:"-"`
	Songs           *SongLibraryPanel `gd:"-"`
	Scriptures      *ScripturePanel   `gd:"-"`
	Media           *MediaPanel       `gd:"-"`
	Themes          *ThemePanel       `gd:"-"`
	ResourcePreview *MonitorHost      `gd:"-"` // host slot for the resource preview
	Editor          *SongEditor       `gd:"-"`
	ThemeEditor     *ThemeEditor      `gd:"-"`
	Settings        *SettingsDialog   `gd:"-"` // Edit ▸ Options… (settings.go)
	Confirm         *ConfirmDialog    `gd:"-"`
	Message         *MessageDialog    `gd:"-"` // errors for the operator

	// Phase 2 resource tabs (their own block keeps gofmt from realigning the one above).
	Presentations *PresentationPanel `gd:"-"`

	// Remote is the remote control's pairing dialog (remote.go).
	Remote *RemoteDialog `gd:"-"`

	// Alerts ▾ dialogs (alerts.go).
	AlertManager *AlertManager    `gd:"-"`
	AlertEditor  *AlertEditor     `gd:"-"`
	AlertFill    *AlertTagFill    `gd:"-"`
	Nursery      *NurseryDialog   `gd:"-"`
	Countdown    *CountdownDialog `gd:"-"`

	// Transport bars under the output monitors, shown while the monitor's
	// slide is a video or an audio file (transport.go).
	PreviewTransport *Transport `gd:"-"`
	LiveTransport    *Transport `gd:"-"`

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

	// OnMonitorOutput is the Live Output monitor's output selector: Live
	// Output or Foldback Output (output_select.go).
	OnMonitorOutput func(foldback bool) `gd:"-"`
	outSel          *outputSelect

	// StageMessage is Live ▸ Stage Message… (stage_message.go).
	StageMessage *StageMessageDialog `gd:"-"`

	// AudioPlaylist is the background audio under the schedule (playlist.go).
	AudioPlaylist *AudioPlaylist `gd:"-"`

	// The Preview Output monitor's context menu and its dialogs (previewmenu.go).
	PreviewMenu *PreviewMenu `gd:"-"`
	ColorDialog *ColorDialog `gd:"-"`
	MediaPicker *MediaPicker `gd:"-"`

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

	w.Menus = NewMenus()
	w.Toolbar = NewMainToolbar()
	w.MainSplit = NewSplitter(SplitVertical, MainPercent, 300, 0, 0, 150)
	expand(w.MainSplit.AsControl())
	add(col, w.Menus.AsControl(), w.Toolbar.AsControl(), w.MainSplit.AsControl())

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
	w.ThemeEditor = NewThemeEditor()
	w.Settings = NewSettingsDialog()
	w.Confirm = NewConfirmDialog()
	w.Message = NewMessageDialog()
	w.StageMessage = NewStageMessageDialog()
	w.Remote = NewRemoteDialog()
	w.buildAlertDialogs()
	w.buildPreviewMenu()
	for _, d := range w.dialogs() {
		d.host = w.dialogHost
		d.setTheme(theme)
		w.AsNode().AddChild(d.win.AsNode())
	}
	w.render()
	return w
}

// dialogs lists the dialog windows.
func (w *MainWindow) dialogs() []*dialogWindow {
	return append([]*dialogWindow{w.Editor.dlg, w.ThemeEditor.dlg, w.Settings.dlg, w.Confirm.dlg, w.Message.dlg, w.StageMessage.dlg, w.Remote.dlg},
		append(w.alertDialogs(), w.ColorDialog.dlg, w.MediaPicker.dlg, w.Editor.sp.dlg.dlg)...)
}

// dialogName names a dialog window node for TopDialogHost.
func (w *MainWindow) dialogName(n Node.Instance) string {
	switch n.ID() {
	case w.Editor.dlg.win.AsNode().ID():
		return "editor"
	case w.ThemeEditor.dlg.win.AsNode().ID():
		return "theme-editor"
	case w.Settings.dlg.win.AsNode().ID():
		return "settings"
	case w.Confirm.dlg.win.AsNode().ID():
		return "confirm"
	case w.Message.dlg.win.AsNode().ID():
		return "message"
	case w.StageMessage.dlg.win.AsNode().ID():
		return "stage-message"
	case w.Remote.dlg.win.AsNode().ID():
		return "remote"
	}
	if name := w.alertDialogName(n); name != "" {
		return name
	}
	switch n.ID() {
	case w.ColorDialog.dlg.win.AsNode().ID():
		return "color"
	case w.MediaPicker.dlg.win.AsNode().ID():
		return "media-picker"
	}
	if n.ID() == w.Editor.sp.dlg.dlg.win.AsNode().ID() {
		return "spelling"
	}
	return "other"
}

// buildPreviewMenu attaches the Preview Output monitor's context menu and
// creates its dialogs (previewmenu.go).
func (w *MainWindow) buildPreviewMenu() {
	w.PreviewMenu = NewPreviewMenu(w.PreviewMonitor.AsControl())
	w.ColorDialog = NewColorDialog()
	w.MediaPicker = NewMediaPicker()
}

// TopDialogHost names the window the topmost open dialog is a child of:
// "main" for the operator window, else the dialog under it ("editor", …).
// Dialogs are always children of the operator window or of each other, never
// of the projector output. It is "" when no dialog is open.
func (w *MainWindow) TopDialogHost() string {
	top := w.dialogHost(0)
	if top.ID() == w.AsNode().ID() {
		return ""
	}
	parent := top.GetParent()
	if parent == (Node.Instance{}) || parent.ID() == w.AsNode().ID() {
		return "main"
	}
	return w.dialogName(parent)
}

// dialogHost is where a dialog that is opening must live: under the topmost
// open dialog, else under the main window. Godot gives a window only one
// exclusive child, so e.g. the quit question over the song editor must be the
// editor's child. skip is the opening dialog itself.
func (w *MainWindow) dialogHost(skip Node.ID) Node.Instance {
	host := w.AsNode()
	for {
		next := Node.Instance{}
		for _, d := range w.dialogs() {
			n := d.win.AsNode()
			if n.ID() == skip || !d.win.Visible() {
				continue
			}
			if p := n.GetParent(); p != (Node.Instance{}) && p.ID() == host.ID() {
				next = n
				break
			}
		}
		if next == (Node.Instance{}) {
			return host
		}
		host = next
	}
}

// DialogOpen reports whether any dialog is on screen.
func (w *MainWindow) DialogOpen() bool {
	for _, d := range w.dialogs() {
		if d.win.Visible() {
			return true
		}
	}
	return false
}

// AcceptDialog presses the topmost dialog's OK button. It reports whether a
// dialog was open.
func (w *MainWindow) AcceptDialog() bool {
	top := w.dialogHost(0)
	for _, d := range w.dialogs() {
		if d.win.AsNode().ID() == top.ID() {
			if d.onOK != nil {
				d.onOK()
			}
			return true
		}
	}
	return false
}

// DismissDialog closes the topmost dialog as its Cancel / Esc would (the
// message dialog's OK). It reports whether a dialog was open.
func (w *MainWindow) DismissDialog() bool {
	top := w.dialogHost(0)
	for _, d := range w.dialogs() {
		if d.win.AsNode().ID() == top.ID() {
			d.close()
			return true
		}
	}
	return false
}

func (w *MainWindow) Ready() { w.render() }

// SetPalette switches the interface color (View → Interface Color).
func (w *MainWindow) SetPalette(p Palette) {
	w.palette = p
	theme := BuildTheme(p)
	w.AsControl().SetTheme(theme)
	for _, d := range w.dialogs() {
		d.setTheme(theme)
	}
}

// CurrentPalette returns the active palette.
func (w *MainWindow) CurrentPalette() Palette { return w.palette }

// SetScheduleTitle sets the window title: "FreeBeams — <title>" with a "•"
// dirty marker.
func (w *MainWindow) SetScheduleTitle(title string, dirty bool) {
	w.title, w.dirty = title, dirty
	w.render()
}

// WindowTitle is the title the window shows for the current state.
func (w *MainWindow) WindowTitle() string {
	t := "FreeBeams — " + w.title
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
	w.AudioPlaylist = NewAudioPlaylist()
	add(col, w.AudioPlaylist.AsControl())
	return col.AsControl()
}

func (w *MainWindow) buildPreviewLive() Control.Instance {
	// Preview column.
	w.PreviewGrid = NewSlideGrid()
	w.PreviewMonitor = NewMonitorHost()
	w.PreviewTransport = NewTransport()
	w.PreviewTransport.AsControl().AsCanvasItem().SetVisible(false)
	w.PreviewSplit = NewSplitter(SplitVertical, SlideGridPercent, 150, 0, 0, 200)
	w.PreviewSplit.SetPanes(
		column(panelHeader("Preview", nil, gridSizer(w.PreviewGrid)), surface(vGridBk, w.PreviewGrid.AsControl())),
		columnFooter(w.PreviewMonitor.AsControl(), w.PreviewTransport.AsControl(),
			outputBar(hexpand(label("Preview Output", "").AsControl()))),
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
	liveCaption := label("Live Output", "")
	w.wireOutputSelect(swap, liveCaption)
	w.LiveTransport = NewTransport()
	w.LiveTransport.AsControl().AsCanvasItem().SetVisible(false)
	w.LiveSplit = NewSplitter(SplitVertical, SlideGridPercent, 150, 0, 0, 200)
	w.LiveSplit.SetPanes(
		column(liveHeader, surface(vGridBk, w.LiveGrid.AsControl())),
		columnFooter(w.LiveMonitor.AsControl(), w.LiveTransport.AsControl(),
			outputBar(swap.AsControl(), fixed(4, 0), hexpand(liveCaption.AsControl()))),
	)

	w.PreviewLiveSplit = NewSplitter(SplitHorizontal, PreviewLivePercent, 50, 50, 0, 0)
	w.PreviewLiveSplit.SetPanes(w.PreviewSplit.AsControl(), w.LiveSplit.AsControl())
	return w.PreviewLiveSplit.AsControl()
}

// surface puts a control on a themed panel (the surface's own palette token).
func surface(variation string, c Control.Instance) Control.Instance {
	bg := panel(variation)
	add(bg, c)
	return bg.AsControl()
}

// column stacks a fixed header over an expanding body.
func column(header, body Control.Instance) Control.Instance {
	col := vbox(0)
	expand(body)
	add(col, header, body)
	return col.AsControl()
}

// columnFooter stacks an expanding body over fixed footers.
func columnFooter(body Control.Instance, footers ...Control.Instance) Control.Instance {
	col := vbox(0)
	expand(body)
	add(col, body)
	add(col, footers...)
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
	w.Media = NewMediaPanel()
	w.Presentations = NewPresentationPanel()
	w.Themes = NewThemePanel()
	w.pages = []Control.Instance{
		w.Songs.AsControl(),
		w.Scriptures.AsControl(),
		w.Media.AsControl(),
		w.Presentations.AsControl(),
		w.Themes.AsControl(),
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
	bg := panel(vResource)
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
