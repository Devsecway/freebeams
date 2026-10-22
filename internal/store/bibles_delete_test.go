package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"freebeams/internal/models"
)

func TestDeleteBible(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	verses := []models.Verse{
		{Book: 43, Chapter: 3, Verse: 16, Text: "For God so loved the world"},
		{Book: 1, Chapter: 1, Verse: 1, Text: "In the beginning God created the heaven and the earth."},
	}
	a, err := st.CreateBibleWithVerses(ctx, models.Bible{Abbrev: "AAA"}, verses)
	if err != nil {
		t.Fatal(err)
	}
	var reports [][2]int
	b, err := st.CreateBibleWithVersesProgress(ctx, models.Bible{Abbrev: "BBB"}, verses[:1], func(done, total int) {
		reports = append(reports, [2]int{done, total})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reports, [][2]int{{1, 1}}) {
		t.Errorf("progress = %v", reports)
	}
	counts, err := st.BibleVerseCounts(ctx)
	if err != nil || counts[a.ID] != 2 || counts[b.ID] != 1 {
		t.Fatalf("counts = %v, %v", counts, err)
	}
	all, err := st.AllVerses(ctx, a.ID)
	if err != nil || len(all) != 2 || all[0].Book != 1 || all[1].Book != 43 {
		t.Fatalf("AllVerses = %+v, %v", all, err)
	}

	if err := st.DeleteBible(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	bibles, _ := st.ListBibles(ctx)
	if len(bibles) != 1 || bibles[0].Abbrev != "BBB" {
		t.Fatalf("bibles after delete = %+v", bibles)
	}
	if all, _ := st.AllVerses(ctx, a.ID); len(all) != 0 {
		t.Errorf("verses left: %+v", all)
	}
	var fts int
	if err := st.db.QueryRowContext(ctx, `SELECT count(*) FROM verses_fts WHERE rowid BETWEEN ? AND ?`, a.ID<<24, a.ID<<24|0xFFFFFF).Scan(&fts); err != nil || fts != 0 {
		t.Errorf("index rows left: %d, %v", fts, err)
	}
	// The other bible's search still works; the deleted one finds nothing.
	if hits, err := st.SearchVerses(ctx, b.ID, "loved world", 5); err != nil || len(hits) != 1 {
		t.Errorf("BBB search = %+v, %v", hits, err)
	}
	if hits, _ := st.SearchVerses(ctx, a.ID, "beginning", 5); len(hits) != 0 {
		t.Errorf("deleted bible still found: %+v", hits)
	}
	if err := st.DeleteBible(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
	// The abbreviation can be installed again (a removed built-in re-added),
	// even when SQLite reuses the deleted ID.
	again, err := st.CreateBibleWithVerses(ctx, models.Bible{Abbrev: "aaa"}, verses)
	if err != nil {
		t.Fatal(err)
	}
	if hits, err := st.SearchVerses(ctx, again.ID, "beginning", 5); err != nil || len(hits) != 1 {
		t.Errorf("re-added search = %+v, %v", hits, err)
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := st.DeleteBible(cctx, again.ID); err == nil {
		t.Error("cancelled delete: no error")
	}
}
