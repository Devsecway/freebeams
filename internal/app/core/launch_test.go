package core

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestClassifyOpenFiles(t *testing.T) {
	cwd := filepath.FromSlash("/work")
	have := map[string]bool{}
	for _, p := range []string{"/work/a.fbs", "/work/b.FWS", "/x/KJV.fbbible", "/work/NIV.FBBIBLE", "/work/notes.txt"} {
		have[filepath.FromSlash(p)] = true
	}
	exists := func(p string) bool { return have[p] }
	abs := func(p string) string { return filepath.FromSlash(p) }
	tests := []struct {
		name     string
		args     []string
		schedule string
		bibles   []string
		ignored  []string
	}{
		{"nothing", nil, "", nil, nil},
		{"godot flags and the user-args separator", []string{"--path", "-v", "--", "++"}, "", nil, nil},
		{"a schedule, relative", []string{"a.fbs"}, abs("/work/a.fbs"), nil, nil},
		{"the legacy extension, any case", []string{abs("/work/b.FWS")}, abs("/work/b.FWS"), nil, nil},
		{"packages after a schedule", []string{abs("/x/KJV.fbbible"), "a.fbs", "NIV.FBBIBLE"},
			abs("/work/a.fbs"), []string{abs("/x/KJV.fbbible"), abs("/work/NIV.FBBIBLE")}, nil},
		{"one schedule only", []string{"a.fbs", "b.FWS"}, abs("/work/a.fbs"), nil, []string{"b.FWS"}},
		{"other files and missing ones", []string{"notes.txt", "gone.fbs", "a.fbs", "./a.fbs"}, abs("/work/a.fbs"), nil, []string{"notes.txt", "gone.fbs"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := ClassifyOpenFiles(tt.args, cwd, exists)
			if o.Schedule != tt.schedule || !slices.Equal(o.Bibles, tt.bibles) || !slices.Equal(o.Ignored, tt.ignored) {
				t.Fatalf("got %+v, want schedule %q bibles %q ignored %q", o, tt.schedule, tt.bibles, tt.ignored)
			}
			if o.Empty() != (tt.schedule == "" && len(tt.bibles) == 0) {
				t.Fatalf("Empty() = %v", o.Empty())
			}
			if got := o.Paths(); len(got) != len(tt.bibles)+map[bool]int{true: 1}[tt.schedule != ""] {
				t.Fatalf("Paths() = %q", got)
			}
		})
	}
}
