package ui

import (
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/GUI"
	"graphics.gd/classdb/GridContainer"
	"graphics.gd/classdb/OptionButton"
	"graphics.gd/classdb/PopupMenu"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Rect2i"
	"graphics.gd/variant/Vector2"
	"graphics.gd/variant/Vector2i"
)

// Per-song themes: the song editor's Theme picker and the Songs list's
// Theme ▸ submenu. Both offer "(Default)" (ThemeID 0: the default song theme)
// followed by the app's themes, and both are projections: the app supplies
// the choices (SetThemeChoices) and the song's theme id, and a pick only
// reports the chosen id.

// DefaultThemeEntry is the first entry of the theme pickers: the song has no
// theme of its own and uses the default song theme.
const DefaultThemeEntry = "(Default)"

// ThemeChoice is one theme a song can use.
type ThemeChoice struct {
	ID   int64
	Name string
}

// themeEntry is the picker index of themeID: 0 for "(Default)" (also for an
// id that is not among the choices: a deleted theme), else k+1.
func themeEntry(choices []ThemeChoice, themeID int64) int {
	if themeID == 0 {
		return 0
	}
	for k, c := range choices {
		if c.ID == themeID {
			return k + 1
		}
	}
	return 0
}

// themeEntryID is the theme id of picker entry idx (0 for "(Default)").
func themeEntryID(choices []ThemeChoice, idx int) int64 {
	if idx <= 0 || idx > len(choices) {
		return 0
	}
	return choices[idx-1].ID
}

// ---- Song editor ------------------------------------------------------------

// addThemePicker adds the Theme row to the editor's field grid.
func (e *SongEditor) addThemePicker(grid GridContainer.Instance) {
	l := label("Theme", "")
	l.SetHorizontalAlignment(GUI.HorizontalAlignmentRight)
	l.AsControl().SetCustomMinimumSize(Vector2.New(84, 0))
	e.themeOpt = OptionButton.New()
	e.themeOpt.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	e.themeOpt.AsControl().SetCustomMinimumSize(Vector2.New(0, 26))
	e.themeOpt.AsControl().SetTooltipText("The theme this song is shown with. (Default) uses the default song theme.")
	e.themeOpt.AsNode().SetName("ThemePicker")
	e.themeOpt.OnItemSelected(func(idx int) { e.themeID = themeEntryID(e.themeChoices, idx) })
	add(grid, l.AsControl(), e.themeOpt.AsControl())
	e.themesDirty = true
}

// SetThemeChoices sets the themes the editor's Theme picker offers after
// "(Default)".
func (e *SongEditor) SetThemeChoices(choices []ThemeChoice) {
	e.themeChoices = append([]ThemeChoice(nil), choices...)
	e.themesDirty = true
	e.renderTheme()
}

// PickTheme chooses themeID in the picker as the operator would (0 =
// "(Default)"); OK then saves it.
func (e *SongEditor) PickTheme(themeID int64) {
	e.themeID = themeEntryID(e.themeChoices, themeEntry(e.themeChoices, themeID))
	e.renderTheme()
}

// ThemeName is the picker's current entry ("(Default)" or a theme name).
func (e *SongEditor) ThemeName() string {
	if i := themeEntry(e.themeChoices, e.themeID); i > 0 {
		return e.themeChoices[i-1].Name
	}
	return DefaultThemeEntry
}

// savedThemeID is the theme id OK saves: the picked theme, or 0 when it is no
// longer offered (deleted while the editor was open). Before the app has
// supplied any choices the song keeps its own id.
func (e *SongEditor) savedThemeID() int64 {
	if len(e.themeChoices) == 0 {
		return e.themeID
	}
	return themeEntryID(e.themeChoices, themeEntry(e.themeChoices, e.themeID))
}

// renderTheme projects the choices and the song's theme onto the picker
// (OptionButton.select does not emit item_selected), ending with a getter so
// the buffered calls apply now.
func (e *SongEditor) renderTheme() {
	if e.themeOpt == OptionButton.Nil {
		return
	}
	if e.themesDirty {
		e.themesDirty = false
		e.themeOpt.Clear()
		e.themeOpt.AddItem(DefaultThemeEntry)
		for _, c := range e.themeChoices {
			e.themeOpt.AddItem(c.Name)
		}
	}
	e.themeOpt.Select(themeEntry(e.themeChoices, e.themeID))
	_ = e.themeOpt.Selected()
}

