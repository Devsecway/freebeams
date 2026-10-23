package media

import (
	"context"
	"errors"
)

// Chain is a Prober and Thumbnailer that asks First and, when First cannot
// (any error), Then: on macOS AVFoundation first, ffmpeg for what it cannot
// read (WebM, WebP / BMP stills). A nil member is skipped.
type Chain struct {
	First interface {
		Prober
		Thumbnailer
	}
	Then interface {
		Prober
		Thumbnailer
	}
}

// Probe implements Prober.
func (c Chain) Probe(ctx context.Context, path string) (Info, error) {
	var first error = ErrUnavailable
	if c.First != nil {
		info, err := c.First.Probe(ctx, path)
		if err == nil || ctx.Err() != nil {
			return info, err
		}
		first = err
	}
	if c.Then == nil {
		return Info{}, first
	}
	info, err := c.Then.Probe(ctx, path)
	return info, pick(first, err)
}

// Thumbnail implements Thumbnailer.
func (c Chain) Thumbnail(ctx context.Context, src, dst string, width int) error {
	var first error = ErrUnavailable
	if c.First != nil {
		err := c.First.Thumbnail(ctx, src, dst, width)
		if err == nil || ctx.Err() != nil {
			return err
		}
		first = err
	}
	if c.Then == nil {
		return first
	}
	return pick(first, c.Then.Thumbnail(ctx, src, dst, width))
}

// pick keeps the more telling error: the second one unless it only says
// that the tool is missing while the first one says what went wrong.
func pick(first, then error) error {
	if then == nil {
		return nil
	}
	if errors.Is(then, ErrUnavailable) && !errors.Is(first, ErrUnavailable) {
		return first
	}
	return then
}
