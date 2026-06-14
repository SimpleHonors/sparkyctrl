//go:build windows

package server

import (
	"context"
	"os"
	"os/exec"
	"syscall"
)

// defaultShellCommand runs script through cmd.exe on Windows. The command line is
// built by hand (see buildWindowsShellLine) so trailing backslashes and
// metacharacters survive cmd's parser instead of being mangled by Go's default
// argument escaping.
func defaultShellCommand(ctx context.Context, script string) *exec.Cmd {
	comspec := os.Getenv("ComSpec")
	if comspec == "" {
		comspec = "cmd.exe"
	}
	cmd := exec.CommandContext(ctx, comspec)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: buildWindowsShellLine(comspec, script)}
	return cmd
}
