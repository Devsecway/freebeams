package models

import "testing"

func TestStageFrameHeaders(t *testing.T) {
	tests := []struct {
		f          StageFrame
		next, head string
	}{
		{StageFrame{Title: "Amazing Grace", Label: "Verse 2", Next: "x", NextLabel: "Chorus"}, "NEXT · Chorus", "Amazing Grace · Verse 2"},
		{StageFrame{Title: "Amazing Grace", Label: "Verse 3", Next: "x", NextLabel: "Verse 1", NextTitle: "How Great"}, "NEXT ITEM · How Great · Verse 1", "Amazing Grace · Verse 3"},
		{StageFrame{Title: "Notices", NextTitle: "How Great"}, "NEXT ITEM · How Great", "Notices"},
		{StageFrame{Title: "A", NextTitle: "John 3:16", NextLabel: "John 3:16", Next: "x"}, "NEXT ITEM · John 3:16", "A"},
		{StageFrame{Title: "John 3:16", Label: "John 3:16"}, "LAST SLIDE", "John 3:16"},
		{StageFrame{Title: "Media", Next: "text only"}, "NEXT", "Media"},
		{StageFrame{Label: "Only a label"}, "LAST SLIDE", "Only a label"},
		{StageFrame{}, "", ""},
	}
	for _, tt := range tests {
		if got := tt.f.NextHeader(); got != tt.next {
			t.Errorf("NextHeader(%+v) = %q, want %q", tt.f, got, tt.next)
		}
		if got := tt.f.HeaderTitle(); got != tt.head {
			t.Errorf("HeaderTitle(%+v) = %q, want %q", tt.f, got, tt.head)
		}
	}
}
