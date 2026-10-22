package spell

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func words(toks []Token) []string {
	var out []string
	for _, t := range toks {
		out = append(out, t.Word)
	}
	return out
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"plain", "Amazing grace how sweet", []string{"Amazing", "grace", "how", "sweet"}},
		{"apostrophes", "I'll praise Thy name, Lord’s own 'love'", []string{"I'll", "praise", "Thy", "name", "Lord’s", "own", "love"}},
		{"bracket tags", "[Verse 1]\nHoly\n[Chorus]\n[Pre-Chorus]\nAmen", []string{"Holy", "Amen"}},
		{"bare tags", "V1\nO come\n\nC2\nChorus:\nVerse 2\nBridge\nlet us adore", []string{"O", "come", "let", "us", "adore"}},
		{"custom tag", "[Repeat x2]\n[Vamp]\nsing", []string{"sing"}},
		{"inline brackets", "sing [repeat] it (x2) 2nd time", []string{"sing", "it", "time"}},
		{"digits", "V1 x3 abc4 Psalm 23", []string{"Psalm"}},
		{"hyphen splits", "ever-lasting", []string{"ever", "lasting"}},
		{"crlf", "one\r\ntwo", []string{"one", "two"}},
		{"accents", "Señor café", []string{"Señor", "café"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := words(Tokenize(tt.text)); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Tokenize(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestTokenizePositions(t *testing.T) {
	toks := Tokenize("[V1]\nOh ça\n  recieve it")
	want := []Token{
		{Start: 5, Len: 2, Word: "Oh", Line: 1, Col: 0},
		{Start: 8, Len: 2, Word: "ça", Line: 1, Col: 3},
		{Start: 13, Len: 7, Word: "recieve", Line: 2, Col: 2},
		{Start: 21, Len: 2, Word: "it", Line: 2, Col: 10},
	}
	if !reflect.DeepEqual(toks, want) {
		t.Fatalf("got %+v\nwant %+v", toks, want)
	}
}

// fake is a backend that knows a fixed set of words.
type fake struct {
	known map[string]bool
	asked int
}

func (f *fake) Name() string        { return "fake" }
func (f *fake) Languages() []string { return []string{"en"} }
func (f *fake) Correct(ws []string, _ string) []bool {
	f.asked += len(ws)
	out := make([]bool, len(ws))
	for i, w := range ws {
		out[i] = f.known[strings.ToLower(w)]
	}
	return out
}
func (f *fake) Suggest(w, _ string) []string { return []string{w, "receive", "receive"} }

func newFake(ws ...string) *fake {
	f := &fake{known: map[string]bool{}}
	for _, w := range ws {
		f.known[w] = true
	}
	return f
}

func flagged(ms []Misspelling) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.Word)
	}
	return out
}

func TestEngineFlagsAndPersonalWords(t *testing.T) {
	e := New(newFake("i'll", "praise", "the", "name", "of"), []string{"Jabez"})
	text := "[Chorus]\nI'll praise the name of Jabez\nrecieve Hallelujah Yahweh Selah Thy\nqwzx qwzx"
	if got, want := flagged(e.Check(text, "")), []string{"recieve", "qwzx", "qwzx"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Check = %q, want %q", got, want)
	}
	e.Learn("Recieve")
	if got, want := flagged(e.Check(text, "")), []string{"qwzx", "qwzx"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after Learn: %q, want %q", got, want)
	}
	e.Ignore("QWZX")
	if got := e.Check(text, ""); len(got) != 0 {
		t.Fatalf("after Ignore: %q", flagged(got))
	}
	if got, want := e.Personal(), []string{"jabez", "recieve"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Personal = %q, want %q", got, want)
	}
	e.Forget("recieve")
	if got := flagged(e.Check("recieve", "")); !reflect.DeepEqual(got, []string{"recieve"}) {
		t.Fatalf("after Forget: %q", got)
	}
	// A possessive of a personal word; a typographic apostrophe.
	if got := e.Check("Jabez's Jabez’s I’ll", ""); len(got) != 0 {
		t.Fatalf("possessives: %q", flagged(got))
	}
	if got := e.Suggest("recieve", ""); !reflect.DeepEqual(got, []string{"receive"}) {
		t.Fatalf("Suggest = %q", got)
	}
}

