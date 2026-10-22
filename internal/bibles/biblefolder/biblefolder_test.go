package biblefolder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"freebeams/internal/bibles/biblepkg"
	"freebeams/internal/models"
)

var kjvLike = []models.Verse{
	{Book: 1, Chapter: 1, Verse: 1, Text: "In the beginning God created the heaven and the earth."},
	{Book: 43, Chapter: 3, Verse: 16, Text: "For God so loved the world, that he gave his only begotten Son."},
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fixture fills dir with one translation of every kind, plus files the scan
// ignores or reports.
func fixture(t *testing.T, dir string) {
	t.Helper()
	if err := biblepkg.WriteFile(filepath.Join(dir, "PKG.fbbible"), models.Bible{Abbrev: "PKG", Name: "Package Bible", Language: "en", Copyright: "Public Domain"}, kjvLike); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "obj.json"), `{"abbrev":"OBJ","name":"Object Bible","language":"es","books":{"John":{"3":{"16":"Porque de tal manera amó Dios al mundo."}}}}`)
	books := make([]map[string]any, 66)
	for i := range books {
		books[i] = map[string]any{"abbrev": "x", "name": "x", "chapters": [][]string{}}
	}
	books[0]["chapters"] = [][]string{{"In the beginning."}}
	arr, _ := json.Marshal(books)
	write(t, filepath.Join(dir, "en_arr.json"), string(arr))
	write(t, filepath.Join(dir, "FLD", "new", "John", "3.json"), `{"16": "Folder verse sixteen.", "17": "Folder verse seventeen."}`)
	// Ignored: hidden, other types, a folder that is not a translation.
	write(t, filepath.Join(dir, ".hidden.fbbible"), "x")
	write(t, filepath.Join(dir, "notes.txt"), "x")
	write(t, filepath.Join(dir, "Other", "readme.md"), "x")
	// Reported: unreadable files, and a duplicate abbreviation.
	write(t, filepath.Join(dir, "broken.fbbible"), "not gzip")
	write(t, filepath.Join(dir, "broken.json"), "{")
	write(t, filepath.Join(dir, "dup.json"), `{"abbrev":"pkg","books":{"John":{"3":{"16":"dup"}}}}`)
}

func TestScan(t *testing.T) {
	dir := t.TempDir()
	fixture(t, dir)
	entries, problems, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		abbrev, name, lang string
		kind               Kind
		verses             int
	}
	var got []row
	for _, e := range entries {
		got = append(got, row{e.Bible.Abbrev, e.Bible.Name, e.Bible.Language, e.Kind, e.Verses})
		if e.Size <= 0 || !strings.HasPrefix(e.Path, dir) {
			t.Errorf("%s: size %d, path %q", e.Bible.Abbrev, e.Size, e.Path)
		}
	}
	want := []row{
		{"ARR", "", "en", KindJSON, 1},
		{"FLD", "", "", KindFolder, 2},
		{"OBJ", "Object Bible", "es", KindJSON, 1},
		{"PKG", "Package Bible", "en", KindPackage, 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("entries = %+v\nwant %+v", got, want)
	}
	if len(problems) != 3 || !strings.HasPrefix(problems[0], "broken.fbbible: ") || !strings.HasPrefix(problems[1], "broken.json: ") ||
		problems[2] != "dup.json: PKG is also in PKG.fbbible, which is used" {
		t.Errorf("problems = %q", problems)
	}
	// A missing folder lists nothing.
	if e, p, err := Scan(filepath.Join(dir, "missing")); err != nil || len(e) != 0 || len(p) != 0 {
		t.Errorf("missing folder: %v %v %v", e, p, err)
	}
}

func TestRead(t *testing.T) {
	dir := t.TempDir()
	fixture(t, dir)
	tests := []struct {
		path   string
		abbrev string
		kind   Kind
		verses int
	}{
		{"PKG.fbbible", "PKG", KindPackage, 2},
		{"obj.json", "OBJ", KindJSON, 1},
		{"en_arr.json", "ARR", KindJSON, 1},
		{"FLD", "FLD", KindFolder, 2},
		{"FLD/new/John/3.json", "FLD", KindJSON, 2}, // a chapter file reads its whole folder
	}
	for _, tc := range tests {
		l, err := Read(filepath.Join(dir, tc.path))
		if err != nil || l.Bible.Abbrev != tc.abbrev || l.Kind != tc.kind || len(l.Verses) != tc.verses {
			t.Errorf("Read(%s) = %s %s %d verses, %v", tc.path, l.Bible.Abbrev, l.Kind, len(l.Verses), err)
		}
	}
	for _, bad := range []string{"broken.fbbible", "broken.json", "missing.json"} {
		if _, err := Read(filepath.Join(dir, bad)); err == nil {
			t.Errorf("Read(%s): no error", bad)
		}
	}
	if got := PackagePath(dir, "N/T"); got != filepath.Join(dir, "N_T.fbbible") {
		t.Errorf("PackagePath = %q", got)
	}
}

// The EasyWorship RVA, when installed: listed from its header, read in full.
func TestEWB(t *testing.T) {
	const ewb = "/Users/Shared/Softouch/Easyworship.v8/Plugins/Bibles/RVA.ewb"
	if _, err := os.Stat(ewb); err != nil {
		t.Skip("EasyWorship's RVA.ewb is not installed")
	}
	dir := t.TempDir()
	data, err := os.ReadFile(ewb)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "RVA.ewb"), string(data))
	entries, problems, err := Scan(dir)
	if err != nil || len(problems) != 0 || len(entries) != 1 || entries[0].Bible.Abbrev != "RVA" || entries[0].Verses != 31102 || entries[0].Kind != KindEWB {
		t.Fatalf("Scan = %+v, %q, %v", entries, problems, err)
	}
	l, err := Read(entries[0].Path)
	if err != nil || len(l.Verses) != 31102 || l.Bible.Copyright != "Public Domain" {
		t.Fatalf("Read = %d verses, %+v, %v", len(l.Verses), l.Bible, err)
	}
}
