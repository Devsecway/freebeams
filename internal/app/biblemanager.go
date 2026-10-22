package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"freebeams/internal/app/core"
	"freebeams/internal/appdirs"
	"freebeams/internal/bibles/biblefolder"
	"freebeams/internal/bibles/biblepkg"
	"freebeams/internal/bibles/bundled"
	"freebeams/internal/control"
	"freebeams/internal/importers/bibletext"
	"freebeams/internal/models"
	"freebeams/internal/store"
	"freebeams/internal/ui"
)

// The Bible Manager (Scriptures ▸ + / Manage Bibles…): every translation
// FreeBeams can add, from the built-ins (internal/bibles/bundled) and the
// Bibles folder (appdirs.Bibles, internal/bibles/biblefolder), merged with
// the library. Add installs one in the background with progress, Remove
// deletes one after a question, Save as Package writes an installed one to
// the Bibles folder. See docs/BIBLES.md.
//
// When the removed translation was the one selected in the Scriptures tab
// (or the default), the tab falls back to core.DefaultBible: the first
// installed built-in in bundle order (KJV, ASV, RVA), else the first
// translation in the list. A removed built-in is recorded so a newer bundle
// does not bring it back (bundled.SetRemoved); adding it again clears that.

// bibleManagerState is the Bible Manager's state (App.bm). Only the main
// thread touches it; background jobs return their results through async.
type bibleManagerState struct {
	open      bool
	dir       string // the Bibles folder ("" until first needed)
	folder    []core.BibleAvailable
	problems  []string
	counts    map[int64]int // verses per installed bible ID
	scanning  bool
	scanned   bool
	scanGen   core.Generation
	sel       string  // the selected row's abbreviation
	task      string  // "" | add | remove | save
	taskBible string  // the translation the task works on
	progress  float64 // 0..1
	status    string
	statusErr bool
}

// builtinBibles lists the built-ins as manager sources (read once: the
// headers are embedded and never change).
var builtinBibles = sync.OnceValue(func() []core.BibleAvailable {
	out := []core.BibleAvailable{}
	for _, t := range bundled.Translations {
		h, err := bundled.Header(t.Abbrev)
		if err != nil {
			log.Printf("[freebeams] bundled bible %s: %v", t.Abbrev, err)
			continue
		}
		out = append(out, core.BibleAvailable{Bible: h.AsBible(), Verses: h.Verses, Size: bundled.FileSize(t.Abbrev),
			Source: core.BibleSourceBuiltin, Package: true})
	}
	return out
})

// biblesDir returns the Bibles folder, creating it on first use.
func (a *App) biblesDir() (string, error) {
	if a.bm.dir != "" {
		if _, err := os.Stat(a.bm.dir); err == nil {
			return a.bm.dir, nil
		}
	}
	d, err := appdirs.Bibles()
	if err != nil {
		return "", err
	}
	a.bm.dir = d
	return d, nil
}

// bibleEntries are the manager's rows.
func (a *App) bibleEntries() []core.BibleEntry {
	return core.MergeBibleEntries(builtinBibles(), a.bm.folder, a.bibles, a.bm.counts)
}

// applyBiblesFolder takes a Bibles folder listing (refreshBibles).
func (a *App) applyBiblesFolder(entries []biblefolder.Entry, problems []string, err error) {
	a.bm.scanned = true
	if err != nil {
		a.bm.status, a.bm.statusErr = "Could not list the Bibles folder: "+err.Error(), true
		a.fail("list Bibles folder", err)
		return
	}
	folder := make([]core.BibleAvailable, 0, len(entries))
	for _, e := range entries {
		folder = append(folder, core.BibleAvailable{Bible: e.Bible, Verses: e.Verses, Size: e.Size,
			Source: core.BibleSourceFolder, Path: e.Path, Package: e.Kind == biblefolder.KindPackage})
	}
	a.bm.folder, a.bm.problems = folder, problems
	for _, p := range problems {
		log.Printf("[freebeams] Bibles folder: %s", p)
	}
	if a.bm.task == "" && a.bm.status == "" {
		switch n := len(problems); {
		case n == 1:
			a.bm.status, a.bm.statusErr = "1 file in the Bibles folder could not be read: "+problems[0], true
		case n > 1:
			a.bm.status, a.bm.statusErr = fmt.Sprintf("%d files in the Bibles folder could not be read (see the log)", n), true
		}
	}
}

