package ui

import (
	"freeworship/internal/models"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/PopupMenu"
	"graphics.gd/classdb/Tree"
	"graphics.gd/variant/Vector2"
)

// Media context-menu ids.
const (
	mediaMenuAddToSchedule = iota
	mediaMenuGoLive
	mediaMenuRemove
	mediaMenuAddFiles
	mediaMenuAddFolder

	// The tile menu's further items (UI-SPEC §3.6; itemmenus.go).
	mediaMenuLogo
	mediaMenuDeleteFile
	mediaMenuUpdateSchedule
	mediaMenuReveal
	mediaMenuRefresh
	mediaMenuAspect // the Aspect Ratio ▸ entry
	mediaMenuRepeat // the Repeating ▸ entry
	mediaMenuView   // the View ▸ entry
	mediaMenuSort   // the Sort by ▸ entry
)

// MediaRow is one tile of the media grid.
type MediaRow struct {
	Title string
	// Badge is shown at the tile's right ("0:30" for a video, or a short
	// status such as "Needs ffmpeg" / "Converting 42%").
	Badge string
	// Thumb is the tile's thumbnail file. The panel does not load it (the
	// factory does); it is part of the row's identity, so that SetItems with
	// a new thumbnail rebuilds the tile.
	Thumb string

	// Kind, Aspect and Repeat drive the tile menu: the checked Aspect Ratio ▸
	// and Repeating ▸ entries, and Repeating ▸ is for videos only. Aspect is
	// part of the row's identity too (the thumbnail letterboxes like the slide).
	Kind   models.MediaKind
	Aspect models.MediaAspect
	Repeat models.MediaRepeat
}

// mediaFilters are the library pane's collections (UI-SPEC §3.6: Media
// tab sub-categories), in display order. "" lists every item.
var mediaFilters = []struct {
	name string
	kind models.MediaKind
	icon string
}{
	{"All Media", "", "library"},
	{"Videos", models.MediaVideo, "video"},
	{"Images", models.MediaImage, "image"},
}

// MediaPanel is the Media resource tab: the library pane (search locator and
// the Videos / Images collections) on the left, a grid of thumbnails on the
// right. It is a dumb view: the app fills it through setters and learns of
// user intent through the On... callbacks. Thumbnails come from an injected
// factory, like the slide grids.
type MediaPanel struct {
	OnSearch        func(q string)              // debounced (~200ms); Enter fires immediately
	OnFilter        func(kind models.MediaKind) // collection clicked ("" = all)
	OnSelect        func(i int)                 // tile selected
	OnActivate      func(i int)                 // double-click / Enter: go live (app decides)
	OnAddToSchedule func(i int)
	OnGoLive        func(i int)
	OnRemove        func(i int) // remove from the library (the file is left alone)
	OnAddFiles      func()      // "+" → Add Media Files…
	OnAddFolder     func()      // "+" → Add Media Folder…

	// The tile menu (right-click and the footer gear; itemmenus.go). The
	// choices report the value picked; the app changes state and render
	// projects it back through SetItems (Aspect, Repeat), SetView and SetSort.
	OnAspect         func(i int, aspect models.MediaAspect)
	OnRepeat         func(i int, repeat models.MediaRepeat)
	OnLogo           func(i int) // Use As Logo Background
	OnDeleteFile     func(i int) // Delete File…: the file to the Trash, the item out of the library
	OnUpdateSchedule func(i int) // Update items in Schedule
	OnReveal         func(i int) // Open File Location
	OnView           func(view string)
	OnSort           func(sort string)
	OnRefresh        func()

	split   *Splitter
	search  *searchLocator
	library Tree.Instance
	grid    *SlideGrid
	count   Label.Instance
	addMenu PopupMenu.Instance
	menus   [2]*mediaMenu // the tile context menu and the footer gear menu
	sizer   *itemViewSizer

	rows        []MediaRow
	selected    int
	filter      models.MediaKind
	countText   string
	builtLib    bool
	view        string // ViewLarge…; projected onto the grid, the sizer and the menus
	sort        string // SortName…; projected onto the menus
	appliedView string
}

// mediaMenu is one copy of the tile menu with its submenus (the context
// menu and the gear menu are the same menu; a submenu node has one parent).
type mediaMenu struct {
	menu                       PopupMenu.Instance
	aspect, repeat, view, sort *radioMenu
}

