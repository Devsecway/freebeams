//go:build ignore

// Command gen regenerates the bundled translations from a clone of
// github.com/hargarpay/bible-translations (its json/ folder):
//
//	git clone --depth 1 https://github.com/hargarpay/bible-translations /tmp/bible-translations
//	go run internal/bibles/bundled/gen/main.go -src /tmp/bible-translations/json -out internal/bibles/bundled/data
//
// For every manifest translation (bundled.Translations) it reads the folder
// with bibletext.ReadDir, applies bundled.Patches, writes
// <out>/<abbrev>.json.gz (gzip, best compression, reproducible) and prints the
// counts. It fails when the source has warnings, when a patch does not apply,
// or when the counts differ from the manifest, so the manifest, the patches
// and the data cannot drift apart. The package must compile for this command
// to build, so keep the existing data files in place while regenerating.
package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"freebeams/internal/bibles/bundled"
	"freebeams/internal/importers/bibletext"
)

func main() {
	src := flag.String("src", "", "the bible-translations json/ folder")
	out := flag.String("out", "internal/bibles/bundled/data", "output folder")
	flag.Parse()
	if *src == "" {
		fmt.Fprintln(os.Stderr, "usage: gen -src <bible-translations>/json [-out dir]")
		os.Exit(2)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	for _, t := range bundled.Translations {
		res, err := bibletext.ReadDir(filepath.Join(*src, t.Abbrev))
		if err != nil {
			fail(err)
		}
		if len(res.Warnings) > 0 {
			fail(fmt.Errorf("%s: %d warnings:\n  %s", t.Abbrev, len(res.Warnings), strings.Join(res.Warnings, "\n  ")))
		}
		res.Bible.Name, res.Bible.Language, res.Bible.Copyright = t.Name, t.Language, t.License
		for _, p := range bundled.Patches {
			if !strings.EqualFold(p.Abbrev, t.Abbrev) {
				continue
			}
			applied := false
			for i := range res.Verses {
				v := &res.Verses[i]
				if v.Book == p.Book && v.Chapter == p.Chapter && v.Verse == p.Verse {
					if strings.Count(v.Text, p.Old) != 1 {
						fail(fmt.Errorf("%s %d %d:%d: patch text not found exactly once in %q", t.Abbrev, p.Book, p.Chapter, p.Verse, v.Text))
					}
					v.Text = strings.Replace(v.Text, p.Old, p.New, 1)
					applied = true
				}
			}
			if !applied {
				fail(fmt.Errorf("%s %d %d:%d: patched verse not found", t.Abbrev, p.Book, p.Chapter, p.Verse))
			}
		}
		books := map[int]bool{}
		for _, v := range res.Verses {
			books[v.Book] = true
		}
		if len(books) != t.Books || len(res.Verses) != t.Verses {
			fail(fmt.Errorf("%s: %d books, %d verses; the manifest says %d, %d", t.Abbrev, len(books), len(res.Verses), t.Books, t.Verses))
		}
		var plain bytes.Buffer
		if err := bibletext.Encode(&plain, res.Bible, res.Verses); err != nil {
			fail(err)
		}
		var packed bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&packed, gzip.BestCompression)
		zw.Write(plain.Bytes())
		zw.Close()
		path := filepath.Join(*out, filepath.Base(t.File))
		if err := os.WriteFile(path, packed.Bytes(), 0o644); err != nil {
			fail(err)
		}
		fmt.Printf("%s: %d books, %d verses, %d bytes JSON, %d bytes gzip -> %s\n",
			t.Abbrev, len(books), len(res.Verses), plain.Len(), packed.Len(), path)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gen:", err)
	os.Exit(1)
}
