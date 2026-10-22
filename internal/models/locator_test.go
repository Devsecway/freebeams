package models

import (
	"errors"
	"reflect"
	"testing"
)

func bookNames(bs []Book) []string {
	var out []string
	for _, b := range bs {
		out = append(out, b.Name)
	}
	return out
}

func TestBooksMatching(t *testing.T) {
	tests := []struct {
		prefix string
		want   []string
	}{
		{"", nil},
		{"   ", nil},
		{"xyz", nil},
		{"joh", []string{"John", "1 John", "2 John", "3 John"}},
		{"JOH", []string{"John", "1 John", "2 John", "3 John"}},
		{"jn", []string{"Jonah", "John", "1 John", "2 John", "3 John"}}, // Jonah's "jnh"
		{"jo", []string{"Joshua", "Job", "Joel", "Jonah", "John", "1 John", "2 John", "3 John"}},
		{"gen", []string{"Genesis"}},
		{"ge", []string{"Genesis"}},
		{"ph", []string{"Philippians", "Philemon"}},
		{"1 jo", []string{"1 John"}},
		{"I John", []string{"1 John"}},
		{"first jo", []string{"1 John"}},
		{"1", []string{"1 Samuel", "1 Kings", "1 Chronicles", "1 Corinthians", "1 Thessalonians", "1 Timothy", "1 Peter", "1 John"}},
		{"2 c", []string{"2 Chronicles", "2 Corinthians"}},
		{"sa", []string{"1 Samuel", "2 Samuel"}},
		{"song of", []string{"Song of Solomon"}},
		{"rev.", []string{"Revelation"}},
	}
	for _, tt := range tests {
		if got := bookNames(BooksMatching(tt.prefix)); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("BooksMatching(%q) = %q, want %q", tt.prefix, got, tt.want)
		}
	}
}

func TestFindBookAmbiguousError(t *testing.T) {
	_, err := FindBook("jo")
	var amb *AmbiguousBookError
	if !errors.As(err, &amb) {
		t.Fatalf("FindBook(jo) error %v is not an *AmbiguousBookError", err)
	}
	if !errors.Is(err, ErrBadReference) {
		t.Errorf("the ambiguous error does not wrap ErrBadReference")
	}
	if got := bookNames(amb.Books); !reflect.DeepEqual(got, []string{"Joshua", "Job", "Joel", "Jonah", "John"}) {
		t.Errorf("candidates = %q", got)
	}
	want := `bad scripture reference: ambiguous book "jo" (Joshua, Job, Joel, Jonah, John)`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
	// Through ParseReference too.
	if _, err := ParseReference("jo 3"); !errors.As(err, &amb) {
		t.Errorf("ParseReference(jo 3) error %v is not an *AmbiguousBookError", err)
	}
}

