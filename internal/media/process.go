package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"freeworship/internal/models"
)

// DefaultThumbWidth is the width of library thumbnails (pixels). A video's
// thumbnail doubles as its poster on the output when it cannot play, so it
// is wider than a grid tile needs.
const DefaultThumbWidth = 640

// Processor derives what a library item lacks: dimensions and duration
// (Describe), a thumbnail (Thumbnail) and, for videos, a playable transcode
// (Transcode). Stills are handled in Go where possible; the tools (which may
// be a zero Tools, or nil) cover the rest. Each step reports whether it
// changed the item, so the caller knows what to save.
type Processor struct {
	Prober      Prober      // nil: no probing beyond Go's image decoders
	Thumbnailer Thumbnailer // nil: no video posters, no WebP/BMP thumbnails
	Transcoder  Transcoder  // nil: no transcodes
	CacheDir    string      // where derived files go
	ThumbWidth  int         // 0 = DefaultThumbWidth
}

// NewProcessor returns a processor using t for every tool.
func NewProcessor(t Tools, cacheDir string) *Processor {
	return &Processor{Prober: t, Thumbnailer: t, Transcoder: t, CacheDir: cacheDir}
}

func (p *Processor) thumbWidth() int {
	if p.ThumbWidth <= 0 {
		return DefaultThumbWidth
	}
	return p.ThumbWidth
}

// NeedsThumbnail reports whether m lacks a thumbnail file.
func NeedsThumbnail(m models.Media) bool { return !exists(m.ThumbPath) }

// NeedsTranscode reports whether m is a video without a playable file.
func NeedsTranscode(m models.Media) bool {
	return m.Kind == models.MediaVideo && !exists(m.PlayablePath())
}

// NeedsDescribe reports whether m lacks its dimensions (or, for a video, its duration).
func NeedsDescribe(m models.Media) bool {
	return m.Width <= 0 || m.Height <= 0 || (m.Kind == models.MediaVideo && m.Duration <= 0)
}

// Describe fills in the missing dimensions and duration. A missing source
// file is an error.
func (p *Processor) Describe(ctx context.Context, m *models.Media) (changed bool, err error) {
	if m == nil {
		return false, errors.New("describe media: nil item")
	}
	if !NeedsDescribe(*m) {
		return false, nil
	}
	if _, err := os.Stat(m.Path); err != nil {
		return false, fmt.Errorf("describe %s: %w", filepath.Base(m.Path), err)
	}
	if m.Kind == models.MediaImage && goDecodes(m.Path) {
		w, h, err := imageInfo(m.Path)
		if err != nil {
			return false, fmt.Errorf("describe %s: %w", filepath.Base(m.Path), err)
		}
		m.Width, m.Height = w, h
		return true, nil
	}
	if p.Prober == nil {
		return false, ErrUnavailable
	}
	info, err := p.Prober.Probe(ctx, m.Path)
	if err != nil {
		return false, err
	}
	m.Width, m.Height = info.Width, info.Height
	if m.Kind == models.MediaVideo {
		m.Duration = info.Duration
	}
	return true, nil
}

// Thumbnail makes m's thumbnail in the cache dir unless it exists. Stills
// that Go decodes need no tools; other stills and videos need the
// Thumbnailer (ErrUnavailable without one).
func (p *Processor) Thumbnail(ctx context.Context, m *models.Media) (changed bool, err error) {
	if m == nil {
		return false, errors.New("thumbnail media: nil item")
	}
	if !NeedsThumbnail(*m) {
		return false, nil
	}
	if p.CacheDir == "" {
		return false, errors.New("thumbnail media: no cache dir")
	}
	if _, err := os.Stat(m.Path); err != nil {
		return false, fmt.Errorf("thumbnail %s: %w", filepath.Base(m.Path), err)
	}
	dst := ThumbPath(p.CacheDir, m.Path)
	if !exists(dst) {
		if err := os.MkdirAll(p.CacheDir, 0o755); err != nil {
			return false, fmt.Errorf("thumbnail %s: %w", filepath.Base(m.Path), err)
		}
		switch {
		case m.Kind == models.MediaImage && goDecodes(m.Path):
			if err := imageThumbnail(m.Path, dst, p.thumbWidth()); err != nil {
				return false, fmt.Errorf("thumbnail %s: %w", filepath.Base(m.Path), err)
			}
		case p.Thumbnailer == nil:
			return false, ErrUnavailable
		default:
			if err := p.Thumbnailer.Thumbnail(ctx, m.Path, dst, p.thumbWidth()); err != nil {
				return false, err
			}
		}
	}
	m.ThumbPath = dst
	return true, nil
}

// Transcode makes a video's playable copy in the cache dir unless one
// exists (ErrUnavailable / ErrNoTheora without a capable Transcoder). Stills
// are left alone.
func (p *Processor) Transcode(ctx context.Context, m *models.Media, progress func(float64)) (changed bool, err error) {
	if m == nil {
		return false, errors.New("transcode media: nil item")
	}
	if !NeedsTranscode(*m) {
		return false, nil
	}
	if p.CacheDir == "" {
		return false, errors.New("transcode media: no cache dir")
	}
	if p.Transcoder == nil {
		return false, ErrUnavailable
	}
	if _, err := os.Stat(m.Path); err != nil {
		return false, fmt.Errorf("transcode %s: %w", filepath.Base(m.Path), err)
	}
	dst := TranscodePath(p.CacheDir, m.Path)
	if !exists(dst) {
		if err := os.MkdirAll(p.CacheDir, 0o755); err != nil {
			return false, fmt.Errorf("transcode %s: %w", filepath.Base(m.Path), err)
		}
		if err := p.Transcoder.Transcode(ctx, m.Path, dst, progress); err != nil {
			return false, err
		}
	}
	m.TranscodedPath = dst
	return true, nil
}
