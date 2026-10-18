package ui

import (
	"freeworship/internal/models"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEvent"
	"graphics.gd/classdb/InputEventKey"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/PopupMenu"
	"graphics.gd/classdb/Tree"
	"graphics.gd/classdb/TreeItem"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2"
)

// Song context-menu ids (PopupMenu_SongItemAction).
const (
	songMenuEdit = iota
	songMenuDelete
	songMenuAddToSchedule
	songMenuGoLive
	songMenuNew
)

// SongLibraryPanel is the Songs resource tab: the library (collections) pane
// with its search locator on the left, and the song list on the right.
type SongLibraryPanel struct {
	OnSearch        func(q string) // debounced (~200ms); Enter fires immediately
	OnSelect        func(i int)
	OnActivate      func(i int) // double-click / Enter: show in preview / go live (app decides)
	OnNew           func()
	OnEdit          func(i int)
	OnDelete        func(i int)
	OnAddToSchedule func(i int)
	OnGoLive        func(i int)

	split     *Splitter
	search    *searchLocator
	library   Tree.Instance
	list      Tree.Instance
	count     Label.Instance
	menu      PopupMenu.Instance
	gearMenu  PopupMenu.Instance
	songs     []models.SongSummary
	selected  int
	countText string
	rendering bool
	built     bool
}

