package core

import (
	"reflect"
	"testing"

	"freebeams/internal/models"
)

func TestListedVersesAndVersesIn(t *testing.T) {
	verses := []models.Verse{
		{Book: 43, Chapter: 3, Verse: 15, Text: "a"},
		{Book: 43, Chapter: 3, Verse: 16, Text: "For God so loved"},
		{Book: 43, Chapter: 3, Verse: 17, Text: "  "}, // no text: no row
		{Book: 43, Chapter: 3, Verse: 18, Text: "c"},
		{Book: 43, Chapter: 4, Verse: 1, Text: "d"},
	}
	list := ListedVerses(verses)
	if len(list) != 4 || list[2].Verse != 18 {
		t.Fatalf("ListedVerses = %+v", list)
	}
	if len(VerseRows(list)) != len(list) {
		t.Errorf("rows and listed verses differ")
	}
	tests := []struct {
		ref  string
		want []int
	}{
		{"John 3:16", []int{1}},
		{"John 3:16-18", []int{1, 2}},
		{"John 3", []int{0, 1, 2}},
		{"John 3:18-4:1", []int{2, 3}},
		{"John 3:99", nil},
	}
	for _, tt := range tests {
		p, err := models.ParseReference(tt.ref)
		if err != nil {
			t.Fatal(err)
		}
		if got := VersesIn(list, p); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("VersesIn(%s) = %v, want %v", tt.ref, got, tt.want)
		}
	}
}

func TestPassageStatusAndChooseBook(t *testing.T) {
	if got := PassageStatus("John 3:16-18", 3, "KJV"); got != "John 3:16-18 · 3 verses (KJV)" {
		t.Errorf("PassageStatus = %q", got)
	}
	if got := PassageStatus("John 3:16", 1, ""); got != "John 3:16 · 1 verse" {
		t.Errorf("PassageStatus = %q", got)
	}
	if got := ChooseBookText(models.BooksMatching("joh")); got != "Choose a book: John, 1 John, 2 John, 3 John" {
		t.Errorf("ChooseBookText = %q", got)
	}
}
