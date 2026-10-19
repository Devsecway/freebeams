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

	split    *Splitter          `gd:"-"`
	search   *searchLocator     `gd:"-"`
	library  Tree.Instance      `gd:"-"`
	grid     *SlideGrid         `gd:"-"`
	count    Label.Instance     `gd:"-"`
	menu     PopupMenu.Instance `gd:"-"`
	gearMenu PopupMenu.Instance `gd:"-"`

	rows      []PresentationRow
	selected  int
	countText string
	builtLib  bool
}

// NewPresentationPanel builds the Presentations tab.
func NewPresentationPanel() *PresentationPanel {
	p := &PresentationPanel{selected: -1}
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
		p.enableMenu(p.menu)
		popupAtMouse(p.menu)
	}
	p.menu = PopupMenu.New()
	p.fillMenu(p.menu)
	p.grid.AsNode().AddChild(p.menu.AsNode())

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
	p.gearMenu = PopupMenu.New()
	p.fillMenu(p.gearMenu)
	p.gearMenu.AddSeparator()
	p.gearMenu.MoreArgs().AddItem("Add Presentation…", presMenuAdd, 0)
	gear.AsNode().AddChild(p.gearMenu.AsNode())
	gear.AsBaseButton().OnPressed(func() {
		p.enableMenu(p.gearMenu)
		popupBelow(p.gearMenu, gear.AsControl())
	})
	footer := panel(vMiniBarTL)
	footer.AsControl().SetCustomMinimumSize(Vector2.New(0, 28))
	row := hbox(0)
	add(row, plus.AsControl(), withMargins(newDivider(true).AsControl(), 2, 2), gear.AsControl(), p.count.AsControl())
	add(row, gridSizer(p.grid)...)
	add(footer, row.AsControl())
	right := itemsPane(p.grid.AsControl(), footer.AsControl())

	p.split.SetPanes(left, right)
	add(p, p.split.AsControl())
	p.render()
	return p
}

// fillMenu adds the item actions to a context / gear menu.
func (p *PresentationPanel) fillMenu(m PopupMenu.Instance) {
	m.MoreArgs().AddItem("Add to Schedule", presMenuAddToSchedule, 0)
	m.MoreArgs().AddItem("Go Live", presMenuGoLive, 0)
	m.AddSeparator()
	m.MoreArgs().AddItem("Remove from Library", presMenuRemove, 0)
	m.OnIdPressed(p.menuPressed)
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

// SetThumbnailFactory sets the grid's thumbnail factory and rebuilds the tiles.
func (p *PresentationPanel) SetThumbnailFactory(f func(i int) Control.Instance) {
	p.grid.SetThumbnailFactory(f)
}

// RefreshThumbnails rebuilds every thumbnail from the factory (new pages arrived).
func (p *PresentationPanel) RefreshThumbnails() { p.grid.RefreshThumbnails() }

// render projects the panel state. The library tree has one collection,
// always selected; its programmatic selection runs quietly.
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
}

func (p *PresentationPanel) enableMenu(m PopupMenu.Instance) {
	none := p.selected < 0 || p.selected >= len(p.rows)
	for idx := 0; idx < m.ItemCount(); idx++ {
		switch m.GetItemId(idx) {
		case presMenuAddToSchedule, presMenuGoLive, presMenuRemove:
			m.SetItemDisabled(idx, none)
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
	case presMenuAdd:
		if p.OnAdd != nil {
			p.OnAdd()
		}
	}
}