// openBibleManager shows the manager and lists the Bibles folder again.
func (a *App) openBibleManager(cmd *control.Command) {
	if a.st() == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	if !a.bm.open {
		a.bm.status, a.bm.statusErr = "", false
		if a.bm.sel == "" && a.bibleSel >= 0 && a.bibleSel < len(a.bibles) {
			a.bm.sel = a.bibles[a.bibleSel].Abbrev
		}
	}
	a.bm.open = true
	a.refreshBibles(cmd, true)
}

// selectBibleEntry selects manager row i.
func (a *App) selectBibleEntry(i int) {
	if entries := a.bibleEntries(); i >= 0 && i < len(entries) {
		a.bm.sel = entries[i].Abbrev
	}
}

// bibleEntry finds abbrev among the manager's rows.
func (a *App) bibleEntry(abbrev string) (core.BibleEntry, error) {
	entries := a.bibleEntries()
	i := core.FindBibleEntry(entries, abbrev)
	if i < 0 {
		return core.BibleEntry{}, fmt.Errorf("no Bible %q in the Bible Manager", strings.TrimSpace(abbrev))
	}
	return entries[i], nil
}

// bibleBusy refuses a second task while one runs.
func (a *App) bibleBusy() error {
	if a.bm.task == "" {
		return nil
	}
	return fmt.Errorf("busy: the Bible Manager is still working on %s (%s)", a.bm.taskBible, a.bm.task)
}

// setBibleStatus sets the manager's status line and the Scriptures footer.
func (a *App) setBibleStatus(msg string, isErr bool) {
	a.bm.status, a.bm.statusErr = msg, isErr
	a.setScriptStatus(msg, isErr)
}

// bibleProgress reports a running task's progress from its goroutine. It
// never blocks: a full results queue drops a report (the next one follows).
func (a *App) bibleProgress(task, abbrev string, p float64) {
	select {
	case a.results <- func() {
		if a.bm.task == task && a.bm.taskBible == abbrev {
			a.bm.progress = p
		}
	}:
	default:
	}
}

// addBiblePath is bible-add {path}: a file the Bibles folder lists is added
// like its row; any other file goes through Import Bible File… (the
// question, then the import).
func (a *App) addBiblePath(cmd *control.Command, path string, confirmed bool) {
	clean := filepath.Clean(strings.TrimSpace(path))
	for _, f := range a.bm.folder {
		if filepath.Clean(f.Path) == clean {
			a.bibleAdd(cmd, f.Bible.Abbrev, confirmed)
			return
		}
	}
	a.importBibleFile(cmd, path, confirmed)
}