func TestBookCompletions(t *testing.T) {
	johns := []string{"John", "1 John", "2 John", "3 John"}
	tests := []struct {
		text string
		want []string
	}{
		{"", nil},
		{" ", nil},
		{"j", []string{"Joshua", "Judges", "Job", "Jeremiah", "Joel", "Jonah", "John", "James", "1 John", "2 John", "3 John", "Jude"}},
		{"joh", johns},
		{"john", johns},
		{"John ", nil},   // completed
		{"john ", nil},   // completed, any case
		{"1 John ", nil}, // completed
		{"joh ", johns},  // an alias, not a completed name
		{"gen", []string{"Genesis"}},
		{"Genesis ", nil},
		{"jo 3", []string{"Joshua", "Job", "Joel", "Jonah", "John", "1 John", "2 John", "3 John"}}, // ambiguous with a chapter
		{"john 3", nil},
		{"joh 3:16", nil},
		{"xyz", nil},
		{"xyz 3", nil},
		{"1", []string{"1 Samuel", "1 Kings", "1 Chronicles", "1 Corinthians", "1 Thessalonians", "1 Timothy", "1 Peter", "1 John"}},
		{"3 ", []string{"3 John"}},
		{"1 jo", []string{"1 John"}},
		{"3:16", nil},
	}
	for _, tt := range tests {
		if got := bookNames(BookCompletions(tt.text)); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("BookCompletions(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}

func TestCompleteBook(t *testing.T) {
	john, _ := BookByNumber(43)
	john1, _ := BookByNumber(62)
	tests := []struct {
		text string
		b    Book
		want string
	}{
		{"joh", john, "John "},
		{"joh", john1, "1 John "},
		{"jo 3:16", john, "John 3:16"},
		{"  jo   3 ", john, "John 3"},
		{"1", john1, "1 John "},
		{"1 jo", john1, "1 John "},
	}
	for _, tt := range tests {
		if got := CompleteBook(tt.text, tt.b); got != tt.want {
			t.Errorf("CompleteBook(%q, %s) = %q, want %q", tt.text, tt.b.Name, got, tt.want)
		}
	}
}

func TestParseLocator(t *testing.T) {
	tests := []struct {
		text string
		want Passage
	}{
		{"gen", Passage{1, []VerseRange{{1, 0, 0}}}},
		{"Genesis ", Passage{1, []VerseRange{{1, 0, 0}}}},
		{"joh", Passage{43, []VerseRange{{1, 0, 0}}}}, // "joh" is John's alias
		{"John ", Passage{43, []VerseRange{{1, 0, 0}}}},
		{"rev.", Passage{66, []VerseRange{{1, 0, 0}}}},
		{"jude", Passage{65, []VerseRange{{1, 0, 0}}}},
		{"john 3", Passage{43, []VerseRange{{3, 0, 0}}}},
		{"john 3:16-18", Passage{43, []VerseRange{{3, 16, 18}}}},
	}
	for _, tt := range tests {
		got, err := ParseLocator(tt.text)
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseLocator(%q) = %+v, %v; want %+v", tt.text, got, err, tt.want)
		}
	}
	errs := []struct {
		text      string
		ambiguous bool
	}{
		{"", false},
		{"xyz", false},
		{"xyz 3", false},
		{"jo", true},
		{"jo 3", true},
		{"1", true},
		{"john 99", false},
	}
	for _, tt := range errs {
		_, err := ParseLocator(tt.text)
		if err == nil || !errors.Is(err, ErrBadReference) {
			t.Errorf("ParseLocator(%q) error = %v, want a bad reference", tt.text, err)
			continue
		}
		var amb *AmbiguousBookError
		if errors.As(err, &amb) != tt.ambiguous {
			t.Errorf("ParseLocator(%q) error %v: ambiguous = %v, want %v", tt.text, err, !tt.ambiguous, tt.ambiguous)
		}
	}
}

func TestPassageContains(t *testing.T) {
	p := Passage{43, []VerseRange{{3, 16, 18}, {3, 20, 20}, {4, 0, 0}, {5, 40, 0}, {6, 9, 7}}}
	tests := []struct {
		ch, v int
		want  bool
	}{
		{3, 15, false}, {3, 16, true}, {3, 18, true}, {3, 19, false}, {3, 20, true},
		{4, 1, true}, {4, 54, true}, {5, 39, false}, {5, 47, true},
		{6, 7, true}, {6, 9, true}, {6, 10, false}, {2, 1, false},
	}
	for _, tt := range tests {
		if got := p.Contains(tt.ch, tt.v); got != tt.want {
			t.Errorf("Contains(%d:%d) = %v, want %v", tt.ch, tt.v, got, tt.want)
		}
	}
}

func TestPassageChapters(t *testing.T) {
	tests := []struct {
		in, want Passage
	}{
		{Passage{43, []VerseRange{{3, 16, 16}}}, Passage{43, []VerseRange{{3, 0, 0}}}},
		{Passage{43, []VerseRange{{3, 36, 0}, {4, 1, 2}}}, Passage{43, []VerseRange{{3, 0, 0}, {4, 0, 0}}}},
		{Passage{43, []VerseRange{{4, 1, 1}, {3, 16, 16}, {4, 3, 3}}}, Passage{43, []VerseRange{{3, 0, 0}, {4, 0, 0}}}},
		{Passage{65, []VerseRange{{1, 0, 0}}}, Passage{65, []VerseRange{{1, 0, 0}}}},
	}
	for _, tt := range tests {
		if got := tt.in.Chapters(); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%+v.Chapters() = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

// chapterVerses lists verses 1..n of each chapter (chapter, n pairs).
func chapterVerses(book int, chapters ...int) []Verse {
	var out []Verse
	for i := 0; i+1 < len(chapters); i += 2 {
		for v := 1; v <= chapters[i+1]; v++ {
			out = append(out, Verse{Book: book, Chapter: chapters[i], Verse: v})
		}
	}
	return out
}

func TestSelectionPassage(t *testing.T) {
	john3 := chapterVerses(43, 3, 36)
	john34 := chapterVerses(43, 3, 36, 4, 54)
	ps := chapterVerses(19, 1, 6, 2, 12)
	// Matthew 17 without verse 21 (as in translations that leave it out).
	mt17 := chapterVerses(40, 17, 27)
	mt17 = append(mt17[:20:20], mt17[21:]...)
	rows := func(lo, hi int) []int {
		var out []int
		for i := lo; i <= hi; i++ {
			out = append(out, i)
		}
		return out
	}
	tests := []struct {
		name   string
		listed []Verse
		sel    []int
		want   string
	}{
		{"one verse", john3, []int{15}, "John 3:16"},
		{"a range", john3, rows(15, 17), "John 3:16-18"},
		{"non-contiguous", john3, []int{15, 16, 17, 19}, "John 3:16-18, 20"},
		{"scattered", john3, []int{0, 15, 35}, "John 3:1, 16, 36"},
		{"to the last verse", john3, rows(15, 35), "John 3:16-36"},
		{"the whole chapter", john3, rows(0, 35), "John 3"},
		{"across chapters", john34, rows(35, 37), "John 3:36-4:2"},
		{"a chapter and the next one's start", john34, rows(0, 37), "John 3-4:2"},
		{"two whole chapters", ps, rows(0, 17), "Psalms 1-2"},
		{"the end of one chapter and all of the next", ps, rows(4, 17), "Psalms 1:5ff; 2"},
		{"both chapters, not adjacent", john34, []int{15, 51}, "John 3:16; 4:16"},
		{"a left-out verse does not split the range", mt17, rows(19, 20), "Matthew 17:20-22"},
		{"bad and unsorted rows are ignored", john3, []int{-1, 15, 15, 14, 99}, "John 3:16"},
		{"nothing selected", john3, nil, "John"},
		{"a single-chapter book", chapterVerses(65, 1, 25), rows(0, 24), "Jude"},
	}
	for _, tt := range tests {
		p := SelectionPassage(tt.listed[0].Book, tt.listed, tt.sel)
		if got := p.String(); got != tt.want {
			t.Errorf("%s: SelectionPassage = %q (%+v), want %q", tt.name, got, p.Ranges, tt.want)
		}
		if len(p.Ranges) == 0 {
			continue
		}
		// It parses back to a passage that names the same listed verses.
		back, err := ParseReference(p.String())
		if err != nil {
			t.Errorf("%s: ParseReference(%q): %v", tt.name, p.String(), err)
			continue
		}
		for i, v := range tt.listed {
			if p.Contains(v.Chapter, v.Verse) != back.Contains(v.Chapter, v.Verse) {
				t.Errorf("%s: row %d (%d:%d) differs after the round trip", tt.name, i, v.Chapter, v.Verse)
			}
		}
	}
}
