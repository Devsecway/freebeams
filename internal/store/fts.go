package store

import (
	"strings"
	"unicode"
)

// ftsQuery turns arbitrary user input into a safe FTS5 MATCH expression:
// every word becomes a quoted prefix term ("word"*) and the terms are ANDed.
//
// Words are split the same way the unicode61 tokenizer splits text (any rune
// that is not a letter, number or mark is a separator), so input like
// `pow'r`, `"amazing`, `AND`, `-grace` or `a*b(c)` never produces FTS5 syntax
// errors. Returns "" when the input contains no searchable words.
func ftsQuery(input string) string {
	words := ftsWords(input)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	for i, w := range words {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteByte('"')
		// Words never contain '"' (it is a separator), but escape anyway so
		// the invariant does not depend on ftsWords.
		b.WriteString(strings.ReplaceAll(w, `"`, `""`))
		b.WriteString(`"*`)
	}
	return b.String()
}

// ftsWords splits input into lower-cased tokens (letters, numbers, marks).
func ftsWords(input string) []string {
	const maxWords = 16 // keep pathological input cheap
	fields := strings.FieldsFunc(input, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r))
	})
	if len(fields) > maxWords {
		fields = fields[:maxWords]
	}
	for i, f := range fields {
		fields[i] = strings.ToLower(f)
	}
	return fields
}
