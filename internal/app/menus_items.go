package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"freeworship/internal/app/core"
	"freeworship/internal/control"
	"freeworship/internal/models"
	"freeworship/internal/store"
)

// The library tile menus (the Media and Presentations tabs' right-click and
// gear menus, UI-SPEC §3.6): the events behind Aspect Ratio ▸, Repeating ▸,
// Use As Logo Background, Delete File…, Update items in Schedule, Open File
// Location, View ▸, Sort by ▸ and Refresh, and the control actions
// `media-menu` / `presentation-menu` that drive the same handlers. The pure
// parts are in core/itemmenus.go; the menus are ui.MediaPanel and
// ui.PresentationPanel. View ▸ and Sort by ▸ are persisted per tab.

// wireMediaMenu connects the Media panel's tile-menu callbacks (from wireMedia).
func (a *App) wireMediaMenu() {
	m := a.win.Media
	m.SetFileDeletion(trashSupported)
	m.OnAspect = func(i int, aspect models.MediaAspect) { a.event(func() { a.setMediaOptions(i, aspect, "", nil) }) }
	m.OnRepeat = func(i int, repeat models.MediaRepeat) { a.event(func() { a.setMediaOptions(i, "", repeat, nil) }) }
	m.OnLogo = func(i int) { a.event(func() { a.useMediaAsLogo(i, nil) }) }
	m.OnDeleteFile = func(i int) { a.event(func() { a.deleteMediaFile(i) }) }
	m.OnUpdateSchedule = func(i int) { a.event(func() { a.updateMediaInSchedule(i, nil) }) }
	m.OnReveal = func(i int) { a.event(func() { a.revealMedia(i, nil) }) }
	m.OnView = func(view string) { a.event(func() { a.fail("media view", a.setMediaView(core.ItemView(view))) }) }
	m.OnSort = func(sort string) { a.event(func() { a.fail("media sort", a.setMediaSort(core.ItemSort(sort))) }) }
	m.OnRefresh = func() { a.event(func() { a.refreshMedia(nil, false) }) }
}

// wirePresentationMenu connects the Presentations panel's tile-menu
// callbacks (from wirePresentations).
func (a *App) wirePresentationMenu() {
	p := a.win.Presentations
	p.SetFileDeletion(trashSupported)
	p.OnLogo = func(i int) { a.event(func() { a.usePresentationAsLogo(i, nil) }) }
	p.OnDeleteFile = func(i int) { a.event(func() { a.deletePresentationFile(i) }) }
	p.OnReveal = func(i int) { a.event(func() { a.revealPresentation(i, nil) }) }
	p.OnView = func(view string) {
		a.event(func() { a.fail("presentations view", a.setPresentationsView(core.ItemView(view))) })
	}
	p.OnSort = func(sort string) {
		a.event(func() { a.fail("presentations sort", a.setPresentationsSort(core.ItemSort(sort))) })
	}
	p.OnRefresh = func() { a.event(func() { a.refreshPresentations(nil, false) }) }
}

// loadItemPrefs reads a tab's persisted View ▸ and Sort by ▸ (synchronously,
// at startup: they decide how the first list is shown).
func (a *App) loadItemPrefs(viewKey, sortKey string) (core.ItemView, core.ItemSort) {
	var view, sort string
	if a.st != nil {
		if _, err := a.st.GetSetting(a.ctx, viewKey, &view); err != nil {
			a.fail("read setting "+viewKey, err)
		}
		if _, err := a.st.GetSetting(a.ctx, sortKey, &sort); err != nil {
			a.fail("read setting "+sortKey, err)
		}
	}
	return core.ItemView(view).Normalized(), core.ItemSort(sort).Normalized()
}

// ---- media events ------------------------------------------------------------------

