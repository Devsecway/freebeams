package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"time"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/importers/easyworship"
	"freebeams/internal/models"
	"freebeams/internal/presentations"
	"freebeams/internal/render"
	"freebeams/internal/store"
	"freebeams/internal/ui"

	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/DisplayServer"
)

// The Presentations tab: its state, event handlers, control actions and the
// background job that renders documents to page images. Authoritative state
// lives on the App (presentationState); render projects it onto
// ui.PresentationPanel. It follows the Media tab (media.go).

// presentationPurpose is what to do with a selected presentation.
type presentationPurpose int

const (
	presPreview presentationPurpose = iota
	presGoLive
	presSchedule
)

// presentationState is the Presentations tab's authoritative state.
type presentationState struct {
	query    string
	list     []models.Presentation // the listed items (query)
	ver      int                   // bumped when list changes
	thumbVer int                   // bumped when an item's render or badge changed
	sel      int                   // selected tile, -1 none
	item     *models.Presentation  // selected item (kept while filtered out)
	deck     *models.Deck          // the selected item's deck (resource preview)
	count    int                   // library count
	status   string                // footer text
	gen      core.Generation

	cache      string                         // rendered pages directory
	tools      presentations.Tools            // renderers found
	toolsKnown bool                           // Detect has run
	known      map[string]models.Presentation // every presentation seen, by path (schedule lookups)

	working  bool                          // the render job is running
	again    bool                          // rerun when the current pass ends
	progress map[int64]core.RenderProgress // render progress by presentation id
	failed   map[int64]bool                // items whose render failed this session (not retried)

	// The tile menu's View ▸ and Sort by ▸, persisted (menus_items.go).
	view core.ItemView
	sort core.ItemSort
}

// presentationFilters are the native dialog filters for Add Presentation….
var presentationFilters = []string{"*.pdf, *.pptx, *.ppt, *.key, *.odp ; Presentations"}

// initPresentations prepares the cache dir, detects the renderers in the
// background and loads the library. Called from Ready.
func (a *App) initPresentations() {
	a.pr.sel = -1
	a.pr.known = map[string]models.Presentation{}
	a.pr.progress = map[int64]core.RenderProgress{}
	a.pr.failed = map[int64]bool{}
	dir, err := presentations.DefaultCacheDir(os.Getenv(core.CacheEnv))
	if err != nil {
		a.fail("presentation cache", err)
	} else {
		a.pr.cache = dir
	}
	a.async(nil, func(ctx context.Context) func() error {
		tools := presentations.Detect(ctx)
		return func() error {
			a.pr.tools, a.pr.toolsKnown = tools, true
			log.Printf("[freebeams] presentation renderers: %s", tools.Status())
			a.pr.status = a.presentationsFooter()
			a.startPresentationWork()
			return nil
		}
	})
	a.pr.view, a.pr.sort = a.loadItemPrefs(core.SettingPresentationsView, core.SettingPresentationsSort)
	a.loadPresentations("", nil)
	a.startPresentationImport()
}

// presentationCaps is what the machine can render.
func (a *App) presentationCaps() core.PresentationCaps {
	return core.PresentationCaps{PDF: a.pr.tools.CanRenderPDF(), Office: a.pr.tools.CanConvertOffice()}
}

func (a *App) presentationsFooter() string {
	return core.PresentationsFooter(a.pr.count, core.OfficeCount(a.pr.list), a.presentationCaps(), a.pr.working)
}

// lookupPages maps a document path to the library's current page images
// (for core.ResolvePresentationItem).
func (a *App) lookupPages(source string) ([]string, string, bool) {
	p, ok := a.pr.known[source]
	if !ok || !p.Rendered() || !presentations.Ready(a.pr.cache, p.CacheKey, p.Pages) {
		return nil, "", false
	}
	return core.PresentationPages(p, a.pr.cache), p.CacheKey, true
}

