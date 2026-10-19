package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/importers/easyworship"
)

// scanEWBibles lists EasyWorship's Bible files off the main thread, so the
// Scriptures tab can offer the copyrighted ones (which the first-run import
// leaves out) for import.
func (a *App) scanEWBibles() {
	if a.st == nil || !easyworship.Installed() {
		return
	}
	a.async(nil, func(ctx context.Context) func() error {
		files, err := easyworship.FindBibles("")
		return func() error {
			if err != nil {
				a.fail("scan EasyWorship Bibles", err)
				return err
			}
			a.ewBibles = files
			a.biblesVer++ // the Scriptures tab re-evaluates what it can import
			return nil
		}
	})
}

// importCopyrightedBibles imports EasyWorship's copyrighted translations on
// request. It asks first unless confirmed: the operator takes responsibility
// for holding the licenses. cmd is answered once the translations list has
// been refreshed.
func (a *App) importCopyrightedBibles(cmd *control.Command, confirmed bool) {
	if a.st == nil {
		a.answer(cmd, errNoLibrary)
		return
	}
	if !confirmed {
		names := core.CopyrightedBiblesToImport(a.ewBibles, a.bibles)
		if len(names) == 0 {
			a.setScriptStatus("No copyrighted Bibles to import", false)
			a.answer(cmd, nil)
			return
		}
		a.win.Confirm.Ask("Import Copyrighted Bibles",
			"EasyWorship has these copyrighted translations: "+strings.Join(names, ", ")+".\n\n"+
				"Import them only if your church is licensed to use them. They stay in this computer's library.",
			func() { a.event(func() { a.importCopyrightedBibles(nil, true) }) })
		a.answer(cmd, nil)
		return
	}
	st := a.st
	a.setScriptStatus("Importing Bibles…", false)
	a.async(nil, func(ctx context.Context) func() error {
		rep, err := easyworship.ImportInto(ctx, st, easyworship.Options{BiblesOnly: true, IncludeCopyrightedBibles: true})
		return func() error {
			if err != nil {
				a.setScriptStatus("Bible import failed", true)
				a.alert("Import Bibles", fmt.Errorf("The Bible import failed.\n\n%w", err))
				a.answer(cmd, err)
				return err
			}
			summary := core.BibleImportSummary(rep)
			log.Printf("[freebeams] Bible import: %s", summary)
			for _, w := range rep.Warnings {
				log.Printf("[freebeams] import note: %s", w)
			}
			a.setScriptStatus(summary, false)
			a.loadBibles(cmd) // answers cmd with the refreshed list
			return nil
		}
	})
}
