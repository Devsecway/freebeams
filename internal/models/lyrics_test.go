package models

import (
	"reflect"
	"strings"
	"testing"
)

// sec is a test helper building a Section with the canonical label.
func sec(kind SectionKind, n int, lines ...string) Section {
	return Section{Kind: kind, Number: n, Label: SectionLabel(kind, n), Lines: lines}
}

func other(label string, lines ...string) Section {
	return Section{Kind: SectionOther, Label: label, Lines: lines}
}

func TestSectionLabel(t *testing.T) {
	tests := []struct {
		kind SectionKind
		n    int
		want string
	}{
		{SectionVerse, 2, "Verse 2"},
		{SectionVerse, 0, "Verse"},
		{SectionChorus, 0, "Chorus"},
		{SectionChorus, 3, "Chorus 3"},
		{SectionPreChorus, 0, "Pre-Chorus"},
		{SectionBridge, 1, "Bridge 1"},
		{SectionTag, 0, "Tag"},
		{SectionIntro, 0, "Intro"},
		{SectionEnding, 0, "Ending"},
		{SectionOther, 0, "Other"},
		{SectionKind("vamp"), 2, "Vamp 2"},
		{SectionKind(""), 1, ""},
		{SectionVerse, -1, "Verse"},
	}
	for _, tt := range tests {
		if got := SectionLabel(tt.kind, tt.n); got != tt.want {
			t.Errorf("SectionLabel(%q, %d) = %q, want %q", tt.kind, tt.n, got, tt.want)
		}
	}
}

