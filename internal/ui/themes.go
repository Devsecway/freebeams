package ui

import (
	"freebeams/internal/models"
	"freebeams/internal/ui/uicore"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/PopupMenu"
	"graphics.gd/classdb/Tree"
	"graphics.gd/variant/Vector2"
)

// Theme menu ids (context menu and footer gear).
const (
	themeMenuNew = iota
	themeMenuEdit
	themeMenuDuplicate
	themeMenuDelete
	themeMenuSongDefault
	themeMenuScriptureDefault
	themeMenuApply
)

// Sharing themes (theme files, core/themefile.go).
const (
	themeMenuExport = iota + 100
	themeMenuImport
)

// ThemeRow is one tile of the themes grid.
type ThemeRow struct {
	Name string
	// Badge marks the default themes: "Songs", "Scripture" or "Songs · Scripture".
	Badge string
}

// ThemePanel is the Themes resource tab: the library pane on the left and a
// grid of theme thumbnails (a sample lyric drawn with each theme) on the
// right, with New / Edit / Duplicate / Delete, the two "Set as default"
// actions and "Apply to schedule item". It is a dumb view: the app fills it
// through setters and learns of user intent through the On... callbacks;
// thumbnails come from an injected factory, like the slide grids.
type ThemePanel struct {
	Control.Extension[ThemePanel] `gd:"FwThemePanel"`

	OnSelect    func(i int) `gd:"-"` // tile selected
	OnActivate  func(i int) `gd:"-"` // double-click / Enter: edit (app decides)
	OnNew       func()      `gd:"-"`
	OnEdit      func(i int) `gd:"-"`
	OnDuplicate func(i int) `gd:"-"`
	OnDelete    func(i int) `gd:"-"`
	// OnSetDefault makes theme i the default for songs or scripture items.
	OnSetDefault func(i int, kind models.ItemKind) `gd:"-"`
	// OnApply applies theme i to the selected schedule item (see SetApplyTarget).
	OnApply func(i int) `gd:"-"`

	// OnExport saves theme i as a theme file; OnImport adds one.
	OnExport func(i int) `gd:"-"`
	OnImport func()      `gd:"-"`

	split    *Splitter
	library  Tree.Instance
	grid     *SlideGrid
	count    Label.Instance
	menu     PopupMenu.Instance
	gearMenu PopupMenu.Instance

	rows        []ThemeRow
	selected    int
	countText   string
	applyTarget string // title of the selected schedule item ("" = none)
	builtLib    bool
}

