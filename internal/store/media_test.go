package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"freeworship/internal/models"
)

func mediaTitles(items []models.Media) []string {
	out := make([]string, len(items))
	for i, m := range items {
		out[i] = m.Title
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestMediaCRUD(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)

	m := models.Media{Path: "/media/Videos/Clip One.mp4", Duration: 30*time.Second + 37*time.Millisecond, Width: 1280, Height: 720}
	before := time.Now().Add(-time.Second)
	if err := st.SaveMedia(ctx, &m); err != nil {
		t.Fatal(err)
	}
	if m.ID == 0 || m.Kind != models.MediaVideo || m.Title != "Clip One" || m.Added.Before(before) {
		t.Fatalf("after insert: %+v", m)
	}
	got, err := st.GetMedia(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != m.Path || got.Kind != models.MediaVideo || got.Title != "Clip One" || got.Duration != 30037*time.Millisecond ||
		got.Width != 1280 || got.Height != 720 || got.ThumbPath != "" || got.TranscodedPath != "" || !got.Added.Equal(m.Added) {
		t.Fatalf("GetMedia = %+v, want %+v", got, m)
	}
	byPath, err := st.MediaByPath(ctx, m.Path)
	if err != nil || byPath.ID != m.ID {
		t.Fatalf("MediaByPath = %+v, %v", byPath, err)
	}

	// Update: derived files and a new title; Added is kept.
	got.ThumbPath, got.TranscodedPath, got.Title = "/cache/a.jpg", "/cache/a.ogv", "Renamed"
	if err := st.SaveMedia(ctx, &got); err != nil {
		t.Fatal(err)
	}
	again, err := st.GetMedia(ctx, m.ID)
	if err != nil || again.ThumbPath != "/cache/a.jpg" || again.TranscodedPath != "/cache/a.ogv" || again.Title != "Renamed" || !again.Added.Equal(m.Added) {
		t.Fatalf("after update: %+v, %v", again, err)
	}

	// Duplicates and missing rows.
	dup := models.Media{Path: m.Path, Kind: models.MediaVideo}
	if err := st.SaveMedia(ctx, &dup); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate path: err = %v, want ErrExists", err)
	}
	missing := models.Media{ID: 9999, Path: "/x.png", Kind: models.MediaImage}
	if err := st.SaveMedia(ctx, &missing); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing: err = %v, want ErrNotFound", err)
	}
	if _, err := st.GetMedia(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetMedia missing: %v", err)
	}
	if _, err := st.MediaByPath(ctx, "/nope.png"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("MediaByPath missing: %v", err)
	}
	if err := st.SaveMedia(ctx, &models.Media{Path: "/notes.txt"}); err == nil {
		t.Fatal("saving a non-media file succeeded")
	}
	if err := st.SaveMedia(ctx, &models.Media{Path: "  "}); err == nil {
		t.Fatal("saving an empty path succeeded")
	}
	if err := st.SaveMedia(ctx, &models.Media{Path: "/a.png", Kind: "audio"}); err == nil {
		t.Fatal("saving an unknown kind succeeded")
	}
	if err := st.SaveMedia(ctx, nil); err == nil {
		t.Fatal("nil item succeeded")
	}

	// Delete.
	if err := st.DeleteMedia(ctx, m.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteMedia(ctx, m.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if n, _ := st.CountMedia(ctx, ""); n != 0 {
		t.Fatalf("CountMedia after delete = %d", n)
	}
}

func TestSaveMediaItemsAtomic(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	items := []*models.Media{
		{Path: "/m/a.png"},
		{Path: "/m/b.mp4"},
		{Path: "/m/a.png"}, // duplicate within the batch
	}
	err := st.SaveMediaItems(ctx, items)
	if !errors.Is(err, ErrExists) {
		t.Fatalf("batch with a duplicate: %v", err)
	}
	if items[0].ID != 0 || items[1].ID != 0 {
		t.Fatalf("ids assigned on a failed batch: %+v %+v", items[0], items[1])
	}
	if n, _ := st.CountMedia(ctx, ""); n != 0 {
		t.Fatalf("rows after a failed batch = %d", n)
	}
	if err := st.SaveMediaItems(ctx, items[:2]); err != nil {
		t.Fatal(err)
	}
	if items[0].ID == 0 || items[1].ID == 0 || items[0].Kind != models.MediaImage || items[1].Kind != models.MediaVideo {
		t.Fatalf("batch result: %+v %+v", items[0], items[1])
	}
}

func TestSearchMedia(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	seed := []*models.Media{
		{Path: "/Resources/Images/The Cross Loop 04 Still.jpg"},
		{Path: "/Resources/Images/Snowfall Loop 02 Still.jpg"},
		{Path: "/Resources/Videos/boho_cross_loop_16.mp4", Title: "Boho Cross White Corner Cross"},
		{Path: "/Resources/Videos/plexus.mp4", Title: "Plexus Orange"},
		{Path: "/Other/100%_done.png", Title: "100%_done"},
		{Path: "/Other/Zebra.webp", Title: "Zebra"},
	}
	if err := st.SaveMediaItems(ctx, seed); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		query string
		kind  models.MediaKind
		limit int
		want  []string
	}{
		{"all, by title", "", "", 0, []string{"100%_done", "Boho Cross White Corner Cross", "Plexus Orange", "Snowfall Loop 02 Still", "The Cross Loop 04 Still", "Zebra"}},
		{"images only", "", models.MediaImage, 0, []string{"100%_done", "Snowfall Loop 02 Still", "The Cross Loop 04 Still", "Zebra"}},
		{"videos only", "", models.MediaVideo, 0, []string{"Boho Cross White Corner Cross", "Plexus Orange"}},
		{"word, any case", "CROSS", "", 0, []string{"Boho Cross White Corner Cross", "The Cross Loop 04 Still"}},
		{"word and kind", "cross", models.MediaVideo, 0, []string{"Boho Cross White Corner Cross"}},
		{"two words must both match", "loop still", "", 0, []string{"Snowfall Loop 02 Still", "The Cross Loop 04 Still"}},
		{"matches the path too", "videos", "", 0, []string{"Boho Cross White Corner Cross", "Plexus Orange"}},
		{"LIKE wildcards are literal", "100%", "", 0, []string{"100%_done"}},
		{"underscore is literal", "_done", "", 0, []string{"100%_done"}},
		{"no match", "nothing here", "", 0, []string{}},
		{"limit", "", "", 2, []string{"100%_done", "Boho Cross White Corner Cross"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := st.SearchMedia(ctx, tc.query, tc.kind, tc.limit)
			if err != nil {
				t.Fatal(err)
			}
			if !equalStrings(mediaTitles(got), tc.want) {
				t.Fatalf("SearchMedia(%q, %q) = %v, want %v", tc.query, tc.kind, mediaTitles(got), tc.want)
			}
		})
	}
	counts := map[models.MediaKind]int{"": 6, models.MediaImage: 4, models.MediaVideo: 2}
	for kind, want := range counts {
		if n, err := st.CountMedia(ctx, kind); err != nil || n != want {
			t.Errorf("CountMedia(%q) = %d, %v; want %d", kind, n, err, want)
		}
	}
}
