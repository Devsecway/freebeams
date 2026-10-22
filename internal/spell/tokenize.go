package spell

import (
	"strings"
	"unicode"

	"freebeams/internal/models"
)

// Token is a word of a text to check, located like a Misspelling.
type Token struct {
	Start int    `json:"start"`
	Len   int    `json:"len"`
	Word  string `json:"word"`
	Line  int    `json:"line"`
	Col   int    `json:"col"`
}

// isApostrophe reports the apostrophes a word may contain ("I'll", "Lord’s").
func isApostrophe(r rune) bool { return r == '\'' || r == '’' || r == 'ʼ' }

// Tokenize splits text into the words to check. A word is a run of letters
// (with combining marks) that may contain apostrophes between letters
// ("I'll", "o'er"); leading and trailing apostrophes are quotes, not part of
// the word. These are never words to check:
//   - a section header line ("[Verse 1]", "Chorus:", "V1", "C2": models.IsSectionHeader);
//   - text inside brackets on any line ("[Repeat]", "(x2)" is skipped for its digit);
//   - a run that contains a digit ("V1", "2nd", "x3").
//
// Lines end at "\n" ("\r\n" works: "\r" is not a letter).
func Tokenize(text string) []Token {
	var out []Token
	start := 0 // rune offset of the line's first rune
	for line, s := range strings.Split(text, "\n") {
		if !models.IsSectionHeader(s) {
			out = tokenizeLine(out, []rune(s), line, start)
		}
		start += len([]rune(s)) + 1
	}
	return out
}

// tokenizeLine appends the words of one line (runes rs, line number line,
// starting at rune offset base of the text).
func tokenizeLine(out []Token, rs []rune, line, base int) []Token {
	bracket := 0
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case r == '[':
			bracket++
			i++
			continue
		case r == ']':
			if bracket > 0 {
				bracket--
			}
			i++
			continue
		case !isWordRune(r):
			i++
			continue
		}
		// A run of letters, digits, marks and apostrophes.
		j := i
		digit := false
		for j < len(rs) && (isWordRune(rs[j]) || isApostrophe(rs[j])) {
			if unicode.IsDigit(rs[j]) {
				digit = true
			}
			j++
		}
		a, b := i, j
		for b > a && isApostrophe(rs[b-1]) {
			b--
		}
		if bracket == 0 && !digit && b > a {
			out = append(out, Token{Start: base + a, Len: b - a, Word: string(rs[a:b]), Line: line, Col: a})
		}
		i = j
	}
	return out
}

// isWordRune reports letters, combining marks and digits (a digit makes the
// run a non-word).
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.Is(unicode.Mn, r) || unicode.IsDigit(r)
}
