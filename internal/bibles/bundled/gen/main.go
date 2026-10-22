//go:build ignore

// Command gen regenerates the bundled translations (Bible packages, see
// internal/bibles/biblepkg) from their sources:
//
//	git clone --depth 1 https://github.com/hargarpay/bible-translations /tmp/bible-translations
//	go run internal/bibles/bundled/gen/main.go -src /tmp/bible-translations/json \
//	    -ewb /Users/Shared/Softouch/Easyworship.v8/Plugins/Bibles -out internal/bibles/bundled/data
//
// For every manifest translation (bundled.Translations) it reads the source
// its Origin names: a json/<ABBREV> folder of the bible-translations clone
// (-src) through bibletext.ReadDir, or an EasyWorship <ABBREV>.ewb (-ewb)
// through the .ewb decoder. It applies bundled.Patches, writes
// <out>/<abbrev>.fbbible (reproducible) and prints the counts. A source whose
// flag is not given is re-packed from the committed data instead (bundled.Load),
// so one translation can be added without the other sources at hand.
//
// It fails when the source has warnings, when a patch does not apply, when
// the counts differ from the manifest, or when a translation is not public
// domain, so the manifest, the patches and the data cannot drift apart. The
// package must compile for this command to build, so keep the existing data
// files in place while regenerating (a new manifest entry needs a placeholder
// file: copy any .fbbible to its name first).
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"freebeams/internal/bibles/biblepkg"
	"freebeams/internal/bibles/bundled"
	"freebeams/internal/importers/bibletext"
	"freebeams/internal/importers/easyworship"
	"freebeams/internal/models"
)

func main() {
	src := flag.String("src", "", "the bible-translations json/ folder (KJV, ASV)")
	ewb := flag.String("ewb", "", "the folder holding EasyWorship .ewb Bibles (RVA)")
	out := flag.String("out", "internal/bibles/bundled/data", "output folder")
	only := flag.String("only", "", "regenerate only these abbreviations (comma-separated)")
	flag.Parse()
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	for _, t := range bundled.Translations {
		if *only != "" && !contains(strings.Split(*only, ","), t.Abbrev) {
			continue
		}
		if !models.IsPublicDomainBible(t.Abbrev, t.Name) || t.License != "Public Domain" {
			fail(fmt.Errorf("%s is not public domain: it must never be bundled", t.Abbrev))
		}
		res, from, err := read(t, *src, *ewb)
		if err != nil {
			fail(err)
		}
		if len(res.Warnings) > 0 {
			fail(fmt.Errorf("%s: %d warnings:\n  %s", t.Abbrev, len(res.Warnings), strings.Join(res.Warnings, "\n  ")))
		}
		res.Bible.Abbrev, res.Bible.Name, res.Bible.Language, res.Bible.Copyright = t.Abbrev, t.Name, t.Language, t.License
		patch(t, res.Verses)
		books := map[int]bool{}
		for _, v := range res.Verses {
			books[v.Book] = true
		}
		if len(books) != t.Books || len(res.Verses) != t.Verses {
			fail(fmt.Errorf("%s: %d books, %d verses; the manifest says %d, %d", t.Abbrev, len(books), len(res.Verses), t.Books, t.Verses))
		}
		var packed bytes.Buffer
		if err := biblepkg.Write(&packed, res.Bible, res.Verses); err != nil {
			fail(err)
		}
		path := filepath.Join(*out, filepath.Base(t.File))
		if err := os.WriteFile(path, packed.Bytes(), 0o644); err != nil {
			fail(err)
		}
		fmt.Printf("%s (%s): %d books, %d verses, %d bytes -> %s\n",
			t.Abbrev, from, len(books), len(res.Verses), packed.Len(), path)
	}
}

// read decodes a translation from its source, or from the committed data
// when that source's flag is empty.
func read(t bundled.Translation, src, ewb string) (bibletext.Result, string, error) {
	switch {
	case t.Origin == bundled.OriginBibleTranslations && src != "":
		res, err := bibletext.ReadDir(filepath.Join(src, t.Abbrev))
		return res, "bible-translations json/" + t.Abbrev, err
	case t.Origin == bundled.OriginEWB && ewb != "":
		path := filepath.Join(ewb, t.Abbrev+".ewb")
		b, verses, err := easyworship.ReadBible(path)
		return bibletext.Result{Bible: b, Verses: verses}, path, err
	}
	res, err := bundled.Load(t.Abbrev)
	return res, "the committed " + t.File, err
}

// patch applies bundled.Patches to t's verses (already-applied patches are
// fine: the committed data carries them).
func patch(t bundled.Translation, verses []models.Verse) {
	for _, p := range bundled.Patches {
		if !strings.EqualFold(p.Abbrev, t.Abbrev) {
			continue
		}
		found := false
		for i := range verses {
			v := &verses[i]
			if v.Book != p.Book || v.Chapter != p.Chapter || v.Verse != p.Verse {
				continue
			}
			found = true
			switch {
			case strings.Count(v.Text, p.Old) == 1:
				v.Text = strings.Replace(v.Text, p.Old, p.New, 1)
			case strings.Contains(v.Text, p.New):
				// already applied (re-packing the committed data)
			default:
				fail(fmt.Errorf("%s %d %d:%d: patch text not found exactly once in %q", t.Abbrev, p.Book, p.Chapter, p.Verse, v.Text))
			}
		}
		if !found {
			fail(fmt.Errorf("%s %d %d:%d: patched verse not found", t.Abbrev, p.Book, p.Chapter, p.Verse))
		}
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(strings.TrimSpace(x), s) {
			return true
		}
	}
	return false
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gen:", err)
	os.Exit(1)
}
