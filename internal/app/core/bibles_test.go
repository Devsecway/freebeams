package core

import (
	"reflect"
	"testing"

	"freeworship/internal/importers/easyworship"
	"freeworship/internal/models"
)

func TestCopyrightedBiblesToImport(t *testing.T) {
	files := []easyworship.BibleFile{
		{Abbrev: "HCSB"},
		{Abbrev: "KJV", PublicDomain: true},
		{Abbrev: "NIV"},
		{Abbrev: "RVA", PublicDomain: true},
	}
	tests := []struct {
		name      string
		installed []models.Bible
		want      []string
	}{
		{"nothing installed", nil, []string{"HCSB", "NIV"}},
		{"public domain installed", []models.Bible{{Abbrev: "KJV"}, {Abbrev: "RVA"}}, []string{"HCSB", "NIV"}},
		{"one imported (case-insensitive)", []models.Bible{{Abbrev: "hcsb"}}, []string{"NIV"}},
		{"all imported", []models.Bible{{Abbrev: "HCSB"}, {Abbrev: "NIV"}}, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CopyrightedBiblesToImport(files, tt.installed)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
	if got := CopyrightedBiblesToImport(nil, nil); got == nil || len(got) != 0 {
		t.Errorf("no files: got %#v, want an empty (non-nil) list", got)
	}
}

func TestBibleImportSummary(t *testing.T) {
	tests := []struct {
		rep  easyworship.Report
		want string
	}{
		{easyworship.Report{BiblesImported: []string{"HCSB"}}, "Imported HCSB"},
		{easyworship.Report{BiblesImported: []string{"HCSB", "NIV"}, BiblesSkipped: []string{"KJV (already installed)"}}, "Imported HCSB, NIV"},
		{easyworship.Report{BiblesSkipped: []string{"HCSB (already installed)"}}, "No new Bibles to import"},
		{easyworship.Report{}, "No new Bibles to import"},
	}
	for _, tt := range tests {
		if got := BibleImportSummary(tt.rep); got != tt.want {
			t.Errorf("%+v: got %q, want %q", tt.rep, got, tt.want)
		}
	}
}