func TestParseLyrics(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Section
	}{
		{"empty", "", nil},
		{"whitespace only", " \n\t\n\r\n  ", nil},
		{
			"untagged stanzas",
			"Amazing grace\nhow sweet the sound\n\n\nThat saved a wretch\nlike me\n",
			[]Section{
				sec(SectionVerse, 1, "Amazing grace", "how sweet the sound"),
				sec(SectionVerse, 2, "That saved a wretch", "like me"),
			},
		},
		{
			"crlf and trimming",
			"  \r\n  line one  \r\n\tline two\r\n\r\n line three\rline four \r\n",
			[]Section{
				sec(SectionVerse, 1, "line one", "line two"),
				sec(SectionVerse, 2, "line three", "line four"),
			},
		},
		{
			"bracket tags",
			"[Verse 1]\na\nb\n[Chorus]\nc\n\n[Verse 2]\nd",
			[]Section{
				sec(SectionVerse, 1, "a", "b"),
				sec(SectionChorus, 0, "c"),
				sec(SectionVerse, 2, "d"),
			},
		},
		{
			"short bracket tags",
			"[V1]\na\n[C]\nb\n[C2]\nc\n[B]\nd\n[PC]\ne\n[T]\nf\n[I]\ng\n[E]\nh\n[v 3]\ni",
			[]Section{
				sec(SectionVerse, 1, "a"),
				sec(SectionChorus, 0, "b"),
				sec(SectionChorus, 2, "c"),
				sec(SectionBridge, 0, "d"),
				sec(SectionPreChorus, 0, "e"),
				sec(SectionTag, 0, "f"),
				sec(SectionIntro, 0, "g"),
				sec(SectionEnding, 0, "h"),
				sec(SectionVerse, 3, "i"),
			},
		},
		{
			"case-insensitive and long variants",
			"[VERSE 1]\na\n[chorus 2]\nb\n[Pre-Chorus]\nc\n[PreChorus]\nd\n[pre chorus 2]\ne\n[Bridge 2]\nf\n[Outro]\ng\n[Ending]\nh\n[Refrain]\ni\n[Intro]\nj\n[Tag]\nk",
			[]Section{
				sec(SectionVerse, 1, "a"),
				sec(SectionChorus, 2, "b"),
				sec(SectionPreChorus, 0, "c"),
				sec(SectionPreChorus, 0, "d"),
				sec(SectionPreChorus, 2, "e"),
				sec(SectionBridge, 2, "f"),
				sec(SectionEnding, 0, "g"),
				sec(SectionEnding, 0, "h"),
				sec(SectionChorus, 0, "i"),
				sec(SectionIntro, 0, "j"),
				sec(SectionTag, 0, "k"),
			},
		},
		{
			"colon headers",
			"Verse 1:\na\nb\n\nChorus:\nc\nPre-Chorus 2:\nd",
			[]Section{
				sec(SectionVerse, 1, "a", "b"),
				sec(SectionChorus, 0, "c"),
				sec(SectionPreChorus, 2, "d"),
			},
		},
		{
			"colon lines that are not headers stay lyrics",
			"[Verse 1]\nHe said:\nV:\nFoo:",
			[]Section{sec(SectionVerse, 1, "He said:", "V:", "Foo:")},
		},
		{
			"bare headers at stanza start",
			"Verse 1\na\nb\n\nChorus\nc\n\nverse 2\nd",
			[]Section{
				sec(SectionVerse, 1, "a", "b"),
				sec(SectionChorus, 0, "c"),
				sec(SectionVerse, 2, "d"),
			},
		},
		{
			"bare header mid-stanza is a lyric",
			"[Chorus]\nSing\nChorus\nagain",
			[]Section{sec(SectionChorus, 0, "Sing", "Chorus", "again")},
		},
		{
			"bare short forms are lyrics",
			"V1\nC\na",
			[]Section{sec(SectionVerse, 1, "V1", "C", "a")},
		},
		{
			"unknown tag keeps custom label",
			"[Vamp]\nOh oh\n[Repeat x2]\nyeah\n[ Spoken Word ]\nhey",
			[]Section{
				other("Vamp", "Oh oh"),
				other("Repeat x2", "yeah"),
				other("Spoken Word", "hey"),
			},
		},
		{
			"blank line inside tagged section starts a new slide of the same section",
			"[Chorus]\na\nb\n\nc\nd\n\n\n[Verse 2]\ne",
			[]Section{
				sec(SectionChorus, 0, "a", "b"),
				sec(SectionChorus, 0, "c", "d"),
				sec(SectionVerse, 2, "e"),
			},
		},
		{
			"untagged stanzas before the first tag",
			"intro line\n\nsecond\n[Chorus]\nc",
			[]Section{
				sec(SectionVerse, 1, "intro line"),
				sec(SectionVerse, 2, "second"),
				sec(SectionChorus, 0, "c"),
			},
		},
		{
			"empty sections dropped",
			"[Verse 1]\n\n[Chorus]\n[Bridge]\n\nb\n[Tag]\n",
			[]Section{sec(SectionBridge, 0, "b")},
		},
		{
			"empty brackets are lyrics",
			"[]\n[  ]\n[",
			[]Section{sec(SectionVerse, 1, "[]", "[  ]", "[")},
		},
		{
			"oversized numbers become custom labels",
			"[Verse 1000]\na\n[Verse 99999999999999999999]\nb\n[Verse 007]\nc",
			[]Section{
				other("Verse 1000", "a"),
				other("Verse 99999999999999999999", "b"),
				sec(SectionVerse, 7, "c"),
			},
		},
		{
			"unicode lyrics",
			"[Verse 1]\nJésus, ô toi\n Señor ",
			[]Section{sec(SectionVerse, 1, "Jésus, ô toi", "Señor")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseLyrics(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseLyrics(%q)\n got %#v\nwant %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestFormatLyrics(t *testing.T) {
	tests := []struct {
		name string
		in   []Section
		want string
	}{
		{"nil", nil, ""},
		{
			"basic",
			[]Section{sec(SectionVerse, 1, "a", "b"), sec(SectionChorus, 0, "c")},
			"[Verse 1]\na\nb\n\n[Chorus]\nc",
		},
		{
			"same section continuation",
			[]Section{sec(SectionChorus, 0, "a"), sec(SectionChorus, 0, "b"), sec(SectionVerse, 2, "c")},
			"[Chorus]\na\n\nb\n\n[Verse 2]\nc",
		},
		{
			"continuation repeats header when first line looks like a header",
			[]Section{sec(SectionChorus, 0, "a"), sec(SectionChorus, 0, "Verse", "b")},
			"[Chorus]\na\n\n[Chorus]\nVerse\nb",
		},
		{
			"canonical labels for known kinds, custom for other",
			[]Section{
				{Kind: SectionVerse, Number: 2, Label: "Strophe", Lines: []string{"a"}},
				{Kind: SectionOther, Label: "Vamp", Lines: []string{"b"}},
				{Kind: SectionOther, Lines: []string{"c"}},
				{Kind: SectionKind("solo"), Lines: []string{"d"}},
				{Kind: "", Number: 3, Label: "", Lines: []string{"e"}},
				{Kind: "", Label: "Custom", Lines: []string{"f"}},
			},
			"[Verse 2]\na\n\n[Vamp]\nb\n\n[Other]\nc\n\n[Solo]\nd\n\n[Other 3]\ne\n\n[Custom]\nf",
		},
		{
			"blank and multi-line entries cleaned, empty sections skipped",
			[]Section{
				sec(SectionVerse, 1, " a ", "", "b\nc", "  "),
				sec(SectionChorus, 0),
				sec(SectionBridge, 0, "", " "),
				sec(SectionTag, 0, "t"),
			},
			"[Verse 1]\na\nb\nc\n\n[Tag]\nt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatLyrics(tt.in); got != tt.want {
				t.Errorf("FormatLyrics\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestLyricsRoundTrip(t *testing.T) {
	texts := []string{
		"Amazing grace\nhow sweet\n\nThat saved\na wretch",
		"[Verse 1]\na\nb\n\nc\n[Chorus]\nd\n\ne\n\n[Chorus]\nf\n[Vamp]\ng\n\nh",
		"Verse 1\nChorus\nx\n\nChorus\nVerse\ny",
		"[Chorus]\nA\n[Chorus]\nVerse\n\n[Outro]\nbye",
		"[C]\none\n[Pre Chorus 2]\ntwo\n[Repeat]\nthree\n[]\n",
		"[Verse 1000]\nx\n\n[Ending]\nend",
	}
	for _, text := range texts {
		first := ParseLyrics(text)
		formatted := FormatLyrics(first)
		second := ParseLyrics(formatted)
		if !reflect.DeepEqual(first, second) {
			t.Errorf("round trip of %q via %q\n got %#v\nwant %#v", text, formatted, second, first)
		}
		if again := FormatLyrics(second); again != formatted {
			t.Errorf("FormatLyrics not stable: %q vs %q", again, formatted)
		}
	}

	// Hand-built canonical sections round-trip too.
	sections := []Section{
		sec(SectionIntro, 0, "la la"),
		sec(SectionVerse, 1, "a", "b"),
		sec(SectionPreChorus, 0, "c"),
		sec(SectionChorus, 0, "d"),
		sec(SectionChorus, 0, "e"),
		sec(SectionBridge, 2, "f"),
		other("Vamp", "g"),
		sec(SectionTag, 0, "h"),
		sec(SectionEnding, 0, "i"),
	}
	if got := ParseLyrics(FormatLyrics(sections)); !reflect.DeepEqual(got, sections) {
		t.Errorf("canonical round trip\n got %#v\nwant %#v", got, sections)
	}
}

func lines(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "line" + string(rune('A'+i))
	}
	return out
}

func slideLineCounts(slides []Slide) []int {
	var out []int
	for _, s := range slides {
		out = append(out, len(strings.Split(s.Text, "\n")))
	}
	return out
}

func TestSongSlidesSplitting(t *testing.T) {
	tests := []struct {
		lines, max int
		want       []int
	}{
		{7, 4, []int{4, 3}},
		{8, 3, []int{3, 3, 2}},
		{4, 4, []int{4}},
		{5, 4, []int{3, 2}},
		{9, 4, []int{3, 3, 3}},
		{10, 4, []int{4, 3, 3}},
		{6, 0, []int{6}},
		{6, -1, []int{6}},
		{3, 1, []int{1, 1, 1}},
		{1, 2, []int{1}},
	}
	for _, tt := range tests {
		song := Song{Sections: []Section{sec(SectionVerse, 1, lines(tt.lines)...)}}
		slides := song.Slides(SlideOptions{MaxLines: tt.max})
		if got := slideLineCounts(slides); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%d lines, max %d: got %v, want %v", tt.lines, tt.max, got, tt.want)
		}
		// All lines preserved in order.
		var all []string
		for _, s := range slides {
			all = append(all, strings.Split(s.Text, "\n")...)
			if s.Label != "Verse 1" || s.Group != SectionVerse || s.GroupNumber != 1 || s.Kind != SlideSong {
				t.Errorf("slide metadata wrong: %+v", s)
			}
		}
		if !reflect.DeepEqual(all, lines(tt.lines)) {
			t.Errorf("%d lines, max %d: lines not preserved: %v", tt.lines, tt.max, all)
		}
	}
}

func TestSongSlides(t *testing.T) {
	song := Song{
		Title:     "Amazing Grace",
		Author:    "John Newton",
		Copyright: "Public Domain",
		CCLI:      "22025",
		Sections: []Section{
			sec(SectionVerse, 1, "Amazing grace", "how sweet the sound"),
			sec(SectionChorus, 0),          // empty: skipped
			sec(SectionChorus, 0, " ", ""), // blank: skipped
			sec(SectionChorus, 0, "My chains are gone"),
			{Kind: SectionBridge, Number: 2, Lines: []string{"b"}}, // no label: derived
			other("Vamp", "oh"),
		},
	}
	footer := `"Amazing Grace" by John Newton · © Public Domain · CCLI #22025`

	got := song.Slides(DefaultSlideOptions())
	want := []Slide{
		{Kind: SlideSong, Label: "Verse 1", Group: SectionVerse, GroupNumber: 1, Text: "Amazing grace\nhow sweet the sound"},
		{Kind: SlideSong, Label: "Chorus", Group: SectionChorus, Text: "My chains are gone"},
		{Kind: SlideSong, Label: "Bridge 2", Group: SectionBridge, GroupNumber: 2, Text: "b"},
		{Kind: SlideSong, Label: "Vamp", Group: SectionOther, Text: "oh", Footer: footer},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Slides(default)\n got %#v\nwant %#v", got, want)
	}

	every := song.Slides(SlideOptions{IncludeCopyright: true, CopyrightEverySlide: true})
	for i, s := range every {
		if s.Footer != footer {
			t.Errorf("every slide: slide %d footer %q", i, s.Footer)
		}
	}
	for i, s := range song.Slides(SlideOptions{CopyrightEverySlide: true}) {
		if s.Footer != "" {
			t.Errorf("no copyright: slide %d footer %q", i, s.Footer)
		}
	}
	if got := (Song{Title: "Empty"}).Slides(DefaultSlideOptions()); len(got) != 0 {
		t.Errorf("song without sections: %v", got)
	}
	// No footer fields at all: no footer even when requested.
	bare := Song{Sections: []Section{sec(SectionVerse, 1, "x")}}
	if got := bare.Slides(DefaultSlideOptions()); got[0].Footer != "" {
		t.Errorf("bare song footer %q", got[0].Footer)
	}
}

func TestCopyrightFooter(t *testing.T) {
	tests := []struct {
		song Song
		want string
	}{
		{Song{Title: "T", Author: "A", Copyright: "2001 X", CCLI: "123"}, `"T" by A · © 2001 X · CCLI #123`},
		{Song{Title: "T"}, `"T"`},
		{Song{Author: "A"}, `A`},
		{Song{Title: "T", Copyright: "© 2001 X"}, `"T" · © 2001 X`},
		{Song{Copyright: "(C) 2001 X"}, `© 2001 X`},
		{Song{Copyright: "Copyright © 2001 X"}, `© 2001 X`},
		{Song{Copyright: "copyright"}, ``},
		{Song{CCLI: " #456 "}, `CCLI #456`},
		{Song{Title: "  ", Author: " ", Copyright: " ", CCLI: "#"}, ``},
		{Song{}, ``},
	}
	for _, tt := range tests {
		if got := tt.song.CopyrightFooter(); got != tt.want {
			t.Errorf("CopyrightFooter(%+v) = %q, want %q", tt.song, got, tt.want)
		}
	}
}

func TestSongSummary(t *testing.T) {
	s := Song{ID: 7, Title: "T", Author: "A", Copyright: "C", CCLI: "1", Tags: "x"}
	want := SongSummary{ID: 7, Title: "T", Author: "A", Copyright: "C", CCLI: "1"}
	if got := s.Summary(); got != want {
		t.Errorf("Summary() = %+v, want %+v", got, want)
	}
}

func FuzzParseLyrics(f *testing.F) {
	for _, seed := range []string{
		"", "[Verse 1]\na\n\nb", "Chorus:\nx", "Verse\n\nChorus\nVerse", "[\n]\n[]]", "[V9999]\na",
		"a\r\n\r\nb\rc", "[Chorus]\nA\n[Chorus]\nVerse", " [ C ] \nx", "[Pre--Chorus..2]\nx",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		first := ParseLyrics(text)
		for _, s := range first {
			if len(s.Lines) == 0 || s.Label == "" {
				t.Fatalf("empty section or label: %#v", s)
			}
		}
		second := ParseLyrics(FormatLyrics(first))
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("round trip mismatch for %q\nfirst  %#v\nsecond %#v", text, first, second)
		}
	})
}