// wirePresentations connects the Presentations panel's callbacks (called from buildUI).
func (a *App) wirePresentations() {
	pp := a.win.Presentations
	pp.SetThumbnailFactory(func(i int) Control.Instance {
		if i < 0 || i >= len(a.pr.list) {
			return Control.Instance{}
		}
		bg := core.PresentationThumb(a.pr.list[i], a.pr.cache)
		s := models.Slide{Kind: models.SlidePresentation, Background: &bg}
		v := render.NewSlideView()
		v.SetTransitionDuration(0)
		v.SetFrame(models.Frame{Mode: models.ModeNormal, Slide: &s, Theme: models.DefaultPresentationTheme()})
		return v.AsControl()
	})
	pp.OnSearch = func(q string) { a.event(func() { a.loadPresentations(q, nil) }) }
	pp.OnSelect = func(i int) { a.event(func() { a.selectPresentation(i, presPreview, nil) }) }
	pp.OnActivate = func(i int) { a.event(func() { a.selectPresentation(i, presGoLive, nil) }) }
	pp.OnGoLive = func(i int) { a.event(func() { a.selectPresentation(i, presGoLive, nil) }) }
	pp.OnAddToSchedule = func(i int) { a.event(func() { a.selectPresentation(i, presSchedule, nil) }) }
	pp.OnRemove = func(i int) { a.event(func() { a.removePresentation(i) }) }
	pp.OnAdd = func() { a.event(a.addPresentationDialog) }
	a.wirePresentationMenu() // the tile menu's further items (menus_items.go)
}

// presentationRows builds the grid rows (title + badge) for the listed items.
func (a *App) presentationRows() []ui.PresentationRow {
	rows := make([]ui.PresentationRow, len(a.pr.list))
	caps := a.presentationCaps()
	for i, p := range a.pr.list {
		rows[i] = ui.PresentationRow{
			Title: p.Title,
			Badge: core.PresentationBadge(p, a.pr.progress[p.ID], a.pr.failed[p.ID], caps),
			Thumb: core.PresentationThumb(p, a.pr.cache).Path,

			Rendered: p.Rendered(),
		}
	}
	return rows
}

// presentationsStateDoc summarises the tab for GET /state.
func (a *App) presentationsStateDoc() core.PresentationsState {
	st := core.PresentationsState{
		Count: a.pr.count, Query: a.pr.query, Results: len(a.pr.list), Selected: a.pr.sel,
		CountText: a.pr.status, Working: a.pr.working,
		PDF: a.pr.tools.CanRenderPDF(), Office: a.pr.tools.CanConvertOffice(), Tools: a.pr.tools.Status(),
	}
	if p := a.pr.item; p != nil {
		st.Current, st.CurrentPages, st.CurrentPartial, st.CurrentRenderer = p.Title, p.Pages, p.Partial, p.Renderer
	}
	st.View, st.Sort = string(a.pr.view), string(a.pr.sort)
	for _, p := range a.pr.list {
		if !p.Rendered() {
			st.Pending++
		}
		if a.pr.failed[p.ID] {
			st.Failed++
		}
	}
	return st
}

// ---- events -------------------------------------------------------------------------

// loadPresentations lists the library for a query in the background. Stale
// results are dropped. The selection follows the selected item's id.
func (a *App) loadPresentations(q string, cmd *control.Command) {
	a.pr.query = q
	a.pr.status = a.presentationsFooter()
	if a.st() == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	ticket := a.pr.gen.Next()
	st := a.st()
	a.async(cmd, func(ctx context.Context) func() error {
		rows, err := st.SearchPresentations(ctx, q, 0)
		count, cerr := st.CountPresentations(ctx)
		return func() error {
			if !a.pr.gen.Current(ticket) {
				return nil // superseded
			}
			if cerr == nil {
				a.pr.count = count
			}
			if err != nil {
				a.fail("search presentations", err)
				a.pr.status = "Search failed"
				return err
			}
			a.pr.list = core.SortPresentations(rows, a.pr.sort)
			a.pr.ver++
			a.pr.sel = -1
			for _, p := range rows {
				a.pr.known[p.Path] = p
			}
			if a.pr.item != nil {
				a.pr.sel = core.PresentationIndexOf(rows, a.pr.item.ID)
				if a.pr.sel >= 0 && rows[a.pr.sel] != *a.pr.item {
					a.applyPresentationUpdate(rows[a.pr.sel]) // the selected item's render is newer
				}
			}
			a.pr.status = a.presentationsFooter()
			if a.presentationsNeedWork() {
				a.startPresentationWork()
			}
			return nil
		}
	})
}

