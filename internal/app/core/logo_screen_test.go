package core

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"freebeams/internal/models"
)

func TestLogoScreenPNG(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(LogoScreenPNG()))
	if err != nil {
		t.Fatalf("embedded logo screen: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 1920 || b.Dy() != 1080 {
		t.Errorf("logo screen is %dx%d, want 1920x1080", b.Dx(), b.Dy())
	}
}

func TestWriteDefaultLogo(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "logo")
	got, err := WriteDefaultLogo(dir)
	if err != nil || got != dir {
		t.Fatalf("WriteDefaultLogo = %q, %v", got, err)
	}
	path := filepath.Join(dir, DefaultLogoFile)
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, LogoScreenPNG()) {
		t.Fatalf("written file: %d bytes, %v", len(data), err)
	}
	before, _ := os.Stat(path)

	// A second call leaves an up-to-date file alone.
	if _, err := WriteDefaultLogo(dir); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("an up-to-date file was rewritten")
	}

	// A stale file is replaced.
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteDefaultLogo(dir); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); !bytes.Equal(data, LogoScreenPNG()) {
		t.Errorf("a stale file was not replaced")
	}

	// The directory is what the logo defaults read.
	bg := LogoBackground(dir)
	if bg.Kind != models.BackgroundImage || bg.Path != path || bg.Color != LogoGradient().Color {
		t.Errorf("LogoBackground(dir) = %+v", bg)
	}
	if !IsDefaultLogo(bg) || IsDefaultLogo(LogoGradient()) || IsDefaultLogo(models.Background{Kind: models.BackgroundImage, Path: "/x/logo.png"}) {
		t.Errorf("IsDefaultLogo misreports")
	}
	if got := LogoDescription(bg); got != "Default (FreeBeams logo)" {
		t.Errorf("LogoDescription(default) = %q", got)
	}
	if def := DefaultSettings(dir); def.Logo != bg {
		t.Errorf("DefaultSettings(dir).Logo = %+v", def.Logo)
	}

	// Failure keeps the gradient: callers pass "" on error.
	if _, err := WriteDefaultLogo(""); err == nil {
		t.Errorf("WriteDefaultLogo(\"\") succeeded")
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteDefaultLogo(filepath.Join(file, "logo")); err == nil {
		t.Errorf("WriteDefaultLogo under a file succeeded")
	}
	if bg := LogoBackground(""); bg != LogoGradient() {
		t.Errorf("fallback = %+v", bg)
	}
}