// setMediaOptions changes media item i's Aspect Ratio and Repeating ("" keeps
// the current one). The list, the selected item, its deck and Preview (when
// it shows the library item) follow at once and the tiles rebuild, so the
// thumbnail letterboxes like the slide; the row is saved in the background.
// Live and the schedule keep their snapshots (Update items in Schedule).
func (a *App) setMediaOptions(i int, aspect models.MediaAspect, repeat models.MediaRepeat, cmd *control.Command) {
	if i < 0 || i >= len(a.md.list) {
		a.answer(cmd, fmt.Errorf("no media item at index %d", i))
		return
	}
	m := &a.md.list[i]
	if aspect == "" {
		aspect = m.Aspect.Normalized()
	}
	if repeat == "" {
		repeat = m.Repeat.Normalized()
	}
	if !aspect.Valid() || !repeat.Valid() {
		a.answer(cmd, fmt.Errorf("media options: unknown aspect %q or repeat %q", aspect, repeat))
		return
	}
	if m.Aspect.Normalized() == aspect && m.Repeat.Normalized() == repeat {
		a.answer(cmd, nil)
		return
	}
	m.Aspect, m.Repeat = aspect, repeat
	a.md.thumbVer++
	if a.md.item != nil && a.md.item.ID == m.ID {
		old := a.md.deck
		cp := *m
		a.md.item = &cp
		deck := core.MediaDeck(cp, a.lookupPlayable)
		a.md.deck = &deck
		if old != nil && core.SameDeck(a.pres.State.Preview, old) {
			a.setPreview(&deck)
		}
	}
	log.Printf("[freeworship] media %d %q: aspect %s, repeat %s", m.ID, m.Title, aspect, repeat)
	st := a.st
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	id, title := m.ID, m.Title
	a.async(cmd, func(ctx context.Context) func() error {
		err := st.SetMediaOptions(ctx, id, aspect, repeat)
		return func() error {
			if err != nil {
				a.alert("Media Options", fmt.Errorf("Could not save the options of %q.\n\n%w", title, err))
			}
			return err
		}
	})
}

// useMediaAsLogo makes media item i the logo screen's background.
func (a *App) useMediaAsLogo(i int, cmd *control.Command) {
	if i < 0 || i >= len(a.md.list) {
		a.answer(cmd, fmt.Errorf("no media item at index %d", i))
		return
	}
	m := a.md.list[i]
	a.setLogo(core.MediaLogo(m))
	log.Printf("[freeworship] logo background: media %q", m.Title)
	a.answer(cmd, nil)
}

// deleteMediaFile asks, then moves media item i's file to the Trash and
// removes the item from the library.
func (a *App) deleteMediaFile(i int) {
	if i < 0 || i >= len(a.md.list) || a.st == nil {
		return
	}
	m := a.md.list[i]
	a.win.Confirm.Ask("Delete File", "Move \""+m.Title+"\" to the Trash and remove it from the media library?", func() {
		a.event(func() { a.doDeleteMediaFile(m, nil) })
	})
}

// doDeleteMediaFile trashes the file and removes the item (after
// confirmation). A file that is already gone is only removed from the
// library; any other failure to trash it keeps the item.
func (a *App) doDeleteMediaFile(m models.Media, cmd *control.Command) {
	st := a.st
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	a.async(nil, func(ctx context.Context) func() error {
		err := trashFile(m.Path)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			err = st.DeleteMedia(ctx, m.ID)
		}
		return func() error {
			if err != nil {
				a.alert("Delete File", fmt.Errorf("Could not delete %q.\n\n%w", m.Title, err))
				a.answer(cmd, err)
				return err
			}
			log.Printf("[freeworship] moved %q to the Trash and removed media %d %q", m.Path, m.ID, m.Title)
			if a.md.item != nil && a.md.item.ID == m.ID {
				a.md.item, a.md.deck = nil, nil
			}
			a.loadMedia(a.md.query, a.md.kind, cmd)
			return nil
		}
	})
}

// updateMediaInSchedule rewrites the schedule's items for media item i from
// the library (title, aspect ratio, repeat, poster). The selected row
// reloads into Preview when it is one of them; Live is left as it is.
func (a *App) updateMediaInSchedule(i int, cmd *control.Command) {
	if i < 0 || i >= len(a.md.list) {
		a.answer(cmd, fmt.Errorf("no media item at index %d", i))
		return
	}
	m := a.md.list[i]
	if n := core.UpdateMediaItems(a.sched, m); n > 0 {
		a.schedVer++
		if a.schedSel >= 0 && a.schedSel < len(a.sched.Items) && a.sched.Items[a.schedSel].MediaID == m.ID {
			a.selectScheduleItem(a.schedSel)
		}
		log.Printf("[freeworship] updated %d schedule item(s) from media %q", n, m.Title)
	}
	a.answer(cmd, nil)
}

