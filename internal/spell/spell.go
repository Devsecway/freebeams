// Package spell checks the spelling of lyrics: the song editor's live
// underlines, its right-click suggestions and the Spelling dialog.
//
// An [Engine] tokenizes the text (Tokenize: section tags such as "[Verse 1]"
// or "V1" and words with digits are never checked), drops the words the
// operator added to the dictionary or ignored and the worship vocabulary
// (Seed), and asks a [Backend] about the rest. On macOS the backend is the
// system's NSSpellChecker (System, nsspell_darwin.go); elsewhere, or when
// the system checker is unavailable, it is the embedded SCOWL en_US word
// list ([Wordlist]).
//
// The package is pure Go apart from the darwin backend's cgo (no
// graphics.gd), so it is unit-tested with go test. An Engine is safe for
// concurrent use: the app checks the editor's text on a goroutine.
package spell

import (
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Misspelling is a flagged word of a checked text. Start and Len count runes
// (TextEdit columns) from the start of the text; Line and Col locate the
// word in the text's lines (0-based, Col in runes).
type Misspelling struct {
	Start int    `json:"start"`
	Len   int    `json:"len"`
	Word  string `json:"word"`
	Line  int    `json:"line"`
	Col   int    `json:"col"`
}

// Checker is what the app needs from a spelling checker.
type Checker interface {
	// Check returns the misspelled words of text in language lang ("" is
	// the backend's default language), in text order.
	Check(text, lang string) []Misspelling
	// Suggest returns replacements for word, best first.
	Suggest(word, lang string) []string
	// Learn adds word to the personal dictionary ("Add to Dictionary").
	Learn(word string)
	// Ignore accepts word until the app quits ("Ignore All").
	Ignore(word string)
}

// Backend decides whether single words are spelled correctly.
type Backend interface {
	// Name names the backend for the state document ("macOS", "SCOWL en_US").
	Name() string
	// Correct reports for each word whether it is spelled correctly.
	Correct(words []string, lang string) []bool
	// Suggest returns replacements for a misspelled word, best first.
	Suggest(word, lang string) []string
	// Languages lists the languages the backend checks (codes such as
	// "en", "en_GB"); the first is the default.
	Languages() []string
}

// Seed is the worship vocabulary every checker accepts, whatever its
// dictionary says: names and words of Scripture and liturgy that general
// dictionaries lack or flag.
var Seed = []string{
	"Hallelujah", "Halleluyah", "Alleluia", "Hosanna", "Hosannah", "Emmanuel", "Immanuel",
	"Jehovah", "Yahweh", "Yeshua", "Elohim", "Adonai", "El Shaddai", "Shaddai", "Selah",
	"Maranatha", "Abba", "Amen", "Ebenezer", "Jireh", "Nissi", "Rapha", "Shalom", "Shekinah",
	"Rabboni", "Messiah", "Lamb", "Calvary", "Golgotha", "Gethsemane", "Zion", "Sion", "Jordan",
	"Canaan", "Galilee", "Bethlehem", "Nazareth", "Jerusalem", "Israel", "Judah", "Jacob",
	"Jesse", "Sharon", "Gilead", "Jesus", "Christ", "Saviour", "Savior", "Redeemer",
	"Hallowed", "Doxology", "Trinity", "Triune", "Godhead", "Glory", "Gloria", "Excelsis",
	"Deo", "Kyrie", "Eleison", "Agnus", "Dei", "Magnificat", "Sanctus", "Benedictus",
	"Pentecost", "Seraphim", "Cherubim", "Thee", "Thou", "Thy", "Thine", "Ye", "Hath",
	"Doth", "Didst", "Wouldst", "Shalt", "Wilt", "Art", "Hast", "Dost", "Canst", "Couldst",
	"Shouldst", "Mayest", "Unto", "Ere", "Oft", "Lo", "Behold", "Whence", "Wherefore",
	"Yea", "Nay", "Evermore", "Forevermore", "Everlasting", "Wondrous", "Almighty",
	"Omnipotent", "Holy", "Worthy", "Ancient", "Righteousness", "Oh", "Whoa", "Ooh", "Mmm",
}

// Engine is a Checker over a Backend with the personal dictionary, the
// ignored words and the worship vocabulary. The zero value is not usable;
// call New.
type Engine struct {
	backend Backend

	mu       sync.Mutex
	personal map[string]bool // lower-cased words added to the dictionary
	ignored  map[string]bool // lower-cased words ignored this session
	seed     map[string]bool // lower-cased Seed words
	cache    map[string]bool // lang + "\x00" + word -> correct
}

// maxCache bounds the per-word verdict cache; it is dropped when full.
const maxCache = 20000

// New returns an engine over b (nil: the embedded word list) that accepts
// the personal words (the app loads them from its settings).
func New(b Backend, personal []string) *Engine {
	if b == nil {
		b = Wordlist()
	}
	e := &Engine{backend: b, personal: map[string]bool{}, ignored: map[string]bool{},
		seed: map[string]bool{}, cache: map[string]bool{}}
	for _, w := range Seed {
		for _, part := range strings.Fields(w) {
			e.seed[strings.ToLower(part)] = true
		}
	}
	for _, w := range personal {
		if k := key(w); k != "" {
			e.personal[k] = true
		}
	}
	return e
}

// Default returns an engine over the system checker when there is one
// (macOS), else over the embedded word list.
func Default(personal []string) *Engine {
	if b := System(); b != nil {
		return New(b, personal)
	}
	return New(Wordlist(), personal)
}

// BackendName names the engine's backend.
func (e *Engine) BackendName() string { return e.backend.Name() }

// Languages lists the backend's languages, the default first.
func (e *Engine) Languages() []string { return e.backend.Languages() }

// key normalises a word for the personal, ignored and seed sets.
func key(w string) string {
	return strings.ToLower(normalizeApostrophes(strings.TrimSpace(w)))
}

// normalizeApostrophes turns typographic apostrophes into ASCII ones.
func normalizeApostrophes(w string) string {
	if !strings.ContainsAny(w, "’ʼ‘") {
		return w
	}
	return strings.NewReplacer("’", "'", "ʼ", "'", "‘", "'").Replace(w)
}

// accepted reports whether w is accepted without asking the backend: a
// personal, ignored or seed word (also with a possessive 's), or a single
// letter. The caller holds e.mu.
func (e *Engine) accepted(w string) bool {
	k := key(w)
	if utf8.RuneCountInString(k) <= 1 {
		return true
	}
	for _, c := range []string{k, strings.TrimSuffix(k, "'s"), strings.TrimSuffix(k, "s'")} {
		if e.personal[c] || e.ignored[c] || e.seed[c] {
			return true
		}
	}
	return false
}

// Check returns text's misspellings (Checker).
func (e *Engine) Check(text, lang string) []Misspelling {
	toks := Tokenize(text)
	if len(toks) == 0 {
		return nil
	}
	// Words the sets and the cache do not decide go to the backend once each.
	verdict := map[string]bool{}
	var ask []string
	e.mu.Lock()
	for _, t := range toks {
		w := normalizeApostrophes(t.Word)
		if _, done := verdict[w]; done {
			continue
		}
		if e.accepted(w) {
			verdict[w] = true
			continue
		}
		if ok, hit := e.cache[lang+"\x00"+w]; hit {
			verdict[w] = ok
			continue
		}
		verdict[w] = true // provisional; the backend decides below
		ask = append(ask, w)
	}
	e.mu.Unlock()
	if len(ask) > 0 {
		res := e.backend.Correct(ask, lang)
		e.mu.Lock()
		if len(e.cache)+len(ask) > maxCache {
			e.cache = map[string]bool{}
		}
		for i, w := range ask {
			ok := i >= len(res) || res[i]
			if !ok && strings.HasSuffix(strings.ToLower(w), "'s") {
				// A possessive of an accepted word ("Jabez's").
				ok = e.accepted(w[:len(w)-2])
			}
			verdict[w] = ok
			e.cache[lang+"\x00"+w] = ok
		}
		e.mu.Unlock()
	}
	var out []Misspelling
	for _, t := range toks {
		if !verdict[normalizeApostrophes(t.Word)] {
			out = append(out, Misspelling(t))
		}
	}
	return out
}

// Suggest returns replacements for word (Checker), without duplicates.
func (e *Engine) Suggest(word, lang string) []string {
	var out []string
	seen := map[string]bool{word: true}
	for _, s := range e.backend.Suggest(word, lang) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// Learn adds word to the personal dictionary (Checker). Personal returns the
// list to persist.
func (e *Engine) Learn(word string) {
	if k := key(word); k != "" {
		e.mu.Lock()
		e.personal[k] = true
		e.mu.Unlock()
	}
}

// Forget removes word from the personal dictionary.
func (e *Engine) Forget(word string) {
	e.mu.Lock()
	delete(e.personal, key(word))
	e.mu.Unlock()
}

// Ignore accepts word for the rest of the session (Checker).
func (e *Engine) Ignore(word string) {
	if k := key(word); k != "" {
		e.mu.Lock()
		e.ignored[k] = true
		e.mu.Unlock()
	}
}

// Personal returns the personal dictionary, sorted (lower case).
func (e *Engine) Personal() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, 0, len(e.personal))
	for w := range e.personal {
		out = append(out, w)
	}
	sort.Strings(out)
	return out
}

// MatchCase gives suggestion the capitalisation of word: "Recieve" ->
// "Receive", "RECIEVE" -> "RECEIVE"; a lower-case word keeps the
// suggestion as it is.
func MatchCase(word, suggestion string) string {
	letters, upper := 0, 0
	for _, r := range word {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	switch {
	case letters > 1 && upper == letters:
		return strings.ToUpper(suggestion)
	case upper > 0:
		first, _ := utf8.DecodeRuneInString(word)
		if unicode.IsUpper(first) {
			r, n := utf8.DecodeRuneInString(suggestion)
			return string(unicode.ToUpper(r)) + suggestion[n:]
		}
	}
	return suggestion
}
