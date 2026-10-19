package app

import (
	"reflect"

	"freebeams/internal/app/core"
	"freebeams/internal/models"
	"freebeams/internal/render"
	"freebeams/internal/ui"

	"graphics.gd/classdb/Control"
)

// viewCache remembers what render last applied, so unchanged widgets are not
// rebuilt. It is the only thing render writes.
type viewCache struct {
	seq         uint64
	previewDeck *models.Deck
	liveDeck    *models.Deck
	previewSel  int
	liveSel     int
	songsVer    int
	songSel     int
	songsStatus string
	biblesVer   int
	bibleSel    int
	versesVer   int
	verseSel    int
	status      string
	statusErr   bool
	mediaVer    int
	mediaThumbs int
	mediaRows   []ui.MediaRow
	mediaSel    int
	mediaKind   models.MediaKind
	mediaStatus string
	schedVer    int
	schedRows   []ui.ScheduleRow
	schedSel    int
	themesVer   int
	themeSel    int
	themeStatus string
	themeTarget string
	title       string
	titleDirty  bool
	resTitle    string
	resCaption  string
	alert       core.Alert // the alert the message dialog shows
	toggles     [4]bool    // logo, black, clear, live
	settingsVer int        // settings projected onto the Options dialog
	color       string     // interface color applied to the window
	recent      []string   // recent files listed in the menu
	init        bool

	// Presentations tab (presentations.go).
	presVer    int
	presThumbs int
	presRows   []ui.PresentationRow
	presSel    int
	presStatus string

	// The tile menus' View ▸ / Sort by ▸ (menus_items.go).
	mediaView, mediaSort string
	presView, presSort   string

	// Playback (playback.go): the seek request last applied per monitor (live, preview).
	seekApplied [2]int
}

func newViewCache() viewCache {
	return viewCache{previewSel: -2, liveSel: -2, songSel: -2, songsVer: -1, biblesVer: -1, bibleSel: -2,
		versesVer: -1, verseSel: -2, mediaVer: -1, mediaSel: -2, schedVer: -1, schedSel: -2, presVer: -1, presSel: -2, themesVer: -1, themeSel: -2,
		settingsVer: -1, color: core.ColorDark}
}

