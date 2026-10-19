package core

import (
	"strings"
	"testing"

	"freeworship/internal/models"
)

func TestVerseCountText(t *testing.T) {
	tests := map[int]string{0: "0 verses", 1: "1 verse", 2: "2 verses", 999: "999 verses", 1000: "1,000 verses", 31102: "31,102 verses", 1234567: "1,234,567 verses"}
	for n, want := range tests {
		if got := VerseCountText(n); got != want {
			t.Errorf("VerseCountText(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestBibleFileQuestion(t *testing.T) {
	tests := []struct {
		name     string
		bible    models.Bible
		verses   int
		path     string
		contains []string
		lacks    []string
	}{
		{
			"public domain", models.Bible{Abbrev: "ASV", Name: "American Standard Version", Copyright: "Public Domain"}, 31086, "/x/ASV",
			[]string{"Import American Standard Version (ASV) from ASV: 31,086 verses.", "public domain", "stays in this computer's library"},
			[]string{"responsible"},
		},
		{
			"copyrighted with notice", models.Bible{Abbrev: "NIV", Name: "New International Version", Copyright: "© Biblica"}, 31000, "/x/niv.json",
			[]string{"Import New International Version (NIV) from niv.json: 31,000 verses.", "Copyright notice: © Biblica", "You are responsible for holding a licence"},
			[]string{"public domain"},
		},
		{
			"no notice", models.Bible{Abbrev: "SMK"}, 1, "/x/SMK",
			[]string{"Import SMK from SMK: 1 verse.", "The file states no copyright", "You are responsible for holding a licence"},
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BibleFileQuestion(tt.bible, tt.verses, tt.path)
			for _, s := range tt.contains {
				if !strings.Contains(got, s) {
					t.Errorf("question lacks %q:\n%s", s, got)
				}
			}
			for _, s := range tt.lacks {
				if strings.Contains(got, s) {
					t.Errorf("question has %q:\n%s", s, got)
				}
			}
		})
	}
	if got := BibleFileSummary(models.Bible{Abbrev: "SMK"}, 2); got != "Imported SMK: 2 verses" {
		t.Errorf("BibleFileSummary = %q", got)
	}
}
