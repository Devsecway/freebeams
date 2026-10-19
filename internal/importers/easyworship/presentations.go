package easyworship

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"freeworship/internal/models"
	"freeworship/internal/presentations"
	"freeworship/internal/store"
)

// EasyWorship keeps presentation files (PowerPoint decks the user added)
// under Resources/Presentations; its Presentations.db lists them along with
// the built-in, non-file-based ones (a stock install has only
// "Introduction", with no file). The files are what FreeWorship can show,
// so they are found by scanning the folder.

// FindPresentations lists the presentation files under EasyWorship's
// Resources/Presentations folder (resourcesDir "" = DefaultResourcesDir()),
// as unsaved library items ordered by path. A missing folder yields no items.
func FindPresentations(resourcesDir string) (items []models.Presentation, warnings []string) {
	if resourcesDir == "" {
		resourcesDir = DefaultResourcesDir()
	}
	if resourcesDir == "" {
		return nil, nil
	}
	dir := filepath.Join(resourcesDir, "Presentations")
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return nil, nil
	}
	return presentations.Scan(dir)
}

// ImportPresentations registers EasyWorship's presentation files in st. It
// is idempotent: files already in the library (by path) are skipped. It
// does not render pages; the app does that in the background.
func ImportPresentations(ctx context.Context, st *store.Store, opt Options) (Report, error) {
	var rep Report
	if st == nil {
		return rep, errors.New("import EasyWorship presentations: nil store")
	}
	items, warnings := FindPresentations(opt.ResourcesDir)
	rep.Warnings = append(rep.Warnings, warnings...)
	if len(items) == 0 {
		return rep, nil
	}
	if err := ctx.Err(); err != nil {
		return rep, err
	}
	var added []*models.Presentation
	for i := range items {
		_, err := st.PresentationByPath(ctx, items[i].Path)
		switch {
		case err == nil:
			rep.PresentationsSkipped++
		case errors.Is(err, store.ErrNotFound):
			added = append(added, &items[i])
		default:
			return rep, fmt.Errorf("import EasyWorship presentations: %w", err)
		}
	}
	if len(added) > 0 {
		if err := st.SavePresentations(ctx, added); err != nil {
			return rep, fmt.Errorf("import EasyWorship presentations: %w", err)
		}
		rep.PresentationsImported = len(added)
	}
	return rep, nil
}
