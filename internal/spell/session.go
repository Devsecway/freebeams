package spell

import "unicode/utf8"

// Session is the Spelling dialog's walk through a text: it finds the
// misspellings one at a time from a position that moves forward as the
// operator changes or ignores them. "Ignore All" and "Add to Dictionary" go
// to the Checker (Ignore, Learn) and leave the position where it is: the word
// is accepted from then on.
type Session struct {
	Text string
	pos  int // rune offset: the next misspelling starts at or after it
}

// NewSession starts a walk at the beginning of text.
func NewSession(text string) *Session { return &Session{Text: text} }

// Pos is the rune offset the next misspelling is looked for from.
func (s *Session) Pos() int { return s.pos }

// Next returns the first misspelling at or after the position.
func (s *Session) Next(c Checker, lang string) (Misspelling, bool) {
	for _, m := range c.Check(s.Text, lang) {
		if m.Start >= s.pos {
			return m, true
		}
	}
	return Misspelling{}, false
}

// Skip moves past m ("Ignore": this occurrence only).
func (s *Session) Skip(m Misspelling) { s.pos = m.Start + m.Len }

// Change replaces the occurrence m with repl and moves past it.
func (s *Session) Change(m Misspelling, repl string) {
	rs := []rune(s.Text)
	if m.Start < 0 || m.Start+m.Len > len(rs) || string(rs[m.Start:m.Start+m.Len]) != m.Word {
		return // the text changed under the dialog
	}
	s.Text = string(rs[:m.Start]) + repl + string(rs[m.Start+m.Len:])
	s.pos = m.Start + utf8.RuneCountInString(repl)
}

// ChangeAll replaces every occurrence of m's word (whole words, same case)
// with repl, moves past m and returns how many it replaced.
func (s *Session) ChangeAll(m Misspelling, repl string) int {
	toks := Tokenize(s.Text)
	rs := []rune(s.Text)
	n, shift := 0, 0
	out := make([]rune, 0, len(rs))
	last := 0
	for _, t := range toks {
		if t.Word != m.Word {
			continue
		}
		out = append(out, rs[last:t.Start]...)
		out = append(out, []rune(repl)...)
		last = t.Start + t.Len
		if t.Start < m.Start {
			shift += utf8.RuneCountInString(repl) - t.Len
		}
		n++
	}
	if n == 0 {
		return 0
	}
	out = append(out, rs[last:]...)
	s.Text = string(out)
	s.pos = m.Start + shift + utf8.RuneCountInString(repl)
	return n
}
