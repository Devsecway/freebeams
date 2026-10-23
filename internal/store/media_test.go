package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"freebeams/internal/models"
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
	if err := st.SaveMedia(ctx, &models.Media{Path: "/a.png", Kind: "feed"}); err == nil {
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

func TestMediaOptions(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)

	// Defaults on insert, and the whole-row save round-trips a choice.
	m := models.Media{Path: "/m/clip.mp4"}
	if err := st.SaveMedia(ctx, &m); err != nil {
		t.Fatal(err)
	}
	if m.Aspect != models.AspectFill || m.Repeat != models.RepeatLoop {
		t.Fatalf("defaults after insert: aspect %q repeat %q", m.Aspect, m.Repeat)
	}
	m.Aspect, m.Repeat = models.AspectFit, models.RepeatOnce
	if err := st.SaveMedia(ctx, &m); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.GetMedia(ctx, m.ID); got.Aspect != models.AspectFit || got.Repeat != models.RepeatOnce {
		t.Fatalf("after SaveMedia: aspect %q repeat %q", got.Aspect, got.Repeat)
	}

	tests := []struct {
		name       string
		aspect     models.MediaAspect
		repeat     models.MediaRepeat
		wantAspect models.MediaAspect
		wantRepeat models.MediaRepeat
		wantErr    bool
	}{
		{"stretch, loop", models.AspectStretch, models.RepeatLoop, models.AspectStretch, models.RepeatLoop, false},
		{"fit, once", models.AspectFit, models.RepeatOnce, models.AspectFit, models.RepeatOnce, false},
		{"empty is the default", "", "", models.AspectFill, models.RepeatLoop, false},
		{"unknown aspect is rejected", "zoom", models.RepeatLoop, models.AspectFill, models.RepeatLoop, true},
		{"unknown repeat is rejected", models.AspectFill, "twice", models.AspectFill, models.RepeatLoop, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := st.SetMediaOptions(ctx, m.ID, tc.aspect, tc.repeat)
			if (err != nil) != tc.wantErr {
				t.Fatalf("SetMediaOptions: err = %v, want error %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			got, err := st.GetMedia(ctx, m.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Aspect != tc.wantAspect || got.Repeat != tc.wantRepeat {
				t.Fatalf("after SetMediaOptions: aspect %q repeat %q; want %q %q", got.Aspect, got.Repeat, tc.wantAspect, tc.wantRepeat)
			}
			// The search lists the same values.
			list, err := st.SearchMedia(ctx, "", "", 0)
			if err != nil || len(list) != 1 || list[0].Aspect != tc.wantAspect || list[0].Repeat != tc.wantRepeat {
				t.Fatalf("SearchMedia = %+v, %v", list, err)
			}
		})
	}
	if err := st.SetMediaOptions(ctx, 9999, models.AspectFit, models.RepeatLoop); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing id: %v", err)
	}
	if err := st.SaveMedia(ctx, &models.Media{Path: "/m/x.png", Aspect: "zoom"}); err == nil {
		t.Fatal("saving an unknown aspect succeeded")
	}
	if err := st.SaveMedia(ctx, &models.Media{Path: "/m/x.png", Repeat: "twice"}); err == nil {
		t.Fatal("saving an unknown repeat mode succeeded")
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
		{Path: "/Other/Café Lumière.jpg"},
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
		{"all, by title", "", "", 0, []string{"100%_done", "Boho Cross White Corner Cross", "Café Lumière", "Plexus Orange", "Snowfall Loop 02 Still", "The Cross Loop 04 Still", "Zebra"}},
		{"images only", "", models.MediaImage, 0, []string{"100%_done", "Café Lumière", "Snowfall Loop 02 Still", "The Cross Loop 04 Still", "Zebra"}},
		{"diacritics folded", "cafe lumiere", "", 0, []string{"Café Lumière"}},
		{"accented query, any case", "LUMIÈRE", "", 0, []string{"Café Lumière"}},
		{"limit applies to the matches", "cross", "", 1, []string{"Boho Cross White Corner Cross"}},
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
	counts := map[models.MediaKind]int{"": 7, models.MediaImage: 5, models.MediaVideo: 2}
	for kind, want := range counts {
		if n, err := st.CountMedia(ctx, kind); err != nil || n != want {
			t.Errorf("CountMedia(%q) = %d, %v; want %d", kind, n, err, want)
		}
	}
}

func TestMediaProperties(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	m := models.Media{Path: "/m/walk_in loop.mp4", Kind: models.MediaVideo}
	if err := st.SaveMedia(ctx, &m); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		p       models.MediaProperties
		want    models.Media // Title, Tags, Aspect, Repeat
		wantErr bool
	}{
		{"everything", models.MediaProperties{Title: " Walk-in ", Tags: "intro, Calm,intro", Aspect: models.AspectFit, Repeat: models.RepeatOnce},
			models.Media{Title: "Walk-in", Tags: "intro, Calm", Aspect: models.AspectFit, Repeat: models.RepeatOnce}, false},
		{"a blank title keeps the title", models.MediaProperties{Title: " ", Tags: "", Aspect: models.AspectStretch, Repeat: models.RepeatLoop},
			models.Media{Title: "Walk-in", Tags: "", Aspect: models.AspectStretch, Repeat: models.RepeatLoop}, false},
		{"an unknown aspect is refused", models.MediaProperties{Title: "x", Aspect: "zoom"},
			models.Media{Title: "Walk-in", Aspect: models.AspectStretch, Repeat: models.RepeatLoop}, true},
	}
	for _, tc := range tests {
		err := st.SetMediaProperties(ctx, m.ID, tc.p)
		if (err != nil) != tc.wantErr {
			t.Fatalf("%s: err = %v", tc.name, err)
		}
		got, _ := st.GetMedia(ctx, m.ID)
		if got.Title != tc.want.Title || got.Tags != tc.want.Tags || got.Aspect != tc.want.Aspect || got.Repeat != tc.want.Repeat {
			t.Fatalf("%s: got %q %q %q %q", tc.name, got.Title, got.Tags, got.Aspect, got.Repeat)
		}
	}
	// The pipeline's whole-row save leaves the tags alone; the search finds them.
	if err := st.SetMediaProperties(ctx, m.ID, models.MediaProperties{Tags: "sunrise"}); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetMedia(ctx, m.ID)
	got.ThumbPath = "/c/t.jpg"
	got.Tags = ""
	if err := st.SaveMedia(ctx, &got); err != nil {
		t.Fatal(err)
	}
	if res, err := st.SearchMedia(ctx, "sunrise", "", 0); err != nil || len(res) != 1 || res[0].Tags != "sunrise" {
		t.Fatalf("search by tag = %+v, %v", res, err)
	}
	if err := st.SetMediaProperties(ctx, 999, models.MediaProperties{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing id: %v", err)
	}
}
