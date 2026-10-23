package core

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"freebeams/internal/models"
)

func TestDroppedKindOf(t *testing.T) {
	tests := []struct {
		path  string
		isDir bool
		want  DroppedKind
	}{
		{"/a/photo.JPG", false, DroppedMedia},
		{"/a/clip.mov", false, DroppedMedia},
		{"/a/song.mp3", false, DroppedMedia},
		{"/a/sermon.pdf", false, DroppedPresentation},
		{"/a/slides.PPTX", false, DroppedPresentation},
		{"/a/talk.key", false, DroppedPresentation},
		{"/a/service.fbs", false, DroppedSchedule},
		{"/a/old.FWS", false, DroppedSchedule},
		{"/a/KJV.fbbible", false, DroppedBible},
		{"/a/notes.txt", false, DroppedOther},
		{"/a/noext", false, DroppedOther},
		{"/a/Photos", true, DroppedFolder},
		{"/a/odd.png", true, DroppedFolder}, // a folder whatever its name
	}
	for _, tt := range tests {
		if got := DroppedKindOf(filepath.FromSlash(tt.path), tt.isDir); got != tt.want {
			t.Errorf("DroppedKindOf(%q, %v) = %q, want %q", tt.path, tt.isDir, got, tt.want)
		}
	}
}

func TestPlanFileDrop(t *testing.T) {
	p := func(s string) string { return filepath.FromSlash(s) }
	dirs := map[string]bool{p("/d/Photos"): true}
	files := map[string]bool{}
	for _, f := range []string{"/d/a.png", "/d/b.mp4", "/d/c.pdf", "/d/d.pptx", "/d/s.fbs", "/d/t.fbs", "/d/KJV.fbbible", "/d/notes.txt", "/d/song.mp3"} {
		files[p(f)] = true
	}
	stat := func(path string) (bool, bool) { return files[path] || dirs[path], dirs[path] }
	ins := func(pairs ...string) []DroppedFile {
		var out []DroppedFile
		for i := 0; i < len(pairs); i += 2 {
			out = append(out, DroppedFile{Path: p(pairs[i]), Kind: DroppedKind(pairs[i+1])})
		}
		return out
	}
	paths := func(ss ...string) []string {
		var out []string
		for _, s := range ss {
			out = append(out, p(s))
		}
		return out
	}
	skips := func(pairs ...string) []SkippedFile {
		var out []SkippedFile
		for i := 0; i < len(pairs); i += 2 {
			out = append(out, SkippedFile{Path: p(pairs[i]), Reason: pairs[i+1]})
		}
		return out
	}
	tests := []struct {
		name   string
		target string
		in     []string
		want   FileDropPlan
	}{
		{"nothing", FileDropSchedule, nil, FileDropPlan{Target: FileDropSchedule}},
		{"schedule: media and presentations keep their order", FileDropSchedule,
			paths("/d/c.pdf", "/d/a.png", "/d/song.mp3", "/d/d.pptx", "/d/b.mp4"),
			FileDropPlan{Target: FileDropSchedule,
				Media: paths("/d/a.png", "/d/song.mp3", "/d/b.mp4"), Presentations: paths("/d/c.pdf", "/d/d.pptx"),
				Insert: ins("/d/c.pdf", "presentation", "/d/a.png", "media", "/d/song.mp3", "media", "/d/d.pptx", "presentation", "/d/b.mp4", "media")}},
		{"schedule: blanks and repeats are ignored", FileDropSchedule, []string{"", "  ", p("/d/a.png"), p("/d/./a.png")},
			FileDropPlan{Target: FileDropSchedule, Media: paths("/d/a.png"), Insert: ins("/d/a.png", "media")}},
		{"schedule: unsupported, missing and folders are skipped", FileDropSchedule,
			paths("/d/notes.txt", "/d/a.png", "/d/gone.png", "/d/Photos"),
			FileDropPlan{Target: FileDropSchedule, Media: paths("/d/a.png"), Insert: ins("/d/a.png", "media"),
				Skipped: skips("/d/notes.txt", SkipUnsupported, "/d/gone.png", SkipMissing, "/d/Photos", SkipFolder)}},
		{"schedule: a schedule file opens, one per drop", FileDropSchedule, paths("/d/s.fbs", "/d/t.fbs"),
			FileDropPlan{Target: FileDropSchedule, Schedule: p("/d/s.fbs"), Skipped: skips("/d/t.fbs", SkipOneSchedule)}},
		{"schedule: a schedule pre-empts the resources dropped with it", FileDropSchedule,
			paths("/d/a.png", "/d/s.fbs", "/d/c.pdf", "/d/KJV.fbbible"),
			FileDropPlan{Target: FileDropSchedule, Schedule: p("/d/s.fbs"), Bibles: paths("/d/KJV.fbbible"),
				Skipped: skips("/d/a.png", SkipWithSchedule, "/d/c.pdf", SkipWithSchedule)}},
		{"schedule: a Bible package goes to the Bible Manager", FileDropSchedule, paths("/d/KJV.fbbible", "/d/b.mp4"),
			FileDropPlan{Target: FileDropSchedule, Bibles: paths("/d/KJV.fbbible"), Media: paths("/d/b.mp4"), Insert: ins("/d/b.mp4", "media")}},
		{"media: media files and folders only", FileDropMedia, paths("/d/a.png", "/d/Photos", "/d/c.pdf", "/d/s.fbs"),
			FileDropPlan{Target: FileDropMedia, Media: paths("/d/a.png", "/d/Photos"),
				Skipped: skips("/d/c.pdf", SkipUnsupported, "/d/s.fbs", SkipUnsupported)}},
		{"presentations: presentation files and folders only", FileDropPresentations, paths("/d/a.png", "/d/d.pptx", "/d/Photos"),
			FileDropPlan{Target: FileDropPresentations, Presentations: paths("/d/d.pptx", "/d/Photos"),
				Skipped: skips("/d/a.png", SkipUnsupported)}},
		{"outside: everything is skipped", "", paths("/d/a.png", "/d/gone.png"),
			FileDropPlan{Skipped: skips("/d/a.png", SkipOutside, "/d/gone.png", SkipMissing)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PlanFileDrop(tt.target, tt.in, stat)
			if got.Target != tt.want.Target || got.Schedule != tt.want.Schedule ||
				!slices.Equal(got.Media, tt.want.Media) || !slices.Equal(got.Presentations, tt.want.Presentations) ||
				!slices.Equal(got.Insert, tt.want.Insert) || !slices.Equal(got.Bibles, tt.want.Bibles) ||
				!slices.Equal(got.Skipped, tt.want.Skipped) {
				t.Fatalf("got  %+v\nwant %+v", got, tt.want)
			}
			if got.Empty() != (len(tt.want.Media) == 0 && len(tt.want.Presentations) == 0 && tt.want.Schedule == "" && len(tt.want.Bibles) == 0) {
				t.Fatalf("Empty() = %v", got.Empty())
			}
		})
	}
}