// bibleAdd installs a translation from its source (a built-in, else its
// Bibles folder file) in the background, reporting progress. A copyrighted
// translation asks the licence question first unless confirmed. cmd is
// answered once the library and the list are refreshed (or with the question
// on screen).
func (a *App) bibleAdd(cmd *control.Command, abbrev string, confirmed bool) {
	if a.st() == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	if err := a.bibleBusy(); err != nil {
		a.answer(cmd, err)
		return
	}
	e, err := a.bibleEntry(abbrev)
	switch {
	case err != nil:
	case e.Installed:
		err = fmt.Errorf("%s is already installed", e.Abbrev)
	case !e.CanAdd():
		err = fmt.Errorf("%s has no package or file to add it from", e.Abbrev)
	}
	if err != nil {
		a.bm.status, a.bm.statusErr = err.Error(), true
		a.answer(cmd, err)
		return
	}
	a.bm.sel = e.Abbrev
	if !confirmed && !e.Builtin && !e.PublicDomain {
		a.win.Confirm.Ask("Add Bible", core.BibleAddQuestion(e), func() {
			a.event(func() { a.bibleAdd(nil, e.Abbrev, true) })
		})
		a.answer(cmd, nil)
		return
	}
	st := a.st()
	a.bm.task, a.bm.taskBible, a.bm.progress = "add", e.Abbrev, 0
	a.setBibleStatus("Adding "+e.Abbrev+"…", false)
	a.async(nil, func(ctx context.Context) func() error {
		var (
			bible  models.Bible
			verses []models.Verse
			err    error
		)
		if e.Builtin {
			var r bibletext.Result
			r, err = bundled.Load(e.Abbrev)
			bible, verses = r.Bible, r.Verses
		} else {
			var l biblefolder.Loaded
			l, err = biblefolder.Read(e.Path)
			bible, verses = l.Bible, l.Verses
		}
		if err == nil && !strings.EqualFold(bible.Abbrev, e.Abbrev) {
			err = fmt.Errorf("%s now holds %s: Refresh the list", filepath.Base(e.Path), bible.Abbrev)
		}
		const readShare = 0.15 // reading is about this much of the work
		a.bibleProgress("add", e.Abbrev, readShare)
		if err == nil {
			_, err = st.CreateBibleWithVersesProgress(ctx, bible, verses, func(done, total int) {
				a.bibleProgress("add", e.Abbrev, readShare+(1-readShare)*float64(done)/float64(max(total, 1)))
			})
		}
		if err == nil && e.Builtin {
			err = bundled.SetRemoved(ctx, st, e.Abbrev, false)
		}
		n := len(verses)
		return func() error {
			a.bm.task, a.bm.taskBible, a.bm.progress = "", "", 0
			if err != nil {
				if errors.Is(err, store.ErrExists) {
					err = fmt.Errorf("%s is already installed", e.Abbrev)
				}
				a.setBibleStatus("Could not add "+e.Abbrev, true)
				a.alert("Add Bible", fmt.Errorf("%s could not be added.\n\n%w", e.Abbrev, err))
				a.answer(cmd, err)
				a.refreshBibles(nil, a.bm.open)
				return err
			}
			log.Printf("[freebeams] added Bible %s (%s) from %s: %d verses", e.Abbrev, e.Name, core.BibleSourceLabel(e.Source), n)
			a.setBibleStatus("Added "+e.Abbrev+": "+core.VerseCountText(n), false)
			a.refreshBibles(cmd, a.bm.open)
			return nil
		}
	})
}

// removeBibleAt is "Remove Bible…" on Scriptures translation i (its context
// menu, Delete / Backspace): the question first.
func (a *App) removeBibleAt(i int) {
	if i >= 0 && i < len(a.bibles) {
		a.bibleRemove(nil, a.bibles[i].Abbrev, false)
	}
}

