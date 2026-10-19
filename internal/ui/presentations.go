package ui

import (
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/PopupMenu"
	"graphics.gd/classdb/Tree"
	"graphics.gd/variant/Vector2"
)

// Presentation context-menu ids.
const (
	presMenuAddToSchedule = iota
	presMenuGoLive
	presMenuRemove
	presMenuAdd

	// The tile menu's further items (UI-SPEC §3.6; itemmenus.go).
	presMenuLogo
	presMenuDeleteFile
	presMenuReveal
	presMenuRefresh
	presMenuView // the View ▸ entry
	presMenuSort // the Sort by ▸ entry
)

// PresentationRow is one tile of the presentation grid.
type PresentationRow struct {
	Title string
	// Badge is shown at the tile's right ("12 slides", "Rendering 3/12",
	// "Embedded images", "Needs LibreOffice").
	Badge string
	// Thumb is the tile's thumbnail file (page 1). The panel does not load it
	// (the factory does); it is part of the row's identity, so that SetItems
	// with a new thumbnail rebuilds the tile.
	Thumb string

	// Rendered says the pages exist: Use As Logo Background needs page 1.
	Rendered bool
}

// PresentationPanel (FwPresentationPanel) is the Presentations resource tab:
// the library pane (search locator and the one collection) on the left, a
// grid of first-page thumbnails on the right. It is a dumb view: the app
// fills it through setters and learns of user intent through the On...
// callbacks. Thumbnails come from an injected factory, like the slide grids.
type PresentationPanel struct {
	Control.Extension[PresentationPanel] `gd:"FwPresentationPanel"`

	OnSearch        func(q string) `gd:"-"` // debounced (~200ms); Enter fires immediately
	OnSelect        func(i int)    `gd:"-"` // tile selected
	OnActivate      func(i int)    `gd:"-"` // double-click / Enter: go live (app decides)
	OnAddToSchedule func(i int)    `gd:"-"`
	OnGoLive        func(i int)    `gd:"-"`
	OnRemove        func(i int)    `gd:"-"` // remove from the library (the file is left alone)
	OnAdd           func()         `gd:"-"` // "+" → Add Presentation…

	// The tile menu (right-click and the footer gear; itemmenus.go). The
	// choices report the value picked; the app changes state and render
	// projects it back through SetView and SetSort.
	OnLogo       func(i int)       `gd:"-"` // Use As Logo Background (page 1)
	OnDeleteFile func(i int)       `gd:"-"` // Delete File…: the document to the Trash, the item out of the library
	OnReveal     func(i int)       `gd:"-"` // Open File Location
	OnView       func(view string) `gd:"-"`
	OnSort       func(sort string) `gd:"-"`
	OnRefresh    func()            `gd:"-"`

	split   *Splitter          `gd:"-"`
	search  *searchLocator     `gd:"-"`
	library Tree.Instance      `gd:"-"`
	grid    *SlideGrid         `gd:"-"`
	count   Label.Instance     `gd:"-"`
	menus   [2]*presMenu       `gd:"-"` // the tile context menu and the footer gear menu
	sizer   *itemViewSizer     `gd:"-"`
	plus    []Control.Instance `gd:"-"`

	rows        []PresentationRow
	selected    int
	countText   string
	builtLib    bool
	view        string // ViewLarge…; projected onto the grid, the sizer and the menus
	sort        string // SortName…; projected onto the menus
	appliedView string
}

// presMenu is one copy of the tile menu with its submenus.
type presMenu struct {
	menu       PopupMenu.Instance
	view, sort *radioMenu
}

