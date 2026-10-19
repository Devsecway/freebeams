package easyworship

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFindAndImportPresentationsSynthetic(t *testing.T) {
	ctx := context.Background()
	resDir := t.TempDir()
	// No Presentations folder: nothing, no warnings.
	if items, warnings := FindPresentations(resDir); len(items) != 0 || len(warnings) != 0 {
		t.Fatalf("FindPresentations without the folder = %v, %v", items, warnings)
	}
	for _, f := range []string{"Presentations/Welcome.pptx", "Presentations/Old/Notes.pdf", "Presentations/readme.txt", "Images/x.jpg"} {
		p := filepath.Join(resDir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	items, warnings := FindPresentations(resDir)
	if len(items) != 2 || len(warnings) != 0 || items[0].Title != "Notes" || items[1].Title != "Welcome" {
		t.Fatalf("FindPresentations = %+v, %v", items, warnings)
	}

	st := openStore(t)
	opt := Options{ResourcesDir: resDir}
	rep, err := ImportPresentations(ctx, st, opt)
	if err != nil || rep.PresentationsImported != 2 || rep.PresentationsSkipped != 0 {
		t.Fatalf("ImportPresentations = %+v, %v", rep, err)
	}
	if n, _ := st.CountPresentations(ctx); n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}
	// Idempotent.
	rep, err = ImportPresentations(ctx, st, opt)
	if err != nil || rep.PresentationsImported != 0 || rep.PresentationsSkipped != 2 {
		t.Fatalf("second ImportPresentations = %+v, %v", rep, err)
	}
	if _, err := ImportPresentations(ctx, nil, opt); err == nil {
		t.Fatal("nil store must fail")
	}
}

func TestRealPresentations(t *testing.T) {
	requireLocalEW(t)
	items, warnings := FindPresentations(localResourcesDir())
	t.Logf("EasyWorship presentation files: %d (%d warnings)", len(items), len(warnings))
	for _, it := range items {
		t.Logf("  %s", it.Path)
	}
}
