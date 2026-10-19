package appdirs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDir(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(base string)
		wantOld bool   // the former directory still exists afterwards
		marker  string // a file expected inside the new directory
	}{
		{"fresh", func(string) {}, false, ""},
		{"former moved", func(base string) {
			os.MkdirAll(filepath.Join(base, FormerName), 0o755)
			os.WriteFile(filepath.Join(base, FormerName, "library.db"), []byte("x"), 0o644)
		}, false, "library.db"},
		{"both present: the new one wins, the former stays", func(base string) {
			os.MkdirAll(filepath.Join(base, FormerName), 0o755)
			os.MkdirAll(filepath.Join(base, Name), 0o755)
			os.WriteFile(filepath.Join(base, Name, "new.db"), []byte("x"), 0o644)
		}, true, "new.db"},
		{"former is a file, not a dir", func(base string) {
			os.WriteFile(filepath.Join(base, FormerName), []byte("x"), 0o644)
		}, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			tt.setup(base)
			got, err := dir(base)
			if err != nil {
				t.Fatal(err)
			}
			if got != filepath.Join(base, Name) {
				t.Errorf("dir = %q", got)
			}
			if fi, err := os.Stat(got); err != nil || !fi.IsDir() {
				t.Errorf("new dir missing: %v", err)
			}
			if tt.marker != "" {
				if _, err := os.Stat(filepath.Join(got, tt.marker)); err != nil {
					t.Errorf("marker %s: %v", tt.marker, err)
				}
			}
			_, err = os.Stat(filepath.Join(base, FormerName))
			if (err == nil) != tt.wantOld {
				t.Errorf("former exists = %v, want %v", err == nil, tt.wantOld)
			}
		})
	}
}

func TestConfigAndCache(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir()) // macOS ignores XDG; keep the real dirs untouched either way
	for _, f := range []func() (string, error){Config, Cache} {
		d, err := f()
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(d) != Name {
			t.Errorf("dir = %q", d)
		}
	}
}