// NewPresentationPanel builds the Presentations tab.
func NewPresentationPanel() *PresentationPanel {
	p := &PresentationPanel{selected: -1, view: ViewMedium, sort: SortName}
	root := p.AsControl()
	root.SetClipContents(true)
	p.split = NewSplitter(SplitHorizontal, LibraryPercent, 150, 150, 0, 0)
	p.split.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)

	// Left: library selector with the one collection.
	p.search = newSearchLocator("Search Presentations", func(q string) {
		if p.OnSearch != nil {
			p.OnSearch(q)
		}
	})
	p.library = Tree.New()
	p.library.AsControl().SetThemeTypeVariation(vLibraryTree)
	p.library.SetHideRoot(true)
	p.library.SetColumns(1)
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

	// Footer: [+] | [⚙▾]   count   [▤|▾]
	p.count = label("", vCaption)
	p.count.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
	p.count.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	plus := newMiniButton(miniPlain, "plus", 28, 24, "Add Presentation…")
	plus.AsBaseButton().OnPressed(func() {
		if p.OnAdd != nil {
			p.OnAdd()
		}
	})
	gear := newMiniButton(miniDropdown, "gear", 40, 24, "Presentation Actions")
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
	add(p, p.split.AsControl())
	p.render()
	return p
}

// buildMenu builds the tile menu (EasyWorship's presentation menu, UI-SPEC
// §3.6): add, schedule / live, logo, remove / delete, file location, then
// the panel's View ▸, Sort by ▸ and Refresh.
func (p *PresentationPanel) buildMenu() *presMenu {
	m := &presMenu{menu: PopupMenu.New()}
	pm := m.menu
	pm.MoreArgs().AddItem("Add Presentation…", presMenuAdd, 0)
	pm.AddSeparator()
	pm.MoreArgs().AddItem("Add to Schedule", presMenuAddToSchedule, 0)
	pm.MoreArgs().AddItem("Go Live", presMenuGoLive, 0)
	pm.AddSeparator()
	pm.MoreArgs().AddItem("Use As Logo Background", presMenuLogo, 0)
	pm.AddSeparator()
	pm.MoreArgs().AddItem("Remove from Library", presMenuRemove, 0)
	pm.MoreArgs().AddItem("Delete File…", presMenuDeleteFile, 0)
	pm.AddSeparator()
	pm.MoreArgs().AddItem("Open File Location", presMenuReveal, 0)
	pm.AddSeparator()
	m.view = newRadioMenu(pm, "View", "View", presMenuView, viewChoices, func(v string) {
		if p.OnView != nil {
			p.OnView(v)
		}
	})
	m.sort = newRadioMenu(pm, "SortBy", "Sort by", presMenuSort, sortChoices, func(v string) {
		if p.OnSort != nil {
			p.OnSort(v)
		}
	})
	pm.MoreArgs().AddItem("Refresh", presMenuRefresh, 0)
	pm.OnIdPressed(p.menuPressed)
	return m
}

// libraryFooter is the library pane's footer: [+] (add presentation) | [⚙▾].
func (p *PresentationPanel) libraryFooter() Control.Instance {
	footer := panel(vMiniBarTL)
	footer.AsControl().SetCustomMinimumSize(Vector2.New(0, 28))
	row := hbox(0)
	plus := newMiniButton(miniPlain, "plus", 28, 24, "Add Presentation…")
	plus.AsBaseButton().OnPressed(func() {
		if p.OnAdd != nil {
			p.OnAdd()
		}
	})
	add(row, plus.AsControl(), withMargins(newDivider(true).AsControl(), 2, 2),
		newMiniButton(miniDropdown, "gear", 40, 24, "Collection Actions").AsControl())
	add(footer, row.AsControl())
	return footer.AsControl()
}

// Splitter returns the library | grid splitter.
func (p *PresentationPanel) Splitter() *Splitter { return p.split }

// Grid returns the thumbnail grid (keyboard focus lives there).
func (p *PresentationPanel) Grid() *SlideGrid { return p.grid }

// SetItems replaces the tiles. The selection is cleared.
func (p *PresentationPanel) SetItems(rows []PresentationRow) {
	p.rows = append([]PresentationRow(nil), rows...)
	p.selected = -1
	items := make([]SlideGridItem, len(rows))
	for i, r := range rows {
		items[i] = SlideGridItem{Label: r.Title, Badge: r.Badge}
	}
	p.grid.SetItems(items)
	p.render()
}

// SetSelected selects tile i (-1 for none) without calling OnSelect.
func (p *PresentationPanel) SetSelected(i int) {
	p.selected = i
	p.grid.SetSelected(i)
	p.render()
}

