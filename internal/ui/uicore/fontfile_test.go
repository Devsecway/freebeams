package uicore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadableFontFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, body []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, body, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	tests := []struct {
		path string
		want bool
	}{
		{write("a.ttf", []byte{0, 1, 0, 0, 9}), true},
		{write("b.otf", []byte("OTTO....")), true},
		{write("c.ttc", []byte("ttcf....")), true},
		{write("d.dfont", []byte{0, 0, 1, 0, 0}), false},
		{write("e.ttf", nil), false},
		{filepath.Join(dir, "missing.ttf"), false},
		{"", false},
		{"/System/Library/PrivateFrameworks/FontServices.framework/Versions/A/Resources/Reserved/PingFangUI.ttc", false},
	}
	for _, tt := range tests {
		if got := LoadableFontFile(tt.path); got != tt.want {
			t.Errorf("LoadableFontFile(%s) = %v, want %v", filepath.Base(tt.path), got, tt.want)
		}
	}
}