// revealMedia shows media item i's file in the file manager.
func (a *App) revealMedia(i int, cmd *control.Command) {
	if i < 0 || i >= len(a.md.list) {
		a.answer(cmd, fmt.Errorf("no media item at index %d", i))
		return
	}
	a.revealFile(a.md.list[i].Path, cmd)
}

// setMediaView changes the Media tab's View ▸ and persists it.
func (a *App) setMediaView(v core.ItemView) error {
	if !v.Valid() {
		return fmt.Errorf("unknown view %q", v)
	}
	if a.md.view == v {
		return nil
	}
	a.md.view = v
	a.saveSetting(core.SettingMediaView, v)
	return nil
}

// setMediaSort changes the Media tab's Sort by ▸, re-sorts the list (the
// selection follows the item) and persists it.
func (a *App) setMediaSort(s core.ItemSort) error {
	if !s.Valid() {
		return fmt.Errorf("unknown sort %q", s)
	}
	if a.md.sort == s {
		return nil
	}
	a.md.sort = s
	core.SortMedia(a.md.list, s)
	a.md.ver++
	a.md.sel = -1
	if a.md.item != nil {
		a.md.sel = core.MediaIndexOf(a.md.list, a.md.item.ID)
	}
	a.saveSetting(core.SettingMediaSort, s)
	return nil
}

// refreshMedia re-checks the library's files: items whose files are gone
// are removed after a question (confirm skips it), failed derivations are
// retried and the list reloads.
func (a *App) refreshMedia(cmd *control.Command, confirm bool) {
	st := a.st
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	a.async(nil, func(ctx context.Context) func() error {
		items, err := st.SearchMedia(ctx, "", "", 0)
		var missing []models.Media
		if err == nil {
			missing = core.MissingMedia(items)
		}
		return func() error {
			if err != nil {
				a.alert("Refresh Media", fmt.Errorf("Could not read the media library.\n\n%w", err))
				a.answer(cmd, err)
				return err
			}
			if len(missing) == 0 || confirm {
				a.doRefreshMedia(missing, cmd)
				return nil
			}
			a.win.Confirm.Ask("Refresh Media", core.RefreshQuestion(len(missing), "media"), func() {
				a.event(func() { a.doRefreshMedia(missing, nil) })
			})
			a.answer(cmd, nil)
			return nil
		}
	})
}

// doRefreshMedia removes the missing items, forgets this session's failed
// derivations, reloads the list and runs the pipeline again.
func (a *App) doRefreshMedia(missing []models.Media, cmd *control.Command) {
	st := a.st
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	a.async(nil, func(ctx context.Context) func() error {
		var err error
		for _, m := range missing {
			if e := st.DeleteMedia(ctx, m.ID); e != nil && !errors.Is(e, store.ErrNotFound) {
				err = e
				break
			}
		}
		return func() error {
			if err != nil {
				a.alert("Refresh Media", fmt.Errorf("Could not remove the missing items.\n\n%w", err))
				a.answer(cmd, err)
				return err
			}
			for _, m := range missing {
				log.Printf("[freeworship] refresh: removed media %d %q (file missing: %s)", m.ID, m.Title, m.Path)
				if a.md.item != nil && a.md.item.ID == m.ID {
					a.md.item, a.md.deck = nil, nil
				}
			}
			a.md.failed = map[int64]bool{}
			a.loadMedia(a.md.query, a.md.kind, cmd)
			a.startMediaWork()
			return nil
		}
	})
}

// ---- presentation events ----------------------------------------------------------

// usePresentationAsLogo makes presentation i's first page the logo screen's background.
func (a *App) usePresentationAsLogo(i int, cmd *control.Command) {
	if i < 0 || i >= len(a.pr.list) {
		a.answer(cmd, fmt.Errorf("no presentation at index %d", i))
		return
	}
	p := a.pr.list[i]
	bg, ok := core.PresentationLogo(p, a.pr.cache)
	if !ok {
		err := fmt.Errorf("%q has no slides yet: wait for it to finish rendering", p.Title)
		a.pr.status = "Wait for the slides to finish rendering before using one as the logo"
		a.answer(cmd, err)
		return
	}
	a.setLogo(bg)
	log.Printf("[freeworship] logo background: presentation %q, page 1", p.Title)
	a.answer(cmd, nil)
}

