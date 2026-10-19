package bundled

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"freebeams/internal/models"
	"freebeams/internal/store"
)

// asvOmitted are the New Testament verses the ASV leaves out (its critical
// text does not have them); the KJV has all 16. That is why the ASV counts
// 31,086 verses and the KJV 31,102.
var asvOmitted = [][3]int{
	{40, 17, 21}, {40, 18, 11}, {40, 23, 14},
	{41, 7, 16}, {41, 9, 44}, {41, 9, 46}, {41, 11, 26}, {41, 15, 28},
	{42, 17, 36}, {42, 23, 17},
	{43, 5, 4},
	{44, 8, 37}, {44, 15, 34}, {44, 24, 7}, {44, 28, 29},
	{45, 16, 24},
}

func index(verses []models.Verse) map[[3]int]string {
	m := make(map[[3]int]string, len(verses))
	for _, v := range verses {
		m[[3]int{v.Book, v.Chapter, v.Verse}] = v.Text
	}
	return m
}

func TestBundledTranslations(t *testing.T) {
	if got := Abbrevs(); !slices.Equal(got, []string{"KJV", "ASV"}) {
		t.Fatalf("Abbrevs = %q: only the KJV and the ASV may be bundled", got)
	}
	if n := Size(); n <= 0 || n > 4<<20 {
		t.Errorf("embedded size = %d bytes", n)
	}
	loaded := map[string]map[[3]int]string{}
	for _, tr := range Translations {
		t.Run(tr.Abbrev, func(t *testing.T) {
			if tr.License != "Public Domain" || !models.IsPublicDomainBible(tr.Abbrev, tr.Name) {
				t.Fatalf("%s is not public domain: it must not be bundled", tr.Abbrev)
			}
			if tr.Attribution == "" || !strings.Contains(tr.Attribution, SourceURL) {
				t.Errorf("attribution = %q", tr.Attribution)
			}
			res, err := Load(tr.Abbrev)
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Warnings) != 0 {
				t.Errorf("warnings: %q", res.Warnings)
			}
			want := models.Bible{Abbrev: tr.Abbrev, Name: tr.Name, Language: tr.Language, Copyright: "Public Domain"}
			if res.Bible != want {
				t.Errorf("bible = %+v, want %+v", res.Bible, want)
			}
			if len(res.Verses) != tr.Verses {
				t.Errorf("%d verses, want %d", len(res.Verses), tr.Verses)
			}
			books := map[int]bool{}
			for _, v := range res.Verses {
				books[v.Book] = true
				if n := models.ChapterCount(v.Book); v.Chapter < 1 || v.Chapter > n || v.Verse < 1 || v.Verse > models.MaxVerse {
					t.Errorf("bad reference %d %d:%d", v.Book, v.Chapter, v.Verse)
				}
				if v.Text == "" || strings.ContainsAny(v.Text, "[] \t\n") || v.Text != strings.TrimSpace(v.Text) {
					t.Errorf("%d %d:%d not cleaned: %q", v.Book, v.Chapter, v.Verse, v.Text)
				}
			}
			if len(books) != tr.Books {
				t.Errorf("%d books, want %d", len(books), tr.Books)
			}
			loaded[tr.Abbrev] = index(res.Verses)
		})
	}
	kjv, asv := loaded["KJV"], loaded["ASV"]
	if kjv == nil || asv == nil {
		t.Fatal("a translation did not load")
	}
	checks := []struct {
		abbrev string
		ref    [3]int
		want   string
	}{
		{"KJV", [3]int{43, 3, 16}, "For God so loved the world, that he gave his only begotten Son, that whosoever believeth in him should not perish, but have everlasting life."},
		{"ASV", [3]int{43, 3, 16}, "For God so loved the world, that he gave his only begotten Son, that whosoever believeth on him should not perish, but have eternal life."},
		{"KJV", [3]int{1, 1, 1}, "In the beginning God created the heaven and the earth."},
		{"KJV", [3]int{66, 22, 21}, "The grace of our Lord Jesus Christ be with you all. Amen."},
		{"ASV", [3]int{19, 23, 1}, "Jehovah is my shepherd; I shall not want."},
		{"ASV", [3]int{19, 36, 1}, "The transgression of the wicked saith within my heart, There is no fear of God before his eyes."},
		{"ASV", [3]int{9, 13, 1}, "Saul was forty years old when he began to reign; and when he had reigned two years over Israel,"},
		{"ASV", [3]int{40, 18, 24}, "And when he had begun to reckon, one was brought unto him, that owed him ten thousand talents."},
	}
	for _, c := range checks {
		if got := loaded[c.abbrev][c.ref]; got != c.want {
			t.Errorf("%s %v = %q, want %q", c.abbrev, c.ref, got, c.want)
		}
	}
	for _, ref := range asvOmitted {
		if _, ok := asv[ref]; ok {
			t.Errorf("ASV has %v, which it should omit", ref)
		}
		if _, ok := kjv[ref]; !ok {
			t.Errorf("KJV lacks %v", ref)
		}
	}
	if len(kjv)-len(asv) != len(asvOmitted) {
		t.Errorf("KJV has %d more verses than the ASV, want %d", len(kjv)-len(asv), len(asvOmitted))
	}
}

