package easyworship

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
)

// TestImportIntoSkipsBundledBibles: the translations FreeBeams bundles are
// reported as skipped, whatever their case, so an EasyWorship KJV never
// duplicates the bundled one.
func TestImportIntoSkipsBundledBibles(t *testing.T) {
	ctx := context.Background()
	biblesDir := t.TempDir()
	writeEWB(t, filepath.Join(biblesDir, "KJV.ewb"), "King James Version", "KJV", "EN", "", 17,
		[][]fakeVerse{{{1, 1, 1, "In the beginning God created the heaven and the earth. "}}})
	writeEWB(t, filepath.Join(biblesDir, "RVA.ewb"), "Reina-Valera Antigua", "RVA", "ES", "", 36,
		[][]fakeVerse{{{1, 1, 1, "En el principio crió Dios los cielos y la tierra. "}}})
	writeEWB(t, filepath.Join(biblesDir, "HCSB.ewb"), "Holman Christian Standard Bible", "HCSB", "EN", "", 14,
		[][]fakeVerse{{{1, 1, 1, "In the beginning God created the heavens and the earth. "}}})

	st := openStore(t)
	rep, err := ImportInto(ctx, st, Options{BiblesOnly: true, BiblesDir: biblesDir, BundledBibles: []string{"kjv", " ASV "}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rep.BiblesImported, []string{"RVA"}) {
		t.Errorf("imported %q, want RVA", rep.BiblesImported)
	}
	if !slices.Equal(rep.BiblesSkipped, []string{"HCSB (copyrighted)", "KJV (bundled)"}) {
		t.Errorf("skipped %q", rep.BiblesSkipped)
	}
	bibles, _ := st.ListBibles(ctx)
	if len(bibles) != 1 || bibles[0].Abbrev != "RVA" {
		t.Errorf("bibles = %+v", bibles)
	}
}
