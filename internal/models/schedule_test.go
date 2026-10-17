package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func ids(s *Schedule) string {
	var out []string
	for _, it := range s.Items {
		out = append(out, it.ID)
	}
	return strings.Join(out, ",")
}

func TestNewItemID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := NewItemID()
		u, err := uuid.Parse(id)
		if err != nil || u.Version() != 4 {
			t.Fatalf("NewItemID() = %q: not a v4 uuid (%v)", id, err)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

func TestNewSchedule(t *testing.T) {
	s := NewSchedule()
	if s.Version != 1 || s.Title != "Untitled" || s.Dirty || len(s.Items) != 0 || s.Path != "" {
		t.Errorf("NewSchedule() = %+v", s)
	}
}

func TestScheduleAddInsert(t *testing.T) {
	s := NewSchedule()
	if i := s.Add(ScheduleItem{ID: "a"}); i != 0 || !s.Dirty {
		t.Fatalf("Add = %d, dirty %v", i, s.Dirty)
	}
	if i := s.Add(ScheduleItem{ID: "b"}); i != 1 {
		t.Fatalf("Add = %d", i)
	}
	i := s.Add(ScheduleItem{Title: "no id"})
	if i != 2 || s.Items[2].ID == "" {
		t.Fatalf("Add without id: index %d id %q", i, s.Items[2].ID)
	}
	s.Items[2].ID = "c"

	tests := []struct {
		at, want int
		order    string
	}{
		{0, 0, "x,a,b,c"},
		{2, 2, "a,b,x,c"},
		{3, 3, "a,b,c,x"},
		{99, 3, "a,b,c,x"},
		{-5, 0, "x,a,b,c"},
	}
	for _, tt := range tests {
		c := &Schedule{Items: append([]ScheduleItem(nil), s.Items...)}
		if got := c.Insert(tt.at, ScheduleItem{ID: "x"}); got != tt.want {
			t.Errorf("Insert(%d) = %d, want %d", tt.at, got, tt.want)
		}
		if got := ids(c); got != tt.order || !c.Dirty {
			t.Errorf("Insert(%d): order %s (dirty %v), want %s", tt.at, got, c.Dirty, tt.order)
		}
	}
	var empty Schedule
	if got := empty.Insert(5, ScheduleItem{}); got != 0 || len(empty.Items) != 1 || empty.Items[0].ID == "" {
		t.Errorf("Insert into empty: %d %+v", got, empty.Items)
	}
}

func scheduleOf(idList ...string) *Schedule {
	s := &Schedule{}
	for _, id := range idList {
		s.Items = append(s.Items, ScheduleItem{ID: id})
	}
	return s
}

func TestScheduleRemove(t *testing.T) {
	tests := []struct {
		i     int
		ok    bool
		order string
	}{
		{0, true, "b,c"}, {1, true, "a,c"}, {2, true, "a,b"}, {3, false, "a,b,c"}, {-1, false, "a,b,c"},
	}
	for _, tt := range tests {
		s := scheduleOf("a", "b", "c")
		if got := s.Remove(tt.i); got != tt.ok || s.Dirty != tt.ok || ids(s) != tt.order {
			t.Errorf("Remove(%d) = %v dirty %v order %s; want %v %s", tt.i, got, s.Dirty, ids(s), tt.ok, tt.order)
		}
	}
}

func TestScheduleMove(t *testing.T) {
	tests := []struct {
		from, to int
		ok       bool
		dirty    bool
		order    string
	}{
		{0, 2, true, true, "b,c,a,d"},
		{3, 0, true, true, "d,a,b,c"},
		{1, 2, true, true, "a,c,b,d"},
		{2, 1, true, true, "a,c,b,d"},
		{0, 3, true, true, "b,c,d,a"},
		{1, 1, true, false, "a,b,c,d"},
		{-1, 0, false, false, "a,b,c,d"},
		{0, 4, false, false, "a,b,c,d"},
		{4, 0, false, false, "a,b,c,d"},
	}
	for _, tt := range tests {
		s := scheduleOf("a", "b", "c", "d")
		got := s.Move(tt.from, tt.to)
		if got != tt.ok || s.Dirty != tt.dirty || ids(s) != tt.order {
			t.Errorf("Move(%d,%d) = %v dirty %v order %s; want %v %v %s",
				tt.from, tt.to, got, s.Dirty, ids(s), tt.ok, tt.dirty, tt.order)
		}
	}
}

func TestScheduleIndexOf(t *testing.T) {
	s := scheduleOf("a", "b", "c")
	for id, want := range map[string]int{"a": 0, "c": 2, "z": -1, "": -1} {
		if got := s.IndexOf(id); got != want {
			t.Errorf("IndexOf(%q) = %d, want %d", id, got, want)
		}
	}
}

func sampleSchedule() *Schedule {
	s := NewSchedule()
	song := Song{Title: "Amazing Grace", Author: "John Newton", Sections: ParseLyrics("[Verse 1]\nAmazing grace\n\n[Chorus]\nMy chains")}
	s.Add(ScheduleItem{
		Kind: ItemSong, Title: song.Title, Subtitle: song.Author, SongID: 42,
		Theme: DefaultSongTheme(), Slides: song.Slides(DefaultSlideOptions()),
	})
	p, _ := ParseReference("John 3:16-17")
	bg := Background{Kind: BackgroundImage, Path: "/tmp/bg.png"}
	slides := ScriptureSlides(p, []Verse{{Book: 43, Chapter: 3, Verse: 16, Text: "For God"}, {Book: 43, Chapter: 3, Verse: 17, Text: "For God sent"}}, "KJV", OneVersePerSlide)
	slides[0].Background = &bg
	s.Add(ScheduleItem{
		Kind: ItemScripture, Title: p.String(), Subtitle: "KJV", Passage: &p, BibleID: 1,
		Theme: DefaultScriptureTheme(), Slides: slides,
	})
	return s
}

func TestScheduleSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Sunday Service"+ScheduleFileExt)
	s := sampleSchedule()
	if !s.Dirty {
		t.Fatal("sample should be dirty")
	}
	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if s.Dirty || s.Path != path || s.Title != "Sunday Service" {
		t.Errorf("after Save: dirty %v path %q title %q", s.Dirty, s.Path, s.Title)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "{\n  \"version\": 1,") || !strings.HasSuffix(string(data), "}\n") {
		t.Errorf("file not indented JSON:\n%.200s", data)
	}
	if strings.Contains(string(data), "Dirty") || strings.Contains(string(data), dir) {
		t.Errorf("Path/Dirty must not be serialised")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temp files left behind: %v", entries)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("file mode %v, %v", info.Mode(), err)
	}

	loaded, err := LoadSchedule(path)
	if err != nil {
		t.Fatalf("LoadSchedule: %v", err)
	}
	if !reflect.DeepEqual(loaded, s) {
		t.Errorf("round trip mismatch\n got %+v\nwant %+v", loaded, s)
	}

	// A named schedule keeps its title when saved elsewhere.
	loaded.Title = "Easter"
	loaded.Add(ScheduleItem{Kind: ItemSong, Title: "x"})
	other := filepath.Join(dir, "copy.fws")
	if err := loaded.Save(other); err != nil {
		t.Fatal(err)
	}
	if loaded.Title != "Easter" || loaded.Path != other || loaded.Dirty {
		t.Errorf("save as: %+v", loaded)
	}
	// Overwrite in place.
	if err := loaded.Save(other); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
}

