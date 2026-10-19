package core

import (
	"strings"

	"freeworship/internal/importers/easyworship"
	"freeworship/internal/models"
)

// CopyrightedBiblesToImport lists the abbreviations of EasyWorship's
// copyrighted Bibles (files) that are not in the library (installed): what an
// on-request import (easyworship.Options.IncludeCopyrightedBibles) would add.
func CopyrightedBiblesToImport(files []easyworship.BibleFile, installed []models.Bible) []string {
	have := make(map[string]bool, len(installed))
	for _, b := range installed {
		have[strings.ToUpper(b.Abbrev)] = true
	}
	out := []string{}
	for _, f := range files {
		if !f.PublicDomain && !have[strings.ToUpper(f.Abbrev)] {
			out = append(out, f.Abbrev)
		}
	}
	return out
}

// BibleImportSummary is the Scriptures footer text after an on-request
// Bible import.
func BibleImportSummary(r easyworship.Report) string {
	if len(r.BiblesImported) > 0 {
		return "Imported " + strings.Join(r.BiblesImported, ", ")
	}
	return "No new Bibles to import"
}
