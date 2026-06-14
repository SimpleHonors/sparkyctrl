//go:build !windows

package server

import (
	"context"
	"os/exec"
)

// defaultShellCommand runs script through /bin/sh on non-Windows platforms.
func defaultShellCommand(ctx context.Context, script string) *exec.Cmd {
	return exec.CommandContext(ctx, "/bin/sh", "-c", script)
}
