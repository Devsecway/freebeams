package media

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLocate(t *testing.T) {
	write := func(p string) {
		t.Helper()
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	onPath := func(name string) (string, error) { return "/usr/bin/" + name, nil }
	noPath := func(string) (string, error) { return "", errors.New("not found") }
	exeDir, envDir := t.TempDir(), t.TempDir()

	// Nothing bundled: PATH.
	if got := locate([]string{exeDir}, onPath); got.FFmpeg != "/usr/bin/ffmpeg" || got.FFprobe != "/usr/bin/ffprobe" {
		t.Fatalf("PATH: %+v", got)
	}
	if got := locate(nil, noPath); got.FFmpeg != "" || got.FFprobe != "" || got.Available() {
		t.Fatalf("nothing: %+v", got)
	}
	// Bundled beside the executable wins over PATH, per tool.
	write(filepath.Join(exeDir, exeName("ffmpeg")))
	got := locate([]string{exeDir}, onPath)
	if got.FFmpeg != filepath.Join(exeDir, exeName("ffmpeg")) || got.FFprobe != "/usr/bin/ffprobe" {
		t.Fatalf("bundled: %+v", got)
	}
	// The first folder wins (FREEBEAMS_FFMPEG_DIR comes before the bundle).
	write(filepath.Join(envDir, exeName("ffmpeg")))
	write(filepath.Join(envDir, exeName("ffprobe")))
	got = locate([]string{envDir, exeDir}, noPath)
	if got.FFmpeg != filepath.Join(envDir, exeName("ffmpeg")) || got.FFprobe != filepath.Join(envDir, exeName("ffprobe")) {
		t.Fatalf("env folder: %+v", got)
	}
	// A directory named like the tool is not the tool.
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, exeName("ffprobe")), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := locate([]string{dir}, noPath); got.FFprobe != "" {
		t.Fatalf("directory taken for ffprobe: %+v", got)
	}
}
