package ui

import (
	"path/filepath"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/MenuBar"
	"graphics.gd/classdb/PopupMenu"
)

// MenuAction identifies a menu item (UI-SPEC §4, the minimal set).
type MenuAction int

const (
	MenuNewSchedule MenuAction = iota
	MenuNewSong
	MenuOpenSchedule
	MenuSaveSchedule
	MenuSaveScheduleAs
	MenuQuit
	MenuOptions
	MenuInterfaceDark
	MenuInterfaceLight
	MenuAbout
	menuActionCount
)

var menuActionNames = [...]string{"new-schedule", "new-song", "open-schedule", "save-schedule", "save-schedule-as",
	"quit", "options", "interface-dark", "interface-light", "about"}

// String is the action's control-API name ("open-schedule").
func (a MenuAction) String() string {
	if a < 0 || a >= menuActionCount {
		return "unknown"
	}
	return menuActionNames[a]
}

// ParseMenuAction finds an action by its name.
func ParseMenuAction(name string) (MenuAction, bool) {
	for i, n := range menuActionNames {
		if n == name {
			return MenuAction(i), true
		}
	}
	return 0, false
}

// Menus is the menu bar: File, Edit, View and Help (UI-SPEC §4). On macOS
// Godot's MenuBar puts them in the system menu bar (prefer_global_menu) and
// takes no space in the window; elsewhere it is a row above the toolbar. It is
// a dumb view: items report through OnAction / OnOpenRecent, and SetRecent /
// SetInterfaceColor project the app's state back.
type Menus struct {
	OnAction     func(MenuAction)
	OnOpenRecent func(path string)

	bar    MenuBar.Instance
	recent PopupMenu.Instance // File ▸ Open Recent
	color  PopupMenu.Instance // View ▸ Interface Color
	paths  []string
	dark   bool
}

// NewMenus builds the menus.
func NewMenus() *Menus {
	m := &Menus{dark: true}
	m.bar = MenuBar.New()
	m.bar.SetPreferGlobalMenu(true)
	m.bar.SetFlat(true)
	cmd := func(k Input.Key) Input.Key { return Input.Key(Input.KeyMaskCmdOrCtrl) | k }
	fire := func(id int) {
		if id < int(menuActionCount) && m.OnAction != nil {
			m.OnAction(MenuAction(id))
		}
	}

	file := PopupMenu.New()
	file.AsNode().SetName("File")
	file.MoreArgs().AddItem("New Schedule", int(MenuNewSchedule), cmd(Input.KeyN))
	file.MoreArgs().AddItem("New Song", int(MenuNewSong), 0)
	file.AddSeparator()
	file.MoreArgs().AddItem("Open Schedule…", int(MenuOpenSchedule), cmd(Input.KeyO))
	m.recent = PopupMenu.New()
	m.recent.AsNode().SetName("OpenRecent")
	m.recent.OnIndexPressed(func(i int) {
		if i >= 0 && i < len(m.paths) && m.OnOpenRecent != nil {
			m.OnOpenRecent(m.paths[i])
		}
	})
	// Submenus are children of their menu and are referenced by node name:
	// graphics.gd treats the node argument of add_submenu_node_item as an
	// ownership transfer, which a node already in the tree cannot do.
	file.AsNode().AddChild(m.recent.AsNode())
	file.MoreArgs().AddSubmenuItem("Open Recent", "OpenRecent", 1000)
	file.AddSeparator()
	file.MoreArgs().AddItem("Save Schedule", int(MenuSaveSchedule), cmd(Input.KeyS))
	file.MoreArgs().AddItem("Save Schedule As…", int(MenuSaveScheduleAs), Input.Key(Input.KeyMaskShift)|cmd(Input.KeyS))
	file.AddSeparator()
	file.MoreArgs().AddItem("Quit FreeBeams", int(MenuQuit), cmd(Input.KeyQ))
	file.OnIdPressed(fire)

	edit := PopupMenu.New()
	edit.AsNode().SetName("Edit")
	edit.MoreArgs().AddItem("Options…", int(MenuOptions), cmd(Input.KeyComma))
	edit.OnIdPressed(fire)

	view := PopupMenu.New()
	view.AsNode().SetName("View")
	m.color = PopupMenu.New()
	m.color.AsNode().SetName("InterfaceColor")
	m.color.MoreArgs().AddRadioCheckItem("Dark", int(MenuInterfaceDark), 0)
	m.color.MoreArgs().AddRadioCheckItem("Light", int(MenuInterfaceLight), 0)
	m.color.OnIdPressed(fire)
	view.AsNode().AddChild(m.color.AsNode())
	view.MoreArgs().AddSubmenuItem("Interface Color", "InterfaceColor", 1001)

	help := PopupMenu.New()
	help.AsNode().SetName("Help")
	help.MoreArgs().AddItem("About FreeBeams", int(MenuAbout), 0)
	help.OnIdPressed(fire)

	for _, p := range []PopupMenu.Instance{file, edit, view, help} {
		m.bar.AsNode().AddChild(p.AsNode())
	}
	m.render()
	return m
}

// AsControl returns the menu bar control (zero-height when the system menu
// bar is used).
func (m *Menus) AsControl() Control.Instance { return m.bar.AsControl() }

// Native reports whether the menus are in the system menu bar.
func (m *Menus) Native() bool { return m.bar.IsNativeMenu() }

// SetRecent lists the recent schedule files (newest first) in Open Recent.
func (m *Menus) SetRecent(paths []string) {
	m.paths = append(m.paths[:0:0], paths...)
	m.render()
}

// SetInterfaceColor marks the active interface color.
func (m *Menus) SetInterfaceColor(dark bool) {
	m.dark = dark
	m.render()
}

func (m *Menus) render() {
	m.recent.Clear()
	if len(m.paths) == 0 {
		m.recent.MoreArgs().AddItem("No Recent Schedules", 0, 0)
		m.recent.SetItemDisabled(0, true)
	}
	for i, p := range m.paths {
		m.recent.MoreArgs().AddItem(filepath.Base(p), i, 0)
		m.recent.SetItemTooltip(i, p)
	}
	m.color.SetItemChecked(m.color.GetItemIndex(int(MenuInterfaceDark)), m.dark)
	m.color.SetItemChecked(m.color.GetItemIndex(int(MenuInterfaceLight)), !m.dark)
}
