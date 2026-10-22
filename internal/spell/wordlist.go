package spell

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// The fallback checker: SCOWL's American English word list up to size 60
// (data/en_US.txt.gz, built by gen/main.go; licence in data/SCOWL-LICENSE),
// about 123,000 words in 330 KB, with a small Norvig-style suggester. It runs
// on every platform without cgo, which is why it backs the checker on
// Windows and Linux and in the unit tests.

//go:embed data/en_US.txt.gz
var wordlistGz []byte

// wordlist is a Backend over the embedded word list.
type wordlist struct {
	once  sync.Once
	words map[string]int // word -> rank (its line: lower is more common)
}

var defaultWordlist wordlist

// Wordlist returns the embedded SCOWL en_US backend (loaded on first use).
func Wordlist() Backend { return &defaultWordlist }

func (w *wordlist) load() {
	w.once.Do(func() {
		w.words = make(map[string]int, 130000)
		zr, err := gzip.NewReader(bytes.NewReader(wordlistGz))
		if err != nil {
			return // a corrupt build: every word is then unknown
		}
		sc := bufio.NewScanner(zr)
		for n := 0; sc.Scan(); n++ {
			if word := sc.Text(); word != "" {
				if _, dup := w.words[word]; !dup {
					w.words[word] = n
				}
			}
		}
	})
}

// Name names the backend.
func (w *wordlist) Name() string { return "SCOWL en_US" }

// Languages lists the one language of the list.
func (w *wordlist) Languages() []string { return []string{"en_US"} }

// Correct reports each word's spelling (Backend).
func (w *wordlist) Correct(words []string, _ string) []bool {
	w.load()
	out := make([]bool, len(words))
	for i, word := range words {
		_, out[i] = w.rank(word)
	}
	return out
}

// rank returns the rank of the listed form of word and whether there is one:
// the word as written, its lower-case form ("Thy" -> "thy", "THY"), or the
// capitalised form of an all-caps word ("JESUS" -> "Jesus"). A word listed
// capitalised only ("Jesus") is not accepted in lower case.
func (w *wordlist) rank(word string) (int, bool) {
	word = normalizeApostrophes(word)
	if r, ok := w.words[word]; ok {
		return r, true
	}
	lower := strings.ToLower(word)
	if r, ok := w.words[lower]; ok {
		return r, true
	}
	if word == strings.ToUpper(word) {
		if r, ok := w.words[capitalize(lower)]; ok {
			return r, true
		}
	}
	return 0, false
}

func capitalize(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// maxSuggestions bounds the suggestions of one word.
const maxSuggestions = 8

// Suggest returns the listed words one edit away from word (a deleted,
// transposed, replaced or inserted letter), else two edits away, the
// transpositions first and then the most common; each in word's case.
func (w *wordlist) Suggest(word, _ string) []string {
	w.load()
	lower := strings.ToLower(normalizeApostrophes(word))
	type cand struct {
		word string
		cost int // 0 transposition, 1 other single edit, 2 two edits
		rank int
	}
	seen := map[string]bool{lower: true}
	var cands []cand
	consider := func(c string, cost int) {
		if seen[c] {
			return
		}
		seen[c] = true
		if r, ok := w.rank(c); ok {
			cands = append(cands, cand{c, cost, r})
		} else if r, ok := w.words[capitalize(c)]; ok {
			cands = append(cands, cand{capitalize(c), cost, r}) // listed capitalised only ("Jesus")
		}
	}
	first := edits(lower)
	for _, e := range first {
		consider(e.word, e.cost)
	}
	if len(cands) == 0 && utf8.RuneCountInString(lower) <= 12 {
		for _, e := range first {
			for _, e2 := range edits(e.word) {
				consider(e2.word, 2)
			}
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].cost != cands[j].cost {
			return cands[i].cost < cands[j].cost
		}
		return cands[i].rank < cands[j].rank
	})
	var out []string
	for _, c := range cands {
		if len(out) == maxSuggestions {
			break
		}
		out = append(out, MatchCase(word, c.word))
	}
	return out
}

type edit struct {
	word string
	cost int
}

const alphabet = "abcdefghijklmnopqrstuvwxyz'"

// edits returns the strings one edit away from word (lower case).
func edits(word string) []edit {
	rs := []rune(word)
	var out []edit
	for i := 0; i+1 < len(rs); i++ { // transpositions
		t := append([]rune{}, rs...)
		t[i], t[i+1] = t[i+1], t[i]
		out = append(out, edit{string(t), 0})
	}
	for i := range rs { // deletions
		out = append(out, edit{string(rs[:i]) + string(rs[i+1:]), 1})
	}
	for i := range rs { // replacements
		for _, c := range alphabet {
			if c != rs[i] {
				out = append(out, edit{string(rs[:i]) + string(c) + string(rs[i+1:]), 1})
			}
		}
	}
	for i := 0; i <= len(rs); i++ { // insertions
		for _, c := range alphabet {
			out = append(out, edit{string(rs[:i]) + string(c) + string(rs[i:]), 1})
		}
	}
	return out
}