// bibleRemove deletes an installed translation and its verses from the
// library, after a question unless confirmed. Schedule items keep their
// slides. The Scriptures tab falls back to core.DefaultBible when it showed
// the removed translation (refreshBibles), and looks the reference up again.
func (a *App) bibleRemove(cmd *control.Command, abbrev string, confirmed bool) {
	if a.st() == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	if err := a.bibleBusy(); err != nil {
		a.answer(cmd, err)
		return
	}
	i := core.FindBible(a.bibles, strings.TrimSpace(abbrev))
	if i < 0 || strings.TrimSpace(abbrev) == "" {
		err := fmt.Errorf("%s is not installed", strings.TrimSpace(abbrev))
		a.bm.status, a.bm.statusErr = err.Error(), true
		a.answer(cmd, err)
		return
	}
	b := a.bibles[i]
	if !confirmed {
		e, err := a.bibleEntry(b.Abbrev)
		if err != nil {
			e = core.BibleEntry{Abbrev: b.Abbrev, Name: b.Name, Installed: true}
		}
		a.win.Confirm.Ask("Remove Bible", core.BibleRemoveQuestion(e), func() {
			a.event(func() { a.bibleRemove(nil, b.Abbrev, true) })
		})
		a.answer(cmd, nil)
		return
	}
	st := a.st()
	_, builtin := bundled.Lookup(b.Abbrev)
	a.bm.task, a.bm.taskBible, a.bm.progress = "remove", b.Abbrev, 0.5
	a.bm.sel = b.Abbrev
	a.setBibleStatus("Removing "+b.Abbrev+"…", false)
	a.async(nil, func(ctx context.Context) func() error {
		err := st.DeleteBible(ctx, b.ID)
		if err == nil && builtin {
			err = bundled.SetRemoved(ctx, st, b.Abbrev, true)
		}
		return func() error {
			a.bm.task, a.bm.taskBible, a.bm.progress = "", "", 0
			if err != nil {
				a.setBibleStatus("Could not remove "+b.Abbrev, true)
				a.alert("Remove Bible", fmt.Errorf("%s could not be removed.\n\n%w", b.Abbrev, err))
				a.answer(cmd, err)
				a.refreshBibles(nil, a.bm.open)
				return err
			}
			log.Printf("[freebeams] removed Bible %s (%s)", b.Abbrev, b.Name)
			a.setBibleStatus("Removed "+b.Abbrev, false)
			// Drop the removed translation from the list at once, so the
			// Scriptures tab never looks a reference up in it.
			kept := make([]models.Bible, 0, len(a.bibles))
			for _, x := range a.bibles {
				if x.ID != b.ID {
					kept = append(kept, x)
				}
			}
			a.applyBibles(kept, a.bm.counts)
			if strings.EqualFold(a.passageBible.Abbrev, b.Abbrev) && strings.TrimSpace(a.reference) != "" {
				a.lookupReference(a.reference, nil) // in the fallback translation
			}
			a.refreshBibles(cmd, a.bm.open)
			return nil
		}
	})
}

// bibleSavePackage writes an installed translation that has no package to
// the Bibles folder as <ABBREV>.fbbible (Save as Package), so it can be
// removed and added again, or copied to another computer.
func (a *App) bibleSavePackage(cmd *control.Command, abbrev string) {
	if a.st() == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	if err := a.bibleBusy(); err != nil {
		a.answer(cmd, err)
		return
	}
	e, err := a.bibleEntry(abbrev)
	dir := ""
	switch {
	case err != nil:
	case !e.Installed:
		err = fmt.Errorf("%s is not installed", e.Abbrev)
	case e.Builtin:
		err = fmt.Errorf("%s ships with FreeBeams: it is a package already", e.Abbrev)
	case e.HasPackage:
		err = fmt.Errorf("%s already has a package in the Bibles folder", e.Abbrev)
	default:
		dir, err = a.biblesDir()
	}
	path := ""
	if err == nil {
		path = biblefolder.PackagePath(dir, e.Abbrev)
		if _, serr := os.Stat(path); serr == nil {
			err = fmt.Errorf("%s already exists in the Bibles folder", filepath.Base(path))
		}
	}
	i := core.FindBible(a.bibles, e.Abbrev)
	if err == nil && i < 0 {
		err = fmt.Errorf("%s is not installed", e.Abbrev)
	}
	if err != nil {
		a.bm.status, a.bm.statusErr = err.Error(), true
		a.answer(cmd, err)
		return
	}
	b := a.bibles[i]
	st := a.st()
	a.bm.task, a.bm.taskBible, a.bm.progress = "save", b.Abbrev, 0.1
	a.bm.sel = b.Abbrev
	a.setBibleStatus("Saving "+b.Abbrev+" as a package…", false)
	a.async(nil, func(ctx context.Context) func() error {
		verses, err := st.AllVerses(ctx, b.ID)
		a.bibleProgress("save", b.Abbrev, 0.4)
		if err == nil {
			err = biblepkg.WriteFile(path, models.Bible{Abbrev: b.Abbrev, Name: b.Name, Language: b.Language, Copyright: b.Copyright}, verses)
		}
		return func() error {
			a.bm.task, a.bm.taskBible, a.bm.progress = "", "", 0
			if err != nil {
				a.setBibleStatus("Could not save "+b.Abbrev, true)
				a.alert("Save as Package", fmt.Errorf("%s could not be saved as a package.\n\n%w", b.Abbrev, err))
				a.answer(cmd, err)
				return err
			}
			log.Printf("[freebeams] saved Bible %s as %s: %d verses", b.Abbrev, path, len(verses))
			a.setBibleStatus("Saved "+b.Abbrev+" as "+filepath.Base(path), false)
			a.refreshBibles(cmd, true)
			return nil
		}
	})
}