// presentationDeck is the deck of a presentation: its pages, or the
// placeholder while they render.
func (a *App) presentationDeck(p models.Presentation) models.Deck {
	return core.PresentationDeck(p, core.PresentationPages(p, a.pr.cache))
}

// selectPresentation selects tile i and previews it, sends it live or adds
// it to the schedule. The data is in memory, so this is synchronous.
func (a *App) selectPresentation(i int, purpose presentationPurpose, cmd *control.Command) {
	if i < 0 || i >= len(a.pr.list) {
		a.answer(cmd, fmt.Errorf("no presentation at index %d", i))
		return
	}
	p := a.pr.list[i]
	a.pr.sel = i
	a.pr.item = &p
	deck := a.presentationDeck(p)
	a.pr.deck = &deck
	switch purpose {
	case presSchedule:
		a.addPresentationToSchedule(cmd)
		return
	case presGoLive:
		a.previewDeck(&deck)
		a.goLive()
	default:
		a.previewDeck(&deck)
	}
	a.answer(cmd, nil)
}

// addPresentationToSchedule adds the selected presentation (the resource
// tab bar's Add to Schedule, the context menu and the control action). It
// needs the pages: an unrendered deck is not added.
func (a *App) addPresentationToSchedule(cmd *control.Command) {
	p := a.pr.item
	if p == nil {
		a.answer(cmd, errors.New("select a presentation first"))
		return
	}
	pages := core.PresentationPages(*p, a.pr.cache)
	if len(pages) == 0 {
		err := fmt.Errorf("%q has no slides yet: wait for it to finish rendering", p.Title)
		a.pr.status = "Wait for the slides to finish rendering before adding to the schedule"
		a.answer(cmd, err)
		return
	}
	a.addToSchedule(core.PresentationItem(*p, pages))
	a.answer(cmd, nil)
}

// presentationGoLive sends the selected presentation live (the resource tab bar's Go Live).
func (a *App) presentationGoLive() {
	if a.pr.sel >= 0 {
		a.selectPresentation(a.pr.sel, presGoLive, nil)
	}
}

// removePresentation asks for confirmation, then removes tile i from the library.
func (a *App) removePresentation(i int) {
	if i < 0 || i >= len(a.pr.list) || a.st() == nil {
		return
	}
	p := a.pr.list[i]
	a.win.Confirm.Ask("Remove Presentation", "Remove \""+p.Title+"\" from the presentation library? The file is not deleted.", func() {
		a.event(func() { a.doRemovePresentation(p, nil) })
	})
}

// doRemovePresentation removes an item from the library (after confirmation).
func (a *App) doRemovePresentation(p models.Presentation, cmd *control.Command) {
	st := a.st()
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	a.async(nil, func(ctx context.Context) func() error {
		err := st.DeletePresentation(ctx, p.ID)
		return func() error {
			if err != nil {
				a.alert("Remove Presentation", fmt.Errorf("Could not remove %q.\n\n%w", p.Title, err))
				a.answer(cmd, err)
				return err
			}
			log.Printf("[freebeams] removed presentation %d %q", p.ID, p.Title)
			if a.pr.item != nil && a.pr.item.ID == p.ID {
				a.pr.item, a.pr.deck = nil, nil
			}
			a.loadPresentations(a.pr.query, cmd)
			return nil
		}
	})
}