func TestScheduleSaveDefaults(t *testing.T) {
	dir := t.TempDir()
	s := &Schedule{Title: ""} // zero version, nil items
	path := filepath.Join(dir, "evening.service.fws")
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	if s.Version != 1 || s.Title != "evening.service" {
		t.Errorf("defaults: %+v", s)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"items": []`) {
		t.Errorf("nil items should be written as []: %s", data)
	}
}

func TestScheduleSaveError(t *testing.T) {
	dir := t.TempDir()
	s := sampleSchedule()
	bad := filepath.Join(dir, "missing", "dir", "x.fws")
	if err := s.Save(bad); err == nil {
		t.Fatal("Save into missing dir should fail")
	}
	if !s.Dirty || s.Path != "" || s.Title != "Untitled" {
		t.Errorf("failed save changed schedule: %+v", s)
	}
	// Rename onto a directory fails after the temp file was written; temp file is removed.
	target := filepath.Join(dir, "adir.fws")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(target); err == nil {
		t.Fatal("Save onto a directory should fail")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("temp file left behind: %v", entries)
	}
	// Unmarshalable content (NaN) fails cleanly.
	s.Items[0].Theme.Font.Size = float32NaN()
	if err := s.Save(filepath.Join(dir, "nan.fws")); err == nil {
		t.Error("Save with NaN should fail")
	}
}

func float32NaN() float32 {
	var zero float32
	return zero / zero
}

func TestLoadSchedule(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	tests := []struct {
		name, content string
		wantErr       string
		check         func(*Schedule) bool
	}{
		{"v1.fws", `{"version":1,"title":"Morning","items":[{"id":"a","kind":"song","title":"x","theme":{},"slides":[]}]}`, "",
			func(s *Schedule) bool {
				return s.Version == 1 && s.Title == "Morning" && len(s.Items) == 1 && s.Items[0].ID == "a"
			}},
		{"noversion.fws", `{"title":"Old"}`, "", func(s *Schedule) bool { return s.Version == 1 && s.Title == "Old" }},
		{"notitle.fws", `{"version":1}`, "", func(s *Schedule) bool { return s.Title == "notitle" }},
		{"future.fws", `{"version":2,"title":"x"}`, "unsupported file version 2", nil},
		{"negative.fws", `{"version":-1}`, "unsupported file version -1", nil},
		{"garbage.fws", `not json`, "load schedule", nil},
		{"wrongtype.fws", `{"version":"one"}`, "load schedule", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := write(tt.name, tt.content)
			s, err := LoadSchedule(path)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if s.Path != path || s.Dirty || !tt.check(s) {
				t.Errorf("loaded %+v", s)
			}
		})
	}
	if _, err := LoadSchedule(filepath.Join(dir, "absent.fws")); err == nil {
		t.Error("missing file should fail")
	}
}

func TestScheduleJSONShape(t *testing.T) {
	// The serialised field names are a file-format contract.
	s := sampleSchedule()
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"version"`, `"title"`, `"items"`, `"id"`, `"kind"`, `"songId"`, `"passage"`, `"bibleId"`, `"theme"`, `"slides"`} {
		if !strings.Contains(string(data), key) {
			t.Errorf("missing key %s", key)
		}
	}
}