// NewSongLibraryPanel builds the Songs tab.
func NewSongLibraryPanel() *SongLibraryPanel {
	s := &SongLibraryPanel{selected: -1}
	s.split = NewSplitter(SplitHorizontal, 0.35, 150, 150, 0, 0)

	// Left: library selector.
	s.search = newSearchLocator("Search Songs", func(q string) {
		if s.OnSearch != nil {
			s.OnSearch(q)
		}
	})
	s.library = Tree.New()
	s.library.AsControl().SetThemeTypeVariation(vLibraryTree)
	s.library.SetHideRoot(true)
	s.library.SetColumns(1)
	root := s.library.CreateItem()
	lib := root.CreateChild()
	lib.SetText(0, "Library")
	lib.SetIconMaxWidth(0, 16)
	lib.Select(0)
	s.library.AsControl().OnThemeChanged(func() { lib.SetIcon(0, fwIcon(s.library.AsControl(), "library")) })
	left := libraryPane(s.search.root.AsControl(), s.library.AsControl(), libraryFooter())

	// Right: song list.
	s.list = Tree.New()
	s.list.SetColumns(4)
	s.list.SetColumnTitlesVisible(true)
	s.list.SetHideRoot(true)
	s.list.SetSelectMode(Tree.SelectRow)
	s.list.SetAllowRmbSelect(true)
	s.list.SetAllowReselect(true)
	for i, t := range []string{"Title", "Author", "Copyright", "CCLI #"} {
		s.list.SetColumnTitle(i, t)
		s.list.SetColumnTitleAlignment(i, GUI.HorizontalAlignmentLeft)
		s.list.SetColumnClipContent(i, true)
	}
	s.list.SetColumnExpandRatio(0, 3)
	s.list.SetColumnExpandRatio(1, 3)
	s.list.SetColumnExpandRatio(2, 2)
	s.list.SetColumnExpand(3, false)
	s.list.SetColumnCustomMinimumWidth(3, 80)
	s.list.OnItemSelected(func() {
		if s.rendering {
			return
		}
		if i := treeIndex(s.list.GetSelected()); i >= 0 {
			s.selected = i
			if s.OnSelect != nil {
				s.OnSelect(i)
			}
		}
	})
	s.list.OnItemActivated(func() {
		if i := treeIndex(s.list.GetSelected()); i >= 0 && s.OnActivate != nil {
			s.OnActivate(i)
		}
	})
	s.list.OnItemMouseSelected(func(_ Vector2.XY, button int) {
		if Input.MouseButton(button) == Input.MouseButtonRight {
			s.showMenu()
		}
	})
	s.list.AsControl().OnGuiInput(func(ev InputEvent.Instance) {
		if k, ok := Object.As[InputEventKey.Instance](ev); ok && k.AsInputEvent().IsPressed() &&
			(k.Keycode() == Input.KeyDelete) && s.selected >= 0 && s.OnDelete != nil {
			s.OnDelete(s.selected)
			acceptEvent(s.list.AsControl())
		}
	})

	s.menu = PopupMenu.New()
	s.menu.MoreArgs().AddItem("Edit Song…", songMenuEdit, 0)
	s.menu.MoreArgs().AddItem("Delete Song", songMenuDelete, 0)
	s.menu.AddSeparator()
	s.menu.MoreArgs().AddItem("Add to Schedule", songMenuAddToSchedule, 0)
	s.menu.MoreArgs().AddItem("Go Live", songMenuGoLive, 0)
	s.menu.OnIdPressed(s.menuPressed)
	s.list.AsNode().AddChild(s.menu.AsNode())

	// Footer: [+] | [⚙▾]   count   [▤|▾]
	s.count = label("", vCaption)
	s.count.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
	s.count.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	plus := newMiniButton(miniPlain, "plus", 28, 24, "New Song")
	plus.AsBaseButton().OnPressed(func() {
		if s.OnNew != nil {
			s.OnNew()
		}
	})
	gear := newMiniButton(miniDropdown, "gear", 40, 24, "Song Actions")
	s.gearMenu = PopupMenu.New()
	s.gearMenu.MoreArgs().AddItem("New Song…", songMenuNew, 0)
	s.gearMenu.MoreArgs().AddItem("Edit Song…", songMenuEdit, 0)
	s.gearMenu.MoreArgs().AddItem("Delete Song", songMenuDelete, 0)
	s.gearMenu.AddSeparator()
	s.gearMenu.MoreArgs().AddItem("Add to Schedule", songMenuAddToSchedule, 0)
	s.gearMenu.OnIdPressed(s.menuPressed)
	gear.AsNode().AddChild(s.gearMenu.AsNode())
	gear.AsBaseButton().OnPressed(func() {
		s.enableMenu(s.gearMenu)
		popupBelow(s.gearMenu, gear.AsControl())
	})
	footer := panel(vMiniBarTL)
	footer.AsControl().SetCustomMinimumSize(Vector2.New(0, 28))
	row := hbox(0)
	l, r := viewSizer("view_details")
	add(row, plus.AsControl(), withMargins(newDivider(true).AsControl(), 2, 2), gear.AsControl(),
		s.count.AsControl(), l.AsControl(), r.AsControl())
	add(footer, row.AsControl())
	right := itemsPane(s.list.AsControl(), footer.AsControl())

	s.split.SetPanes(left, right)
	s.render()
	return s
}

// AsControl returns the panel's root control.
func (s *SongLibraryPanel) AsControl() Control.Instance { return s.split.AsControl() }

// Splitter returns the library | list splitter (for persisting its percent).
func (s *SongLibraryPanel) Splitter() *Splitter { return s.split }

// SetSongs replaces the rows. The selection is cleared.
func (s *SongLibraryPanel) SetSongs(rows []models.SongSummary) {
	s.songs = append([]models.SongSummary(nil), rows...)
	s.selected = -1
	s.built = false
	s.render()
}

// SetSelected selects row i (-1 for none) without calling OnSelect.
func (s *SongLibraryPanel) SetSelected(i int) {
	s.selected = i
	s.render()
}

// SetCountText sets the footer status text (e.g. "223 songs").
func (s *SongLibraryPanel) SetCountText(text string) {
	s.countText = text
	s.render()
}

// SetSearchText sets the search box text without firing OnSearch.
func (s *SongLibraryPanel) SetSearchText(q string) { s.search.setText(q) }

