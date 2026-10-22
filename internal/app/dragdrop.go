package app

import (
	"errors"
	"fmt"
	"strings"

	"freebeams/internal/control"
	"freebeams/internal/ui"
	"freebeams/internal/ui/uicore"

	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEventMouseButton"
	"graphics.gd/classdb/InputEventMouseMotion"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Vector2"
)

// Drag and drop into and within the schedule. The schedule list reports a
// drop between rows as OnDropInsert (source, index, gap) and a theme dropped
// onto a row as OnDropOnto; a row dragged elsewhere is its OnMove. A drop
// goes through the same path as Add to Schedule, with the insert index.

// dropState is the app's drag and drop state (App.drop).
type dropState struct {
	inserting bool // addToSchedule inserts at `at` instead of appending
	at        int
	held      *Vector2.XY // drag-to-schedule mode "hold": the pointer, still pressed (canvas units)
}

func (a *App) wireDragDrop() {
	s := a.win.Schedule
	s.OnDropInsert = func(source string, index, at int) {
		a.event(func() { a.dropIntoSchedule(source, index, at, nil) })
	}
	s.OnDropOnto = func(source string, index, row int) {
		a.event(func() { a.dropOntoScheduleRow(source, index, row, nil) })
	}
}

// insertingAt runs fn with addToSchedule inserting at schedule index at; a
// negative at leaves it appending.
func (a *App) insertingAt(at int, fn func()) {
	if at < 0 {
		fn()
		return
	}
	prev := a.drop
	a.drop.inserting, a.drop.at = true, at
	defer func() { a.drop.inserting, a.drop.at = prev.inserting, prev.at }()
	fn()
}

// dropIntoSchedule adds the dragged resource at schedule index at: the
// Add to Schedule path of its tab, with an insert index.
func (a *App) dropIntoSchedule(source string, index, at int, cmd *control.Command) {
	at = max(0, min(at, len(a.sched.Items)))
	switch source {
	case uicore.DragSong:
		a.selectSongAt(index, songSchedule, at, cmd) // the song loads in the background
	case uicore.DragScripture:
		// The selected verses; a drag that started on a row outside the
		// selection takes that row.
		if index >= 0 && index < len(a.listVerses) && !a.verseSel.Has(index) {
			a.verseClick(index, false, false)
		}
		a.insertingAt(at, func() { a.addScriptureToSchedule(cmd) })
	case uicore.DragMedia:
		a.insertingAt(at, func() { a.selectMedia(index, mediaSchedule, cmd) })
	case uicore.DragPresentation:
		a.insertingAt(at, func() { a.selectPresentation(index, presSchedule, cmd) })
	default:
		a.answer(cmd, fmt.Errorf("drop: %q cannot be inserted into the schedule", source))
	}
}

// dropOntoScheduleRow handles a drop onto a schedule row: a theme applies
// to that item (as the Theme submenu does).
func (a *App) dropOntoScheduleRow(source string, index, row int, cmd *control.Command) {
	if source != uicore.DragTheme {
		a.answer(cmd, fmt.Errorf("drop: %q does not go onto a schedule row", source))
		return
	}
	a.applyThemeToItem(index, row, cmd)
}

// ---- the drag-to-schedule hook -----------------------------------------------------

// dragToSchedule drives a real mouse drag through Godot's input pipeline:
// press on the source (a song row, a verse row, a media / presentation /
// theme tile or a schedule row, by index or title; its resource tab is shown
// first), move to the target in steps frames, release. The target is the
// insertion gap at (default: the end), schedule row `row` (a theme), or
// "outside" the list (the Preview grid). Mode "hold" stops before the release
// (for a mid-drag screenshot); mode "release" then lets go.
func (a *App) dragToSchedule(cmd *control.Command, args actionArgs) {
	if args.Mode == "release" {
		if a.drop.held == nil {
			cmd.RespondErr(errors.New("drag-to-schedule: no drag is held"))
			return
		}
		at := *a.drop.held
		a.drop.held = nil
		a.mouseButton(at, false)
		a.after(nativeKeyFrames+1, func() { a.answer(cmd, nil) })
		return
	}
	if a.drop.held != nil {
		cmd.RespondErr(errors.New("drag-to-schedule: a held drag is still pressed (mode release)"))
		return
	}
	tabs := map[string]int{uicore.DragSong: ui.TabSongs, uicore.DragScripture: ui.TabScriptures,
		uicore.DragMedia: ui.TabMedia, uicore.DragPresentation: ui.TabPresentations, uicore.DragTheme: ui.TabThemes}
	tab, isResource := tabs[args.Source]
	if !isResource && args.Source != uicore.DragSchedule {
		cmd.RespondErr(fmt.Errorf("drag-to-schedule: unknown source %q (song|scripture|media|presentation|theme|schedule)", args.Source))
		return
	}
	index, err := a.dragSourceIndex(args)
	if err != nil {
		cmd.RespondErr(err)
		return
	}
	if isResource {
		a.win.Resources.SetTab(tab)
	}
	steps := args.Steps
	if steps <= 0 {
		steps = 8
	}
	// Frame 1 scrolls the source into view (the point getters do), frame 3
	// reads the settled positions and starts the drag.
	a.after(1, func() { _, _ = a.dragSourcePoint(args.Source, index) })
	a.after(3, func() {
		from, ok := a.dragSourcePoint(args.Source, index)
		if !ok {
			cmd.RespondErr(fmt.Errorf("drag-to-schedule: %s %d is not visible", args.Source, index))
			return
		}
		to, err := a.dragTargetPoint(args)
		if err != nil {
			cmd.RespondErr(err)
			return
		}
		a.runDrag(cmd, from, to, steps, args.Mode == "hold")
	})
}