// NewMediaPanel builds the Media tab.
func NewMediaPanel() *MediaPanel {
	p := &MediaPanel{selected: -1, view: ViewMedium, sort: SortName}
	p.split = NewSplitter(SplitHorizontal, LibraryPercent, 150, 150, 0, 0)

	// Left: library selector with the collections.
	p.search = newSearchLocator("Search Media", func(q string) {
		if p.OnSearch != nil {
			p.OnSearch(q)
		}
	})
	p.library = Tree.New()
	p.library.AsControl().SetThemeTypeVariation(vLibraryTree)
	p.library.SetHideRoot(true)
	p.library.SetColumns(1)
	p.library.OnItemSelected(func() {
		i := treeIndex(p.library.GetSelected())
		if i < 0 || i >= len(mediaFilters) {
			return
		}
		p.filter = mediaFilters[i].kind
		if p.OnFilter != nil {
			p.OnFilter(p.filter)
		}
	})
	onThemeReady(p.library.AsControl(), func() { p.builtLib = false; p.render() })
	left := libraryPane(p.search.root.AsControl(), p.library.AsControl(), p.libraryFooter())

	// Right: the thumbnail grid.
	p.grid = NewSlideGrid()
	p.grid.OnSelect = func(i int) {
		p.selected = i
		if p.OnSelect != nil {
			p.OnSelect(i)
		}
	}
	p.grid.OnActivate = func(i int) {
		if p.OnActivate != nil {
			p.OnActivate(i)
		}
	}
	p.grid.OnContextMenu = func(i int) {
		p.selected = i
		p.enableMenu(p.menus[0])
		popupAtMouse(p.menus[0].menu)
	}
	p.menus[0] = p.buildMenu()
	p.grid.AsNode().AddChild(p.menus[0].menu.AsNode())

	// Footer: [+▾] | [⚙▾]   count   [▤|▾]
	p.count = label("", vCaption)
	p.count.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
	p.count.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	plus := newMiniButton(miniDropdown, "plus", 40, 24, "Add Media")
	p.addMenu = PopupMenu.New()
	p.addMenu.MoreArgs().AddItem("Add Media Files…", mediaMenuAddFiles, 0)
	p.addMenu.MoreArgs().AddItem("Add Media Folder…", mediaMenuAddFolder, 0)
	p.addMenu.OnIdPressed(p.menuPressed)
	plus.AsNode().AddChild(p.addMenu.AsNode())
	plus.AsBaseButton().OnPressed(func() { popupBelow(p.addMenu, plus.AsControl()) })
	gear := newMiniButton(miniDropdown, "gear", 40, 24, "Media Actions")
	p.menus[1] = p.buildMenu()
	gear.AsNode().AddChild(p.menus[1].menu.AsNode())
	gear.AsBaseButton().OnPressed(func() {
		p.enableMenu(p.menus[1])
		popupBelow(p.menus[1].menu, gear.AsControl())
	})
	p.sizer = newItemViewSizer(func(view string) {
		if p.OnView != nil {
			p.OnView(view)
		}
	})
	footer := panel(vMiniBarTL)
	footer.AsControl().SetCustomMinimumSize(Vector2.New(0, 28))
	row := hbox(0)
	add(row, plus.AsControl(), withMargins(newDivider(true).AsControl(), 2, 2), gear.AsControl(), p.count.AsControl())
	add(row, p.sizer.controls...)
	add(footer, row.AsControl())
	right := itemsPane(p.grid.AsControl(), footer.AsControl())

	p.split.SetPanes(left, right)
	p.render()
	return p
}

