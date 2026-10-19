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
	longNotice := strings.Repeat("Copyright words ", 10) // 160 characters
	tests := []struct {
		name   string
		bible  models.Bible
		verses int
		path   string
		want   string
	}{
		{
			"public domain by abbreviation",
			models.Bible{Abbrev: "ASV", Name: "American Standard Version", Copyright: "Public Domain"}, 31086, "/x/ASV",
			"Import American Standard Version (ASV) from ASV: 31,086 verses.\n\n" + bibleFilePublic,
		},
		{
			"public domain by notice only",
			models.Bible{Abbrev: "XPD", Copyright: " public domain "}, 2, "/x/xpd.json",
			"Import XPD from xpd.json: 2 verses.\n\n" + bibleFilePublic,
		},
		{
			"copyrighted with notice",
			models.Bible{Abbrev: "NIV", Name: "New International Version", Copyright: "© Biblica, Inc.\nAll rights reserved."}, 31000, "/x/niv.json",
			"Import New International Version (NIV) from niv.json: 31,000 verses (© Biblica, Inc.).\n\n" + bibleFileLicensed,
		},
		{
			"long notice is cut",
			models.Bible{Abbrev: "LNG", Copyright: "\n\n" + longNotice}, 1, "/x/LNG",
			"Import LNG from LNG: 1 verse (" + strings.TrimSpace(longNotice[:bibleFileNoticeRune-1]) + "…).\n\n" + bibleFileLicensed,
		},
		{
			"no notice",
			models.Bible{Abbrev: "SMK"}, 3, "/x/SMK",
			"Import SMK from SMK: 3 verses; the file states no copyright.\n\n" + bibleFileUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BibleFileQuestion(tt.bible, tt.verses, tt.path)
			if got != tt.want {
				t.Errorf("got\n%s\nwant\n%s", got, tt.want)
			}
			if strings.Count(got, "\n") != 2 || len(got) > 260 {
				t.Errorf("question must stay short for the fixed-size dialog (%d chars, %d newlines)", len(got), strings.Count(got, "\n"))
			}
		})
	}
	if got := BibleFileSummary(models.Bible{Abbrev: "SMK"}, 2); got != "Imported SMK: 2 verses" {
		t.Errorf("BibleFileSummary = %q", got)
	}
}
