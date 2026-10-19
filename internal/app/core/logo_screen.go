package core

import (
	"bytes"
	_ "embed" // the built-in Logo screen
	"fmt"
	"os"
	"path/filepath"

	"freebeams/internal/models"
)

// The built-in Logo screen: the FreeBeams screen gradient with the wordmark
// centred, 1920x1080. docs/branding/tools/build.py --logo-screen writes it;
// docs/branding/README.md describes it. Godot exports do not ship loose
// resources, so it is embedded here and WriteDefaultLogo materialises it as a
// file that the output shows like any other image background.
//
//go:embed logo-screen.png
var logoScreenPNG []byte

// DefaultLogoFile is the name of the file WriteDefaultLogo writes.
const DefaultLogoFile = "FreeBeams.png"

// LogoScreenPNG returns the embedded Logo screen image.
func LogoScreenPNG() []byte {
	return logoScreenPNG
}

// WriteDefaultLogo writes the built-in Logo screen into dir (created) as
// DefaultLogoFile and returns dir: the directory to hand to LogoBackground,
// DefaultSettings and Normalized as the logo directory, which then pick the
// file up as the default logo. A file that already holds the current image
// is left alone, so the operator's own files are never touched and a second
// start is a read, not a write. Callers keep the gradient (an empty logo
// directory) when it returns an error.
func WriteDefaultLogo(dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("write default logo: no directory")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("write default logo: %w", err)
	}
	path := filepath.Join(dir, DefaultLogoFile)
	if cur, err := os.ReadFile(path); err == nil && bytes.Equal(cur, logoScreenPNG) {
		return dir, nil
	}
	if err := os.WriteFile(path, logoScreenPNG, 0o644); err != nil {
		return "", fmt.Errorf("write default logo: %w", err)
	}
	return dir, nil
}

// IsDefaultLogo reports whether bg is the built-in Logo screen (the image
// WriteDefaultLogo writes).
func IsDefaultLogo(bg models.Background) bool {
	return bg.Kind == models.BackgroundImage && bg.Path != "" && filepath.Base(bg.Path) == DefaultLogoFile
}
