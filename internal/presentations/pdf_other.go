//go:build !darwin

package presentations

import (
	"context"
	"fmt"
	"path/filepath"
)

// coreGraphicsSupported: Core Graphics exists on macOS only.
const coreGraphicsSupported = false

// coreGraphicsRenderer is never selected off macOS; it only satisfies the build.
type coreGraphicsRenderer struct{}

func (coreGraphicsRenderer) Render(ctx context.Context, src, dir string, progress Progress) (Result, error) {
	return Result{}, fmt.Errorf("render %s: %w", filepath.Base(src), ErrUnavailable)
}
