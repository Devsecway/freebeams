package app

import (
	"context"
	"fmt"
	"log"

	"freeworship/internal/app/core"
	"freeworship/internal/bibles/bundled"
	"freeworship/internal/importers/easyworship"
)

// bundledStatus is the Scriptures footer text while the bundled
// translations install.
const bundledStatus = "Installing bundled Bibles…"

// startBundledInstall installs the bundled translations (KJV, ASV) the
// library lacks, off the main thread, when the library has not recorded the
// current bundle version (a first launch, or a newer bundle). The Scriptures
// footer reports progress; /state.bundled and /state.import.running reflect
// it. Abbreviations already present (an EasyWorship KJV in an older library)
// are skipped.
func (a *App) startBundledInstall() {
	if a.st == nil {
		return
	}
	needed, err := bundled.Needed(a.ctx, a.st)
	if err != nil {
		a.fail("read bundled Bibles setting", err)
	}
	if !needed {
		a.bundled = core.BundledState{Done: true, Version: bundled.Version}
		return
	}
	a.bundled = core.BundledState{Running: true}
	a.setScriptStatus(bundledStatus, false)
	st := a.st
	log.Printf("[freeworship] installing bundled Bibles %v (%d bytes embedded)", bundled.Abbrevs(), bundled.Size())
	a.async(nil, func(ctx context.Context) func() error {
		rep, err := bundled.Install(ctx, st)
		return func() error {
			a.bundled.Running = false
			if err != nil {
				a.bundled.Error = err.Error()
				a.setScriptStatus("Bundled Bibles could not be installed", true)
				a.alert("Bundled Bibles", fmt.Errorf("The bundled Bibles (KJV, ASV) could not be installed.\n\n%w", err))
				return err
			}
			a.bundled.Done, a.bundled.Version = true, rep.Version
			a.bundled.Installed, a.bundled.Skipped = rep.Installed, rep.Skipped
			log.Printf("[freeworship] bundled Bibles: installed %v, skipped %v (bundle version %d)", rep.Installed, rep.Skipped, rep.Version)
			if s := core.BundledSummary(rep); s != "" {
				a.setScriptStatus(s, false)
			} else if a.scriptStatus == bundledStatus {
				a.setScriptStatus("", false)
			}
			a.loadBibles(nil)
			return nil
		}
	})
}

// importDoc is /state.import: the EasyWorship import's state, whose running
// flag also covers the bundled install, so a first-launch wait on it covers
// both imports.
func (a *App) importDoc() core.ImportState {
	imp := a.importState
	if a.bundled.Running {
		imp.Running = true
	}
	return imp
}

// ewImportOptions are the first-run EasyWorship import's options: the
// bundled translations are skipped (reported "(bundled)"), so an EasyWorship
// KJV never duplicates or races the bundled one.
func (a *App) ewImportOptions() easyworship.Options {
	return easyworship.Options{BundledBibles: bundled.Abbrevs()}
}