func TestFileDropInsertAt(t *testing.T) {
	tests := []struct {
		name             string
		gap, selected, n int
		want             int
	}{
		{"the gap under the pointer", 2, 0, 5, 2},
		{"the gap before the first row", 0, 3, 5, 0},
		{"the gap after the last row", 5, -1, 5, 5},
		{"a gap past the end is clamped", 9, -1, 5, 5},
		{"no gap: after the selected row", -1, 1, 5, 2},
		{"no gap: after the selected last row", -1, 4, 5, 5},
		{"no gap, no selection: the end", -1, -1, 5, 5},
		{"a stale selection: the end", -1, 7, 5, 5},
		{"an empty schedule", -1, -1, 0, 0},
	}
	for _, tt := range tests {
		if got := FileDropInsertAt(tt.gap, tt.selected, tt.n); got != tt.want {
			t.Errorf("%s: FileDropInsertAt(%d, %d, %d) = %d, want %d", tt.name, tt.gap, tt.selected, tt.n, got, tt.want)
		}
	}
}

func TestFileDropMessage(t *testing.T) {
	p := func(s string) string { return filepath.FromSlash(s) }
	tests := []struct {
		name    string
		target  string
		skipped []SkippedFile
		want    string
	}{
		{"nothing skipped", FileDropSchedule, nil, ""},
		{"one unsupported file on the schedule", FileDropSchedule, []SkippedFile{{p("/d/notes.txt"), SkipUnsupported}},
			"notes.txt: not a media file, presentation, schedule or Bible package."},
		{"several reasons, a line each, in a fixed order", FileDropSchedule, []SkippedFile{
			{p("/d/gone.png"), SkipMissing}, {p("/d/a.txt"), SkipUnsupported}, {p("/d/b.zip"), SkipUnsupported}, {p("/d/Photos"), SkipFolder}},
			"a.txt, b.zip: none of these is a media file, presentation, schedule or Bible package.\n" +
				"Photos: folders go onto the Media or Presentations tab.\ngone.png: not found."},
		{"many names are counted", FileDropMedia, []SkippedFile{
			{p("/d/1.pdf"), SkipUnsupported}, {p("/d/2.pdf"), SkipUnsupported}, {p("/d/3.pdf"), SkipUnsupported}, {p("/d/4.pdf"), SkipUnsupported}, {p("/d/5.pdf"), SkipUnsupported}},
			"1.pdf, 2.pdf, 3.pdf and 2 more: none of these is an image, video or audio file."},
		{"the Presentations tab", FileDropPresentations, []SkippedFile{{p("/d/a.png"), SkipUnsupported}},
			"a.png: not a PDF, PowerPoint or Keynote file."},
		{"outside the targets", "", []SkippedFile{{p("/d/a.png"), SkipOutside}},
			"a.png: drop files onto the schedule or the Media or Presentations tab."},
		{"with a schedule", FileDropSchedule, []SkippedFile{{p("/d/t.fbs"), SkipOneSchedule}, {p("/d/a.png"), SkipWithSchedule}},
			"t.fbs: one schedule opens at a time.\na.png: not added, the dropped schedule replaces the current one."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FileDropMessage(tt.target, tt.skipped); got != tt.want {
				t.Fatalf("got\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestPendingPresentationFill(t *testing.T) {
	doc := models.Presentation{ID: 7, Path: "/d/c.pdf", Title: "c"}
	pending := PendingPresentationItem(doc)
	if pending.Kind != models.ItemPresentation || pending.CacheKey != "" || len(pending.Slides) != 1 || pending.Source != doc.Path {
		t.Fatalf("pending item = %+v", pending)
	}
	if !strings.Contains(pending.Slides[0].Text, "Preparing") {
		t.Fatalf("pending slide = %+v", pending.Slides[0])
	}
	other := PresentationItem(models.Presentation{ID: 8, Path: "/d/other.pdf", Title: "other", Pages: 1, CacheKey: "k8"}, []string{"/cache/k8/1.png"})
	done := PresentationItem(models.Presentation{ID: 7, Path: "/d/c.pdf", Title: "c", Pages: 1, CacheKey: "old"}, []string{"/cache/old/1.png"})
	pending.ID, pending.Title = "x1", "Renamed by the operator"
	items := []models.ScheduleItem{other, pending, done, {Kind: models.ItemSong, Title: "song"}}

	if got := FillPendingPresentation(items, doc, nil); got != nil {
		t.Fatalf("no pages: changed %v", got)
	}
	rendered := doc
	rendered.Pages, rendered.CacheKey = 2, "k7"
	pages := []string{"/cache/k7/1.png", "/cache/k7/2.png"}
	if got := FillPendingPresentation(items, rendered, pages); !slices.Equal(got, []int{1}) {
		t.Fatalf("changed %v, want [1]", got)
	}
	it := items[1]
	if it.ID != "x1" || it.Title != "Renamed by the operator" || it.CacheKey != "k7" || len(it.Slides) != 2 ||
		it.Slides[1].Background.Path != pages[1] || it.PresentationID != 7 || !strings.Contains(it.Subtitle, "2 slides") {
		t.Fatalf("filled item = %+v", it)
	}
	if items[2].CacheKey != "old" || items[0].CacheKey != "k8" {
		t.Fatalf("other items changed: %+v / %+v", items[0], items[2])
	}
	if got := FillPendingPresentation(items, rendered, pages); got != nil {
		t.Fatalf("a second fill changed %v", got)
	}
}
