package core

import (
	"reflect"
	"strings"
	"testing"

	"freebeams/internal/models"
)

func TestMergeBibleEntries(t *testing.T) {
	builtin := []BibleAvailable{
		{Bible: models.Bible{Abbrev: "KJV", Name: "King James Version", Language: "en", Copyright: "Public Domain"}, Verses: 31102, Size: 1259957, Source: BibleSourceBuiltin},
		{Bible: models.Bible{Abbrev: "RVA", Name: "Reina-Valera Antigua", Language: "es", Copyright: "Public Domain"}, Verses: 31102, Size: 1292070, Source: BibleSourceBuiltin},
	}
	folder := []BibleAvailable{
		{Bible: models.Bible{Abbrev: "kjv", Name: "My KJV"}, Verses: 5, Size: 10, Source: BibleSourceFolder, Path: "/b/kjv.json"},
		{Bible: models.Bible{Abbrev: "SMK", Name: "Smoke", Language: "en", Copyright: "© Smoke"}, Verses: 3, Size: 400, Source: BibleSourceFolder, Path: "/b/SMK.fbbible", Package: true},
		{Bible: models.Bible{Abbrev: "AMP", Name: "Amplified Bible"}, Verses: 31103, Size: 2000000, Source: BibleSourceFolder, Path: "/b/AMP.fbbible", Package: true},
	}
	installed := []models.Bible{
		{ID: 1, Abbrev: "AMP", Name: "Amplified Bible", Language: "en"},
		{ID: 2, Abbrev: "KJV", Name: "King James Version", Language: "en", Copyright: "Public Domain"},
		{ID: 3, Abbrev: "NIV", Name: "New International Version", Language: "en", Copyright: "© Biblica"},
	}
	counts := map[int64]int{1: 31103, 2: 31102, 3: 31087}
	got := MergeBibleEntries(builtin, folder, installed, counts)
	want := []BibleEntry{
		{Abbrev: "AMP", Name: "Amplified Bible", Language: "en", Source: BibleSourceFolder, Installed: true, Verses: 31103, Size: 2000000, Path: "/b/AMP.fbbible", HasPackage: true},
		{Abbrev: "KJV", Name: "King James Version", Language: "en", Copyright: "Public Domain", Source: BibleSourceBuiltin, Installed: true, Verses: 31102, Size: 1259957, Path: "/b/kjv.json", Builtin: true, PublicDomain: true},
		{Abbrev: "NIV", Name: "New International Version", Language: "en", Copyright: "© Biblica", Source: BibleSourceLibrary, Installed: true, Verses: 31087},
		{Abbrev: "RVA", Name: "Reina-Valera Antigua", Language: "es", Copyright: "Public Domain", Source: BibleSourceBuiltin, Verses: 31102, Size: 1292070, Builtin: true, PublicDomain: true},
		{Abbrev: "SMK", Name: "Smoke", Language: "en", Copyright: "© Smoke", Source: BibleSourceFolder, Verses: 3, Size: 400, Path: "/b/SMK.fbbible", HasPackage: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entries:\n%+v\nwant\n%+v", got, want)
	}
	adds := map[string]bool{}
	saves := map[string]bool{}
	for _, e := range got {
		adds[e.Abbrev], saves[e.Abbrev] = e.CanAdd(), e.CanSavePackage()
	}
	if !reflect.DeepEqual(adds, map[string]bool{"AMP": false, "KJV": false, "NIV": false, "RVA": true, "SMK": true}) {
		t.Errorf("CanAdd = %v", adds)
	}
	if !reflect.DeepEqual(saves, map[string]bool{"AMP": false, "KJV": false, "NIV": true, "RVA": false, "SMK": false}) {
		t.Errorf("CanSavePackage = %v", saves)
	}
	if FindBibleEntry(got, "rva") != 3 || FindBibleEntry(got, "XYZ") != -1 || FindBibleEntry(got, "") != -1 {
		t.Error("FindBibleEntry")
	}
	if e := MergeBibleEntries(nil, nil, nil, nil); e == nil || len(e) != 0 {
		t.Errorf("empty merge = %#v", e)
	}
}

func TestBibleManagerTexts(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{BibleDisplayName("KJV", "King James Version"), "King James Version (KJV)"},
		{BibleDisplayName("SMK", ""), "SMK"},
		{BibleDisplayName("SMK", "smk"), "SMK"},
		{LanguageName("es"), "Spanish"},
		{LanguageName(" EN "), "English"},
		{LanguageName("xx"), "XX"},
		{LanguageName(""), "—"},
		{BibleSourceLabel(BibleSourceBuiltin), "Built-in"},
		{BibleSourceLabel(BibleSourceFolder), "Bibles folder"},
		{BibleSourceLabel(BibleSourceLibrary), "Library only"},
		{BibleStatusLabel(true), "Installed"},
		{BibleStatusLabel(false), "Not installed"},
		{SizeText(0), "—"},
		{SizeText(512), "512 B"},
		{SizeText(819_600), "820 KB"},
		{SizeText(1_292_070), "1.3 MB"},
		{BibleEntryDetail(BibleEntry{Abbrev: "KJV", Name: "King James Version", Verses: 31102, Builtin: true, PublicDomain: true}), "31,102 verses · Public domain · ships with FreeBeams"},
		{BibleEntryDetail(BibleEntry{Abbrev: "SMK", Verses: 3, Copyright: "© Smoke\nAll rights", Path: "/b/SMK.fbbible"}), "3 verses · © Smoke · SMK.fbbible"},
		{BibleEntryDetail(BibleEntry{Abbrev: "NIV", Verses: 1, Installed: true}), "1 verse · Copyright not stated · no package: Save as Package keeps a copy"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
	q := BibleRemoveQuestion(BibleEntry{Abbrev: "KJV", Name: "King James Version", Builtin: true})
	if !strings.HasPrefix(q, "Remove King James Version (KJV) from the library?") || !strings.Contains(q, "add it again") {
		t.Errorf("remove question = %q", q)
	}
	if q := BibleRemoveQuestion(BibleEntry{Abbrev: "NIV", Installed: true}); !strings.Contains(q, "no package") {
		t.Errorf("remove question (library only) = %q", q)
	}
	// Both fit the fixed confirm dialog: a heading line and about three more.
	for _, q := range []string{q, BibleRemoveQuestion(BibleEntry{Abbrev: "AMPC", Name: "Amplified Bible, Classic Edition", Path: "/x"})} {
		if len(q) > 190 {
			t.Errorf("question too long for ui.ConfirmDialog (%d): %q", len(q), q)
		}
	}
	if q := BibleAddQuestion(BibleEntry{Abbrev: "SMK", Name: "Smoke", Copyright: "© Smoke", Verses: 3, Path: "/b/SMK.fbbible"}); !strings.Contains(q, "licence") || !strings.Contains(q, "SMK.fbbible") {
		t.Errorf("add question = %q", q)
	}
}

func TestOpenFolderCommand(t *testing.T) {
	for goos, want := range map[string][]string{
		"darwin": {"open", "/b"}, "windows": {"explorer", "/b"}, "linux": {"xdg-open", "/b"},
	} {
		name, args := OpenFolderCommand("/b", goos)
		if got := append([]string{name}, args...); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %q", goos, got)
		}
	}
}

func TestDefaultBibleFallsBackInBundleOrder(t *testing.T) {
	bibles := []models.Bible{{Abbrev: "AMP"}, {Abbrev: "ASV"}, {Abbrev: "NIV"}, {Abbrev: "RVA"}}
	if got := DefaultBible(bibles); got != 1 {
		t.Errorf("without KJV: %d, want the ASV (1)", got)
	}
	if got := DefaultBible([]models.Bible{{Abbrev: "AMP"}, {Abbrev: "RVA"}}); got != 1 {
		t.Errorf("only RVA built in: %d", got)
	}
	if got := DefaultBible([]models.Bible{{Abbrev: "AMP"}, {Abbrev: "NIV"}}); got != 0 {
		t.Errorf("no built-in: %d", got)
	}
}
