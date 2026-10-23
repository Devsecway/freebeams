package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/media"
	"freebeams/internal/models"
	"freebeams/internal/presentations"
	"freebeams/internal/store"
	"freebeams/internal/ui"

	"graphics.gd/classdb/SceneTree"
	"graphics.gd/classdb/Viewport"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2"
)

// Files dropped from the Finder (or Explorer) onto the operator window, as
// EasyWorship allows: the pastor's slides or a video minutes before the
// service. Godot reports a drop through the root window's files_dropped
// signal, paths only; the pointer says where it landed (ui.FileDropSpot).
//
//   - On the schedule: media and presentation files are added to their
//     libraries (as the tabs' "+" does) and inserted at the gap under the
//     pointer, in drop order (core.PlanFileDrop). A presentation that is not
//     rendered yet goes in as a "Preparing slides…" item that takes its pages
//     when the render job finishes (core.FillPendingPresentation). A schedule
//     file opens as File ▸ Open would and a Bible package goes to the Bible
//     Manager, both through the open-files queue (openfiles.go).
//   - On the Media or Presentations tab (its button, or its page while it is
//     shown): added to that library only.
//   - Anything that cannot go there is skipped, with one message naming it.
//
// `drop-files {paths, target, at, mode}` drives the same path: it warps the
// pointer over the target and emits files_dropped (mode "direct" skips the
// pointer and drops on the target, at index `at` or after the selection).

// fileDropState is the app's file drop state (App.fdrop).
type fileDropState struct {
	waiting *control.Command // drop-files: the command the next files_dropped answers
	last    core.FileDropState
}

// fileDropFrames is how long drop-files waits for its files_dropped signal.
const fileDropFrames = 30

func (a *App) wireFileDrop() {
	root := SceneTree.Get(a.AsNode()).Root()
	root.OnFilesDropped(func(files []string) {
		a.event(func() { a.filesDropped(files) })
	})
}

// filesDropped is the root window's files_dropped: the drop lands where the
// pointer is.
func (a *App) filesDropped(paths []string) {
	cmd := a.fdrop.waiting
	a.fdrop.waiting = nil
	p := a.win.AsControl().AsCanvasItem().GetGlobalMousePosition()
	spot := a.win.FileDropAt(p)
	target := ""
	switch {
	case spot.Schedule:
		target = core.FileDropSchedule
	case spot.Tab == ui.TabMedia:
		target = core.FileDropMedia
	case spot.Tab == ui.TabPresentations:
		target = core.FileDropPresentations
	}
	log.Printf("[freebeams] files dropped at (%.0f, %.0f): %d", p.X, p.Y, len(paths))
	if target != "" && spot.Tab >= 0 && a.win.Resources.Tab() != spot.Tab {
		a.win.Resources.SetTab(spot.Tab) // dropped on the tab's button: show what was added
	}
	a.dropFiles(paths, target, spot.Gap, cmd)
}

// statPath reports whether path exists and is a folder.
func statPath(path string) (exists, isDir bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return false, false
	}
	return true, fi.IsDir()
}

// dropFiles drops paths on target (core.FileDrop…, "" outside every target);
// gap is the schedule's insertion gap under the pointer, -1 when unknown
// (then the files go after the selected row). cmd is answered once the
// libraries are updated and the items inserted.
func (a *App) dropFiles(paths []string, target string, gap int, cmd *control.Command) {
	abs := make([]string, 0, len(paths))
	for _, p := range paths {
		if ap, err := filepath.Abs(p); err == nil && p != "" {
			p = ap
		}
		abs = append(abs, p)
	}
	plan := core.PlanFileDrop(target, abs, statPath)
	at := -1
	if target == core.FileDropSchedule {
		at = core.FileDropInsertAt(gap, a.schedSel, len(a.sched.Items))
	}
	log.Printf("[freebeams] file drop on %s (insert at %d)", core.FileDropSummary(plan), at)
	last := core.FileDropState{Target: target, At: at}
	for _, s := range plan.Skipped {
		last.Skipped = append(last.Skipped, filepath.Base(s.Path))
	}
	a.fdrop.last = last
	if msg := core.FileDropMessage(target, plan.Skipped); msg != "" {
		a.alert("Add Files", errors.New(msg))
	}
	if plan.Schedule != "" {
		a.opening.Schedule = plan.Schedule
	}
	a.opening.Bibles = append(a.opening.Bibles, plan.Bibles...)
	if len(plan.Media) == 0 && len(plan.Presentations) == 0 {
		a.fdrop.last.Done = true
		a.answerWhenIdle(cmd) // a schedule or package opens on the next frames
		return
	}
	st := a.st()
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	a.async(nil, func(ctx context.Context) func() error {
		md, mn, merr := registerMediaFiles(ctx, st, plan.Media)
		pr, pn, perr := registerPresentationFiles(ctx, st, plan.Presentations)
		return func() error {
			for _, w := range append(md.warnings, pr.warnings...) {
				log.Printf("[freebeams] file drop: %s", w)
			}
			if err := errors.Join(merr, perr); err != nil {
				a.alert("Add Files", fmt.Errorf("Could not add the dropped files.\n\n%w", err))
				a.answer(cmd, err)
				return err
			}
			log.Printf("[freebeams] file drop: media %d found, %d new; presentations %d found, %d new",
				len(md.byPath), mn, len(pr.byPath), pn)
			if target != core.FileDropSchedule && len(md.byPath) == 0 && len(pr.byPath) == 0 {
				// A folder with nothing usable in it (the files were checked by the plan).
				err := errors.New("no files to add found")
				what := "No image, video or audio files were found in the folder."
				if target == core.FileDropPresentations {
					what = "No PDF, PowerPoint or Keynote files were found in the folder."
				}
				a.alert("Add Files", errors.New(what))
				a.answer(cmd, err)
				return err
			}
			if target == core.FileDropSchedule {
				a.insertDropped(plan.Insert, at, md.byPath, pr.byPath)
			}
			if len(plan.Media) > 0 {
				a.loadMedia(a.md.query, a.md.kind, nil) // and derives the new items' thumbnails
			}
			if len(plan.Presentations) > 0 {
				a.loadPresentations(a.pr.query, nil)
				a.startPresentationWork() // render the new documents even when the list filters them out
			}
			a.fdrop.last.Done = true
			a.answerWhenIdle(cmd) // once the lists have reloaded
			return nil
		}
	})
}