// openBiblesFolder shows the Bibles folder in Finder / Explorer.
func (a *App) openBiblesFolder(cmd *control.Command) {
	dir, err := a.biblesDir()
	if err != nil {
		a.alert("Open Bibles Folder", err)
		a.answer(cmd, err)
		return
	}
	name, args := core.OpenFolderCommand(dir, runtime.GOOS)
	a.async(cmd, func(ctx context.Context) func() error {
		out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
		return func() error {
			if err != nil {
				err = fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
				a.fail("open Bibles folder", err)
				return err
			}
			log.Printf("[freebeams] opened %s", dir)
			return nil
		}
	})
}

// wireBibleManager connects the manager and the Scriptures list's
// Remove Bible… (called from buildUI).
func (a *App) wireBibleManager() {
	m := a.win.BibleManager
	abbrevAt := func(i int, then func(string)) func() {
		return func() {
			if entries := a.bibleEntries(); i >= 0 && i < len(entries) {
				a.bm.sel = entries[i].Abbrev
				then(entries[i].Abbrev)
			}
		}
	}
	m.OnSelect = func(i int) { a.event(func() { a.selectBibleEntry(i) }) }
	m.OnActivate = func(i int) {
		a.event(abbrevAt(i, func(ab string) {
			if e, err := a.bibleEntry(ab); err == nil && e.CanAdd() {
				a.bibleAdd(nil, ab, false)
			}
		}))
	}
	m.OnAdd = func(i int) { a.event(abbrevAt(i, func(ab string) { a.bibleAdd(nil, ab, false) })) }
	m.OnRemove = func(i int) { a.event(abbrevAt(i, func(ab string) { a.bibleRemove(nil, ab, false) })) }
	m.OnSave = func(i int) { a.event(abbrevAt(i, func(ab string) { a.bibleSavePackage(nil, ab) })) }
	m.OnAddFile = func() { a.event(a.importBibleFileDialog) }
	m.OnOpenFolder = func() { a.event(func() { a.openBiblesFolder(nil) }) }
	m.OnRefresh = func() { a.event(func() { a.refreshBibles(nil, true) }) }
	m.OnClose = func() { a.event(func() { a.bm.open = false }) }

	sc := a.win.Scriptures
	sc.OnManageBibles = func() { a.event(func() { a.openBibleManager(nil) }) }
	sc.OnRemoveBible = func(i int) { a.event(func() { a.removeBibleAt(i) }) }
}

// ---- view -------------------------------------------------------------------------

// bibleManagerValues is the dialog's projection of the state.
func (a *App) bibleManagerValues() ui.BibleManagerValues {
	entries := a.bibleEntries()
	v := ui.BibleManagerValues{Rows: make([]ui.BibleManagerRow, 0, len(entries)), Selected: core.FindBibleEntry(entries, a.bm.sel),
		Folder: a.bm.dir, Status: a.bm.status, Error: a.bm.statusErr, Busy: a.bm.task != "", Progress: a.bm.progress}
	for _, e := range entries {
		v.Rows = append(v.Rows, ui.BibleManagerRow{Name: core.BibleDisplayName(e.Abbrev, e.Name), Language: core.LanguageName(e.Language),
			Source: core.BibleSourceLabel(e.Source), Status: core.BibleStatusLabel(e.Installed), Size: core.SizeText(e.Size), Installed: e.Installed})
	}
	if v.Status == "" && a.bm.scanning {
		v.Status = "Reading the Bibles folder…"
	}
	if v.Status == "" {
		v.Status = fmt.Sprintf("%d installed · %d available", len(a.bibles), len(entries))
	}
	if i := v.Selected; i >= 0 {
		e := entries[i]
		v.Detail = core.BibleEntryDetail(e)
		v.CanAdd, v.CanDel, v.CanSave = e.CanAdd(), e.Installed, e.CanSavePackage()
	}
	return v
}

