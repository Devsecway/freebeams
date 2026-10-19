package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"freebeams/internal/models"
)

func presentationTitles(items []models.Presentation) []string {
	out := make([]string, len(items))
	for i, p := range items {
		out[i] = p.Title
	}
	return out
}

func TestPresentationCRUD(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)

	p := models.Presentation{Path: "/docs/Sermon Notes.pdf"}
	before := time.Now().Add(-time.Second)
	if err := st.SavePresentation(ctx, &p); err != nil {
		t.Fatal(err)
	}
	if p.ID == 0 || p.Title != "Sermon Notes" || p.Pages != 0 || p.Added.Before(before) {
		t.Fatalf("after insert: %+v", p)
	}
	got, err := st.GetPresentation(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != p.Path || got.Title != "Sermon Notes" || got.Pages != 0 || got.CacheKey != "" || got.Renderer != "" ||
		got.Partial || !got.Added.Equal(p.Added) || got.Rendered() {
		t.Fatalf("GetPresentation = %+v, want %+v", got, p)
	}
	byPath, err := st.PresentationByPath(ctx, p.Path)
	if err != nil || byPath.ID != p.ID {
		t.Fatalf("PresentationByPath = %+v, %v", byPath, err)
	}

	// Update: the render result and a new title; Added is kept.
	got.Pages, got.CacheKey, got.Renderer, got.Partial, got.Title = 12, "abc-Sermon_Notes", "coregraphics", true, "Renamed"
	if err := st.SavePresentation(ctx, &got); err != nil {
		t.Fatal(err)
	}
	again, err := st.GetPresentation(ctx, p.ID)
	if err != nil || again.Pages != 12 || again.CacheKey != "abc-Sermon_Notes" || again.Renderer != "coregraphics" ||
		!again.Partial || again.Title != "Renamed" || !again.Added.Equal(p.Added) || !again.Rendered() {
		t.Fatalf("after update: %+v, %v", again, err)
	}

	// Validation, duplicates and missing rows.
	if err := st.SavePresentation(ctx, &models.Presentation{Path: "  "}); err == nil {
		t.Fatal("empty path must fail")
	}
	if err := st.SavePresentation(ctx, &models.Presentation{Path: "/x/notes.txt"}); err == nil {
		t.Fatal("a non-presentation file must fail")
	}
	if err := st.SavePresentation(ctx, nil); err == nil {
		t.Fatal("nil item must fail")
	}
	dup := models.Presentation{Path: p.Path}
	if err := st.SavePresentation(ctx, &dup); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate path: err = %v, want ErrExists", err)
	}
	missing := models.Presentation{ID: 9999, Path: "/x.pdf"}
	if err := st.SavePresentation(ctx, &missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing: err = %v, want ErrNotFound", err)
	}
	if _, err := st.GetPresentation(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetPresentation missing: %v", err)
	}
	if _, err := st.PresentationByPath(ctx, "/nope.pdf"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("PresentationByPath missing: %v", err)
	}

	// Count and delete.
	if n, err := st.CountPresentations(ctx); err != nil || n != 1 {
		t.Fatalf("CountPresentations = %d, %v", n, err)
	}
	if err := st.DeletePresentation(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeletePresentation(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: err = %v, want ErrNotFound", err)
	}
	if n, err := st.CountPresentations(ctx); err != nil || n != 0 {
		t.Fatalf("CountPresentations after delete = %d, %v", n, err)
	}
}

func TestSavePresentationsAtomic(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	first := &models.Presentation{Path: "/d/a.pdf"}
	if err := st.SavePresentation(ctx, first); err != nil {
		t.Fatal(err)
	}
	items := []*models.Presentation{{Path: "/d/b.pptx"}, {Path: "/d/a.pdf"}} // the second is a duplicate
	if err := st.SavePresentations(ctx, items); !errors.Is(err, ErrExists) {
		t.Fatalf("err = %v, want ErrExists", err)
	}
	if items[0].ID != 0 {
		t.Error("a failed batch must not assign IDs")
	}
	if n, _ := st.CountPresentations(ctx); n != 1 {
		t.Fatalf("count after failed batch = %d, want 1", n)
	}
	ok := []*models.Presentation{{Path: "/d/b.pptx", Title: " Deck B "}, {Path: "/d/c.key"}}
	if err := st.SavePresentations(ctx, ok); err != nil {
		t.Fatal(err)
	}
	if ok[0].ID == 0 || ok[1].ID == 0 || ok[0].Title != "Deck B" || ok[1].Title != "c" {
		t.Fatalf("batch saved = %+v, %+v", *ok[0], *ok[1])
	}
}

func TestSearchPresentations(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	for _, path := range []string{"/d/Easter Sunday.pdf", "/d/Christmas Eve.pptx", "/d/Youth_Night 50%.pdf", "/other/easter-kids.key", "/d/Noël Célébration.pdf"} {
		if err := st.SavePresentation(ctx, &models.Presentation{Path: path}); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		query string
		limit int
		want  []string
	}{
		{"", 0, []string{"Christmas Eve", "Easter Sunday", "easter-kids", "Noël Célébration", "Youth Night 50%"}},
		{"easter", 0, []string{"Easter Sunday", "easter-kids"}},
		{"noel celebration", 0, []string{"Noël Célébration"}}, // diacritics folded
		{"CÉLÉBRATION", 0, []string{"Noël Célébration"}},      // accented, any case
		{"easter", 1, []string{"Easter Sunday"}},              // the limit applies to the matches
		{"EASTER kids", 0, []string{"easter-kids"}},
		{"/other/", 0, []string{"easter-kids"}},
		{"50%", 0, []string{"Youth Night 50%"}}, // LIKE wildcards are literal
		{"_", 0, []string{"Youth Night 50%"}},   // the path keeps the underscore
		{"", 2, []string{"Christmas Eve", "Easter Sunday"}},
		{"nothing here", 0, []string{}},
	}
	for _, tc := range tests {
		got, err := st.SearchPresentations(ctx, tc.query, tc.limit)
		if err != nil {
			t.Fatalf("SearchPresentations(%q): %v", tc.query, err)
		}
		if !equalStrings(presentationTitles(got), tc.want) {
			t.Errorf("SearchPresentations(%q, %d) = %v, want %v", tc.query, tc.limit, presentationTitles(got), tc.want)
		}
	}
}