// addPresentationPaths registers the presentation files (and the files of
// folders) that are not in the library yet, then reloads the list, which
// starts rendering them. cmd is answered once the list is reloaded.
func (a *App) addPresentationPaths(paths []string, cmd *control.Command) {
	if len(paths) == 0 {
		a.answer(cmd, errArgs)
		return
	}
	st := a.st()
	if st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	a.async(nil, func(ctx context.Context) func() error {
		items, warnings := presentations.Scan(paths...)
		var (
			added []*models.Presentation
			err   error
		)
		for i := range items {
			_, lookupErr := st.PresentationByPath(ctx, items[i].Path)
			switch {
			case lookupErr == nil:
				continue // already registered
			case errors.Is(lookupErr, store.ErrNotFound):
				added = append(added, &items[i])
			default:
				err = lookupErr
			}
			if err != nil {
				break
			}
		}
		if err == nil && len(added) > 0 {
			err = st.SavePresentations(ctx, added)
		}
		return func() error {
			for _, w := range warnings {
				log.Printf("[freebeams] add presentation: %s", w)
			}
			if err != nil {
				a.alert("Add Presentation", fmt.Errorf("Could not add the presentation.\n\n%w", err))
				a.answer(cmd, err)
				return err
			}
			if len(items) == 0 {
				err := errors.New("no presentation files found")
				a.alert("Add Presentation", fmt.Errorf("No PDF, PowerPoint or Keynote files were found in the selection."))
				a.answer(cmd, err)
				return err
			}
			log.Printf("[freebeams] add presentation: %d found, %d new", len(items), len(added))
			a.loadPresentations(a.pr.query, cmd)
			return nil
		}
	})
}

// addPresentationDialog shows the native Open dialog for presentation files.
func (a *App) addPresentationDialog() {
	err := DisplayServer.FileDialogShow("Add Presentation", a.dialogDir(), "", false, DisplayServer.FileDialogModeOpenFiles,
		presentationFilters, func(ok bool, paths []string, _ int) {
			if !ok || len(paths) == 0 {
				return
			}
			a.event(func() { a.addPresentationPaths(paths, nil) })
		}, 0)
	if err != nil {
		a.alert("Add Presentation", fmt.Errorf("Could not show the Open dialog.\n\n%w", err))
	}
}

// startPresentationImport registers the presentation files of an
// EasyWorship install once (Resources/Presentations; a stock install has none).
func (a *App) startPresentationImport() {
	if a.st() == nil || !easyworship.Installed() {
		return
	}
	var done bool
	if ok, err := a.st().GetSetting(a.ctx(), core.SettingPresentationsImportDone, &done); err != nil {
		a.fail("read presentations import setting", err)
	} else if ok && done {
		return
	}
	st := a.st()
	a.async(nil, func(ctx context.Context) func() error {
		rep, err := easyworship.ImportPresentations(ctx, st, easyworship.Options{})
		if err == nil {
			err = st.SetSetting(ctx, core.SettingPresentationsImportDone, true)
		}
		return func() error {
			if err != nil {
				a.fail("EasyWorship presentations import", err)
				return err
			}
			log.Printf("[freebeams] EasyWorship presentations: %d registered, %d already present", rep.PresentationsImported, rep.PresentationsSkipped)
			for _, w := range rep.Warnings {
				log.Printf("[freebeams] presentations import note: %s", w)
			}
			if rep.PresentationsImported > 0 {
				a.loadPresentations(a.pr.query, nil)
			}
			return nil
		}
	})
}

// ---- background rendering ---------------------------------------------------------------

// presentationsNeedWork reports whether a listed item still lacks its pages
// (failed items are not retried).
func (a *App) presentationsNeedWork() bool {
	for _, p := range a.pr.list {
		if a.pr.failed[p.ID] {
			continue
		}
		if !p.Rendered() || !presentations.Ready(a.pr.cache, p.CacheKey, p.Pages) {
			return true
		}
	}
	return false
}

