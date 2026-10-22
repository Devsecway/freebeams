package sysproc

import (
	"context"
	"os/exec"
	"testing"
)

func TestHideWindow(t *testing.T) {
	cmd := exec.Command("x")
	HideWindow(cmd)
	HideWindow(cmd) // idempotent
	if !hidden(cmd) {
		t.Fatalf("not hidden: %+v", cmd.SysProcAttr)
	}
	c := CommandContext(context.Background(), "x", "a")
	if !hidden(c) || len(c.Args) != 2 || c.Args[1] != "a" {
		t.Fatalf("CommandContext: hidden %v, args %q", hidden(c), c.Args)
	}
}
