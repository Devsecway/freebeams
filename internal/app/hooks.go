package app

import (
	"errors"
	"fmt"
	"strings"

	"freeworship/internal/app/core"
	"freeworship/internal/control"
	"freeworship/internal/models"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEventKey"
	"graphics.gd/classdb/LineEdit"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/classdb/Tree"
	"graphics.gd/classdb/Viewport"
	"graphics.gd/variant/Object"
)

// Control actions that drive the real UI paths for automated tests: native
// key events (through Godot's input pipeline, focus and all), keyboard focus,
// the song editor and the window close request.

// handleHook runs the UI-driving actions; false when the action is unknown.
func (a *App) handleHook(cmd *control.Command, args actionArgs) bool {
	switch cmd.Action {
	case "native-key":
		a.nativeKey(cmd, args)
	case "focus":
		a.answer(cmd, a.focusTarget(args.Target))
	case "editor":
		switch args.Mode {
		case "new":
			a.newSong()
		case "edit":
			if a.song == nil {
				cmd.RespondErr(errors.New("editor: no song selected"))
				return true
			}
			a.useSong(*a.song, songEdit)
		case "close":
			a.win.Editor.Close()
		default:
			cmd.RespondErr(fmt.Errorf("editor: mode must be new, edit or close"))
			return true
		}
		a.answer(cmd, nil)
	case "save-song":
		// The same path as the editor's OK button (SongEditor.OnSave).
		if strings.TrimSpace(args.Title) == "" {
			cmd.RespondErr(errors.New("save-song: title is required"))
			return true
		}
		song := models.Song{ID: args.ID, Title: args.Title, Author: args.Author, Copyright: args.Copyright, CCLI: args.CCLI}
		a.win.Editor.Close()
		a.saveSong(song, args.Lyrics, cmd)
	case "delete-song":
		if args.Index == nil || *args.Index < 0 || *args.Index >= len(a.songs) {
			cmd.RespondErr(errArgs)
			return true
		}
		if !args.Confirm {
			a.deleteSong(*args.Index) // asks first, like the UI
			a.answer(cmd, nil)
			return true
		}
		a.doDeleteSong(a.songs[*args.Index], cmd)
	case "dismiss":
		// Cancel any open dialog (the Cancel button / Esc).
		a.win.Editor.Close()
		a.win.Confirm.AsWindow().Hide()
		a.answer(cmd, nil)
	case "close":
		// What the window manager sends when the operator closes the window.
		root := SceneTree.Get(a.AsNode()).Root()
		// The notification may reach App.Notification after this returns, so
		// callers check the outcome with wait-idle (focus "dialog") or by the
		// process exiting.
		root.AsNode().PropagateNotification(int(Node.NotificationWmCloseRequest))
		a.answer(cmd, nil)
	default:
		return false
	}
	return true
}

// nativeKeyFrames is how many frames native-key waits before replying: the
// parsed events are dispatched at the start of the next frame.
const nativeKeyFrames = 2

// keyCodes maps core key names back to Godot keys (for native-key).
func keyCode(name string) (Input.Key, bool) {
	if len(name) == 1 {
		c := name[0]
		switch {
		case c >= 'a' && c <= 'z':
			return Input.KeyA + Input.Key(c-'a'), true
		case c >= '0' && c <= '9':
			return Input.Key0 + Input.Key(c-'0'), true
		}
	}
	for k, n := range namedKeys {
		if n == name && k != Input.KeyKpEnter {
			return k, true
		}
	}
	return 0, false
}

// nativeKey pushes a real InputEventKey (press + release) through Godot's
// input pipeline, so it reaches App.Input exactly like a typed key and the
// focused widget gets it when the hotkeys do not take it.
func (a *App) nativeKey(cmd *control.Command, args actionArgs) {
	name := core.NormalizeKeyName(args.Name)
	code, ok := keyCode(name)
	if !ok {
		cmd.RespondErr(fmt.Errorf("native-key: unknown key %q", args.Name))
		return
	}
	for _, pressed := range []bool{true, false} {
		ev := InputEventKey.New()
		ev.SetKeycode(code)
		ev.SetPhysicalKeycode(code)
		ev.SetPressed(pressed)
		mods := ev.AsInputEventWithModifiers()
		mods.SetCtrlPressed(args.Ctrl)
		mods.SetMetaPressed(args.Meta)
		mods.SetShiftPressed(args.Shift)
		mods.SetAltPressed(args.Alt)
		if pressed && len(name) == 1 && !args.Ctrl && !args.Meta && !args.Alt {
			r := rune(name[0])
			if args.Shift && r >= 'a' && r <= 'z' {
				r -= 'a' - 'A'
			}
			ev.SetUnicode(int(r)) // what a typed key carries, so text fields insert it
		}
		Input.ParseInputEvent(ev.AsInputEvent())
	}
	// Godot dispatches parsed events at the start of the next frame. Do not
	// flush them here: dispatching from inside Process re-enters Go, and
	// graphics.gd then runs the widgets' GuiInput too late for AcceptEvent to
	// stop focus navigation. Reply once the events have been processed.
	a.after(nativeKeyFrames, func() { a.answer(cmd, nil) })
}

// focusTarget gives keyboard focus to a named widget ("none" releases it).
func (a *App) focusTarget(target string) error {
	var c Control.Instance
	switch target {
	case "none", "":
		Viewport.Get(a.AsNode()).GuiReleaseFocus()
		return nil
	case "song-search":
		c = findControl(a.win.Songs.AsControl().AsNode(), isLineEdit)
	case "reference":
		c = findControl(a.win.Scriptures.AsControl().AsNode(), isLineEdit)
	case "song-list":
		c = findControl(a.win.Songs.AsControl().AsNode(), isSongTable)
	case "schedule":
		c = a.win.Schedule.AsControl()
	case "preview-grid":
		c = a.win.PreviewGrid.AsControl()
	case "live-grid":
		c = a.win.LiveGrid.AsControl()
	default:
		return fmt.Errorf("focus: unknown target %q", target)
	}
	if c == (Control.Instance{}) {
		return fmt.Errorf("focus: %s not found", target)
	}
	c.GrabFocus()
	return nil
}

// findControl returns the first descendant of n (depth first) that match
// accepts.
func findControl(n Node.Instance, match func(Node.Instance) (Control.Instance, bool)) Control.Instance {
	for _, child := range n.GetChildren() {
		if c, ok := match(child); ok {
			return c
		}
		if c := findControl(child, match); c != (Control.Instance{}) {
			return c
		}
	}
	return Control.Instance{}
}

func isLineEdit(n Node.Instance) (Control.Instance, bool) {
	if e, ok := Object.As[LineEdit.Instance](n); ok {
		return e.AsControl(), true
	}
	return Control.Instance{}, false
}

// isSongTable matches the 4-column song list (not the library tree).
func isSongTable(n Node.Instance) (Control.Instance, bool) {
	if t, ok := Object.As[Tree.Instance](n); ok && t.Columns() == 4 {
		return t.AsControl(), true
	}
	return Control.Instance{}, false
}
