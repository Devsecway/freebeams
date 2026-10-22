//go:build windows

package sysproc

import (
	"os/exec"
	"syscall"
)

// createNoWindow is CREATE_NO_WINDOW: a console program started from a GUI
// app gets no console window of its own.
const createNoWindow = 0x08000000

// HideWindow keeps a console tool (ffmpeg, ffprobe, …) from flashing a
// console window when the GUI app starts it.
func HideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}

// hidden reports whether HideWindow was applied (tests).
func hidden(cmd *exec.Cmd) bool {
	return cmd.SysProcAttr != nil && cmd.SysProcAttr.HideWindow && cmd.SysProcAttr.CreationFlags&createNoWindow != 0
}
