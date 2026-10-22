package ui

import "graphics.gd/classdb/PopupMenu"

// LiveMenuState is what the Live menu's check marks show.
type LiveMenuState struct {
	Logo, Black, Clear bool // the output mode
	Live               bool // the output is on (Show Live)
	Stage              bool // the stage display is on
}

// buildLive builds the Live menu (UI-SPEC §4: Go Live · Go Back · — · Show
// Logo · Black Output · Clear Output · — · Show Live), plus Stage Display
// (EasyWorship's Foldback). No accelerators: the hotkeys (Ctrl+B, Ctrl+L, …)
// are the app's own, handled wherever focus is, and a native menu shortcut
// would take them first.
func (m *Menus) buildLive(fire func(id int)) PopupMenu.Instance {
	live := PopupMenu.New()
	live.AsNode().SetName("Live")
	live.MoreArgs().AddItem("Go Live", int(MenuGoLive), 0)
	live.MoreArgs().AddItem("Go Back", int(MenuGoBack), 0)
	live.AddSeparator()
	live.MoreArgs().AddCheckItem("Show Logo", int(MenuLogo), 0)
	live.MoreArgs().AddCheckItem("Black Output", int(MenuBlack), 0)
	live.MoreArgs().AddCheckItem("Clear Output", int(MenuClear), 0)
	live.AddSeparator()
	live.MoreArgs().AddCheckItem("Show Live", int(MenuShowLive), 0)
	live.MoreArgs().AddCheckItem("Stage Display", int(MenuStageDisplay), 0)
	live.OnIdPressed(fire)
	m.live = live
	return live
}

// SetLiveState projects the output mode, the output and the stage display
// onto the Live menu's check marks.
func (m *Menus) SetLiveState(s LiveMenuState) {
	if s == m.liveState {
		return
	}
	m.liveState = s
	m.renderLive()
}

// LiveState returns what the Live menu shows.
func (m *Menus) LiveState() LiveMenuState { return m.liveState }

func (m *Menus) renderLive() {
	if m.live == PopupMenu.Nil {
		return
	}
	s := m.liveState
	for id, on := range map[MenuAction]bool{MenuLogo: s.Logo, MenuBlack: s.Black, MenuClear: s.Clear,
		MenuShowLive: s.Live, MenuStageDisplay: s.Stage} {
		if i := m.live.GetItemIndex(int(id)); i >= 0 {
			m.live.SetItemChecked(i, on)
		}
	}
}
