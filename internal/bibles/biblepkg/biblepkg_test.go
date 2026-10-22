package biblepkg

import (
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"freebeams/internal/models"
)

var testBible = models.Bible{Abbrev: "TST", Name: "Test Bible", Language: "es", Copyright: "Public Domain"}

// testVerses are made-up and KJV wordings, with accents and quotes.
var testVerses = []models.Verse{
	{Book: 1, Chapter: 1, Verse: 1, Text: "In the beginning God created the heaven and the earth."},
	{Book: 19, Chapter: 23, Verse: 1, Text: "JEHOVÁ es mi pastor; nada me faltará."},
	{Book: 43, Chapter: 3, Verse: 16, Text: `Porque de tal manera amó Dios al mundo, "unigénito" <&> ¿sí? ¡Châm!`},
	{Book: 65, Chapter: 1, Verse: 3, Text: "Jude, verse three."},
}

func pack(t *testing.T, b models.Bible, verses []models.Verse) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(&buf, b, verses); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func gz(s string) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(s))
	zw.Close()
	return buf.Bytes()
}

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		bible  models.Bible
		verses []models.Verse
	}{
		{"accents and quotes", testBible, testVerses},
		{"one verse, no metadata", models.Bible{Abbrev: "X"}, testVerses[:1]},
		{"unsorted input comes out canonical", models.Bible{Abbrev: "U", Copyright: "© 2020 Someone\nAll rights reserved."},
			[]models.Verse{testVerses[3], testVerses[0], testVerses[2], testVerses[1]}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := pack(t, tc.bible, tc.verses)
			h, err := ReadHeader(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			want := Header{Format: Format, Version: Version, Abbrev: tc.bible.Abbrev, Name: tc.bible.Name,
				Language: tc.bible.Language, Copyright: tc.bible.Copyright, Verses: len(tc.verses)}
			if h != want {
				t.Errorf("header = %+v, want %+v", h, want)
			}
			p, err := Read(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if p.Bible != tc.bible || p.Header != want || len(p.Warnings) != 0 {
				t.Errorf("package = %+v / %+v, warnings %q", p.Bible, p.Header, p.Warnings)
			}
			sorted := append([]models.Verse(nil), tc.verses...)
			if tc.name == "unsorted input comes out canonical" {
				sorted = []models.Verse{testVerses[0], testVerses[1], testVerses[2], testVerses[3]}
			}
			if !reflect.DeepEqual(p.Verses, sorted) {
				t.Errorf("verses = %+v\nwant %+v", p.Verses, sorted)
			}
			// Reproducible: the same input gives the same bytes.
			if again := pack(t, tc.bible, tc.verses); !bytes.Equal(again, data) {
				t.Error("two writes differ")
			}
		})
	}
}

