package app

import (
	"path/filepath"

	"freeworship/internal/app/core"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/DisplayServer"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEvent"
	"graphics.gd/classdb/InputEventKey"
	"graphics.gd/classdb/LineEdit"
	"graphics.gd/classdb/TextEdit"
	"graphics.gd/classdb/Tree"
	"graphics.gd/classdb/Viewport"
	"graphics.gd/variant/Object"
)

// Input sees key presses before the GUI does (Node._input), so hotkeys work
// wherever focus is, except where core.Hotkeys says the focused widget keeps
// the key (text fields, list navigation).
func (a *App) Input(event InputEvent.Instance) {
	k, ok := Object.As[InputEventKey.Instance](event)
	if !ok || !k.AsInputEvent().IsPressed() || a.quitting {
		return
	}
	name := keyName(k.Keycode())
	if name == "" {
		name = keyName(k.PhysicalKeycode())
	}
	if name == "" {
		return
	}
	mods := k.AsInputEventWithModifiers()
	ev := core.KeyEvent{
		Key:   name,
		Ctrl:  mods.CtrlPressed(),
		Meta:  mods.MetaPressed(),
		Shift: mods.ShiftPressed(),
		Alt:   mods.AltPressed(),
		Echo:  k.AsInputEvent().IsEcho(),
	}
	if a.handleKey(ev) {
		// graphics.gd buffers void engine calls and does not flush them when
		// _input returns; read the flag back so the "handled" mark reaches
		// Godot before it hands the key to the focused widget.
		vp := Viewport.Get(a.AsNode())
		vp.SetInputAsHandled()
		_ = vp.IsInputHandled()
	}
}

// handleKey runs a key press through the hotkey state machine. Real key
// events and the control server's `key` action both come through here. It
// reports whether the key was consumed.
func (a *App) handleKey(ev core.KeyEvent) bool {
	var consumed bool
	a.event(func() {
		cmd, eaten := a.keys.Handle(ev, a.focus())
		consumed = eaten
		if cmd.Action != core.ActNone {
			a.execHotkey(cmd)
		}
	})
	return consumed
}

// focus classifies the keyboard focus for the hotkey state machine.
func (a *App) focus() core.Focus {
	if a.win.Editor.Visible() || a.win.Confirm.AsWindow().Visible() {
		return core.FocusDialog
	}
	owner := Viewport.Get(a.AsNode()).GuiGetFocusOwner()
	if owner == (Control.Instance{}) {
		return core.FocusNone
	}
	if _, ok := Object.As[LineEdit.Instance](owner); ok {
		return core.FocusText
	}
	if _, ok := Object.As[TextEdit.Instance](owner); ok {
		return core.FocusText
	}
	if _, ok := Object.As[Tree.Instance](owner); ok {
		return core.FocusList
	}
	switch owner.ID() {
	case a.win.Schedule.AsControl().ID():
		return core.FocusList
	case a.win.PreviewGrid.AsControl().ID():
		return core.FocusGrid
	}
	return core.FocusNone
}

// namedKeys maps Godot keys to core key names.
var namedKeys = map[Input.Key]string{
	Input.KeyRight: "right", Input.KeyLeft: "left", Input.KeyUp: "up", Input.KeyDown: "down",
	Input.KeyHome: "home", Input.KeyEnd: "end", Input.KeyPageup: "pageup", Input.KeyPagedown: "pagedown",
	Input.KeySpace: "space", Input.KeyBackspace: "backspace", Input.KeyEnter: "enter", Input.KeyKpEnter: "enter",
	Input.KeyDelete: "delete", Input.KeyEscape: "escape", Input.KeyTab: "tab",
}

// keyName converts a Godot keycode to a core key name ("" if not relevant).
func keyName(k Input.Key) string {
	switch {
	case k >= Input.KeyA && k <= Input.KeyZ:
		return string(rune('a' + (k - Input.KeyA)))
	case k >= Input.Key0 && k <= Input.Key9:
		return string(rune('0' + (k - Input.Key0)))
	case k >= Input.KeyKp0 && k <= Input.KeyKp9:
		return string(rune('0' + (k - Input.KeyKp0)))
	}
	return namedKeys[k]
}

// ---- native file dialogs -----------------------------------------------------------

var scheduleFilters = []string{"*.fws ; FreeWorship Schedules"}

// openScheduleDialog shows the native Open dialog for .fws files.
func (a *App) openScheduleDialog() {
	err := DisplayServer.FileDialogShow("Open Schedule", a.dialogDir(), "", false, DisplayServer.FileDialogModeOpenFile,
		scheduleFilters, func(ok bool, paths []string, _ int) {
			if !ok || len(paths) == 0 {
				return
			}
			a.event(func() { _ = a.openSchedule(paths[0]) })
		}, 0)
	if err != nil {
		a.fail("open dialog", err)
	}
}

// saveScheduleInteractive saves to the schedule's path, or asks for one.
func (a *App) saveScheduleInteractive() {
	if a.sched.Path != "" {
		_ = a.saveSchedule(a.sched.Path)
		return
	}
	a.saveScheduleAsDialog()
}

// saveScheduleAsDialog shows the native Save dialog.
func (a *App) saveScheduleAsDialog() {
	name := a.sched.Title
	if name == "" {
		name = "Untitled"
	}
	err := DisplayServer.FileDialogShow("Save Schedule", a.dialogDir(), core.WithScheduleExt(name), false, DisplayServer.FileDialogModeSaveFile,
		scheduleFilters, func(ok bool, paths []string, _ int) {
			if !ok || len(paths) == 0 {
				return
			}
			a.event(func() { _ = a.saveSchedule(paths[0]) })
		}, 0)
	if err != nil {
		a.fail("save dialog", err)
	}
}

// dialogDir is the folder file dialogs start in.
func (a *App) dialogDir() string {
	if a.sched.Path != "" {
		return filepath.Dir(a.sched.Path)
	}
	return ""
}