// deletePresentationFile asks, then moves presentation i's document to the
// Trash and removes the item from the library.
func (a *App) deletePresentationFile(i int) {
	if i < 0 || i >= len(a.pr.list) || a.st == nil {
		return
	}
	p := a.pr.list[i]
	a.win.Confirm.Ask("Delete File", "Move \""+p.Title+"\" to the Trash and remove it from the presentation library?", func() {
		a.event(func() { a.doDeletePresentationFile(p, nil) })
	})
}

// doDeletePresentationFile trashes the document and removes the item
// (after confirmation), as doDeleteMediaFile.
func (a *App) doDeletePresentationFile(p models.Presentation, cmd *control.Command) {
	st := a.st
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	a.async(nil, func(ctx context.Context) func() error {
		err := trashFile(p.Path)
		if err == nil || errors.Is(err, os.ErrNotExist) {
			err = st.DeletePresentation(ctx, p.ID)
		}
		return func() error {
			if err != nil {
				a.alert("Delete File", fmt.Errorf("Could not delete %q.\n\n%w", p.Title, err))
				a.answer(cmd, err)
				return err
			}
			log.Printf("[freeworship] moved %q to the Trash and removed presentation %d %q", p.Path, p.ID, p.Title)
			if a.pr.item != nil && a.pr.item.ID == p.ID {
				a.pr.item, a.pr.deck = nil, nil
			}
			a.loadPresentations(a.pr.query, cmd)
			return nil
		}
	})
}

// revealPresentation shows presentation i's document in the file manager.
func (a *App) revealPresentation(i int, cmd *control.Command) {
	if i < 0 || i >= len(a.pr.list) {
		a.answer(cmd, fmt.Errorf("no presentation at index %d", i))
		return
	}
	a.revealFile(a.pr.list[i].Path, cmd)
}

// setPresentationsView changes the Presentations tab's View ▸ and persists it.
func (a *App) setPresentationsView(v core.ItemView) error {
	if !v.Valid() {
		return fmt.Errorf("unknown view %q", v)
	}
	if a.pr.view == v {
		return nil
	}
	a.pr.view = v
	a.saveSetting(core.SettingPresentationsView, v)
	return nil
}

// setPresentationsSort changes the Presentations tab's Sort by ▸, re-sorts
// the list (the selection follows the item) and persists it.
func (a *App) setPresentationsSort(s core.ItemSort) error {
	if !s.Valid() {
		return fmt.Errorf("unknown sort %q", s)
	}
	if a.pr.sort == s {
		return nil
	}
	a.pr.sort = s
	core.SortPresentations(a.pr.list, s)
	a.pr.ver++
	a.pr.sel = -1
	if a.pr.item != nil {
		a.pr.sel = core.PresentationIndexOf(a.pr.list, a.pr.item.ID)
	}
	a.saveSetting(core.SettingPresentationsSort, s)
	return nil
}

// refreshPresentations re-checks the library's documents, as refreshMedia.
func (a *App) refreshPresentations(cmd *control.Command, confirm bool) {
	st := a.st
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	a.async(nil, func(ctx context.Context) func() error {
		items, err := st.SearchPresentations(ctx, "", 0)
		var missing []models.Presentation
		if err == nil {
			missing = core.MissingPresentations(items)
		}
		return func() error {
			if err != nil {
				a.alert("Refresh Presentations", fmt.Errorf("Could not read the presentation library.\n\n%w", err))
				a.answer(cmd, err)
				return err
			}
			if len(missing) == 0 || confirm {
				a.doRefreshPresentations(missing, cmd)
				return nil
			}
			a.win.Confirm.Ask("Refresh Presentations", core.RefreshQuestion(len(missing), "presentation"), func() {
				a.event(func() { a.doRefreshPresentations(missing, nil) })
			})
			a.answer(cmd, nil)
			return nil
		}
	})
}