// buildMenu builds the tile menu (EasyWorship's video tile menu, UI-SPEC
// §3.6): add, schedule / live, the item's Aspect Ratio ▸ and Repeating ▸,
// logo, remove / delete, schedule update, file location, then the panel's
// View ▸, Sort by ▸ and Refresh.
func (p *MediaPanel) buildMenu() *mediaMenu {
	m := &mediaMenu{menu: PopupMenu.New()}
	pm := m.menu
	pm.MoreArgs().AddItem("Add Media Files…", mediaMenuAddFiles, 0)
	pm.AddSeparator()
	pm.MoreArgs().AddItem("Add to Schedule", mediaMenuAddToSchedule, 0)
	pm.MoreArgs().AddItem("Go Live", mediaMenuGoLive, 0)
	pm.AddSeparator()
	m.aspect = newRadioMenu(pm, "AspectRatio", "Aspect Ratio", mediaMenuAspect, aspectChoices, func(v string) {
		p.withSelected(func(i int) {
			if p.OnAspect != nil {
				p.OnAspect(i, models.MediaAspect(v))
			}
		})
	})
	m.repeat = newRadioMenu(pm, "Repeating", "Repeating", mediaMenuRepeat, repeatChoices, func(v string) {
		p.withSelected(func(i int) {
			if p.OnRepeat != nil {
				p.OnRepeat(i, models.MediaRepeat(v))
			}
		})
	})
	pm.AddSeparator()
	pm.MoreArgs().AddItem("Use As Logo Background", mediaMenuLogo, 0)
	pm.AddSeparator()
	pm.MoreArgs().AddItem("Remove from Library", mediaMenuRemove, 0)
	pm.MoreArgs().AddItem("Delete File…", mediaMenuDeleteFile, 0)
	pm.AddSeparator()
	pm.MoreArgs().AddItem("Update items in Schedule", mediaMenuUpdateSchedule, 0)
	pm.AddSeparator()
	pm.MoreArgs().AddItem("Open File Location", mediaMenuReveal, 0)
	pm.AddSeparator()
	m.view = newRadioMenu(pm, "View", "View", mediaMenuView, viewChoices, func(v string) {
		if p.OnView != nil {
			p.OnView(v)
		}
	})
	m.sort = newRadioMenu(pm, "SortBy", "Sort by", mediaMenuSort, sortChoices, func(v string) {
		if p.OnSort != nil {
			p.OnSort(v)
		}
	})
	pm.MoreArgs().AddItem("Refresh", mediaMenuRefresh, 0)
	pm.OnIdPressed(p.menuPressed)
	return m
}

// libraryFooter is the library pane's footer: [+▾] (add media) | [⚙▾].
func (p *MediaPanel) libraryFooter() Control.Instance {
	footer := panel(vMiniBarTL)
	footer.AsControl().SetCustomMinimumSize(Vector2.New(0, 28))
	row := hbox(0)
	plus := newMiniButton(miniDropdown, "plus", 40, 24, "Add Media")
	plus.AsBaseButton().OnPressed(func() { popupBelow(p.addMenu, plus.AsControl()) })
	add(row, plus.AsControl(), withMargins(newDivider(true).AsControl(), 2, 2),
		newMiniButton(miniDropdown, "gear", 40, 24, "Collection Actions").AsControl())
	add(footer, row.AsControl())
	return footer.AsControl()
}

// AsControl returns the panel's root control.
func (p *MediaPanel) AsControl() Control.Instance { return p.split.AsControl() }

// Splitter returns the library | grid splitter.
func (p *MediaPanel) Splitter() *Splitter { return p.split }

// Grid returns the thumbnail grid (keyboard focus lives there).
func (p *MediaPanel) Grid() *SlideGrid { return p.grid }

// SetItems replaces the tiles. The selection is cleared.
func (p *MediaPanel) SetItems(rows []MediaRow) {
	p.rows = append([]MediaRow(nil), rows...)
	p.selected = -1
	items := make([]SlideGridItem, len(rows))
	for i, r := range rows {
		items[i] = SlideGridItem{Label: r.Title, Badge: r.Badge}
	}
	p.grid.SetItems(items)
	p.render()
}

// SetSelected selects tile i (-1 for none) without calling OnSelect.
func (p *MediaPanel) SetSelected(i int) {
	p.selected = i
	p.grid.SetSelected(i)
	p.render()
}

// SetFilter selects a collection without calling OnFilter.
func (p *MediaPanel) SetFilter(kind models.MediaKind) {
	p.filter = kind
	p.render()
}

// Filter returns the selected collection ("" = all).
func (p *MediaPanel) Filter() models.MediaKind { return p.filter }

// SetCountText sets the footer status text (e.g. "23 items · 8 videos").
func (p *MediaPanel) SetCountText(text string) {
	p.countText = text
	p.render()
}

// SetSearchText sets the search box text without firing OnSearch.
func (p *MediaPanel) SetSearchText(q string) { p.search.setText(q) }

// SetView shows the grid in a View ▸ choice (ViewLarge…) without calling OnView.
func (p *MediaPanel) SetView(view string) {
	p.view = view
	p.render()
}

// SetSort marks a Sort by ▸ choice (SortName…) without calling OnSort. The
// rows themselves come sorted from the app.
func (p *MediaPanel) SetSort(sort string) {
	p.sort = sort
	p.render()
}

// SetFileDeletion removes Delete File… from the menus when the platform has
// no Trash to move files to (false). Call it once, before the menus open.
func (p *MediaPanel) SetFileDeletion(on bool) {
	if on {
		return
	}
	for _, m := range p.menus {
		if idx := m.menu.GetItemIndex(mediaMenuDeleteFile); idx >= 0 {
			m.menu.RemoveItem(idx)
		}
	}
}

