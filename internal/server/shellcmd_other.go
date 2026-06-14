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

// fileShellCommand runs a script file through /bin/sh.
func fileShellCommand(ctx context.Context, path string) *exec.Cmd {
	return exec.CommandContext(ctx, "/bin/sh", path)
}