func TestReadErrors(t *testing.T) {
	good := pack(t, testBible, testVerses)
	tests := []struct {
		name    string
		data    []byte
		header  bool   // ReadHeader fails too
		wantErr string // substring
		notPkg  bool
	}{
		{"empty", nil, true, "not a FreeBeams Bible package", true},
		{"not gzip", []byte(`{"abbrev":"KJV","books":{}}`), true, "not a FreeBeams Bible package", true},
		{"gzip of plain bible JSON (no header)", gz(`{"abbrev":"KJV","books":{"John":{"3":{"16":"x"}}}}`), true, "not a FreeBeams Bible package", true},
		{"gzip of other JSON", gz("{\"format\":\"other\",\"version\":1,\"abbrev\":\"A\",\"verses\":1,\n\"books\":{}}\n"), true, "not a FreeBeams Bible package", true},
		{"newer format", gz("{\"format\":\"freebeams-bible\",\"version\":9,\"abbrev\":\"A\",\"verses\":1,\n\"books\":{}}\n"), true, "newer", false},
		{"no abbrev", gz("{\"format\":\"freebeams-bible\",\"version\":1,\"abbrev\":\" \",\"verses\":1,\n\"books\":{}}\n"), true, "abbrev", false},
		{"no verses in header", gz("{\"format\":\"freebeams-bible\",\"version\":1,\"abbrev\":\"A\",\"verses\":0,\n\"books\":{}}\n"), true, "0 verses", false},
		{"verse count mismatch", gz("{\"format\":\"freebeams-bible\",\"version\":1,\"abbrev\":\"A\",\"verses\":2,\n\"books\":{\"John\":{\"3\":{\"16\":\"x\"}}}}\n"), false, "damaged: 1 verses, the header says 2", false},
		{"broken books JSON", gz("{\"format\":\"freebeams-bible\",\"version\":1,\"abbrev\":\"A\",\"verses\":1,\n\"books\":{\"John\":{\"3\":{\"16\":"), false, "read bible package A", false},
		{"truncated gzip", good[:len(good)/2], false, "damaged or truncated", false},
		{"truncated to the gzip header", good[:12], true, "", false},
		{"corrupt byte", corrupt(good), false, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Read(bytes.NewReader(tc.data))
			if err == nil {
				t.Fatal("Read: no error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Read error = %v, want %q", err, tc.wantErr)
			}
			if tc.notPkg != errors.Is(err, ErrNotPackage) {
				t.Errorf("errors.Is(ErrNotPackage) = %v for %v", !tc.notPkg, err)
			}
			_, herr := ReadHeader(bytes.NewReader(tc.data))
			if tc.header && herr == nil {
				t.Error("ReadHeader: no error")
			}
			if !tc.header && tc.name != "truncated gzip" && tc.name != "corrupt byte" && herr != nil {
				t.Errorf("ReadHeader: %v", herr)
			}
		})
	}
}

// corrupt flips a byte in the middle of the compressed data.
func corrupt(b []byte) []byte {
	c := append([]byte(nil), b...)
	c[len(c)/2] ^= 0x55
	return c
}

func TestWriteErrors(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, models.Bible{Abbrev: " "}, testVerses); err == nil {
		t.Error("no abbrev: no error")
	}
	if err := Write(&buf, testBible, nil); err == nil {
		t.Error("no verses: no error")
	}
	if err := Write(&buf, testBible, []models.Verse{{Book: 70, Chapter: 1, Verse: 1, Text: "x"}}); err == nil {
		t.Error("bad book: no error")
	}
}

func TestFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName(testBible.Abbrev))
	if err := WriteFile(path, testBible, testVerses); err != nil {
		t.Fatal(err)
	}
	h, err := ReadHeaderFile(path)
	if err != nil || h.Abbrev != "TST" || h.Verses != 4 {
		t.Fatalf("ReadHeaderFile = %+v, %v", h, err)
	}
	p, err := ReadFile(path)
	if err != nil || len(p.Verses) != 4 {
		t.Fatalf("ReadFile = %d verses, %v", len(p.Verses), err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("WriteFile left %d files", len(entries))
	}
	if _, err := ReadFile(filepath.Join(dir, "missing.fbbible")); err == nil {
		t.Error("missing file: no error")
	}
	if err := WriteFile(filepath.Join(dir, "no", "such", "x.fbbible"), testBible, testVerses); err == nil {
		t.Error("missing folder: no error")
	}
}

func TestFileName(t *testing.T) {
	for in, want := range map[string]string{
		"KJV": "KJV.fbbible", "RV/1909": "RV_1909.fbbible", " NIV ": "NIV.fbbible", "..": "bible.fbbible", `a:b*c`: "a_b_c.fbbible",
	} {
		if got := FileName(in); got != want {
			t.Errorf("FileName(%q) = %q, want %q", in, got, want)
		}
	}
}

// Every proper prefix of a package is an error, never a panic or a short Bible.
func TestEveryTruncationFails(t *testing.T) {
	good := pack(t, testBible, testVerses)
	for n := 0; n < len(good); n++ {
		if p, err := Read(bytes.NewReader(good[:n])); err == nil {
			t.Fatalf("prefix of %d/%d bytes read as %d verses", n, len(good), len(p.Verses))
		}
	}
}