// SetThumbnailFactory sets the grid's thumbnail factory and rebuilds the tiles.
func (p *MediaPanel) SetThumbnailFactory(f func(i int) Control.Instance) {
	p.grid.SetThumbnailFactory(f)
}

// RefreshThumbnails rebuilds every thumbnail from the factory (new posters arrived).
func (p *MediaPanel) RefreshThumbnails() { p.grid.RefreshThumbnails() }

// render projects the panel state. Its programmatic Tree changes run quietly
// (signals blocked), so they never come back as OnFilter. The menus' radio
// entries follow the selected row and the view / sort; the grid and the
// footer sizer follow the view.
func (p *MediaPanel) render() {
	quietly(p.library.AsControl(), func() {
		if !p.builtLib {
			p.builtLib = true
			p.library.Clear()
			root := p.library.CreateItem()
			for _, f := range mediaFilters {
				it := root.CreateChild()
				it.SetText(0, f.name)
				it.SetIcon(0, fwIcon(p.library.AsControl(), f.icon))
				it.SetIconMaxWidth(0, 16)
			}
		}
		row := 0
		for i, f := range mediaFilters {
			if f.kind == p.filter {
				row = i
			}
		}
		selectTreeRow(p.library, row)
	})
	p.count.SetText(p.countText)
	var sel *MediaRow
	if p.selected >= 0 && p.selected < len(p.rows) {
		sel = &p.rows[p.selected]
	}
	for _, m := range p.menus {
		p.projectMenu(m, sel)
	}
	p.sizer.setView(p.view)
	if p.view != p.appliedView {
		p.appliedView = p.view
		applyView(p.grid, p.view)
	}
	flushCalls(p.count.AsControl())
}

// projectMenu checks the radio entries for the selected row and the panel's
// view and sort. Repeating ▸ applies to videos only.
func (p *MediaPanel) projectMenu(m *mediaMenu, sel *MediaRow) {
	aspect, repeat, video := models.AspectFill, models.RepeatLoop, false
	if sel != nil {
		aspect, repeat, video = sel.Aspect.Normalized(), sel.Repeat.Normalized(), sel.Kind == models.MediaVideo
	}
	m.aspect.setChecked(string(aspect))
	m.repeat.setChecked(string(repeat))
	m.repeat.setDisabled(!video)
	m.view.setChecked(p.view)
	m.sort.setChecked(p.sort)
}

// enableMenu greys the item actions out when nothing is selected (and
// Repeating ▸ for a still) before the menu opens.
func (p *MediaPanel) enableMenu(m *mediaMenu) {
	none := p.selected < 0 || p.selected >= len(p.rows)
	video := !none && p.rows[p.selected].Kind == models.MediaVideo
	for idx := 0; idx < m.menu.ItemCount(); idx++ {
		switch m.menu.GetItemId(idx) {
		case mediaMenuAddToSchedule, mediaMenuGoLive, mediaMenuRemove, mediaMenuDeleteFile, mediaMenuLogo,
			mediaMenuUpdateSchedule, mediaMenuReveal, mediaMenuAspect:
			m.menu.SetItemDisabled(idx, none)
		case mediaMenuRepeat:
			m.menu.SetItemDisabled(idx, !video)
		}
	}
}

// withSelected calls fn with the selected tile, if any.
func (p *MediaPanel) withSelected(fn func(i int)) {
	if i := p.selected; i >= 0 && i < len(p.rows) {
		fn(i)
	}
}

func (p *MediaPanel) menuPressed(id int) {
	call := func(f func(int)) {
		if f != nil {
			p.withSelected(f)
		}
	}
	switch id {
	case mediaMenuAddToSchedule:
		call(p.OnAddToSchedule)
	case mediaMenuGoLive:
		call(p.OnGoLive)
	case mediaMenuRemove:
		call(p.OnRemove)
	case mediaMenuDeleteFile:
		call(p.OnDeleteFile)
	case mediaMenuLogo:
		call(p.OnLogo)
	case mediaMenuUpdateSchedule:
		call(p.OnUpdateSchedule)
	case mediaMenuReveal:
		call(p.OnReveal)
	case mediaMenuRefresh:
		if p.OnRefresh != nil {
			p.OnRefresh()
		}
	case mediaMenuAddFiles:
		if p.OnAddFiles != nil {
			p.OnAddFiles()
		}
	case mediaMenuAddFolder:
		if p.OnAddFolder != nil {
			p.OnAddFolder()
		}
	}
}