// startPresentationWork runs one pass over the whole library in a goroutine:
// every document without current pages is rendered, one at a time, and its
// row saved and posted. A request while a pass runs queues another pass. The
// job is tracked for shutdown but not counted as pending work: LibreOffice
// can take a while.
func (a *App) startPresentationWork() {
	if a.st() == nil || a.pr.cache == "" || !a.pr.toolsKnown {
		return
	}
	if a.pr.working {
		a.pr.again = true
		return
	}
	a.pr.working = true
	a.pr.status = a.presentationsFooter()
	st, ctx := a.st(), a.ctx()
	svc := presentations.NewService(a.pr.tools, a.pr.cache)
	skip := make(map[int64]bool, len(a.pr.failed))
	for id := range a.pr.failed {
		skip[id] = true
	}
	a.jobs.Add(1)
	go func() {
		defer a.jobs.Done()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[freebeams] panic in presentation work: %v\n%s", r, debug.Stack())
			}
			a.post(func() {
				a.pr.working = false
				a.pr.status = a.presentationsFooter()
				if a.pr.again {
					a.pr.again = false
					a.startPresentationWork()
				}
			})
		}()
		a.runPresentationWork(ctx, st, svc, skip)
	}()
}

// runPresentationWork is the render job's body (goroutine).
func (a *App) runPresentationWork(ctx context.Context, st *store.Store, svc *presentations.Service, skip map[int64]bool) {
	items, err := st.SearchPresentations(ctx, "", 0)
	if err != nil {
		a.post(func() { a.fail("presentation work", err) })
		return
	}
	for i := range items {
		if ctx.Err() != nil {
			return
		}
		p := items[i]
		if skip[p.ID] {
			continue
		}
		key, kerr := presentations.Key(p.Path)
		if kerr != nil {
			a.presentationFail(p, kerr)
			continue
		}
		if p.Rendered() && p.CacheKey == key && presentations.Ready(svc.CacheDir, key, p.Pages) {
			continue
		}
		id := p.ID
		last := time.Time{}
		progress := func(done, total int) {
			if now := time.Now(); now.Sub(last) >= progressInterval || done == total {
				last = now
				a.post(func() {
					a.pr.progress[id] = core.RenderProgress{Active: true, Done: done, Total: total}
					a.pr.thumbVer++
				})
			}
		}
		a.post(func() {
			a.pr.progress[id] = core.RenderProgress{Active: true}
			a.pr.thumbVer++
		})
		key, res, rerr := svc.Render(ctx, p.Path, progress)
		if rerr != nil {
			a.presentationFail(p, rerr)
			continue
		}
		p.Pages, p.CacheKey, p.Renderer, p.Partial = res.Pages, key, res.Renderer, res.Partial
		if err := st.SavePresentation(ctx, &p); err != nil {
			a.post(func() { a.fail("save presentation "+p.Title, err) })
			continue
		}
		log.Printf("[freebeams] rendered %q: %d pages (%s%s)", p.Title, p.Pages, p.Renderer, map[bool]string{true: ", embedded images only", false: ""}[p.Partial])
		a.post(func() { a.applyPresentationUpdate(p) })
	}
}

// presentationFail records a render failure (the item is not retried this
// session) and logs it. Cancellation is not a failure.
func (a *App) presentationFail(p models.Presentation, err error) {
	if err == nil || errors.Is(err, context.Canceled) {
		return
	}
	a.post(func() {
		delete(a.pr.progress, p.ID)
		if !a.pr.failed[p.ID] {
			a.pr.failed[p.ID] = true
			a.pr.thumbVer++
			log.Printf("[freebeams] presentation %q: %v", p.Title, err)
		}
	})
}

// applyPresentationUpdate takes a render result into the state: the listed
// item, the path lookup, the selected item's deck and the tile badges. When
// the selected item is in Preview (not yet live), Preview follows its pages.
func (a *App) applyPresentationUpdate(p models.Presentation) {
	delete(a.pr.progress, p.ID)
	if i := core.PresentationIndexOf(a.pr.list, p.ID); i >= 0 {
		a.pr.list[i] = p
	}
	a.pr.known[p.Path] = p
	a.pr.thumbVer++
	if a.pr.item != nil && a.pr.item.ID == p.ID {
		old := a.pr.deck
		a.pr.item = &p
		deck := a.presentationDeck(p)
		a.pr.deck = &deck
		if old != nil && core.SameDeck(a.pres.State.Preview, old) && !core.SameDeck(old, &deck) {
			a.setPreview(&deck)
		}
	}
	a.fillDroppedPresentations(p) // a document dropped into the schedule before it was rendered (filedrop.go)
}
