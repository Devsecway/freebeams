package models

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestBooksTable(t *testing.T) {
	if len(Books) != 66 {
		t.Fatalf("len(Books) = %d, want 66", len(Books))
	}
	total := 0
	for i, b := range Books {
		if b.Number != i+1 {
			t.Errorf("Books[%d].Number = %d", i, b.Number)
		}
		if b.Name == "" || b.Abbrev == "" {
			t.Errorf("book %d missing name/abbrev: %+v", b.Number, b)
		}
		for _, a := range b.Aliases {
			if a != strings.ToLower(a) {
				t.Errorf("%s alias %q not lower case", b.Name, a)
			}
		}
		total += ChapterCount(b.Number)
	}
	if total != 1189 {
		t.Errorf("total chapters = %d, want 1189", total)
	}
	if Books[0].Name != "Genesis" || Books[65].Name != "Revelation" || Books[18].Name != "Psalms" {
		t.Errorf("unexpected book order")
	}
	for _, n := range []int{0, -1, 67} {
		if ChapterCount(n) != 0 {
			t.Errorf("ChapterCount(%d) != 0", n)
		}
	}
}

// TestBookAliasesUnique ensures no name, abbreviation or alias names two books.
func TestBookAliasesUnique(t *testing.T) {
	owner := map[string]string{}
	for _, b := range Books {
		for _, name := range append([]string{b.Name, b.Abbrev}, b.Aliases...) {
			k := bookKey(name)
			if prev, ok := owner[k]; ok && prev != b.Name {
				t.Errorf("key %q (%q) used by %s and %s", k, name, prev, b.Name)
			}
			owner[k] = b.Name
		}
	}
	// Every name/abbrev/alias resolves to its own book.
	for _, b := range Books {
		for _, name := range append([]string{b.Name, b.Abbrev}, b.Aliases...) {
			got, err := FindBook(name)
			if err != nil || got.Number != b.Number {
				t.Errorf("FindBook(%q) = %v, %v; want %s", name, got.Name, err, b.Name)
			}
		}
	}
}

func TestBuildBookIndexDuplicates(t *testing.T) {
	idx := buildBookIndex([]Book{
		{1, "Alpha", "Al", []string{"x", "", "shared"}},
		{2, "Beta", "Be", []string{"shared"}},
	})
	if _, ok := idx.exact["shared"]; ok {
		t.Error("a key naming two books must not be an exact match")
	}
	if idx.exact["x"] != 1 || idx.exact["beta"] != 2 {
		t.Errorf("index %v", idx.exact)
	}
}

func TestBookByNumber(t *testing.T) {
	for _, tt := range []struct {
		n    int
		name string
		ok   bool
	}{{1, "Genesis", true}, {43, "John", true}, {66, "Revelation", true}, {0, "", false}, {67, "", false}, {-5, "", false}} {
		b, ok := BookByNumber(tt.n)
		if ok != tt.ok || b.Name != tt.name {
			t.Errorf("BookByNumber(%d) = %q, %v", tt.n, b.Name, ok)
		}
	}
}