// answerWhenIdle answers cmd with the state once the background work it
// started (the library reloads) is done, as wait-idle does.
func (a *App) answerWhenIdle(cmd *control.Command) {
	if cmd != nil {
		a.waiters = append(a.waiters, cmd)
	}
}

// insertDropped inserts the dropped files' library items at schedule index
// at, in drop order.
func (a *App) insertDropped(files []core.DroppedFile, at int, md map[string]models.Media, pr map[string]models.Presentation) {
	for _, f := range files {
		var item models.ScheduleItem
		switch f.Kind {
		case core.DroppedMedia:
			m, ok := md[f.Path]
			if !ok {
				continue
			}
			item = core.MediaItem(m)
		case core.DroppedPresentation:
			p, ok := pr[f.Path]
			if !ok {
				continue
			}
			a.pr.known[p.Path] = p
			if pages, _, ok := a.lookupPages(p.Path); ok {
				item = core.PresentationItem(p, pages)
			} else {
				item = core.PendingPresentationItem(p) // its pages arrive with the render (fillDroppedPresentations)
			}
		default:
			continue
		}
		a.insertingAt(at, func() { a.addToSchedule(item) })
		a.fdrop.last.Inserted = append(a.fdrop.last.Inserted, item.Title)
		at++
	}
}

// fillDroppedPresentations gives the schedule's pending items of document p
// its rendered pages (called with every render result).
func (a *App) fillDroppedPresentations(p models.Presentation) {
	if !p.Rendered() || !presentations.Ready(a.pr.cache, p.CacheKey, p.Pages) {
		return
	}
	changed := core.FillPendingPresentation(a.sched.Items, p, core.PresentationPages(p, a.pr.cache))
	if len(changed) == 0 {
		return
	}
	a.sched.Dirty = true
	a.schedVer++
	for _, i := range changed {
		log.Printf("[freebeams] file drop: %q in the schedule has its %d slides", p.Title, p.Pages)
		if i == a.schedSel {
			a.selectScheduleItem(i) // Preview shows the placeholder: show the pages
		}
	}
}

// droppedFiles is what a registration found, by path.
type droppedFiles[T any] struct {
	byPath   map[string]T
	warnings []string
}

// registerMediaFiles scans paths (files or folders, as Add Media does),
// registers the files the library lacks and returns every found file's
// library item by path, with the number of new ones (goroutine).
func registerMediaFiles(ctx context.Context, st *store.Store, paths []string) (droppedFiles[models.Media], int, error) {
	out := droppedFiles[models.Media]{byPath: map[string]models.Media{}}
	if len(paths) == 0 {
		return out, 0, nil
	}
	items, warnings := media.Scan(paths...)
	out.warnings = warnings
	var fresh []*models.Media
	for i := range items {
		m, err := st.MediaByPath(ctx, items[i].Path)
		switch {
		case err == nil:
			out.byPath[m.Path] = m
		case errors.Is(err, store.ErrNotFound):
			fresh = append(fresh, &items[i])
		default:
			return out, 0, err
		}
	}
	if len(fresh) > 0 {
		if err := st.SaveMediaItems(ctx, fresh); err != nil {
			return out, 0, err
		}
	}
	for _, m := range fresh {
		out.byPath[m.Path] = *m
	}
	return out, len(fresh), nil
}