func TestEngineCachesVerdicts(t *testing.T) {
	f := newFake("a")
	e := New(f, nil)
	e.Check("foo bar foo", "")
	if f.asked != 2 {
		t.Fatalf("asked %d words, want 2 (each once)", f.asked)
	}
	e.Check("foo bar", "")
	if f.asked != 2 {
		t.Fatalf("asked again: %d", f.asked)
	}
}

func TestWordlist(t *testing.T) {
	e := New(Wordlist(), nil)
	text := "[Verse 1]\nAmazing grace, how sweet the sound\nThat saved a wretch like me\nI'll recieve Thy love, THY LOVE, Jesus' name\nO Lord’s glory, Hosanna, Emmanuel, Maranatha"
	if got, want := flagged(e.Check(text, "")), []string{"recieve"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Check = %q, want %q", got, want)
	}
	if got := flagged(e.Check("london", "")); !reflect.DeepEqual(got, []string{"london"}) {
		t.Errorf("a proper noun in lower case should be flagged, got %q", got)
	}
	tests := []struct{ word, want string }{
		{"recieve", "receive"},
		{"Recieve", "Receive"},
		{"RECIEVE", "RECEIVE"},
		{"gracefull", "graceful"},
		{"frinedship", "friendship"},
	}
	for _, tt := range tests {
		got := e.Suggest(tt.word, "")
		if len(got) == 0 || got[0] != tt.want {
			t.Errorf("Suggest(%q) = %q, want %q first", tt.word, got, tt.want)
		}
	}
	if got := e.Suggest("jesuss", ""); !slices.Contains(got, "Jesus") {
		t.Errorf("Suggest(jesuss) = %q, want Jesus among them", got)
	}
}

func TestMatchCase(t *testing.T) {
	for _, tt := range []struct{ word, sugg, want string }{
		{"recieve", "receive", "receive"},
		{"Recieve", "receive", "Receive"},
		{"RECIEVE", "receive", "RECEIVE"},
	} {
		if got := MatchCase(tt.word, tt.sugg); got != tt.want {
			t.Errorf("MatchCase(%q, %q) = %q, want %q", tt.word, tt.sugg, got, tt.want)
		}
	}
}

func TestSession(t *testing.T) {
	e := New(newFake("a", "b", "fine"), nil)
	s := NewSession("xx fine yy\nxx b zz")
	m, ok := s.Next(e, "")
	if !ok || m.Word != "xx" || m.Start != 0 {
		t.Fatalf("first = %+v %v", m, ok)
	}
	s.Skip(m) // Ignore (once)
	if m, _ = s.Next(e, ""); m.Word != "yy" {
		t.Fatalf("after Skip: %+v", m)
	}
	s.Change(m, "fine")
	if s.Text != "xx fine fine\nxx b zz" {
		t.Fatalf("after Change: %q", s.Text)
	}
	if m, _ = s.Next(e, ""); m.Word != "xx" || m.Line != 1 {
		t.Fatalf("next: %+v", m)
	}
	if n := s.ChangeAll(m, "a"); n != 2 || s.Text != "a fine fine\na b zz" {
		t.Fatalf("ChangeAll = %d, %q", n, s.Text)
	}
	if m, _ = s.Next(e, ""); m.Word != "zz" {
		t.Fatalf("after ChangeAll: %+v (pos %d)", m, s.Pos())
	}
	e.Learn("zz") // Add to Dictionary
	if m, ok = s.Next(e, ""); ok {
		t.Fatalf("after Learn: %+v", m)
	}
}
