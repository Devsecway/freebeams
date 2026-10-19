// Package media manages the files of the media library: scanning folders for
// images and videos, deriving thumbnails and metadata, and transcoding videos
// into the one format Godot plays (Ogg Theora). It must not import graphics.gd.
//
// The external tools (ffprobe, ffmpeg) sit behind small interfaces (Prober,
// Thumbnailer, Transcoder), so everything above them is testable without
// ffmpeg, and the app degrades gracefully without it: stills are fully
// handled in Go, videos are listed but cannot get a poster frame or a
// transcode until ffmpeg (with the libtheora encoder) is installed.
package media

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"freebeams/internal/appdirs"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"freebeams/internal/models"
)

// ErrUnavailable is returned when a step needs ffmpeg/ffprobe and none is available.
var ErrUnavailable = errors.New("ffmpeg is not available")

// ErrNoTheora is returned by a transcode when ffmpeg lacks the libtheora encoder.
var ErrNoTheora = errors.New("ffmpeg has no Theora (libtheora) encoder")

// Scan lists the media files in dirs (recursively, hidden entries skipped),
// as unsaved library items with Path (absolute), Kind and Title filled in,
// ordered by path. A dir may also be a single file. Problems with a folder
// (missing, unreadable) become warnings, not errors.
func Scan(dirs ...string) (items []models.Media, warnings []string) {
	seen := map[string]bool{}
	for _, dir := range dirs {
		abs, err := filepath.Abs(dir)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", dir, err))
			continue
		}
		fi, err := os.Stat(abs)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", dir, err))
			continue
		}
		if !fi.IsDir() {
			if m, ok := item(abs); ok && !seen[abs] {
				seen[abs] = true
				items = append(items, m)
			} else if !ok {
				warnings = append(warnings, fmt.Sprintf("%s: not an image, video or audio file", dir))
			}
			continue
		}
		err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s: %v", path, err))
				return nil
			}
			if path != abs && strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() || !d.Type().IsRegular() {
				return nil
			}
			if m, ok := item(path); ok && !seen[path] {
				seen[path] = true
				items = append(items, m)
			}
			return nil
		})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", dir, err))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	return items, warnings
}

// item builds the unsaved library item for a file, or false if it is not a
// media file.
func item(path string) (models.Media, bool) {
	kind, ok := models.MediaKindOf(path)
	if !ok {
		return models.Media{}, false
	}
	return models.Media{Path: path, Kind: kind, Title: models.MediaTitle(path)}, true
}

// Info is what a probe learns about a file.
type Info struct {
	Width, Height int
	Duration      time.Duration
}

// Prober reads a media file's dimensions and duration (ffprobe).
type Prober interface {
	Probe(ctx context.Context, path string) (Info, error)
}

// Thumbnailer writes a small still of a media file (a video's poster frame
// or a scaled image) to dst, width pixels wide, as JPEG (ffmpeg).
type Thumbnailer interface {
	Thumbnail(ctx context.Context, src, dst string, width int) error
}

// Transcoder converts a video to Ogg Theora at dst. progress (may be nil) is
// called with the fraction done in [0, 1] as the work proceeds.
type Transcoder interface {
	Transcode(ctx context.Context, src, dst string, progress func(float64)) error
}

// DefaultCacheDir is where derived files go: <os.UserCacheDir()>/FreeBeams/media
// (directory created). override, when not empty, is used instead.
func DefaultCacheDir(override string) (string, error) {
	dir := override
	if dir == "" {
		base, err := appdirs.Cache()
		if err != nil {
			return "", fmt.Errorf("media cache dir: %w", err)
		}
		dir = filepath.Join(base, "media")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("media cache dir: %w", err)
	}
	return dir, nil
}

// cacheName is a stable file name for a derived file of src: a hash of the
// source path (so one source always maps to the same cache file) and its
// base name (so the cache is readable), with the given extension.
func cacheName(src, ext string) string {
	sum := sha1.Sum([]byte(filepath.Clean(src)))
	base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	base = sanitize(base)
	if len(base) > 48 {
		base = base[:48]
	}
	return hex.EncodeToString(sum[:8]) + "-" + base + ext
}

// sanitize keeps a file name portable: letters, digits, '-', '_' and '.'.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('_')
		}
	}
	return b.String()
}

// ThumbPath is the cache file for src's thumbnail (JPEG).
func ThumbPath(cacheDir, src string) string {
	return filepath.Join(cacheDir, cacheName(src, ".jpg"))
}

// TranscodePath is the cache file for src's playable copy (Ogg Theora).
func TranscodePath(cacheDir, src string) string {
	return filepath.Join(cacheDir, cacheName(src, models.PlayableVideoExt))
}

// exists reports whether path is an existing regular file with content.
func exists(path string) bool {
	if path == "" {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular() && fi.Size() > 0
}