func TestFindBook(t *testing.T) {
	tests := map[string]string{
		"Gen": "Genesis", "gn": "Genesis", "GE": "Genesis", "genesis": "Genesis", "Gen.": "Genesis",
		"Ex": "Exodus", "Exod": "Exodus", "Lev": "Leviticus", "Lv": "Leviticus", "Num": "Numbers", "Nm": "Numbers",
		"Deut": "Deuteronomy", "Dt": "Deuteronomy", "Josh": "Joshua", "Judg": "Judges", "Jdg": "Judges",
		"Ruth": "Ruth", "Ru": "Ruth",
		"1 Sam": "1 Samuel", "1Sa": "1 Samuel", "1 Sm": "1 Samuel", "I Samuel": "1 Samuel", "First Samuel": "1 Samuel",
		"1st Sam": "1 Samuel", "II Samuel": "2 Samuel", "2 Sam": "2 Samuel", "Second Sam.": "2 Samuel",
		"1 Kgs": "1 Kings", "2 Ki": "2 Kings", "1 Chr": "1 Chronicles", "II Chron": "2 Chronicles",
		"Ezra": "Ezra", "Neh": "Nehemiah", "Esth": "Esther", "Job": "Job",
		"Ps": "Psalms", "Psa": "Psalms", "Psalm": "Psalms", "Psalms": "Psalms", "pss": "Psalms",
		"Prov": "Proverbs", "Pr": "Proverbs", "Eccl": "Ecclesiastes", "Ecc": "Ecclesiastes", "Qoh": "Ecclesiastes",
		"Song": "Song of Solomon", "SoS": "Song of Solomon", "Song of Songs": "Song of Solomon",
		"Song of Solomon": "Song of Solomon", "Canticles": "Song of Solomon", "song of sol": "Song of Solomon",
		"Isa": "Isaiah", "Jer": "Jeremiah", "Lam": "Lamentations", "Ezek": "Ezekiel", "Eze": "Ezekiel",
		"Dan": "Daniel", "Hos": "Hosea", "Joel": "Joel", "Amos": "Amos", "Obad": "Obadiah", "Jonah": "Jonah",
		"Jon": "Jonah", "Mic": "Micah", "Nah": "Nahum", "Hab": "Habakkuk", "Zeph": "Zephaniah", "Hag": "Haggai",
		"Zech": "Zechariah", "Mal": "Malachi",
		"Matt": "Matthew", "Mt": "Matthew", "Mark": "Mark", "Mk": "Mark", "Mr": "Mark", "Luke": "Luke", "Lk": "Luke",
		"John": "John", "Jn": "John", "Jhn": "John", "Acts": "Acts", "Rom": "Romans",
		"1 Cor": "1 Corinthians", "1Co": "1 Corinthians", "I Cor.": "1 Corinthians", "2 Cor": "2 Corinthians",
		"Gal": "Galatians", "Eph": "Ephesians", "Phil": "Philippians", "Php": "Philippians", "Col": "Colossians",
		"1 Thess": "1 Thessalonians", "1Th": "1 Thessalonians", "2 Thess": "2 Thessalonians",
		"1 Tim": "1 Timothy", "2 Tim": "2 Timothy", "Titus": "Titus", "Tit": "Titus",
		"Phlm": "Philemon", "Philem": "Philemon", "Phm": "Philemon", "Heb": "Hebrews", "Jas": "James", "Jm": "James",
		"1 Pet": "1 Peter", "1Pe": "1 Peter", "1 Pt": "1 Peter", "2 Pet": "2 Peter",
		"1 John": "1 John", "1Jn": "1 John", "2 John": "2 John", "3 John": "3 John", "III John": "3 John",
		"Third John": "3 John", "Jude": "Jude",
		"Rev": "Revelation", "Re": "Revelation", "Rv": "Revelation", "Revelation": "Revelation", "Revelations": "Revelation",
		// Unambiguous prefixes.
		"Gene": "Genesis", "Phile": "Philemon", "Philip": "Philippians", "Lament": "Lamentations", "Revel": "Revelation",
		"1 Corin": "1 Corinthians", "Mar": "Mark", "  john  ": "John", "JOHN": "John",
	}
	for in, want := range tests {
		b, err := FindBook(in)
		if err != nil {
			t.Errorf("FindBook(%q): %v", in, err)
			continue
		}
		if b.Name != want {
			t.Errorf("FindBook(%q) = %s, want %s", in, b.Name, want)
		}
	}

	for _, in := range []string{"", " ", "J", "Ph", "Phi", "Jud", "Jo", "Ma", "1 C", "1 T", "Xyz", "4 John", "Samuel", "Corinthians", "ii"} {
		b, err := FindBook(in)
		if err == nil {
			t.Errorf("FindBook(%q) = %s, want error", in, b.Name)
		} else if !errors.Is(err, ErrBadReference) {
			t.Errorf("FindBook(%q) error %v does not wrap ErrBadReference", in, err)
		}
	}
	if _, err := FindBook("Ph"); err == nil || !strings.Contains(err.Error(), "Philippians") || !strings.Contains(err.Error(), "Philemon") {
		t.Errorf("ambiguous error should list candidates: %v", err)
	}
}

func vr(c, s, e int) VerseRange { return VerseRange{Chapter: c, VerseStart: s, VerseEnd: e} }

