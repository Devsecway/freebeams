//go:build ignore

// Command gen builds data/en_US.txt.gz, the fallback spelling checker's word
// list, from a SCOWL release (http://wordlist.aspell.net/, licence in
// data/SCOWL-LICENSE): the American English lists up to size 60 (the size of
// the usual en_US Hunspell dictionary), in size order, so a word's position
// in the file says how common it is (spell.Wordlist ranks suggestions by it).
//
//	curl -LO https://downloads.sourceforge.net/wordlist/scowl-2020.12.07.tar.gz
//	tar xzf scowl-2020.12.07.tar.gz
//	go run internal/spell/gen/main.go scowl-2020.12.07/final internal/spell/data/en_US.txt.gz
package main

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	sizes     = []string{"10", "20", "35", "40", "50", "55", "60"}
	spellings = []string{"english", "american"}
	kinds     = []string{"words", "upper", "contractions", "proper-names", "abbreviations"}
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: gen <scowl final dir> <out.txt.gz>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(dir, out string) error {
	seen := map[string]bool{}
	var words []string
	for _, size := range sizes {
		for _, sp := range spellings {
			for _, kind := range kinds {
				data, err := os.ReadFile(filepath.Join(dir, sp+"-"+kind+"."+size))
				if os.IsNotExist(err) {
					continue
				}
				if err != nil {
					return err
				}
				for _, w := range strings.Split(latin1(data), "\n") {
					if w = strings.TrimSpace(w); w != "" && !seen[w] {
						seen[w] = true
						words = append(words, w)
					}
				}
			}
		}
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	zw, err := gzip.NewWriterLevel(f, gzip.BestCompression)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(zw)
	for _, w := range words {
		bw.WriteString(w)
		bw.WriteByte('\n')
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	fmt.Printf("%d words\n", len(words))
	return f.Close()
}

// latin1 decodes ISO-8859-1 (SCOWL's encoding).
func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return string(r)
}