// registerPresentationFiles is registerMediaFiles for presentations.
func registerPresentationFiles(ctx context.Context, st *store.Store, paths []string) (droppedFiles[models.Presentation], int, error) {
	out := droppedFiles[models.Presentation]{byPath: map[string]models.Presentation{}}
	if len(paths) == 0 {
		return out, 0, nil
	}
	items, warnings := presentations.Scan(paths...)
	out.warnings = warnings
	var fresh []*models.Presentation
	for i := range items {
		p, err := st.PresentationByPath(ctx, items[i].Path)
		switch {
		case err == nil:
			out.byPath[p.Path] = p
		case errors.Is(err, store.ErrNotFound):
			fresh = append(fresh, &items[i])
		default:
			return out, 0, err
		}
	}
	if len(fresh) > 0 {
		if err := st.SavePresentations(ctx, fresh); err != nil {
			return out, 0, err
		}
	}
	for _, p := range fresh {
		out.byPath[p.Path] = *p
	}
	return out, len(fresh), nil
}

// ---- the drop-files hook --------------------------------------------------------------

// dropFilesHook is `drop-files {paths, target: schedule|media|presentations|
// outside, at, mode}`. By default it warps the pointer over the target (the
// schedule's insertion gap at, default the end; the tab's button; the Preview
// grid for "outside") and emits the root window's files_dropped, so the drop
// goes through the signal handler and its pointer lookup. Mode "direct"
// calls the drop path without the pointer (at omitted: after the selected
// row). It is answered when the drop is done.
func (a *App) dropFilesHook(cmd *control.Command, args actionArgs) {
	paths := append(append([]string{}, args.Paths...), args.Path)
	if args.Path == "" {
		paths = paths[:len(paths)-1]
	}
	if len(paths) == 0 {
		cmd.RespondErr(errors.New("drop-files: no paths"))
		return
	}
	targets := map[string]bool{core.FileDropSchedule: true, core.FileDropMedia: true, core.FileDropPresentations: true, "outside": true}
	if !targets[args.Target] {
		cmd.RespondErr(fmt.Errorf("drop-files: unknown target %q (schedule|media|presentations|outside)", args.Target))
		return
	}
	if args.Mode == "direct" {
		target, gap := args.Target, -1
		if target == "outside" {
			target = ""
		}
		if args.At != nil {
			gap = *args.At
		}
		a.dropFiles(paths, target, gap, cmd)
		return
	}
	if args.Mode != "" {
		cmd.RespondErr(fmt.Errorf("drop-files: unknown mode %q (direct, or none for the pointer)", args.Mode))
		return
	}
	if a.fdrop.waiting != nil {
		cmd.RespondErr(errors.New("drop-files: a drop is still waiting for its signal"))
		return
	}
	var (
		to Vector2.XY
		ok bool
	)
	switch args.Target {
	case core.FileDropSchedule:
		at := len(a.sched.Items)
		if args.At != nil {
			at = *args.At
		}
		a.win.Schedule.ScrollRowIntoView(min(at, len(a.sched.Items)-1))
		to, ok = a.win.FileDropPoint(true, at, -1)
	case core.FileDropMedia:
		to, ok = a.win.FileDropPoint(false, 0, ui.TabMedia)
	case core.FileDropPresentations:
		to, ok = a.win.FileDropPoint(false, 0, ui.TabPresentations)
	default:
		r := a.win.PreviewGrid.AsControl().GetGlobalRect()
		to, ok = Vector2.New(r.Position.X+r.Size.X/2, r.Position.Y+r.Size.Y/2), true
	}
	if !ok {
		cmd.RespondErr(fmt.Errorf("drop-files: no drop point for %s", args.Target))
		return
	}
	a.fdrop.waiting = cmd
	// Frame 1 settles a scroll; frame 2 warps the pointer there (the drop
	// target is read from the OS cursor, as for a real drop); frame 4
	// emits the signal as Godot does for a drop from the Finder.
	a.after(2, func() {
		if args.Target == core.FileDropSchedule { // the scroll moved the rows
			at := len(a.sched.Items)
			if args.At != nil {
				at = *args.At
			}
			if p, ok := a.win.FileDropPoint(true, at, -1); ok {
				to = p
			}
		}
		Viewport.Get(a.AsNode()).WarpMouse(to)
	})
	a.after(4, func() {
		root := SceneTree.Get(a.AsNode()).Root()
		Object.Call(root, "emit_signal", "files_dropped", paths)
	})
	a.after(4+fileDropFrames, func() {
		if a.fdrop.waiting == cmd {
			a.fdrop.waiting = nil
			cmd.RespondErr(errors.New("drop-files: files_dropped never arrived"))
		}
	})
}