// buildUI creates the main window, the monitors and the output, and connects
// every widget callback to an event handler.
func (a *App) buildUI() {
	a.win = ui.NewMainWindow(ui.DarkPalette())
	a.AsNode().AddChild(a.win.AsNode())

	a.previewView = render.NewSlideView() // 500ms crossfade (default)
	a.liveView = render.NewSlideView()
	a.resView = render.NewSlideView()
	a.resView.SetTransitionDuration(0) // cut
	a.win.PreviewMonitor.SetContent(a.previewView.AsControl())
	a.win.LiveMonitor.SetContent(a.liveView.AsControl())
	a.win.ResourcePreview.SetContent(a.resView.AsControl())

	a.out = render.NewOutput(a.AsNode())
	a.out.SetTransitionDuration(render.DefaultTransition)
	a.out.OnClosed = func() { a.event(func() { a.pres.SetOutputEnabled(false) }) }
	a.out.OnInput = a.outputInput // hotkeys while the windowed output has focus
	a.win.Message.OnClose = func() { a.event(a.dismissAlert) }

	// Thumbnails are built while render rebuilds a grid, from the current state.
	a.win.PreviewGrid.SetThumbnailFactory(func(i int) Control.Instance { return thumbnail(a.pres.State.Preview, i) })
	a.win.LiveGrid.SetThumbnailFactory(func(i int) Control.Instance { return thumbnail(a.pres.State.Live, i) })
	a.win.Schedule.SetThumbnailFactory(func(i int) Control.Instance {
		if i < 0 || i >= len(a.sched.Items) {
			return Control.Instance{}
		}
		return thumbnail(core.ItemDeck(a.sched.Items[i]), 0)
	})

	tb := a.win.Toolbar
	tb.SetEnabled(ui.ActionWeb, false)
	tb.SetEnabled(ui.ActionRemote, false)
	tb.SetEnabled(ui.ActionAlerts, false)
	tb.OnAction = func(act ui.ToolbarAction) { a.event(func() { a.onToolbar(act) }) }

	a.win.OnGoBack = func() { a.event(a.goBack) }

	// Presenting.
	a.win.PreviewGrid.OnSelect = func(i int) { a.event(func() { a.pres.SelectPreviewSlide(i) }) }
	a.win.PreviewGrid.OnActivate = func(i int) { a.event(func() { a.liveChange(func() { a.pres.GoLiveSlide(i) }) }) }
	a.win.LiveGrid.OnSelect = func(i int) { a.event(func() { a.pres.SelectLiveSlide(i) }) }
	a.win.LiveGrid.OnActivate = func(i int) { a.event(func() { a.pres.SelectLiveSlide(i) }) }

	// Schedule.
	s := a.win.Schedule
	s.OnSelect = func(i int) { a.event(func() { a.selectScheduleItem(i) }) }
	s.OnActivate = func(i int) {
		a.event(func() {
			a.selectScheduleItem(i)
			a.goLive()
		})
	}
	s.OnRemove = func(i int) { a.event(func() { a.removeScheduleItem(i) }) }
	s.OnMove = func(from, to int) { a.event(func() { a.moveScheduleItem(from, to) }) }

	// Resources.
	r := a.win.Resources
	r.OnTab = func(int) { a.event(func() {}) }
	r.OnAddToSchedule = func() { a.event(func() { a.addCurrentToSchedule(nil) }) }
	r.OnGoLive = func() { a.event(func() { a.resourceGoLive() }) }

	songs := a.win.Songs
	songs.OnSearch = func(q string) { a.event(func() { a.searchSongs(q, nil) }) }
	songs.OnSelect = func(i int) { a.event(func() { a.selectSong(i, songPreview, nil) }) }
	songs.OnActivate = func(i int) { a.event(func() { a.selectSong(i, songGoLive, nil) }) }
	songs.OnGoLive = func(i int) { a.event(func() { a.selectSong(i, songGoLive, nil) }) }
	songs.OnAddToSchedule = func(i int) { a.event(func() { a.selectSong(i, songSchedule, nil) }) }
	songs.OnEdit = func(i int) { a.event(func() { a.selectSong(i, songEdit, nil) }) }
	songs.OnNew = func() { a.event(a.newSong) }
	songs.OnDelete = func(i int) { a.event(func() { a.deleteSong(i) }) }

	a.win.Editor.OnSave = func(song models.Song, lyrics string) { a.event(func() { a.saveSong(song, lyrics, nil) }) }

	sc := a.win.Scriptures
	sc.OnBibleChanged = func(i int) {
		a.event(func() {
			a.bibleSel = i
			if a.reference != "" {
				a.lookupReference(a.reference, nil)
			}
		})
	}
	sc.OnReference = func(text string) { a.event(func() { a.lookupReference(text, nil) }) }
	sc.OnSelect = func(i int) { a.event(func() { a.selectVerse(i, false) }) }
	sc.OnActivate = func(i int) { a.event(func() { a.selectVerse(i, true) }) }
	sc.OnAddToSchedule = func() { a.event(func() { a.addScriptureToSchedule(nil) }) }
	sc.OnGoLive = func() { a.event(a.goLiveVerse) }
	sc.OnImportCopyrighted = func() { a.event(func() { a.importCopyrightedBibles(nil, false) }) }
	sc.OnImportFile = func() { a.event(a.importBibleFileDialog) }

	// Splitter positions persist across runs (layout.go).
	for name, sp := range a.splitters() {
		sp.OnChanged = func(float64) { a.event(func() { a.layoutChanged(name, sp) }) }
		sp.OnCollapsedChanged = func(int) { a.event(func() { a.layoutChanged(name, sp) }) }
	}

	a.wireMedia()
	a.wirePresentations()
	a.wireThemes()
	a.wireSettings() // the Options dialog and the menus (settings.go)
	a.wirePlayback() // monitor player roles and the transport bars (playback.go)
}