func TestParseReference(t *testing.T) {
	tests := []struct {
		in     string
		book   int
		ranges []VerseRange
		str    string // canonical String()
	}{
		{"jn 3", 43, []VerseRange{vr(3, 0, 0)}, "John 3"},
		{"jn 3:16", 43, []VerseRange{vr(3, 16, 16)}, "John 3:16"},
		{"jn3:16", 43, []VerseRange{vr(3, 16, 16)}, "John 3:16"},
		{"John 3:16-18", 43, []VerseRange{vr(3, 16, 18)}, "John 3:16-18"},
		{"John 3:16-18,20", 43, []VerseRange{vr(3, 16, 18), vr(3, 20, 20)}, "John 3:16-18, 20"},
		{"John 3:16,18", 43, []VerseRange{vr(3, 16, 16), vr(3, 18, 18)}, "John 3:16, 18"},
		{"John 3:36-4:2", 43, []VerseRange{vr(3, 36, 0), vr(4, 1, 2)}, "John 3:36-4:2"},
		{"John 3:36-5:2", 43, []VerseRange{vr(3, 36, 0), vr(4, 0, 0), vr(5, 1, 2)}, "John 3:36-5:2"},
		{"John 3:16-3:18", 43, []VerseRange{vr(3, 16, 18)}, "John 3:16-18"},
		{"1co13:4-7", 46, []VerseRange{vr(13, 4, 7)}, "1 Corinthians 13:4-7"},
		{"1 cor 13", 46, []VerseRange{vr(13, 0, 0)}, "1 Corinthians 13"},
		{"I Cor. 13:4", 46, []VerseRange{vr(13, 4, 4)}, "1 Corinthians 13:4"},
		{"First Corinthians 13:4", 46, []VerseRange{vr(13, 4, 4)}, "1 Corinthians 13:4"},
		{"ps 23:1-3", 19, []VerseRange{vr(23, 1, 3)}, "Psalms 23:1-3"},
		{"Ps 23", 19, []VerseRange{vr(23, 0, 0)}, "Psalms 23"},
		{"Ps 1-3", 19, []VerseRange{vr(1, 0, 0), vr(2, 0, 0), vr(3, 0, 0)}, "Psalms 1-3"},
		{"Ps 1, 3", 19, []VerseRange{vr(1, 0, 0), vr(3, 0, 0)}, "Psalms 1; 3"},
		{"Ps 119:176", 19, []VerseRange{vr(119, 176, 176)}, "Psalms 119:176"},
		{"Song of Solomon 2:1", 22, []VerseRange{vr(2, 1, 1)}, "Song of Solomon 2:1"},
		{"song of songs 2:1", 22, []VerseRange{vr(2, 1, 1)}, "Song of Solomon 2:1"},
		{"rev 22:21", 66, []VerseRange{vr(22, 21, 21)}, "Revelation 22:21"},
		{"Gen. 1:1", 1, []VerseRange{vr(1, 1, 1)}, "Genesis 1:1"},
		{"John 3:16–18", 43, []VerseRange{vr(3, 16, 18)}, "John 3:16-18"},
		{"John 3:16—18", 43, []VerseRange{vr(3, 16, 18)}, "John 3:16-18"},
		{"  John   3 : 16 - 18 ,  20  ", 43, []VerseRange{vr(3, 16, 18), vr(3, 20, 20)}, "John 3:16-18, 20"},
		{"John 3 v 16", 43, []VerseRange{vr(3, 16, 16)}, "John 3:16"},
		{"John 3v16", 43, []VerseRange{vr(3, 16, 16)}, "John 3:16"},
		{"John 3 vv 16-18", 43, []VerseRange{vr(3, 16, 18)}, "John 3:16-18"},
		{"John 3 verse 16", 43, []VerseRange{vr(3, 16, 16)}, "John 3:16"},
		{"John 3: v 16", 43, []VerseRange{vr(3, 16, 16)}, "John 3:16"},
		{"John 3.16", 43, []VerseRange{vr(3, 16, 16)}, "John 3:16"},
		{"John 3 16", 43, []VerseRange{vr(3, 16, 16)}, "John 3:16"},
		{"John 3:16.", 43, []VerseRange{vr(3, 16, 16)}, "John 3:16"},
		{"John 3:16ff", 43, []VerseRange{vr(3, 16, 0)}, "John 3:16ff"},
		{"John 3:16; 4:2", 43, []VerseRange{vr(3, 16, 16), vr(4, 2, 2)}, "John 3:16; 4:2"},
		{"John 3:16, 4:2", 43, []VerseRange{vr(3, 16, 16), vr(4, 2, 2)}, "John 3:16; 4:2"},
		{"John 3:16; 4", 43, []VerseRange{vr(3, 16, 16), vr(4, 0, 0)}, "John 3:16; 4"},
		{"John 3:16, 4", 43, []VerseRange{vr(3, 16, 16), vr(3, 4, 4)}, "John 3:16, 4"},
		{"John 3-4:2", 43, []VerseRange{vr(3, 0, 0), vr(4, 1, 2)}, "John 3-4:2"},
		{"John 3, 4:1-2", 43, []VerseRange{vr(3, 0, 0), vr(4, 1, 2)}, "John 3-4:2"},
		{"John 3-3:5", 43, []VerseRange{vr(3, 1, 5)}, "John 3:1-5"},
		{"John 3:16, 18-4:2", 43, []VerseRange{vr(3, 16, 16), vr(3, 18, 0), vr(4, 1, 2)}, "John 3:16, 18-4:2"},
		{"John 3:16ff; 4-5", 43, []VerseRange{vr(3, 16, 0), vr(4, 0, 0), vr(5, 0, 0)}, "John 3:16ff; 4-5"},
		{"John 3:16ff; 4", 43, []VerseRange{vr(3, 16, 0), vr(4, 0, 0)}, "John 3:16ff; 4"},
		{"John 3:36-4:2,4", 43, []VerseRange{vr(3, 36, 0), vr(4, 1, 2), vr(4, 4, 4)}, "John 3:36-4:2, 4"},
		{"Ca 1, 2 1ff", 22, []VerseRange{vr(1, 0, 0), vr(2, 1, 0)}, "Song of Solomon 1; 2:1ff"}, // fuzz regression
		{"John 3:16-16", 43, []VerseRange{vr(3, 16, 16)}, "John 3:16"},
		{"Jn 3:16;", 43, nil, ""}, // error (trailing separator)
		// Single-chapter books.
		{"Jude 3", 65, []VerseRange{vr(1, 3, 3)}, "Jude 1:3"},
		{"Jude 3-5", 65, []VerseRange{vr(1, 3, 5)}, "Jude 1:3-5"},
		{"Jude 1:3", 65, []VerseRange{vr(1, 3, 3)}, "Jude 1:3"},
		{"Jude", 65, []VerseRange{vr(1, 0, 0)}, "Jude"},
		{"Obad 1-4", 31, []VerseRange{vr(1, 1, 4)}, "Obadiah 1:1-4"},
		{"Phlm 6", 57, []VerseRange{vr(1, 6, 6)}, "Philemon 1:6"},
		{"2 John 5", 63, []VerseRange{vr(1, 5, 5)}, "2 John 1:5"},
		{"3jn 2,4", 64, []VerseRange{vr(1, 2, 2), vr(1, 4, 4)}, "3 John 1:2, 4"},
		{"1 John 1:9", 62, []VerseRange{vr(1, 9, 9)}, "1 John 1:9"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			p, err := ParseReference(tt.in)
			if tt.ranges == nil {
				if err == nil {
					t.Fatalf("ParseReference(%q) = %+v, want error", tt.in, p)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseReference(%q): %v", tt.in, err)
			}
			want := Passage{Book: tt.book, Ranges: tt.ranges}
			if !reflect.DeepEqual(p, want) {
				t.Fatalf("ParseReference(%q) = %+v, want %+v", tt.in, p, want)
			}
			if got := p.String(); got != tt.str {
				t.Errorf("String() = %q, want %q", got, tt.str)
			}
			// The canonical form parses back to the same passage.
			again, err := ParseReference(p.String())
			if err != nil || !reflect.DeepEqual(again, p) {
				t.Errorf("reparse %q = %+v, %v; want %+v", p.String(), again, err, p)
			}
		})
	}
}