// doRefreshPresentations removes the missing items, forgets this session's
// failed renders, reloads the list and runs the render job again.
func (a *App) doRefreshPresentations(missing []models.Presentation, cmd *control.Command) {
	st := a.st
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	a.async(nil, func(ctx context.Context) func() error {
		var err error
		for _, p := range missing {
			if e := st.DeletePresentation(ctx, p.ID); e != nil && !errors.Is(e, store.ErrNotFound) {
				err = e
				break
			}
		}
		return func() error {
			if err != nil {
				a.alert("Refresh Presentations", fmt.Errorf("Could not remove the missing items.\n\n%w", err))
				a.answer(cmd, err)
				return err
			}
			for _, p := range missing {
				log.Printf("[freeworship] refresh: removed presentation %d %q (file missing: %s)", p.ID, p.Title, p.Path)
				if a.pr.item != nil && a.pr.item.ID == p.ID {
					a.pr.item, a.pr.deck = nil, nil
				}
			}
			a.pr.failed = map[int64]bool{}
			a.loadPresentations(a.pr.query, cmd)
			a.startPresentationWork()
			return nil
		}
	})
}

// ---- shared ---------------------------------------------------------------------------

// revealFile shows a file in the platform's file manager (Open File
// Location) in the background. cmd is answered when the command returned;
// a failure is logged (lastError), not shown.
func (a *App) revealFile(path string, cmd *control.Command) {
	name, args := core.RevealCommand(path, runtime.GOOS)
	a.async(cmd, func(ctx context.Context) func() error {
		out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
		return func() error {
			if err != nil {
				a.fail("open file location", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out))))
				return err
			}
			log.Printf("[freeworship] revealed %s", path)
			return nil
		}
	})
}

// ---- control -----------------------------------------------------------------------

// handleItemMenuCommand runs the tile-menu actions `media-menu {item}` and
// `presentation-menu {item}`; false when the action is not one of them.
func (a *App) handleItemMenuCommand(cmd *control.Command, args actionArgs) bool {
	switch cmd.Action {
	case "media-menu":
		a.mediaMenuAction(cmd, args)
	case "presentation-menu":
		a.presentationMenuAction(cmd, args)
	default:
		return false
	}
	return true
}

// mediaItemArg resolves the media item an action targets: {index}, {title},
// else the selected one; -1 when none.
func (a *App) mediaItemArg(args actionArgs) int {
	switch {
	case args.Index != nil:
		return *args.Index
	case args.Title != "":
		return core.FindMedia(a.md.list, args.Title)
	}
	return a.md.sel
}

// presentationItemArg resolves the presentation an action targets, as mediaItemArg.
func (a *App) presentationItemArg(args actionArgs) int {
	switch {
	case args.Index != nil:
		return *args.Index
	case args.Title != "":
		return core.FindPresentation(a.pr.list, args.Title)
	}
	return a.pr.sel
}

const mediaMenuItems = "add-to-schedule|go-live|aspect-fill|aspect-fit|aspect-stretch|repeat-loop|repeat-once|logo-background|" +
	"remove|delete-file|update-schedule|reveal|view-large|view-medium|view-small|view-details|sort-name|sort-date|sort-kind|refresh"

