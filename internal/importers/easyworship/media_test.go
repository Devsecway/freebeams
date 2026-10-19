package easyworship

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"freeworship/internal/models"
)

func localResourcesDir() string { return filepath.Join(localEW, "Easyworship/Default/Resources") }

// writeMediaDB writes a synthetic Media.db (the columns ReadMedia uses, with
// EasyWorship's collation) and the Resources files its rows point at.
func writeMediaDB(t *testing.T, dataDir, resourcesDir string) {
	t.Helper()
	ensureCollations()
	db, err := sql.Open("sqlite", filepath.Join(dataDir, mediaDB))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mustExec(t, db, `CREATE TABLE media ("rowid" integer PRIMARY KEY AUTOINCREMENT NOT NULL UNIQUE, media_uid text UNIQUE,
		media_type int8, title text NOT NULL COLLATE UTF8_U_CI, author text COLLATE UTF8_U_CI, folder text, path text,
		filename text COLLATE UTF8_U_CI, width integer, height integer, duration int64)`)
	mustExec(t, db, `INSERT INTO media (rowid, media_type, title, folder, path, filename, width, height, duration) VALUES
		(3, 2, 'Zebra  Still', NULL, '<images>', 'zebra.jpg', 1920, 1080, 0),
		(1, 1, 'Clouds Loop', NULL, '<videos>', 'clouds #1.mp4', 1280, 720, 300633670),
		(2, 2, '', 'Sub', '<images>', 'nested.png', 800, 600, 400000),
		(4, 2, 'Missing File', NULL, '<images>', 'gone.jpg', 1920, 1080, 0),
		(5, 3, 'Some Audio', NULL, '<audio>', 'song.mp3', 0, 0, 1000000),
		(6, 2, 'Weird Location', NULL, '<dvd>', 'x.jpg', 0, 0, 0),
		(7, 1, 'Wrong Type', NULL, '<videos>', 'poster.jpg', 10, 10, 0),
		(8, 2, 'No Name', NULL, '<images>', '', 0, 0, 0)`)
	for _, f := range []string{"Images/zebra.jpg", "Videos/clouds #1.mp4", "Images/Sub/nested.png", "Videos/poster.jpg", "Audio/song.mp3"} {
		p := filepath.Join(resourcesDir, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadMediaSynthetic(t *testing.T) {
	dataDir, resDir := t.TempDir(), t.TempDir()
	writeMediaDB(t, dataDir, resDir)
	items, warnings, err := ReadMedia(dataDir, resDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("got %d items: %+v", len(items), items)
	}
	// EasyWorship's row order.
	clouds, nested, zebra, audio := items[0], items[1], items[2], items[3]
	if audio.Kind != models.MediaAudio || audio.Title != "Some Audio" || audio.Duration != 100*time.Millisecond ||
		audio.Width != 0 || audio.Path != filepath.Join(resDir, "Audio", "song.mp3") {
		t.Fatalf("audio = %+v", audio)
	}
	if clouds.Kind != models.MediaVideo || clouds.Title != "Clouds Loop" || clouds.Width != 1280 || clouds.Height != 720 ||
		clouds.Duration != 30063367*time.Microsecond || clouds.Path != filepath.Join(resDir, "Videos", "clouds #1.mp4") {
		t.Fatalf("video = %+v", clouds)
	}
	if nested.Kind != models.MediaImage || nested.Title != "nested" || nested.Duration != 0 ||
		nested.Path != filepath.Join(resDir, "Images", "Sub", "nested.png") {
		t.Fatalf("nested image = %+v", nested)
	}
	if zebra.Title != "Zebra Still" || zebra.Kind != models.MediaImage || zebra.Width != 1920 {
		t.Fatalf("zebra = %+v", zebra)
	}
	// Missing file, unknown location, a video row pointing at an image, and
	// an empty file name are reported.
	if len(warnings) != 4 {
		t.Fatalf("warnings = %q", warnings)
	}
	for i, want := range []string{"Missing File", "Weird Location", "Wrong Type", "No Name"} {
		if !strings.Contains(warnings[i], want) {
			t.Errorf("warning %d = %q, want it to mention %q", i, warnings[i], want)
		}
	}
	// No Media.db: no items, no error.
	items, warnings, err = ReadMedia(t.TempDir(), resDir)
	if err != nil || len(items) != 0 || len(warnings) != 0 {
		t.Fatalf("without Media.db: %v, %q, %v", items, warnings, err)
	}
	// A corrupt Media.db is an error.
	bad := t.TempDir()
	os.WriteFile(filepath.Join(bad, mediaDB), []byte("garbage"), 0o644)
	if _, _, err := ReadMedia(bad, resDir); err == nil {
		t.Fatal("corrupt Media.db: no error")
	}
}

func TestMediaFile(t *testing.T) {
	res := filepath.Join("/", "res")
	tests := []struct {
		path, folder, name string
		want               string
		ok                 bool
	}{
		{"<images>", "", "a.jpg", filepath.Join(res, "Images", "a.jpg"), true},
		{"<Videos>", "", "b.mp4", filepath.Join(res, "Videos", "b.mp4"), true},
		{"<images>", "Sub\\Deeper", "c.png", filepath.Join(res, "Images", "Sub", "Deeper", "c.png"), true},
		{"<images>", "/Sub/", "c.png", filepath.Join(res, "Images", "Sub", "c.png"), true},
		{filepath.Join("/", "Users", "me", "Pictures"), "", "d.jpg", filepath.Join("/", "Users", "me", "Pictures", "d.jpg"), true},
		{"relative/dir", "", "e.jpg", "", false},
		{"<audio>", "", "f.mp3", filepath.Join(res, "Audio", "f.mp3"), true},
		{"<feeds>", "", "f.xml", "", false},
		{"", "", "g.jpg", "", false},
		{"<images>", "", "  ", "", false},
	}
	for _, tc := range tests {
		got, ok := mediaFile(res, tc.path, tc.folder, tc.name)
		if got != tc.want || ok != tc.ok {
			t.Errorf("mediaFile(%q, %q, %q) = %q, %v; want %q, %v", tc.path, tc.folder, tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

func TestImportMediaSynthetic(t *testing.T) {
	ctx := context.Background()
	dataDir, resDir := t.TempDir(), t.TempDir()
	writeMediaDB(t, dataDir, resDir)
	st := openStore(t)
	opt := Options{SongsDir: dataDir, BiblesDir: t.TempDir(), ResourcesDir: resDir}

	rep, err := ImportMedia(ctx, st, opt)
	if err != nil {
		t.Fatal(err)
	}
	if rep.MediaImported != 4 || rep.MediaSkipped != 0 || len(rep.Warnings) != 4 {
		t.Fatalf("report = %+v", rep)
	}
	n, _ := st.CountMedia(ctx, "")
	v, _ := st.CountMedia(ctx, models.MediaVideo)
	if n != 4 || v != 1 {
		t.Fatalf("CountMedia = %d (%d videos)", n, v)
	}
	// Second run: nothing new.
	rep, err = ImportMedia(ctx, st, opt)
	if err != nil || rep.MediaImported != 0 || rep.MediaSkipped != 4 {
		t.Fatalf("second run = %+v, %v", rep, err)
	}
	// ImportInto covers media too, unless told not to.
	rep, err = ImportInto(ctx, st, opt)
	if err != nil || rep.MediaSkipped != 4 {
		t.Fatalf("ImportInto = %+v, %v", rep, err)
	}
	opt.SkipMedia = true
	rep, err = ImportInto(ctx, st, opt)
	if err != nil || rep.MediaSkipped != 0 || rep.MediaImported != 0 {
		t.Fatalf("ImportInto with SkipMedia = %+v, %v", rep, err)
	}
	if _, err := ImportMedia(ctx, nil, opt); err == nil {
		t.Fatal("nil store: no error")
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := ImportMedia(cctx, openStore(t), Options{SongsDir: dataDir, ResourcesDir: resDir}); err == nil {
		t.Fatal("cancelled context: no error")
	}
	// Titles are searchable in the store.
	found, err := st.SearchMedia(ctx, "zebra", "", 0)
	if err != nil || len(found) != 1 || found[0].Title != "Zebra Still" {
		t.Fatalf("SearchMedia = %+v, %v", found, err)
	}
}

// ── Real EasyWorship install (skipped when absent) ─────────────────────

func TestRealMedia(t *testing.T) {
	requireLocalEW(t)
	items, warnings, err := ReadMedia(localSongsDir(), localResourcesDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range warnings {
		t.Logf("warning: %s", w)
	}
	if len(items) < 20 {
		t.Fatalf("got %d items, want the 23 stock images and videos", len(items))
	}
	var videos, images int
	byTitle := map[string]models.Media{}
	for _, m := range items {
		byTitle[m.Title] = m
		switch m.Kind {
		case models.MediaVideo:
			videos++
			if m.Duration < 10*time.Second || m.Duration > 2*time.Minute || m.Width != 1280 || m.Height != 720 {
				t.Errorf("video %q: %v %dx%d", m.Title, m.Duration, m.Width, m.Height)
			}
		case models.MediaImage:
			images++
			if m.Duration != 0 || m.Width != 1920 || m.Height != 1080 {
				t.Errorf("image %q: %v %dx%d", m.Title, m.Duration, m.Width, m.Height)
			}
		}
		if fi, err := os.Stat(m.Path); err != nil || !fi.Mode().IsRegular() {
			t.Errorf("%q: file missing: %v", m.Title, err)
		}
		if kind, ok := models.MediaKindOf(m.Path); !ok || kind != m.Kind {
			t.Errorf("%q: kind %q for %s", m.Title, m.Kind, m.Path)
		}
	}
	if videos != 8 || images != 15 {
		t.Errorf("%d videos, %d images; want 8 and 15", videos, images)
	}
	if clouds, ok := byTitle["Heavenly Clouds Blue"]; !ok || clouds.Duration != 60*time.Second {
		t.Errorf("Heavenly Clouds Blue = %+v", clouds)
	}

	st := openStore(t)
	rep, err := ImportMedia(context.Background(), st, Options{})
	if err != nil || rep.MediaImported != len(items) {
		t.Fatalf("ImportMedia = %+v, %v", rep, err)
	}
}
