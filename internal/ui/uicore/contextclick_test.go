package uicore

import "testing"

func TestIsContextClick(t *testing.T) {
	cases := []struct {
		button      int
		ctrl, macOS bool
		want        bool
	}{
		{MouseRight, false, false, true},
		{MouseRight, true, true, true},
		{MouseLeft, false, true, false},
		{MouseLeft, true, true, true},   // Control-click on a Mac
		{MouseLeft, true, false, false}, // Ctrl-click elsewhere is a plain click
		{3, false, true, false},         // middle
	}
	for _, tc := range cases {
		if got := IsContextClick(tc.button, tc.ctrl, tc.macOS); got != tc.want {
			t.Errorf("IsContextClick(%d, ctrl %v, mac %v) = %v", tc.button, tc.ctrl, tc.macOS, got)
		}
	}
}