// NewThemePanel builds the Themes tab.
func NewThemePanel() *ThemePanel {
	p := &ThemePanel{selected: -1}
	p.AsControl().SetClipContents(true)
	p.split = NewSplitter(SplitHorizontal, LibraryPercent, 150, 150, 0, 0)
	p.split.AsControl().SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	add(p, p.split.AsControl())

	// Left: the library selector (one collection).
	p.library = Tree.New()
	p.library.AsControl().SetThemeTypeVariation(vLibraryTree)
	p.library.SetHideRoot(true)
	p.library.SetColumns(1)
	onThemeReady(p.library.AsControl(), func() { p.builtLib = false; p.render() })
	left := libraryPane(libraryTitle("Themes"), p.library.AsControl(), p.libraryFooter())

	// Right: the thumbnail grid.
	p.grid = NewSlideGrid()
	p.grid.DragSource = uicore.DragTheme
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
	p.menu = p.buildMenu(false)
	p.grid.AsNode().AddChild(p.menu.AsNode())

	// Footer: [+] | [⚙▾]   count   [▤|▾]
	p.count = label("", vCaption)
	p.count.SetHorizontalAlignment(GUI.HorizontalAlignmentCenter)
	p.count.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	plus := newMiniButton(miniPlain, "plus", 28, 24, "New Theme")
	plus.AsBaseButton().OnPressed(func() {
		if p.OnNew != nil {
			p.OnNew()
		}
	})
	gear := newMiniButton(miniDropdown, "gear", 40, 24, "Theme Actions")
	p.gearMenu = p.buildMenu(true)
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

// buildMenu builds the theme actions menu; withNew adds "New Theme…" (the
// footer gear has it, the tile context menu does not).
func (p *ThemePanel) buildMenu(withNew bool) PopupMenu.Instance {
	m := PopupMenu.New()
	if withNew {
		m.MoreArgs().AddItem("New Theme…", themeMenuNew, 0)
	}
	m.MoreArgs().AddItem("Edit Theme…", themeMenuEdit, 0)
	m.MoreArgs().AddItem("Duplicate Theme", themeMenuDuplicate, 0)
	m.MoreArgs().AddItem("Delete Theme", themeMenuDelete, 0)
	m.AddSeparator()
	m.MoreArgs().AddItem("Export Theme…", themeMenuExport, 0)
	m.MoreArgs().AddItem("Import Theme…", themeMenuImport, 0)
	m.AddSeparator()
	m.MoreArgs().AddItem("Set as Songs Default", themeMenuSongDefault, 0)
	m.MoreArgs().AddItem("Set as Scripture Default", themeMenuScriptureDefault, 0)
	m.AddSeparator()
	m.MoreArgs().AddItem("Apply to Schedule Item", themeMenuApply, 0)
	m.OnIdPressed(p.menuPressed)
	return m
}

// libraryTitle is the strip at the top of the library pane (the Themes tab
// has no search locator): the collection's title on the search strip panel.
func libraryTitle(text string) Control.Instance {
	strip := panel(vSearchPanel)
	strip.AsControl().SetCustomMinimumSize(Vector2.New(0, 24))
	l := label(text, vCaption)
	add(strip, l.AsControl())
	return strip.AsControl()
}

// libraryFooter is the library pane's footer: [+] (new theme) | [⚙▾].
func (p *ThemePanel) libraryFooter() Control.Instance {
	footer := panel(vMiniBarTL)
	footer.AsControl().SetCustomMinimumSize(Vector2.New(0, 28))
	row := hbox(0)
	plus := newMiniButton(miniPlain, "plus", 28, 24, "New Theme")
	plus.AsBaseButton().OnPressed(func() {
		if p.OnNew != nil {
			p.OnNew()
		}
	})
	add(row, plus.AsControl(), withMargins(newDivider(true).AsControl(), 2, 2),
		newMiniButton(miniDropdown, "gear", 40, 24, "Collection Actions").AsControl())
	add(footer, row.AsControl())
	return footer.AsControl()
}

// Splitter returns the library | grid splitter.
func (p *ThemePanel) Splitter() *Splitter { return p.split }

// Grid returns the thumbnail grid (keyboard focus lives there).
func (p *ThemePanel) Grid() *SlideGrid { return p.grid }

// SetItems replaces the tiles (and rebuilds their thumbnails). The selection
// is cleared.
func (p *ThemePanel) SetItems(rows []ThemeRow) {
	p.rows = append([]ThemeRow(nil), rows...)
	p.selected = -1
	items := make([]SlideGridItem, len(rows))
	for i, r := range rows {
		items[i] = SlideGridItem{Label: r.Name, Badge: r.Badge}
	}
	p.grid.SetItems(items)
	p.render()
}

// SetSelected selects tile i (-1 for none) without calling OnSelect.
func (p *ThemePanel) SetSelected(i int) {
	p.selected = i
	p.grid.SetSelected(i)
	p.render()
}

// SetCountText sets the footer status text ("3 themes").
func (p *ThemePanel) SetCountText(text string) {
	p.countText = text
	p.render()
}

// SetApplyTarget names the schedule item "Apply to Schedule Item" acts on
// ("" disables it).
func (p *ThemePanel) SetApplyTarget(title string) {
	p.applyTarget = title
	p.render()
}

// SetThumbnailFactory sets the grid's thumbnail factory and rebuilds the tiles.
func (p *ThemePanel) SetThumbnailFactory(f func(i int) Control.Instance) {
	p.grid.SetThumbnailFactory(f)
}

// render projects the panel state. Its programmatic Tree changes run quietly.
func (p *ThemePanel) render() {
	quietly(p.library.AsControl(), func() {
		if !p.builtLib {
			p.builtLib = true
			p.library.Clear()
			root := p.library.CreateItem()
			it := root.CreateChild()
			it.SetText(0, "All Themes")
			it.SetIcon(0, fwIcon(p.library.AsControl(), "theme"))
			it.SetIconMaxWidth(0, 16)
		}
		selectTreeRow(p.library, 0)
	})
	p.count.SetText(p.countText)
	for _, m := range []PopupMenu.Instance{p.menu, p.gearMenu} {
		if idx := m.GetItemIndex(themeMenuApply); idx >= 0 {
			text := "Apply to Schedule Item"
			if p.applyTarget != "" {
				text = "Apply to “" + p.applyTarget + "”"
			}
			m.SetItemText(idx, text)
		}
	}
}

func (p *ThemePanel) enableMenu(m PopupMenu.Instance) {
	none := p.selected < 0 || p.selected >= len(p.rows)
	for idx := 0; idx < m.ItemCount(); idx++ {
		switch m.GetItemId(idx) {
		case themeMenuEdit, themeMenuDuplicate, themeMenuDelete, themeMenuSongDefault, themeMenuScriptureDefault, themeMenuExport:
			m.SetItemDisabled(idx, none)
		case themeMenuApply:
			m.SetItemDisabled(idx, none || p.applyTarget == "")
		}
	}
}

func (p *ThemePanel) menuPressed(id int) {
	i := p.selected
	call := func(f func(int)) {
		if f != nil && i >= 0 && i < len(p.rows) {
			f(i)
		}
	}
	switch id {
	case themeMenuNew:
		if p.OnNew != nil {
			p.OnNew()
		}
	case themeMenuEdit:
		call(p.OnEdit)
	case themeMenuDuplicate:
		call(p.OnDuplicate)
	case themeMenuDelete:
		call(p.OnDelete)
	case themeMenuSongDefault:
		call(func(i int) { p.setDefault(i, models.ItemSong) })
	case themeMenuScriptureDefault:
		call(func(i int) { p.setDefault(i, models.ItemScripture) })
	case themeMenuApply:
		call(p.OnApply)
	case themeMenuExport:
		call(p.OnExport)
	case themeMenuImport:
		if p.OnImport != nil {
			p.OnImport()
		}
	}
}

func (p *ThemePanel) setDefault(i int, kind models.ItemKind) {
	if p.OnSetDefault != nil {
		p.OnSetDefault(i, kind)
	}
}