// dragSourceIndex resolves the dragged row or tile from index or title.
func (a *App) dragSourceIndex(args actionArgs) (int, error) {
	var titles []string
	switch args.Source {
	case uicore.DragSong:
		for _, s := range a.songs {
			titles = append(titles, s.Title)
		}
	case uicore.DragMedia:
		for _, m := range a.md.list {
			titles = append(titles, m.Title)
		}
	case uicore.DragPresentation:
		for _, p := range a.pr.list {
			titles = append(titles, p.Title)
		}
	case uicore.DragTheme:
		titles = a.themeNames()
	case uicore.DragSchedule:
		for _, it := range a.sched.Items {
			titles = append(titles, it.Title)
		}
	case uicore.DragScripture:
		// A verse row; by default the first selected one.
		if args.Index != nil {
			return *args.Index, nil
		}
		if len(a.verseSel.Rows) == 0 {
			return 0, errors.New("drag-to-schedule: no verses are selected (look up a reference first)")
		}
		return a.verseSel.Rows[0], nil
	}
	if args.Index != nil {
		if *args.Index < 0 || *args.Index >= len(titles) {
			return 0, fmt.Errorf("drag-to-schedule: no %s at index %d", args.Source, *args.Index)
		}
		return *args.Index, nil
	}
	for i, t := range titles {
		if strings.EqualFold(t, args.Title) {
			return i, nil
		}
	}
	return 0, fmt.Errorf("drag-to-schedule: no %s titled %q", args.Source, args.Title)
}

// dragSourcePoint is the source row or tile's centre (canvas units),
// scrolled into view.
func (a *App) dragSourcePoint(source string, i int) (Vector2.XY, bool) {
	switch source {
	case uicore.DragSong:
		return a.win.Songs.RowCenter(i)
	case uicore.DragScripture:
		a.win.Scriptures.ScrollVerseIntoView(i)
		return a.win.Scriptures.VerseRowCenter(i)
	case uicore.DragMedia:
		return a.win.Media.Grid().TileCenter(i)
	case uicore.DragPresentation:
		return a.win.Presentations.Grid().TileCenter(i)
	case uicore.DragTheme:
		return a.win.Themes.Grid().TileCenter(i)
	case uicore.DragSchedule:
		return a.win.Schedule.RowCenter(i)
	}
	return Vector2.XY{}, false
}

// dragTargetPoint is where the drag ends (canvas units).
func (a *App) dragTargetPoint(args actionArgs) (Vector2.XY, error) {
	switch {
	case args.Target == "outside":
		r := a.win.PreviewGrid.AsControl().GetGlobalRect()
		return Vector2.New(r.Position.X+r.Size.X/2, r.Position.Y+r.Size.Y/2), nil
	case args.Target != "":
		return Vector2.XY{}, fmt.Errorf("drag-to-schedule: unknown target %q (outside)", args.Target)
	case args.Row != nil:
		p, ok := a.win.Schedule.RowCenter(*args.Row)
		if !ok {
			return Vector2.XY{}, fmt.Errorf("drag-to-schedule: no schedule row %d", *args.Row)
		}
		return p, nil
	}
	at := len(a.sched.Items)
	if args.At != nil {
		at = *args.At
	}
	p, ok := a.win.Schedule.DropGapPoint(at)
	if !ok {
		return Vector2.XY{}, fmt.Errorf("drag-to-schedule: no insertion gap %d (0..%d)", at, len(a.sched.Items))
	}
	return p, nil
}

// runDrag presses at from, moves to `to` over steps frames (one motion per
// frame, with the relative motion Godot's drag detection accumulates), then
// releases there unless hold.
func (a *App) runDrag(cmd *control.Command, from, to Vector2.XY, steps int, hold bool) {
	a.mouseButton(from, true)
	prev := from
	for k := 1; k <= steps+1; k++ {
		f := float32(min(k, steps)) / float32(steps)
		p := Vector2.New(from.X+(to.X-from.X)*f, from.Y+(to.Y-from.Y)*f)
		rel := Vector2.New(p.X-prev.X, p.Y-prev.Y)
		prev = p
		a.after(k, func() { a.mouseMotion(p, rel) })
	}
	if hold {
		a.drop.held = &to
		a.after(steps+1+nativeKeyFrames, func() { a.answer(cmd, nil) })
		return
	}
	a.after(steps+2, func() { a.mouseButton(to, false) })
	a.after(steps+2+nativeKeyFrames, func() { a.answer(cmd, nil) })
}

// mouseButton pushes a left-button press or release at p (canvas units).
func (a *App) mouseButton(p Vector2.XY, pressed bool) {
	scale := Window.Get(a.AsNode()).ContentScaleFactor()
	p = Vector2.New(p.X*scale, p.Y*scale)
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

// mouseMotion pushes a motion with the left button held to p, rel from the
// previous one (canvas units).
func (a *App) mouseMotion(p, rel Vector2.XY) {
	scale := Window.Get(a.AsNode()).ContentScaleFactor()
	ev := InputEventMouseMotion.New()
	ev.AsInputEventMouse().SetButtonMask(Input.MouseButtonMaskLeft)
	ev.SetRelative(Vector2.New(rel.X*scale, rel.Y*scale))
	ev.AsInputEventMouse().SetPosition(Vector2.New(p.X*scale, p.Y*scale))
	ev.AsInputEventMouse().SetGlobalPosition(Vector2.New(p.X*scale, p.Y*scale))
	Input.ParseInputEvent(ev.AsInputEvent())
}