func TestPatchesApplied(t *testing.T) {
	for _, p := range Patches {
		res, err := Load(p.Abbrev)
		if err != nil {
			t.Fatal(err)
		}
		text := index(res.Verses)[[3]int{p.Book, p.Chapter, p.Verse}]
		if !strings.Contains(text, p.New) || strings.Contains(text, p.Old) {
			t.Errorf("%s %d %d:%d = %q: patch not applied", p.Abbrev, p.Book, p.Chapter, p.Verse, text)
		}
	}
}

func TestLookupAndLoadErrors(t *testing.T) {
	if _, ok := Lookup("kjv"); !ok {
		t.Error("Lookup is not case-insensitive")
	}
	if _, ok := Lookup("NIV"); ok {
		t.Error("NIV is bundled")
	}
	if _, err := Load("NIV"); err == nil {
		t.Error("Load(NIV): no error")
	}
}

func openTemp(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestInstall(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if needed, err := Needed(ctx, st); err != nil || !needed {
		t.Fatalf("Needed on a fresh library = %v, %v", needed, err)
	}
	rep, err := Install(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rep.Installed, []string{"KJV", "ASV"}) || len(rep.Skipped) != 0 || rep.Version != Version {
		t.Fatalf("first install: %+v", rep)
	}
	if needed, err := Needed(ctx, st); err != nil || needed {
		t.Fatalf("Needed after install = %v, %v", needed, err)
	}
	bibles, err := st.ListBibles(ctx)
	if err != nil || len(bibles) != 2 || bibles[0].Abbrev != "ASV" || bibles[1].Abbrev != "KJV" || bibles[1].Copyright != "Public Domain" {
		t.Fatalf("ListBibles = %+v, %v", bibles, err)
	}
	p, _ := models.ParseReference("jn 3:16")
	verses, err := st.Verses(ctx, bibles[0].ID, p)
	if err != nil || len(verses) != 1 || !strings.Contains(verses[0].Text, "believeth on him") {
		t.Fatalf("ASV John 3:16 = %+v, %v", verses, err)
	}
	if n, _ := st.ChapterCount(ctx, bibles[1].ID, 19); n != 150 {
		t.Errorf("KJV Psalms has %d chapters", n)
	}
	hits, err := st.SearchVerses(ctx, bibles[1].ID, "shepherd want", 5)
	if err != nil || len(hits) == 0 || hits[0].Book != 19 || hits[0].Chapter != 23 {
		t.Errorf("KJV search = %+v, %v", hits, err)
	}

	// Idempotent: a second run adds nothing.
	rep, err = Install(ctx, st)
	if err != nil || len(rep.Installed) != 0 || !reflect.DeepEqual(rep.Skipped, []string{"KJV (already installed)", "ASV (already installed)"}) {
		t.Fatalf("second install: %+v, %v", rep, err)
	}
	if bibles, _ := st.ListBibles(ctx); len(bibles) != 2 {
		t.Fatalf("second install duplicated: %+v", bibles)
	}
}

func TestInstallSkipsExistingAbbrev(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	// A KJV imported from EasyWorship (any case) is left alone.
	if _, err := st.CreateBibleWithVerses(ctx, models.Bible{Abbrev: "kjv", Name: "EasyWorship KJV"},
		[]models.Verse{{Book: 43, Chapter: 3, Verse: 16, Text: "ew"}}); err != nil {
		t.Fatal(err)
	}
	rep, err := Install(ctx, st)
	if err != nil || !slices.Equal(rep.Installed, []string{"ASV"}) || !slices.Equal(rep.Skipped, []string{"KJV (already installed)"}) {
		t.Fatalf("install = %+v, %v", rep, err)
	}
	bibles, _ := st.ListBibles(ctx)
	if len(bibles) != 2 || bibles[1].Name != "EasyWorship KJV" {
		t.Fatalf("bibles = %+v", bibles)
	}
}

func TestInstallErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := Install(ctx, nil); err == nil {
		t.Error("nil store: no error")
	}
	if _, err := Needed(ctx, nil); err == nil {
		t.Error("Needed(nil): no error")
	}
	st := openTemp(t)
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := Install(cctx, st); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled install: %v", err)
	}
	if needed, _ := Needed(ctx, st); !needed {
		t.Error("a cancelled install recorded the version")
	}
	if bibles, _ := st.ListBibles(ctx); len(bibles) != 0 {
		t.Errorf("a cancelled install left %+v", bibles)
	}
}
