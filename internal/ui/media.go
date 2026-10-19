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

	split    *Splitter
	search   *searchLocator
	library  Tree.Instance
	grid     *SlideGrid
	count    Label.Instance
	menu     PopupMenu.Instance
	addMenu  PopupMenu.Instance
	gearMenu PopupMenu.Instance

	rows      []MediaRow
	selected  int
	filter    models.MediaKind
	countText string
	builtLib  bool
}

// NewMediaPanel builds the Media tab.
func NewMediaPanel() *MediaPanel {
	p := &MediaPanel{selected: -1}
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
	p.library.AsControl().OnThemeChanged(func() { p.builtLib = false; p.render() })
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
	p.menu.MoreArgs().AddItem("Add to Schedule", mediaMenuAddToSchedule, 0)
	p.menu.MoreArgs().AddItem("Go Live", mediaMenuGoLive, 0)
	p.menu.AddSeparator()
	p.menu.MoreArgs().AddItem("Remove from Library", mediaMenuRemove, 0)
	p.menu.OnIdPressed(p.menuPressed)
	p.grid.AsNode().AddChild(p.menu.AsNode())

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
	p.gearMenu = PopupMenu.New()
	p.gearMenu.MoreArgs().AddItem("Add to Schedule", mediaMenuAddToSchedule, 0)
	p.gearMenu.MoreArgs().AddItem("Go Live", mediaMenuGoLive, 0)
	p.gearMenu.AddSeparator()
	p.gearMenu.MoreArgs().AddItem("Remove from Library", mediaMenuRemove, 0)
	p.gearMenu.OnIdPressed(p.menuPressed)
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
	p.render()
	return p
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

// SetThumbnailFactory sets the grid's thumbnail factory and rebuilds the tiles.
func (p *MediaPanel) SetThumbnailFactory(f func(i int) Control.Instance) {
	p.grid.SetThumbnailFactory(f)
}

// RefreshThumbnails rebuilds every thumbnail from the factory (new posters arrived).
func (p *MediaPanel) RefreshThumbnails() { p.grid.RefreshThumbnails() }

// render projects the panel state. Its programmatic Tree changes run quietly
// (signals blocked), so they never come back as OnFilter.
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
}

func (p *MediaPanel) enableMenu(m PopupMenu.Instance) {
	none := p.selected < 0 || p.selected >= len(p.rows)
	for idx := 0; idx < m.ItemCount(); idx++ {
		switch m.GetItemId(idx) {
		case mediaMenuAddToSchedule, mediaMenuGoLive, mediaMenuRemove:
			m.SetItemDisabled(idx, none)
		}
	}
}

func (p *MediaPanel) menuPressed(id int) {
	i := p.selected
	call := func(f func(int)) {
		if f != nil && i >= 0 && i < len(p.rows) {
			f(i)
		}
	}
	switch id {
	case mediaMenuAddToSchedule:
		call(p.OnAddToSchedule)
	case mediaMenuGoLive:
		call(p.OnGoLive)
	case mediaMenuRemove:
		call(p.OnRemove)
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