// mediaMenuAction runs one item of the media tile menu, as a click would:
// `media-menu {item, index|title, confirm}`. Items that act on a tile use the
// given one or the selection; the questions (delete-file, refresh) are
// skipped with confirm.
func (a *App) mediaMenuAction(cmd *control.Command, args actionArgs) {
	item := args.Item
	target := func() (int, bool) {
		i := a.mediaItemArg(args)
		if i < 0 || i >= len(a.md.list) {
			cmd.RespondErr(fmt.Errorf("media-menu %s: no media item matches %s (select one first)", item, string(cmd.Data)))
			return 0, false
		}
		return i, true
	}
	switch {
	case item == "add-to-schedule":
		if i, ok := target(); ok {
			a.selectMedia(i, mediaSchedule, cmd)
		}
	case item == "go-live":
		if i, ok := target(); ok {
			a.selectMedia(i, mediaGoLive, cmd)
		}
	case strings.HasPrefix(item, "aspect-"):
		aspect, err := models.ParseMediaAspect(strings.TrimPrefix(item, "aspect-"))
		if err != nil {
			cmd.RespondErr(fmt.Errorf("media-menu: %w", err))
			return
		}
		if i, ok := target(); ok {
			a.setMediaOptions(i, aspect, "", cmd)
		}
	case strings.HasPrefix(item, "repeat-"):
		repeat, err := models.ParseMediaRepeat(strings.TrimPrefix(item, "repeat-"))
		if err != nil {
			cmd.RespondErr(fmt.Errorf("media-menu: %w", err))
			return
		}
		if i, ok := target(); ok {
			a.setMediaOptions(i, "", repeat, cmd)
		}
	case item == "logo-background":
		if i, ok := target(); ok {
			a.useMediaAsLogo(i, cmd)
		}
	case item == "remove":
		if i, ok := target(); ok {
			if !args.Confirm {
				a.removeMedia(i) // asks first, like the UI
				a.answer(cmd, nil)
				return
			}
			a.doRemoveMedia(a.md.list[i], cmd)
		}
	case item == "delete-file":
		if !trashSupported {
			cmd.RespondErr(errNoTrash)
			return
		}
		if i, ok := target(); ok {
			if !args.Confirm {
				a.deleteMediaFile(i) // asks first, like the UI
				a.answer(cmd, nil)
				return
			}
			a.doDeleteMediaFile(a.md.list[i], cmd)
		}
	case item == "update-schedule":
		if i, ok := target(); ok {
			a.updateMediaInSchedule(i, cmd)
		}
	case item == "reveal":
		if i, ok := target(); ok {
			a.revealMedia(i, cmd)
		}
	case strings.HasPrefix(item, "view-"):
		v, err := core.ParseItemView(strings.TrimPrefix(item, "view-"))
		if err == nil {
			err = a.setMediaView(v)
		}
		a.answer(cmd, err)
	case strings.HasPrefix(item, "sort-"):
		s, err := core.ParseItemSort(strings.TrimPrefix(item, "sort-"))
		if err == nil {
			err = a.setMediaSort(s)
		}
		a.answer(cmd, err)
	case item == "refresh":
		a.refreshMedia(cmd, args.Confirm)
	default:
		cmd.RespondErr(fmt.Errorf("media-menu: unknown item %q (%s)", item, mediaMenuItems))
	}
}

const presentationMenuItems = "add-to-schedule|go-live|logo-background|remove|delete-file|reveal|" +
	"view-large|view-medium|view-small|view-details|sort-name|sort-date|sort-kind|refresh"

// presentationMenuAction runs one item of the presentation tile menu:
// `presentation-menu {item, index|title, confirm}`, as mediaMenuAction.
func (a *App) presentationMenuAction(cmd *control.Command, args actionArgs) {
	item := args.Item
	target := func() (int, bool) {
		i := a.presentationItemArg(args)
		if i < 0 || i >= len(a.pr.list) {
			cmd.RespondErr(fmt.Errorf("presentation-menu %s: no presentation matches %s (select one first)", item, string(cmd.Data)))
			return 0, false
		}
		return i, true
	}
	switch {
	case item == "add-to-schedule":
		if i, ok := target(); ok {
			a.selectPresentation(i, presSchedule, cmd)
		}
	case item == "go-live":
		if i, ok := target(); ok {
			a.selectPresentation(i, presGoLive, cmd)
		}
	case item == "logo-background":
		if i, ok := target(); ok {
			a.usePresentationAsLogo(i, cmd)
		}
	case item == "remove":
		if i, ok := target(); ok {
			if !args.Confirm {
				a.removePresentation(i) // asks first, like the UI
				a.answer(cmd, nil)
				return
			}
			a.doRemovePresentation(a.pr.list[i], cmd)
		}
	case item == "delete-file":
		if !trashSupported {
			cmd.RespondErr(errNoTrash)
			return
		}
		if i, ok := target(); ok {
			if !args.Confirm {
				a.deletePresentationFile(i) // asks first, like the UI
				a.answer(cmd, nil)
				return
			}
			a.doDeletePresentationFile(a.pr.list[i], cmd)
		}
	case item == "reveal":
		if i, ok := target(); ok {
			a.revealPresentation(i, cmd)
		}
	case strings.HasPrefix(item, "view-"):
		v, err := core.ParseItemView(strings.TrimPrefix(item, "view-"))
		if err == nil {
			err = a.setPresentationsView(v)
		}
		a.answer(cmd, err)
	case strings.HasPrefix(item, "sort-"):
		s, err := core.ParseItemSort(strings.TrimPrefix(item, "sort-"))
		if err == nil {
			err = a.setPresentationsSort(s)
		}
		a.answer(cmd, err)
	case item == "refresh":
		a.refreshPresentations(cmd, args.Confirm)
	default:
		cmd.RespondErr(fmt.Errorf("presentation-menu: unknown item %q (%s)", item, presentationMenuItems))
	}
}
