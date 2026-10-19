package store

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// maxTerms caps the terms (and the words of one term) a search uses, so
// pathological input stays cheap.
const maxTerms = 16

// searchTerms splits user input into search terms, each a list of words.
// Whitespace separates terms; a double-quoted run is one term even with
// spaces inside (a phrase), and an unbalanced quote runs to the end of the
// input. Words are runs of letters, numbers and marks (what the unicode61
// tokenizer indexes), so punctuation and FTS5 syntax characters (`"`, `*`,
// `(`, `-`, `:`, `^`, NEAR, AND, …) never reach a query. Terms without
// words are dropped.
func searchTerms(input string) [][]string {
	var terms [][]string
	add := func(piece string) {
		w := words(piece)
		if len(w) > maxTerms {
			w = w[:maxTerms]
		}
		if len(w) > 0 && len(terms) < maxTerms {
			terms = append(terms, w)
		}
	}
	for input != "" {
		input = strings.TrimLeftFunc(input, unicode.IsSpace)
		if input == "" {
			break
		}
		if input[0] == '"' {
			end := strings.IndexByte(input[1:], '"')
			if end < 0 {
				add(input[1:])
				break
			}
			add(input[1 : 1+end])
			input = input[2+end:]
			continue
		}
		end := strings.IndexFunc(input, func(r rune) bool { return unicode.IsSpace(r) || r == '"' })
		if end < 0 {
			add(input)
			break
		}
		add(input[:end])
		input = input[end:]
	}
	return terms
}

// words splits s into its words: runs of letters, numbers and marks.
func words(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r))
	})
}

// ftsQuery turns user input into a safe FTS5 MATCH expression: every term
// is a prefix phrase and all terms must match (implicit AND), in any
// column. Returns "" when the input has no searchable words.
func ftsQuery(input string) string {
	return ftsAll(searchTerms(input))
}

// ftsPhrase is the FTS5 prefix phrase for one term: "w1 w2"*. FTS5 runs the
// tokenizer over the quoted string again, so a contraction typed as I'll
// (words i, ll) matches the adjacent tokens of I'll in the text, not an
// i… and an ll… anywhere in the row. The tokenizer also folds case and
// diacritics, as it did when indexing.
func ftsPhrase(words []string) string {
	// Words never contain '"' (not a letter), but keep the invariant local.
	return `"` + strings.ReplaceAll(strings.Join(words, " "), `"`, `""`) + `"*`
}

// ftsAll ANDs the prefix phrases of terms ("" for no terms).
func ftsAll(terms [][]string) string {
	parts := make([]string, len(terms))
	for i, t := range terms {
		parts[i] = ftsPhrase(t)
	}
	return strings.Join(parts, " ")
}

// ftsColumns restricts the terms to the given columns, e.g.
// `{title author} : ("amazing"* "grace"*)`. With all, every term must match
// in those columns; otherwise any one term is enough (OR).
func ftsColumns(cols []string, terms [][]string, all bool) string {
	parts := make([]string, len(terms))
	for i, t := range terms {
		parts[i] = ftsPhrase(t)
	}
	sep := " "
	if !all {
		sep = " OR "
	}
	return "{" + strings.Join(cols, " ") + "} : (" + strings.Join(parts, sep) + ")"
}

// fold lower-cases s and strips diacritics (Café → cafe, Señor → senor),
// the normalisation the unicode61 tokenizer applies to the FTS indexes, for
// the searches that compare plain text (media and presentation titles and
// paths).
func fold(s string) string {
	s = norm.NFD.String(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) {
			continue // a combining mark, i.e. the diacritic NFD split off
		}
		if f, ok := foldLetters[r]; ok {
			b.WriteString(f)
			continue
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// foldLetters are the Latin letters NFD does not decompose.
var foldLetters = map[rune]string{
	'ø': "o", 'Ø': "o", 'ł': "l", 'Ł': "l", 'đ': "d", 'Đ': "d",
	'ß': "ss", 'æ': "ae", 'Æ': "ae", 'œ': "oe", 'Œ': "oe",
}

// textTerms splits a query for the plain-text searches: whitespace-separated
// words, folded. Punctuation stays, so "100%" or "_done" match literally.
func textTerms(query string) []string {
	terms := strings.Fields(fold(query))
	if len(terms) > maxTerms {
		terms = terms[:maxTerms]
	}
	return terms
}

// matchesAll reports whether every term occurs in at least one of the
// fields (compared folded). No terms match everything.
func matchesAll(terms []string, fields ...string) bool {
	if len(terms) == 0 {
		return true
	}
	folded := make([]string, len(fields))
	for i, f := range fields {
		folded[i] = fold(f)
	}
	for _, t := range terms {
		found := false
		for _, f := range folded {
			if strings.Contains(f, t) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