func TestParseReferenceErrors(t *testing.T) {
	for _, in := range []string{
		"", "   ", "John", "Psalms", "3:16", "16", "1", "J 3:16", "Ph 1:1", "Jud 1", "Xyz 1:1",
		"John 0", "John 0:1", "John 3:0", "John 22", "John 3:177", "Jude 2:1", "Jude 0",
		"John 3:18-16", "John 4:2-3:36", "Ps 3-1", "John 3:16-4", "John 3:16-", "John -3", "John 3::",
		"John 3:16,,17", "John 3:16,", "John :16", "John 3:16 KJV", "John 3:16abc", "John 3ff", "John 3:16-18ff",
		"John 3:16ff17", "John 3:16:17", "John 3 16 18", "John 99999", "John 3:99999", "John 3:1☃",
		"John three", "Ps 151", "Rev 23", "4 John 1", "John 3:16-4:2-5", "John (3:16)",
	} {
		p, err := ParseReference(in)
		if err == nil {
			t.Errorf("ParseReference(%q) = %+v (%s), want error", in, p, p.String())
			continue
		}
		if !errors.Is(err, ErrBadReference) {
			t.Errorf("ParseReference(%q) error %v does not wrap ErrBadReference", in, err)
		}
	}
}

func TestPassageString(t *testing.T) {
	tests := []struct {
		p    Passage
		want string
	}{
		{Passage{Book: 43}, "John"},
		{Passage{Book: 99, Ranges: []VerseRange{vr(1, 1, 1)}}, "Book 99 1:1"},
		{Passage{Book: 43, Ranges: []VerseRange{vr(3, 0, 5)}}, "John 3:1-5"},
		{Passage{Book: 43, Ranges: []VerseRange{vr(3, 0, 0), vr(4, 0, 0)}}, "John 3-4"},
		{Passage{Book: 43, Ranges: []VerseRange{vr(5, 1, 1), vr(3, 2, 2)}}, "John 5:1; 3:2"},
		{Passage{Book: 43, Ranges: []VerseRange{vr(3, 16, 0), vr(4, 0, 0), vr(5, 0, 0), vr(7, 1, 1)}}, "John 3:16ff; 4-5; 7:1"},
		{Passage{Book: 65, Ranges: []VerseRange{vr(1, 0, 0)}}, "Jude"},
		{Passage{Book: 46, Ranges: []VerseRange{vr(13, 0, 0)}}, "1 Corinthians 13"},
	}
	for _, tt := range tests {
		if got := tt.p.String(); got != tt.want {
			t.Errorf("%+v.String() = %q, want %q", tt.p, got, tt.want)
		}
	}
}