func (s *SongLibraryPanel) render() {
	s.rendering = true
	defer func() { s.rendering = false }()
	if !s.built {
		s.built = true
		s.list.Clear()
		root := s.list.CreateItem()
		for _, song := range s.songs {
			it := root.CreateChild()
			it.SetText(0, song.Title)
			it.SetText(1, song.Author)
			it.SetText(2, song.Copyright)
			it.SetText(3, song.CCLI)
		}
	}
	selectTreeRow(s.list, s.selected)
	s.count.SetText(s.countText)
}

// selectTreeRow selects top-level row i of a tree (-1 deselects) and scrolls
// it into view. Callers suppress the resulting item_selected signal.
//
// Tree.set_selected / deselect_all do not queue a redraw (Godot 4.6), so a
// programmatic selection on a tree that has already been drawn stays
// invisible until something else redraws it. Redraw explicitly.
func selectTreeRow(t Tree.Instance, i int) {
	defer t.AsCanvasItem().QueueRedraw()
	root := t.GetRoot()
	if root == (TreeItem.Instance{}) || i < 0 || i >= root.GetChildCount() {
		t.DeselectAll()
		return
	}
	it := root.GetChild(i)
	if !it.IsSelected(0) {
		t.SetSelected(it, 0)
	}
	t.ScrollToItem(it)
}

func (s *SongLibraryPanel) enableMenu(m PopupMenu.Instance) {
	none := s.selected < 0
	for idx := 0; idx < m.ItemCount(); idx++ {
		switch m.GetItemId(idx) {
		case songMenuEdit, songMenuDelete, songMenuAddToSchedule, songMenuGoLive:
			m.SetItemDisabled(idx, none)
		}
	}
}

func (s *SongLibraryPanel) showMenu() {
	s.enableMenu(s.menu)
	popupAtMouse(s.menu)
}

func (s *SongLibraryPanel) menuPressed(id int) {
	i := s.selected
	call := func(f func(int)) {
		if f != nil && i >= 0 {
			f(i)
		}
	}
	switch id {
	case songMenuNew:
		if s.OnNew != nil {
			s.OnNew()
		}
	case songMenuEdit:
		call(s.OnEdit)
	case songMenuDelete:
		call(s.OnDelete)
	case songMenuAddToSchedule:
		call(s.OnAddToSchedule)
	case songMenuGoLive:
		call(s.OnGoLive)
	}
}

// treeIndex is the row index of a top-level tree item (-1 for none).
func treeIndex(it TreeItem.Instance) int {
	if it == (TreeItem.Instance{}) {
		return -1
	}
	return it.GetIndex()
}

// viewSizer builds the 24 + 16 split view-sizer button.
func viewSizer(icon string) (*miniButton, *miniButton) {
	l := newMiniButton(miniSplitLeft, icon, 24, 24, "View")
	r := newMiniButton(miniSplitRight, "", 16, 24, "View Options")
	return l, r
}

// libraryFooter is the library pane's footer: [+▾] | [⚙▾].
func libraryFooter() Control.Instance {
	footer := panel(vMiniBarTL)
	footer.AsControl().SetCustomMinimumSize(Vector2.New(0, 28))
	row := hbox(0)
	add(row,
		newMiniButton(miniDropdown, "plus", 40, 24, "Add Collection").AsControl(),
		withMargins(newDivider(true).AsControl(), 2, 2),
		newMiniButton(miniDropdown, "gear", 40, 24, "Collection Actions").AsControl())
	add(footer, row.AsControl())
	return footer.AsControl()
}

// libraryPane stacks a search locator, the library list and a footer on the
// libraryselector.bk background.
func libraryPane(search, list, footer Control.Instance) Control.Instance {
	bg := panel(vLibrary)
	col := vbox(0)
	add(bg, col.AsControl())
	expand(list)
	add(col, search, list, footer)
	return bg.AsControl()
}

// itemsPane stacks the item list over its footer on window.bk1.
func itemsPane(list, footer Control.Instance) Control.Instance {
	bg := panel(vBk1)
	col := vbox(0)
	add(bg, col.AsControl())
	expand(list)
	add(col, list, footer)
	return bg.AsControl()
}
