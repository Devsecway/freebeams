package ui

import (
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/PanelContainer"
	"graphics.gd/classdb/PopupMenu"
	"graphics.gd/variant/Vector2"
)

// ToolbarAction identifies a main-toolbar command (ToolBar_Main).
type ToolbarAction int

const (
	ActionNew ToolbarAction = iota // New ▾ (opens the New menu; see ActionNewSchedule/ActionNewSong)
	ActionOpen
	ActionSave
	ActionWeb
	ActionRemote
	ActionGoLive
	ActionAlerts
	ActionLogo  // toggle (StaysSelected)
	ActionBlack // toggle
	ActionClear // toggle
	ActionLive  // toggle: output on / off
	ActionNewSchedule
	ActionNewSong
	actionCount
)

func (a ToolbarAction) String() string {
	return [...]string{"New", "Open", "Save", "Web", "Remote", "Go Live", "Alerts", "Logo", "Black", "Clear", "Live", "New Schedule", "New Song"}[a]
}

// MainToolbar is the 58px main toolbar: 56 x 54 icon-over-label buttons and
// etched 2 x 50 dividers on the `toolbar` gradient (UI-SPEC §3.1).
type MainToolbar struct {
	// OnAction reports a button press. Toggle buttons report the click; the app
	// owns the toggled state and reflects it back with SetToggled.
	OnAction func(ToolbarAction)

	root    PanelContainer.Instance
	buttons map[ToolbarAction]*toolButton
	newMenu PopupMenu.Instance

	toggled   map[ToolbarAction]bool
	disabled  map[ToolbarAction]bool
	rendering bool
}

type toolSpec struct {
	action   ToolbarAction
	text     string
	icon     string
	dropdown bool
	toggle   bool
}

// NewMainToolbar builds the toolbar.
func NewMainToolbar() *MainToolbar {
	t := &MainToolbar{
		buttons:  map[ToolbarAction]*toolButton{},
		toggled:  map[ToolbarAction]bool{},
		disabled: map[ToolbarAction]bool{},
	}
	t.root = panel(vToolbar)
	t.root.AsControl().SetCustomMinimumSize(Vector2.New(0, 58))
	row := hbox(0)
	add(t.root, row.AsControl())

	mk := func(s toolSpec) Control.Instance {
		b := newToolButton(s.text, s.icon, s.dropdown, s.toggle)
		t.buttons[s.action] = b
		a := s.action
		b.AsBaseButton().OnPressed(func() { t.pressed(a) })
		return b.AsControl()
	}
	div := func() Control.Instance { return withMargins(newDivider(false).AsControl(), 4, 4) }

	// Left-aligned: New ▾, Open ▾, Save, Web | Remote ▾
	add(row,
		mk(toolSpec{ActionNew, "New", "new", true, false}),
		mk(toolSpec{ActionOpen, "Open", "open", true, false}),
		mk(toolSpec{ActionSave, "Save", "save", false, false}),
		mk(toolSpec{ActionWeb, "Web", "web", false, false}),
		div(),
		mk(toolSpec{ActionRemote, "Remote", "remote", true, false}),
		spacer(),
		// Right-aligned: Go Live | Alerts ▾ | Logo, Black, Clear | Live
		mk(toolSpec{ActionGoLive, "Go Live", "golive", false, false}),
		div(),
		mk(toolSpec{ActionAlerts, "Alerts", "alerts", true, false}),
		div(),
		mk(toolSpec{ActionLogo, "Logo", "logo", false, true}),
		mk(toolSpec{ActionBlack, "Black", "black", false, true}),
		mk(toolSpec{ActionClear, "Clear", "clear", false, true}),
		div(),
		mk(toolSpec{ActionLive, "Live", "live", false, true}),
	)

	// New ▾ menu (PopupMenu_New): Schedule, Song, —, Song from SongSelect.
	t.newMenu = PopupMenu.New()
	t.newMenu.MoreArgs().AddItem("Schedule", int(ActionNewSchedule), 0)
	t.newMenu.MoreArgs().AddItem("Song", int(ActionNewSong), 0)
	t.newMenu.MoreArgs().AddItem("Presentation", 1000, 0)
	t.newMenu.SetItemDisabled(2, true)
	t.newMenu.AddSeparator()
	t.newMenu.MoreArgs().AddItem("Song from SongSelect", 1001, 0)
	t.newMenu.SetItemDisabled(4, true)
	t.newMenu.OnIdPressed(func(id int) {
		if id < int(actionCount) && t.OnAction != nil {
			t.OnAction(ToolbarAction(id))
		}
	})
	t.root.AsNode().AddChild(t.newMenu.AsNode())
	t.render()
	return t
}

// AsControl returns the toolbar's root control.
func (t *MainToolbar) AsControl() Control.Instance { return t.root.AsControl() }

// SetToggled sets a toggle button's selected state (Logo, Black, Clear, Live).
func (t *MainToolbar) SetToggled(a ToolbarAction, on bool) {
	t.toggled[a] = on
	t.render()
}

// SetEnabled enables or disables a button.
func (t *MainToolbar) SetEnabled(a ToolbarAction, on bool) {
	t.disabled[a] = !on
	t.render()
}

func (t *MainToolbar) render() {
	t.rendering = true
	defer func() { t.rendering = false }()
	for a, b := range t.buttons {
		bb := b.AsBaseButton()
		bb.SetDisabled(t.disabled[a])
		if bb.ToggleMode() {
			bb.SetPressedNoSignal(t.toggled[a])
		}
		b.AsCanvasItem().QueueRedraw()
	}
}

func (t *MainToolbar) pressed(a ToolbarAction) {
	if t.rendering {
		return
	}
	// The app owns toggle state: undo Godot's own toggle until SetToggled.
	t.render()
	if a == ActionNew {
		t.popupNew()
		return
	}
	if t.OnAction != nil {
		t.OnAction(a)
	}
}

func (t *MainToolbar) popupNew() {
	popupBelow(t.newMenu, t.buttons[ActionNew].AsControl())
}
