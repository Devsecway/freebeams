package models

import "testing"

func TestIsPresentationFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"deck.pdf", true}, {"/x/Deck.PDF", true}, {"a.pptx", true}, {"a.PPT", true}, {"talk.key", true}, {"a.odp", true},
		{"a.png", false}, {"a.mp4", false}, {"a.docx", false}, {"noext", false}, {"", false},
	}
	for _, tc := range tests {
		if got := IsPresentationFile(tc.path); got != tc.want {
			t.Errorf("IsPresentationFile(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestPresentationRenderedAndTheme(t *testing.T) {
	if (Presentation{}).Rendered() || (Presentation{Pages: 3}).Rendered() || (Presentation{CacheKey: "k"}).Rendered() {
		t.Error("Rendered needs pages and a cache key")
	}
	if !(Presentation{Pages: 1, CacheKey: "k"}).Rendered() {
		t.Error("Rendered with pages and a key")
	}
	th := DefaultPresentationTheme()
	if th.Name != "Presentation" || th.Background.Kind != BackgroundColor || th.Background.Color != (RGBA{A: 1}) {
		t.Errorf("DefaultPresentationTheme = %+v", th)
	}
}
