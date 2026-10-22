// Package sysproc holds the platform details of the helper processes the app
// spawns (ffmpeg, ffprobe, pdftoppm, LibreOffice). It is pure Go.
package sysproc

import (
	"context"
	"os/exec"
)

// CommandContext is exec.CommandContext with HideWindow applied.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	HideWindow(cmd)
	return cmd
}
