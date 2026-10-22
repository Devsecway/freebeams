//go:build !windows

package sysproc

import "os/exec"

// HideWindow does nothing outside Windows (there are no console windows).
func HideWindow(cmd *exec.Cmd) {}

// hidden reports whether HideWindow was applied (tests): always true here.
func hidden(cmd *exec.Cmd) bool { return cmd.SysProcAttr == nil }