func TestScriptureSlides(t *testing.T) {
	p, err := ParseReference("John 3:16-17")
	if err != nil {
		t.Fatal(err)
	}
	verses := []Verse{
		{Book: 43, Chapter: 3, Verse: 16, Text: "  For God so loved\n the world,\t that he gave "},
		{Book: 0, Chapter: 3, Verse: 17, Text: "For God sent not his Son"},
		{Book: 43, Chapter: 3, Verse: 18, Text: "   "},
	}
	got := ScriptureSlides(p, verses, " KJV ", OneVersePerSlide)
	want := []Slide{
		{Kind: SlideScripture, Label: "John 3:16", Text: "For God so loved the world, that he gave", Footer: "John 3:16 (KJV)"},
		{Kind: SlideScripture, Label: "John 3:17", Text: "For God sent not his Son", Footer: "John 3:17 (KJV)"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ScriptureSlides\n got %#v\nwant %#v", got, want)
	}
	noAbbrev := ScriptureSlides(p, verses[:1], "", "")
	if len(noAbbrev) != 1 || noAbbrev[0].Footer != "John 3:16" {
		t.Errorf("no abbrev footer: %#v", noAbbrev)
	}
	if got := ScriptureSlides(p, nil, "KJV", OneVersePerSlide); len(got) != 0 {
		t.Errorf("no verses: %v", got)
	}
	unknown := ScriptureSlides(Passage{Book: 99}, []Verse{{Chapter: 1, Verse: 2, Text: "x"}}, "", OneVersePerSlide)
	if unknown[0].Label != "Book 99 1:2" {
		t.Errorf("unknown book label %q", unknown[0].Label)
	}
}

func FuzzParseReference(f *testing.F) {
	for _, seed := range []string{
		"jn 3:16", "John 3:16-18,20", "1 Cor 13", "1co13:4-7", "Ps 23:1-3", "Song of Solomon 2:1",
		"rev 22:21", "John 3:36-4:2", "Jude 3", "Gen. 1:1", "John 3 v 16", "John 3:16ff; 4-5",
		"Ps 1-3", "III jn 2", "John 3:16–18", "", "3:16", "J 1", "John 3:16,,", "John 3-4:2",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		p, err := ParseReference(s)
		if err != nil {
			if !errors.Is(err, ErrBadReference) {
				t.Fatalf("error %v does not wrap ErrBadReference", err)
			}
			return
		}
		if _, ok := BookByNumber(p.Book); !ok || len(p.Ranges) == 0 {
			t.Fatalf("ParseReference(%q) = %+v: bad book or no ranges", s, p)
		}
		for _, r := range p.Ranges {
			if r.Chapter < 1 || r.Chapter > ChapterCount(p.Book) || r.VerseStart < 0 || r.VerseEnd < 0 ||
				(r.VerseEnd > 0 && r.VerseEnd < r.VerseStart) || r.VerseStart > MaxVerse || r.VerseEnd > MaxVerse {
				t.Fatalf("ParseReference(%q) = %+v: invalid range %+v", s, p, r)
			}
		}
		str := p.String()
		again, err := ParseReference(str)
		if err != nil || !reflect.DeepEqual(again, p) {
			t.Fatalf("ParseReference(%q) = %+v; String %q reparses to %+v, %v", s, p, str, again, err)
		}
	})
}
