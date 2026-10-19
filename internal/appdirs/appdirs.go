// Package appdirs locates FreeBeams's per-user directories: the config dir
// that holds the library and the cache dir for derived files. The app was
// called FreeWorship before 2026-10-20; a directory left under that name is
// moved to the new one the first time the new one is asked for, so a
// library and its caches survive the rename. It is pure Go.
package appdirs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Name is the directory name under the OS's config and cache roots.
const Name = "FreeBeams"

// FormerName is the directory name the app used before it was renamed.
const FormerName = "FreeWorship"

// Config returns <os.UserConfigDir()>/FreeBeams (created; a FreeWorship
// directory is moved to it first).
func Config() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config dir: %w", err)
	}
	return dir(base)
}

// Cache returns <os.UserCacheDir()>/FreeBeams (created; a FreeWorship
// directory is moved to it first).
func Cache() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("cache dir: %w", err)
	}
	return dir(base)
}

// dir returns base/Name, moving base/FormerName there when Name does not
// exist yet, and creates it. A failed move is not an error: the new
// directory is simply created empty.
func dir(base string) (string, error) {
	d := filepath.Join(base, Name)
	if _, err := os.Stat(d); errors.Is(err, fs.ErrNotExist) {
		old := filepath.Join(base, FormerName)
		if fi, err := os.Stat(old); err == nil && fi.IsDir() {
			_ = os.Rename(old, d)
		}
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", fmt.Errorf("app dir %s: %w", d, err)
	}
	return d, nil
}