// ---- Songs list -------------------------------------------------------------

// addThemeMenu adds the Theme ▸ submenu to the song context menu. Its entry
// has an explicit id (songMenuTheme): an id of -1 would make it its index.
func (s *SongLibraryPanel) addThemeMenu() {
	s.themeMenu = PopupMenu.New()
	s.themeMenu.OnIdPressed(func(id int) {
		if s.OnSetTheme == nil || s.selected < 0 || s.selected >= len(s.songs) || id < 0 || id > len(s.themeChoices) {
			return
		}
		s.OnSetTheme(s.selected, themeEntryID(s.themeChoices, id))
	})
	// Named and referenced by name: add_submenu_node_item trips graphics.gd's
	// ownership check.
	s.themeMenu.AsNode().SetName("SongThemeMenu")
	s.menu.AsNode().AddChild(s.themeMenu.AsNode())
	s.menu.AddSeparator()
	s.menu.MoreArgs().AddSubmenuItem("Theme", "SongThemeMenu", songMenuTheme)
}

// SetThemeChoices sets the themes the context menu's Theme ▸ submenu offers
// after "(Default)"; OnSetTheme reports the chosen theme's id.
func (s *SongLibraryPanel) SetThemeChoices(choices []ThemeChoice) {
	s.themeChoices = append([]ThemeChoice(nil), choices...)
}

// projectThemeMenu fills the Theme ▸ submenu for the selected song: one
// radio entry per choice, the song's own theme checked ("(Default)" when it
// has none or its theme is gone). Without a selection the entry is disabled.
func (s *SongLibraryPanel) projectThemeMenu() {
	if idx := s.menu.GetItemIndex(songMenuTheme); idx >= 0 {
		s.menu.SetItemDisabled(idx, s.selected < 0)
	}
	current := 0
	if s.selected >= 0 && s.selected < len(s.songs) {
		current = themeEntry(s.themeChoices, s.songs[s.selected].ThemeID)
	}
	s.themeMenu.Clear()
	s.themeMenu.MoreArgs().AddRadioCheckItem(DefaultThemeEntry, 0, 0)
	for k, c := range s.themeChoices {
		s.themeMenu.MoreArgs().AddRadioCheckItem(c.Name, k+1, 0)
	}
	for k := 0; k <= len(s.themeChoices); k++ {
		s.themeMenu.SetItemChecked(k, k == current)
	}
}

// OpenMenu shows the selected song's context menu at the mouse, projected as
// a right-click does (automation and screenshots).
func (s *SongLibraryPanel) OpenMenu() { s.showMenu() }

// OpenThemeMenu opens the Theme ▸ submenu beside the open context menu, where
// hovering its entry opens it.
func (s *SongLibraryPanel) OpenThemeMenu() {
	m := s.menu.AsWindow()
	n := s.menu.ItemCount()
	y := int(m.Position().Y)
	if idx := s.menu.GetItemIndex(songMenuTheme); idx >= 0 && n > 0 {
		y += int(m.Size().Y) * idx / n
	}
	s.themeMenu.AsWindow().ResetSize()
	s.themeMenu.AsWindow().MoreArgs().Popup(Rect2i.PositionSize{Position: Vector2i.New(int(m.Position().X+m.Size().X), y)})
}

// CloseMenus hides the context menu and its Theme ▸ submenu.
func (s *SongLibraryPanel) CloseMenus() {
	s.themeMenu.AsWindow().Hide()
	s.menu.AsWindow().Hide()
}

// MenuWindow and ThemeMenuWindow are the context menu's and the submenu's
// popup windows (for screenshots).
func (s *SongLibraryPanel) MenuWindow() Window.Instance { return s.menu.AsWindow() }

// ThemeMenuWindow is the Theme ▸ submenu's popup window.
func (s *SongLibraryPanel) ThemeMenuWindow() Window.Instance { return s.themeMenu.AsWindow() }

// ThemeMenuEntries lists the Theme ▸ submenu as last projected, with the
// checked entry (for tests and the control API).
func (s *SongLibraryPanel) ThemeMenuEntries() (names []string, checked int) {
	checked = -1
	for k := 0; k < s.themeMenu.ItemCount(); k++ {
		names = append(names, s.themeMenu.GetItemText(k))
		if s.themeMenu.IsItemChecked(k) {
			checked = k
		}
	}
	return names, checked
}
