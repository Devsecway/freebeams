package core

import (
	"testing"

	"freebeams/internal/bibles/bundled"
)

func TestBundledSummary(t *testing.T) {
	tests := []struct {
		name string
		rep  bundled.Report
		want string
	}{
		{"nothing", bundled.Report{Skipped: []string{"KJV (already installed)", "ASV (already installed)"}}, ""},
		{"both", bundled.Report{Installed: []string{"KJV", "ASV"}}, "Installed KJV, ASV"},
		{"one", bundled.Report{Installed: []string{"ASV"}, Skipped: []string{"KJV (already installed)"}}, "Installed ASV"},
	}
	for _, tt := range tests {
		if got := BundledSummary(tt.rep); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestBuildStateBundled(t *testing.T) {
	st := BuildState(StateInput{Bundled: BundledState{Running: true}})
	if !st.Bundled.Running || st.Bundled.Installed == nil || st.Bundled.Skipped == nil {
		t.Errorf("bundled state = %+v, want running with empty (non-nil) lists", st.Bundled)
	}
	st = BuildState(StateInput{Bundled: BundledState{Done: true, Installed: []string{"KJV", "ASV"}, Version: 1}})
	if !st.Bundled.Done || len(st.Bundled.Installed) != 2 || st.Bundled.Version != 1 {
		t.Errorf("bundled state = %+v", st.Bundled)
	}
}
