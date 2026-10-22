package app

import (
	"errors"
	"fmt"
	"strings"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/models"
	"freebeams/internal/render"
	"freebeams/internal/ui"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEventKey"
	"graphics.gd/classdb/InputEventMouseButton"
	"graphics.gd/classdb/InputEventMouseMotion"
	"graphics.gd/classdb/LineEdit"
	"graphics.gd/classdb/Node"
	"graphics.gd/classdb/SceneTree"
	"graphics.gd/classdb/Tree"
	"graphics.gd/classdb/Viewport"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2"
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
			a.win.ThemeEditor.Close()
			a.th.editing = nil
		case "theme": // the theme editor on the selected theme
			if a.th.sel < 0 || a.th.sel >= len(a.th.list) {
				cmd.RespondErr(errors.New("editor: no theme selected"))
				return true
			}
			a.editTheme(a.th.sel)
		case "new-theme":
			a.newTheme()
		default:
			cmd.RespondErr(fmt.Errorf("editor: mode must be new, edit, theme, new-theme or close"))
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
		themeID, err := a.songThemeArg("save-song", args) // the editor's Theme picker
		if err != nil {
			cmd.RespondErr(err)
			return true
		}
		song.ThemeID = themeID
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
		// Cancel the topmost open dialog (its Cancel button / Esc; OK for an
		// error message), e.g. the quit question but not the editor under it.
		a.win.DismissDialog()
		a.answer(cmd, nil)
	case "accept":
		// Press OK on the topmost open dialog.
		if !a.win.AcceptDialog() {
			cmd.RespondErr(errors.New("accept: no dialog is open"))
			return true
		}
		a.answer(cmd, nil)
	case "resource-go-live":
		// The resource tab bar's Go Live button.
		a.resourceGoLive()
		a.answer(cmd, nil)
	case "click-schedule":
		// A left click on a schedule row, through Godot's input pipeline.
		if args.Index == nil {
			cmd.RespondErr(errArgs)
			return true
		}
		a.clickScheduleRow(cmd, *args.Index)
	case "click":
		// A real mouse click through Godot's input pipeline on a named
		// target: a verse row, a book suggestion or the Preview monitor, with
		// modifiers, a double-click or the right button.
		a.clickTarget(cmd, args)
	case "split":
		// Move a splitter, as a drag does (layout runs on the next frame). A
		// snapped-closed pane is reopened first, as a click on its grip does.
		sp := a.splitter(args.Name)
		if sp == nil {
			cmd.RespondErr(fmt.Errorf("split: unknown splitter %q", args.Name))
			return true
		}
		sp.SetCollapsed(ui.CollapseNone)
		sp.SetPercent(args.Percent)
		a.answer(cmd, nil)
	case "replay-input":
		// Re-inject a FREEBEAMS_INPUT_LOG recording (see inputrec.go).
		a.replayInput(cmd, args.Path)
	case "drag-split":
		// A real mouse drag on a splitter's handle, one motion per frame,
		// through Godot's input pipeline (press, Steps moves of Delta
		// pixels along the split axis, release).
		sp := a.splitter(args.Name)
		if sp == nil {
			cmd.RespondErr(fmt.Errorf("drag-split: unknown splitter %q", args.Name))
			return true
		}
		a.dragSplit(cmd, sp, args.Delta, args.Steps, max(args.Burst, 1))
	case "output-native":
		// What the OS says about the projector window (macOS): on screen, level,
		// hidden while the app is inactive. For the smoke test's app-switch check.
		st, ok := a.out.NativeState()
		if !ok {
			cmd.RespondErr(errors.New("output-native: the output is closed or not probeable here"))
			return true
		}
		a.replies = append(a.replies, func() {
			cmd.RespondJSON(struct {
				render.NativeWindowState
				State core.State `json:"state"`
			}{st, a.state()})
		})
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

// dragSplit presses on sp's handle, moves it by delta logical pixels per
// frame for steps frames along the split axis, then releases.
func (a *App) dragSplit(cmd *control.Command, sp *ui.Splitter, delta float64, steps, burst int) {
	at, ok := sp.HandleCenter()
	if !ok || steps <= 0 {
		cmd.RespondErr(errors.New("drag-split: handle not visible or steps <= 0"))
		return
	}
	scale := Window.Get(a.AsNode()).ContentScaleFactor()
	horizontal := sp.Orientation == ui.SplitHorizontal
	pos := func(k float64) Vector2.XY {
		d := float32(delta * k)
		p := at
		if horizontal {
			p.X += d
		} else {
			p.Y += d
		}
		return Vector2.New(p.X*scale, p.Y*scale)
	}
	button := func(pressed bool, p Vector2.XY) {
		ev := InputEventMouseButton.New()
		ev.SetButtonIndex(Input.MouseButtonLeft)
		ev.SetPressed(pressed)
		if pressed {
			ev.AsInputEventMouse().SetButtonMask(Input.MouseButtonMaskLeft)
		}
		ev.AsInputEventMouse().SetPosition(p)
		ev.AsInputEventMouse().SetGlobalPosition(p)
		Input.ParseInputEvent(ev.AsInputEvent())
	}
	button(true, pos(0))
	for k := 1; k <= steps; k++ {
		k := k
		a.after(k, func() {
			// A real trackpad delivers several motions per frame.
			for b := 1; b <= burst; b++ {
				p := pos(float64(k-1) + float64(b)/float64(burst))
				ev := InputEventMouseMotion.New()
				ev.AsInputEventMouse().SetButtonMask(Input.MouseButtonMaskLeft)
				ev.AsInputEventMouse().SetPosition(p)
				ev.AsInputEventMouse().SetGlobalPosition(p)
				Input.ParseInputEvent(ev.AsInputEvent())
			}
		})
	}
	a.after(steps+1, func() { button(false, pos(float64(steps))) })
	a.after(steps+1+nativeKeyFrames, func() { a.answer(cmd, nil) })
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
	window := 0 // the main window
	switch args.Window {
	case "", "main":
	case "output":
		// As if the (windowed, focusable) output window had keyboard focus.
		if window = a.out.WindowID(); window < 0 {
			cmd.RespondErr(errors.New("native-key: the output window is not open"))
			return
		}
	case "stage":
		// As if the windowed stage display had keyboard focus.
		if window = a.stage.win.WindowID(); window < 0 {
			cmd.RespondErr(errors.New("native-key: the stage display is not open"))
			return
		}
	case "settings":
		// Typing into the Options dialog (a hotkey capture).
		if !a.win.Settings.Visible() {
			cmd.RespondErr(errors.New("native-key: the Options dialog is not open"))
			return
		}
		window = a.win.Settings.AsWindow().GetWindowId()
	case "stage-message":
		// Typing into Live ▸ Stage Message… (its own OS window).
		if !a.win.StageMessage.Visible() {
			cmd.RespondErr(errors.New("native-key: the stage message dialog is not open"))
			return
		}
		window = a.win.StageMessage.AsWindow().GetWindowId()
	default:
		cmd.RespondErr(fmt.Errorf("native-key: unknown window %q (main|output|stage|stage-message|settings)", args.Window))
		return
	}
	for _, pressed := range []bool{true, false} {
		ev := InputEventKey.New()
		ev.AsInputEventFromWindow().SetWindowId(window)
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
	case "verse-list":
		c = findControl(a.win.Scriptures.AsControl().AsNode(), isRowTree)
	case "schedule":
		c = a.win.Schedule.AsControl()
	case "preview-grid":
		c = a.win.PreviewGrid.AsControl()
	case "live-grid":
		c = a.win.LiveGrid.AsControl()
	case "media-grid":
		c = a.win.Media.Grid().AsControl()
	case "presentation-grid":
		c = a.win.Presentations.Grid().AsControl()
	case "theme-grid":
		c = a.win.Themes.Grid().AsControl()
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

// isRowTree matches the multi-select verse list (not the Bibles).
func isRowTree(n Node.Instance) (Control.Instance, bool) {
	if t, ok := Object.As[Tree.Instance](n); ok && t.SelectMode() == Tree.SelectMulti {
		return t.AsControl(), true
	}
	return Control.Instance{}, false
}

// clickScheduleRow pushes a left click (press + release) at the centre of
// schedule row i through Godot's input pipeline, like the operator's mouse.
func (a *App) clickScheduleRow(cmd *control.Command, i int) {
	pos, ok := a.win.Schedule.RowCenter(i)
	if !ok {
		cmd.RespondErr(fmt.Errorf("click-schedule: no row %d", i))
		return
	}
	// Events are in window pixels; the GUI divides by the content scale.
	scale := Window.Get(a.AsNode()).ContentScaleFactor()
	pos = Vector2.New(pos.X*scale, pos.Y*scale)
	for _, pressed := range []bool{true, false} {
		ev := InputEventMouseButton.New()
		ev.SetButtonIndex(Input.MouseButtonLeft)
		ev.SetPressed(pressed)
		if pressed {
			ev.AsInputEventMouse().SetButtonMask(Input.MouseButtonMaskLeft)
		}
		ev.AsInputEventMouse().SetPosition(pos)
		ev.AsInputEventMouse().SetGlobalPosition(pos)
		Input.ParseInputEvent(ev.AsInputEvent())
	}
	a.after(nativeKeyFrames, func() { a.answer(cmd, nil) })
}

// isSongTable matches the multi-column song list (Title, Author, Copyright,
// CCLI #, Match), not the one-column library tree.
func isSongTable(n Node.Instance) (Control.Instance, bool) {
	if t, ok := Object.As[Tree.Instance](n); ok && t.Columns() > 1 {
		return t.AsControl(), true
	}
	return Control.Instance{}, false
}

// clickTarget clicks a named target like the operator's mouse:
// "verse" (row index, scrolled into view first), "suggestion" (index in the
// reference field's dropdown) or "preview-monitor". shift / meta / ctrl are
// held; double sends a double-click; button "right" clicks with the right
// button.
func (a *App) clickTarget(cmd *control.Command, args actionArgs) {
	index := -1
	if args.Index != nil {
		index = *args.Index
	}
	at := func() (Vector2.XY, bool) {
		switch args.Target {
		case "verse":
			return a.win.Scriptures.VerseRowCenter(index)
		case "suggestion":
			return a.win.Scriptures.SuggestionCenter(index)
		case "preview-monitor":
			r := a.win.PreviewMonitor.AsControl().GetGlobalRect()
			return Vector2.New(r.Position.X+r.Size.X/2, r.Position.Y+r.Size.Y/2), r.Size.X > 0
		}
		return Vector2.XY{}, false
	}
	switch args.Target {
	case "verse", "suggestion", "preview-monitor":
	default:
		cmd.RespondErr(fmt.Errorf("click: unknown target %q (verse|suggestion|preview-monitor)", args.Target))
		return
	}
	button, mask := Input.MouseButtonLeft, Input.MouseButtonMaskLeft
	if args.Button == "right" {
		button, mask = Input.MouseButtonRight, Input.MouseButtonMaskRight
	}
	if args.Target == "verse" {
		a.win.Scriptures.ScrollVerseIntoView(index)
	}
	// One frame for the scroll to apply before the row's position is read.
	a.after(1, func() {
		pos, ok := at()
		if !ok {
			cmd.RespondErr(fmt.Errorf("click: %s %d is not visible", args.Target, index))
			return
		}
		scale := Window.Get(a.AsNode()).ContentScaleFactor()
		pos = Vector2.New(pos.X*scale, pos.Y*scale)
		clicks := 1
		if args.Double {
			clicks = 2
		}
		for c := 0; c < clicks; c++ {
			for _, pressed := range []bool{true, false} {
				ev := InputEventMouseButton.New()
				ev.SetButtonIndex(button)
				ev.SetPressed(pressed)
				ev.SetDoubleClick(pressed && c == 1)
				if pressed {
					ev.AsInputEventMouse().SetButtonMask(mask)
				}
				mods := ev.AsInputEventWithModifiers()
				mods.SetShiftPressed(args.Shift)
				mods.SetMetaPressed(args.Meta)
				mods.SetCtrlPressed(args.Ctrl)
				ev.AsInputEventMouse().SetPosition(pos)
				ev.AsInputEventMouse().SetGlobalPosition(pos)
				Input.ParseInputEvent(ev.AsInputEvent())
			}
		}
		a.after(nativeKeyFrames, func() { a.answer(cmd, nil) })
	})
}
