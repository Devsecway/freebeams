package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"graphics.gd/classdb/DisplayServer"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/importers/bibletext"
	"freebeams/internal/importers/easyworship"
	"freebeams/internal/models"
	"freebeams/internal/store"
)

// bibleFileImport is a parsed Bible file waiting for the operator's
// confirmation (importBibleFile).
type bibleFileImport struct {
	path     string
	bible    models.Bible
	verses   []models.Verse
	warnings []string
}

// bibleFileFilters are the native dialog filters for Import Bible File….
var bibleFileFilters = []string{
	"*.json, *.ewb ; Bible files",
	"*.json ; Bible JSON (a bible-translations folder or chapter, a single file, or an en_kjv.json array)",
	"*.ewb ; EasyWorship Bible",
}

// importBibleFileDialog shows the native Open dialog for a Bible file or a
// translation folder (the Scriptures gear menu's Import Bible File…).
func (a *App) importBibleFileDialog() {
	err := DisplayServer.FileDialogShow("Import Bible File", a.dialogDir(), "", false, DisplayServer.FileDialogModeOpenAny,
		bibleFileFilters, func(ok bool, paths []string, _ int) {
			if !ok || len(paths) == 0 {
				return
			}
			a.event(func() { a.importBibleFile(nil, paths[0], false) })
		}, 0)
	if err != nil {
		a.alert("Import Bible File", fmt.Errorf("Could not show the Open dialog.\n\n%w", err))
	}
}

// readBibleFile parses a Bible file or folder: .ewb through the EasyWorship
// reader, everything else through bibletext (a translation folder, a chapter
// file inside one, or a single-file Bible JSON).
func readBibleFile(path string) (*bibleFileImport, error) {
	if strings.EqualFold(filepath.Ext(path), ".ewb") {
		b, verses, err := easyworship.ReadBible(path)
		if err != nil {
			return nil, err
		}
		if len(verses) == 0 {
			return nil, fmt.Errorf("read bible %s: no verses", filepath.Base(path))
		}
		return &bibleFileImport{path: path, bible: b, verses: verses}, nil
	}
	res, err := bibletext.Read(path)
	if err != nil {
		return nil, err
	}
	return &bibleFileImport{path: path, bible: res.Bible, verses: res.Verses, warnings: res.Warnings}, nil
}

// importBibleFile imports the translation in a file or folder the operator
// chose. The file is parsed off the main thread; unless confirmed, the
// operator is then asked (the question names the translation and states the
// licence responsibility); on OK the translation is written in one
// transaction and the translations list refreshed. A translation whose
// abbreviation is installed is reported, not imported twice. Errors go to the
// message dialog. cmd is answered when the import is done, or with the
// question on screen.
func (a *App) importBibleFile(cmd *control.Command, path string, confirmed bool) {
	if a.st() == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	path = strings.TrimSpace(path)
	if path == "" {
		a.answer(cmd, errors.New("import-bible-file: missing path"))
		return
	}
	if f := a.bibleFile; f != nil && f.path == path {
		a.confirmOrImportBibleFile(cmd, f, confirmed)
		return
	}
	a.setScriptStatus("Reading "+filepath.Base(path)+"…", false)
	a.async(nil, func(ctx context.Context) func() error {
		f, err := readBibleFile(path)
		return func() error {
			if err != nil {
				a.bibleFile = nil
				a.setScriptStatus("Could not read "+filepath.Base(path), true)
				a.alert("Import Bible File", fmt.Errorf("The file could not be read as a Bible.\n\n%w", err))
				a.answer(cmd, err)
				return err
			}
			a.bibleFile = f
			a.confirmOrImportBibleFile(cmd, f, confirmed)
			return nil
		}
	})
}

// confirmOrImportBibleFile asks about a parsed file, or imports it.
func (a *App) confirmOrImportBibleFile(cmd *control.Command, f *bibleFileImport, confirmed bool) {
	if core.FindBible(a.bibles, f.bible.Abbrev) >= 0 {
		a.bibleFile = nil
		err := fmt.Errorf("%s is already installed", f.bible.Abbrev)
		a.setScriptStatus(err.Error(), true)
		a.alert("Import Bible File", fmt.Errorf("%w, so %s was not imported.", err, filepath.Base(f.path)))
		a.answer(cmd, err)
		return
	}
	if !confirmed {
		a.setScriptStatus("", false)
		path := f.path
		a.win.Confirm.Ask("Import Bible File", core.BibleFileQuestion(f.bible, len(f.verses), f.path),
			func() { a.event(func() { a.importBibleFile(nil, path, true) }) })
		a.answer(cmd, nil)
		return
	}
	st := a.st()
	a.setScriptStatus("Importing "+f.bible.Abbrev+"…", false)
	a.async(nil, func(ctx context.Context) func() error {
		_, err := st.CreateBibleWithVerses(ctx, f.bible, f.verses)
		return func() error {
			a.bibleFile = nil
			if err != nil {
				if errors.Is(err, store.ErrExists) {
					err = fmt.Errorf("%s is already installed", f.bible.Abbrev)
				}
				a.setScriptStatus("Bible import failed", true)
				a.alert("Import Bible File", fmt.Errorf("The Bible import failed.\n\n%w", err))
				a.answer(cmd, err)
				return err
			}
			log.Printf("[freebeams] imported Bible %s (%s) from %s: %d verses, %d notes",
				f.bible.Abbrev, f.bible.Name, f.path, len(f.verses), len(f.warnings))
			for _, w := range f.warnings {
				log.Printf("[freebeams] import note: %s", w)
			}
			a.setScriptStatus(core.BibleFileSummary(f.bible, len(f.verses)), false)
			a.loadBibles(cmd) // answers cmd with the refreshed list
			return nil
		}
	})
}