// bibleManagerView is what renderBibleManager last applied.
type bibleManagerView struct {
	values  ui.BibleManagerValues
	applied bool
}

// renderBibleManager projects the state onto the dialog (part of render).
func (a *App) renderBibleManager(v *bibleManagerView) {
	w := a.win.BibleManager
	if a.bm.open || w.Visible() {
		vals := a.bibleManagerValues()
		if !v.applied || !reflect.DeepEqual(vals, v.values) {
			v.values, v.applied = vals, true
			w.SetValues(vals)
		}
	}
	switch {
	case a.bm.open && !w.Visible():
		w.Open()
	case !a.bm.open && w.Visible():
		w.Hide()
	}
}

// bibleManagerStateDoc is /state.bibleManager.
func (a *App) bibleManagerStateDoc() core.BibleManagerState {
	return core.BibleManagerState{
		Open: a.bm.open, Folder: a.bm.dir, Scanning: a.bm.scanning, Scanned: a.bm.scanned,
		Task: a.bm.task, TaskBible: a.bm.taskBible, Progress: a.bm.progress,
		Status: a.bm.status, Error: a.bm.statusErr, Selected: a.bm.sel,
		Problems: append([]string{}, a.bm.problems...), Entries: a.bibleEntries(),
	}
}

// ---- control ----------------------------------------------------------------------

// handleBibleManagerCommand runs the Bible Manager's actions; false when the
// action is not one of them.
//
//	bibles-available                    list the Bibles folder again; /state.bibleManager.entries
//	bible-manager {action|mode: open|close}
//	bible-select {abbrev}               select a manager row
//	bible-add {abbrev | path, confirm}  add (copyrighted: asks unless confirm)
//	bible-remove {abbrev, confirm}      remove (asks unless confirm)
//	bible-save-package {abbrev}         Save as Package
//	bible-open-folder                   Open Bibles Folder
func (a *App) handleBibleManagerCommand(cmd *control.Command, args actionArgs) bool {
	switch cmd.Action {
	case "bibles-available":
		a.refreshBibles(cmd, true)
	case "bible-manager":
		switch op := strings.ToLower(strings.TrimSpace(args.Action + args.Mode)); {
		case op == "open" || (op == "" && (args.On == nil || *args.On)):
			a.openBibleManager(cmd)
		case op == "close" || (args.On != nil && !*args.On):
			a.bm.open = false
			a.answer(cmd, nil)
		default:
			a.answer(cmd, fmt.Errorf("bible-manager: unknown action %q (open|close)", op))
		}
	case "bible-select":
		e, err := a.bibleEntry(args.Abbrev)
		if err == nil {
			a.bm.sel = e.Abbrev
		}
		a.answer(cmd, err)
	case "bible-add":
		switch {
		case strings.TrimSpace(args.Abbrev) != "":
			a.bibleAdd(cmd, args.Abbrev, args.Confirm)
		case strings.TrimSpace(args.Path) != "":
			a.addBiblePath(cmd, args.Path, args.Confirm)
		default:
			a.answer(cmd, errors.New("bible-add: give an abbrev or a path"))
		}
	case "bible-remove":
		a.bibleRemove(cmd, args.Abbrev, args.Confirm)
	case "bible-save-package":
		a.bibleSavePackage(cmd, args.Abbrev)
	case "bible-open-folder":
		a.openBiblesFolder(cmd)
	default:
		return false
	}
	return true
}