// SetCountText sets the footer status text (e.g. "3 presentations").
func (p *PresentationPanel) SetCountText(text string) {
	p.countText = text
	p.render()
}

// SetSearchText sets the search box text without firing OnSearch.
func (p *PresentationPanel) SetSearchText(q string) { p.search.setText(q) }

// SetView shows the grid in a View ▸ choice (ViewLarge…) without calling OnView.
func (p *PresentationPanel) SetView(view string) {
	p.view = view
	p.render()
}

// SetSort marks a Sort by ▸ choice (SortName…) without calling OnSort. The
// rows themselves come sorted from the app.
func (p *PresentationPanel) SetSort(sort string) {
	p.sort = sort
	p.render()
}

// SetFileDeletion removes Delete File… from the menus when the platform has
// no Trash to move files to (false). Call it once, before the menus open.
func (p *PresentationPanel) SetFileDeletion(on bool) {
	if on {
		return
	}
	for _, m := range p.menus {
		if idx := m.menu.GetItemIndex(presMenuDeleteFile); idx >= 0 {
			m.menu.RemoveItem(idx)
		}
	}
}

// SetThumbnailFactory sets the grid's thumbnail factory and rebuilds the tiles.
func (p *PresentationPanel) SetThumbnailFactory(f func(i int) Control.Instance) {
	p.grid.SetThumbnailFactory(f)
}

// RefreshThumbnails rebuilds every thumbnail from the factory (new pages arrived).
func (p *PresentationPanel) RefreshThumbnails() { p.grid.RefreshThumbnails() }

// render projects the panel state. The library tree has one collection,
// always selected; its programmatic selection runs quietly. The menus'
// radio entries follow the view / sort; the grid and the footer sizer
// follow the view.
func (p *PresentationPanel) render() {
	quietly(p.library.AsControl(), func() {
		if !p.builtLib {
			p.builtLib = true
			p.library.Clear()
			root := p.library.CreateItem()
			it := root.CreateChild()
			it.SetText(0, "Presentations")
			it.SetIcon(0, fwIcon(p.library.AsControl(), "presentation"))
			it.SetIconMaxWidth(0, 16)
		}
		selectTreeRow(p.library, 0)
	})
	p.count.SetText(p.countText)
	for _, m := range p.menus {
		m.view.setChecked(p.view)
		m.sort.setChecked(p.sort)
	}
	p.sizer.setView(p.view)
	if p.view != p.appliedView {
		p.appliedView = p.view
		applyView(p.grid, p.view)
	}
	flushCalls(p.count.AsControl())
}

// enableMenu greys the item actions out when nothing is selected (and the
// logo item until the pages exist) before the menu opens.
func (p *PresentationPanel) enableMenu(m *presMenu) {
	none := p.selected < 0 || p.selected >= len(p.rows)
	rendered := !none && p.rows[p.selected].Rendered
	for idx := 0; idx < m.menu.ItemCount(); idx++ {
		switch m.menu.GetItemId(idx) {
		case presMenuAddToSchedule, presMenuGoLive, presMenuRemove, presMenuDeleteFile, presMenuReveal:
			m.menu.SetItemDisabled(idx, none)
		case presMenuLogo:
			m.menu.SetItemDisabled(idx, !rendered)
		}
	}
}

func (p *PresentationPanel) menuPressed(id int) {
	i := p.selected
	call := func(f func(int)) {
		if f != nil && i >= 0 && i < len(p.rows) {
			f(i)
		}
	}
	switch id {
	case presMenuAddToSchedule:
		call(p.OnAddToSchedule)
	case presMenuGoLive:
		call(p.OnGoLive)
	case presMenuRemove:
		call(p.OnRemove)
	case presMenuDeleteFile:
		call(p.OnDeleteFile)
	case presMenuLogo:
		call(p.OnLogo)
	case presMenuReveal:
		call(p.OnReveal)
	case presMenuRefresh:
		if p.OnRefresh != nil {
			p.OnRefresh()
		}
	case presMenuAdd:
		if p.OnAdd != nil {
			p.OnAdd()
		}
	}
}