// thumbnail is a SlideView (cut, no transition) showing slide i of d. A
// video background is drawn as its poster still: thumbnails never play.
func thumbnail(d *models.Deck, i int) Control.Instance {
	if d == nil || i < 0 || i >= len(d.Slides) {
		return Control.Instance{}
	}
	v := render.NewSlideView()
	v.SetTransitionDuration(0)
	s := d.Slides[i]
	s.Background = core.StillBackground(s.Background)
	v.SetFrame(models.Frame{Mode: models.ModeNormal, Slide: &s, Theme: d.Theme})
	return v.AsControl()
}

func gridItems(d *models.Deck) []ui.SlideGridItem {
	labels := core.GridLabels(d)
	items := make([]ui.SlideGridItem, len(labels))
	for i, l := range labels {
		items[i] = ui.SlideGridItem{Label: l.Label, Number: l.Number}
	}
	return items
}

// render projects the state onto the nodes. It is idempotent and never
// writes state (only a.view, its memo of what it applied).
func (a *App) render() {
	st := &a.pres.State
	w := a.win
	v := &a.view
	v.seq = st.Seq

	// Preview / Live grids.
	if st.Preview != v.previewDeck {
		v.previewDeck = st.Preview
		w.PreviewGrid.SetItems(gridItems(st.Preview))
		v.previewSel = -2
	}
	if st.PreviewIndex != v.previewSel {
		v.previewSel = st.PreviewIndex
		w.PreviewGrid.SetSelected(st.PreviewIndex)
	}
	if st.Live != v.liveDeck {
		v.liveDeck = st.Live
		w.LiveGrid.SetItems(gridItems(st.Live))
		v.liveSel = -2
	}
	if st.LiveIndex != v.liveSel {
		v.liveSel = st.LiveIndex
		w.LiveGrid.SetSelected(st.LiveIndex)
		w.LiveGrid.SetMarked(st.LiveIndex)
	}

	// Monitors and the output (SetFrame is idempotent per Seq / content).
	a.previewView.SetFrame(a.pres.PreviewFrame())
	live := a.pres.LiveFrame()
	a.liveView.SetTransition(a.settings.Transition) // the output's view: renderTransition below, once it exists
	a.liveView.SetFrame(live)
	screen := a.outputScreen() // the configured display, if connected (settings.go)
	switch {
	case st.OutputEnabled && !a.out.IsOpen():
		a.out.Open(screen)
	case !st.OutputEnabled && a.out.IsOpen():
		a.out.Close()
	case st.OutputEnabled && a.out.Screen() != screen:
		a.out.Open(screen) // the output display setting changed: move it
	}
	a.renderTransition(a.settings.Transition)
	a.out.SetFrame(live)
	a.renderPlayback() // the monitors' players and transport bars (playback.go)

	// Toolbar toggles.
	toggles := [4]bool{st.Mode == models.ModeLogo, st.Mode == models.ModeBlack, st.Mode == models.ModeClear, st.OutputEnabled}
	if !v.init || toggles != v.toggles {
		v.toggles = toggles
		for i, act := range []ui.ToolbarAction{ui.ActionLogo, ui.ActionBlack, ui.ActionClear, ui.ActionLive} {
			w.Toolbar.SetToggled(act, toggles[i])
		}
	}

	// Schedule rows, live marker and selection.
	liveItem := core.LiveItemIndex(a.sched, st.Live)
	rows := make([]ui.ScheduleRow, len(a.sched.Items))
	for i, it := range a.sched.Items {
		rows[i] = ui.ScheduleRow{Title: it.Title, Subtitle: it.Subtitle, Live: i == liveItem, Theme: it.Theme.Name}
	}
	if a.schedVer != v.schedVer || !reflect.DeepEqual(rows, v.schedRows) {
		v.schedVer, v.schedRows = a.schedVer, rows
		w.Schedule.SetRows(rows)
		v.schedSel = -2
	}
	if a.schedSel != v.schedSel {
		v.schedSel = a.schedSel
		w.Schedule.SetSelected(a.schedSel)
	}
	if a.sched.Title != v.title || a.sched.Dirty != v.titleDirty || !v.init {
		v.title, v.titleDirty = a.sched.Title, a.sched.Dirty
		w.SetScheduleTitle(a.sched.Title, a.sched.Dirty)
	}

	// Songs tab.
	if a.songsVer != v.songsVer {
		v.songsVer = a.songsVer
		w.Songs.SetSongs(a.songs)
		v.songSel = -2
	}
	if a.songSel != v.songSel {
		v.songSel = a.songSel
		w.Songs.SetSelected(a.songSel)
	}
	if a.songsStatus != v.songsStatus || !v.init {
		v.songsStatus = a.songsStatus
		w.Songs.SetCountText(a.songsStatus)
	}

	// Scriptures tab.
	if a.biblesVer != v.biblesVer || a.bibleSel != v.bibleSel {
		v.biblesVer, v.bibleSel = a.biblesVer, a.bibleSel
		w.Scriptures.SetBibles(a.bibles, a.bibleSel)
		w.Scriptures.SetImportCopyrightedAvailable(len(core.CopyrightedBiblesToImport(a.ewBibles, a.bibles)) > 0)
	}
	verseSel := a.verseSelection()
	if a.versesVer != v.versesVer {
		v.versesVer, v.verseSel = a.versesVer, verseSel
		w.Scriptures.SetVerses(a.verseRows, verseSel)
	} else if verseSel != v.verseSel {
		v.verseSel = verseSel
		w.Scriptures.SetSelected(verseSel)
	}
	if a.scriptStatus != v.status || a.scriptErr != v.statusErr {
		v.status, v.statusErr = a.scriptStatus, a.scriptErr
		w.Scriptures.SetStatus(a.scriptStatus, a.scriptErr)
	}

	// Media tab: the tiles are rebuilt when the list changes or when derived
	// files / badges arrived (SetItems rebuilds the thumbnails too).
	if a.md.ver != v.mediaVer || a.md.thumbVer != v.mediaThumbs {
		rows := a.mediaRows()
		if a.md.ver != v.mediaVer || !reflect.DeepEqual(rows, v.mediaRows) {
			w.Media.SetItems(rows)
			v.mediaSel = -2
		}
		v.mediaVer, v.mediaThumbs, v.mediaRows = a.md.ver, a.md.thumbVer, rows
	}
	if a.md.sel != v.mediaSel {
		v.mediaSel = a.md.sel
		w.Media.SetSelected(a.md.sel)
	}
	if a.md.kind != v.mediaKind || !v.init {
		v.mediaKind = a.md.kind
		w.Media.SetFilter(a.md.kind)
	}
	if a.md.status != v.mediaStatus || !v.init {
		v.mediaStatus = a.md.status
		w.Media.SetCountText(a.md.status)
	}
	if view, sort := string(a.md.view), string(a.md.sort); view != v.mediaView || sort != v.mediaSort || !v.init {
		v.mediaView, v.mediaSort = view, sort
		w.Media.SetView(view)
		w.Media.SetSort(sort)
	}

	// Presentations tab, as the Media tab: tiles rebuilt when the list
	// changes or when rendered pages / badges arrived.
	if a.pr.ver != v.presVer || a.pr.thumbVer != v.presThumbs {
		rows := a.presentationRows()
		if a.pr.ver != v.presVer || !reflect.DeepEqual(rows, v.presRows) {
			w.Presentations.SetItems(rows)
			v.presSel = -2
		}
		v.presVer, v.presThumbs, v.presRows = a.pr.ver, a.pr.thumbVer, rows
	}
	if a.pr.sel != v.presSel {
		v.presSel = a.pr.sel
		w.Presentations.SetSelected(a.pr.sel)
	}
	if a.pr.status != v.presStatus || !v.init {
		v.presStatus = a.pr.status
		w.Presentations.SetCountText(a.pr.status)
	}
	if view, sort := string(a.pr.view), string(a.pr.sort); view != v.presView || sort != v.presSort || !v.init {
		v.presView, v.presSort = view, sort
		w.Presentations.SetView(view)
		w.Presentations.SetSort(sort)
	}
	// Themes tab: tiles (and their thumbnails) follow the list and defaults;
	// the schedule's Theme submenu lists the same names.
	if a.th.ver != v.themesVer {
		v.themesVer = a.th.ver
		w.Themes.SetItems(a.themeRows())
		w.Schedule.SetThemeNames(a.themeNames())
		v.themeSel = -2
	}
	if a.th.sel != v.themeSel {
		v.themeSel = a.th.sel
		w.Themes.SetSelected(a.th.sel)
	}
	if status := core.ThemeCountText(len(a.th.list)); status != v.themeStatus || !v.init {
		v.themeStatus = status
		w.Themes.SetCountText(status)
	}
	target := ""
	if a.schedSel >= 0 && a.schedSel < len(a.sched.Items) {
		target = a.sched.Items[a.schedSel].Title
	}
	if target != v.themeTarget || !v.init {
		v.themeTarget = target
		w.Themes.SetApplyTarget(target)
	}
	if a.th.editing != nil {
		a.th.editView.SetFrame(editorFrame(*a.th.editing)) // compared by content
	}

	// Resource preview for the visible tab.
	frame, title, caption := a.resourcePreviewFrame()
	a.resView.SetFrame(frame)
	if title != v.resTitle || caption != v.resCaption {
		v.resTitle, v.resCaption = title, caption
		w.SetResourcePreviewInfo(title, caption)
	}

	// Settings: the Options dialog, the menus and the interface theme.
	a.renderSettings(v)

	// Error messages: the first queued alert is on screen.
	switch {
	case len(a.alerts) == 0:
		if w.Message.Visible() {
			w.Message.Hide()
		}
		v.alert = core.Alert{}
	case !w.Message.Visible() || a.alerts[0] != v.alert:
		v.alert = a.alerts[0]
		w.Message.Show(v.alert.Title, v.alert.Message)
	}
	v.init = true
}

// resourcePreview returns what the resource preview shows for the visible
// tab: the loaded song, or the selected verse of the looked-up passage (the
// same verse the list highlights and Go Live sends).
func (a *App) resourcePreview() resourcePreview {
	switch a.win.Resources.Tab() {
	case ui.TabSongs:
		return a.songRes
	case ui.TabScriptures:
		return resourcePreview{deck: a.scriptureDeck, index: max(a.verseSelection(), 0)}
	case ui.TabMedia:
		return resourcePreview{deck: a.md.deck}
	case ui.TabPresentations:
		return resourcePreview{deck: a.pr.deck} // page 1
	case ui.TabThemes:
		return resourcePreview{deck: a.th.deck}
	}
	return resourcePreview{}
}

// resourcePreviewFrame is the resource preview's frame, title and caption.
func (a *App) resourcePreviewFrame() (frame models.Frame, title, caption string) {
	frame.Mode = models.ModeNormal
	res := a.resourcePreview()
	if d := res.deck; d != nil && res.index >= 0 && res.index < len(d.Slides) {
		s := d.Slides[res.index]
		frame.Slide, frame.Theme = &s, d.Theme
		title, caption = d.Title, core.SlideCaption(d, res.index)
	}
	return frame, title, caption
}
