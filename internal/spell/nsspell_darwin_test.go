//go:build darwin

package spell

import (
	"reflect"
	"slices"
	"testing"
)

// The macOS system checker (NSSpellChecker), from a test goroutine: the app
// calls Correct from a goroutine too.
func TestSystemChecker(t *testing.T) {
	b := System()
	if got := b.Suggest("recieve", "en"); !slices.Contains(got, "receive") {
		t.Fatalf("Suggest(recieve) = %q, want receive among them", got)
	}
	if got, want := b.Correct([]string{"receive", "recieve", "grace"}, "en"), []bool{true, false, true}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Correct = %v, want %v", got, want)
	}
	if langs := b.Languages(); len(langs) == 0 {
		t.Fatal("no languages")
	}
	e := New(b, nil)
	done := make(chan []Misspelling)
	go func() { done <- e.Check("[Verse 1]\nI'll recieve Thy grace, Hallelujah\nV2", "") }()
	if got := flagged(<-done); !reflect.DeepEqual(got, []string{"recieve"}) {
		t.Fatalf("Check = %q, want [recieve]", got)
	}
}
