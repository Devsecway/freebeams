package core

import (
	"strings"
	"testing"
)

func TestAboutText(t *testing.T) {
	tests := []struct{ version, build, head string }{
		{"0.3.0", "", "FreeBeams 0.3.0\n"},
		{"0.3.0", "v0.3.0", "FreeBeams 0.3.0\n"},
		{"0.3.0", "v0.0.0-20261023-abcdef", "FreeBeams 0.3.0 (v0.0.0-20261023-abcdef)\n"},
	}
	for _, tt := range tests {
		got := AboutText(tt.version, tt.build)
		if !strings.HasPrefix(got, tt.head) {
			t.Errorf("AboutText(%q, %q) = %q, want it to start with %q", tt.version, tt.build, got, tt.head)
		}
		for _, want := range []string{"MIT License", "Devsecway", "THIRD-PARTY-NOTICES.txt"} {
			if !strings.Contains(got, want) {
				t.Errorf("AboutText misses %q: %q", want, got)
			}
		}
	}
}
