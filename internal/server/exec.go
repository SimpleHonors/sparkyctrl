package server

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
)

func timeout(sec int) time.Duration {
	if sec <= 0 {
		sec = protocol.DefaultTimeoutSec
	}
	return time.Duration(sec) * time.Second
}

// captureRun runs cmd, capturing stdout/stderr, and fills an ExecResponse.
func captureRun(cmd *exec.Cmd, stdin string) protocol.ExecResponse {
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	start := time.Now()
	err := cmd.Run()
	dur := time.Since(start).Milliseconds()

	resp := protocol.ExecResponse{
		Stdout:     out.String(),
		Stderr:     errb.String(),
		DurationMs: dur,
		ExitCode:   0,
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			resp.ExitCode = exitErr.ExitCode()
		} else {
			// Setup failure: binary not found, ctx deadline, etc.
			resp.ExitCode = -1
			resp.Error = err.Error()
		}
	}
	return resp
}

// RunExec executes argv directly with NO shell. This is the mangle-proof path.
func RunExec(req protocol.ExecRequest) protocol.ExecResponse {
	if len(req.Argv) == 0 {
		return protocol.ExecResponse{ExitCode: -1, Error: "argv is required and must be non-empty"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout(req.TimeoutSec))
	defer cancel()

	cmd := exec.CommandContext(ctx, req.Argv[0], req.Argv[1:]...)
	// cwd is intentionally NOT fence-checked. exec/shell are unfenced by design
	// ("sharp tool, not a nanny") — an unrestricted exec can `cd` anywhere
	// anyway, so fencing the working dir would be security theatre, not a
	// boundary. Real containment for exec/shell is OS-level (run the worker in a
	// container with only the intended shares bind-mounted). See the design spec
	// §6 "exec/shell cwd is unfenced by design".
	cmd.Dir = req.Cwd
	if len(req.Env) > 0 {
		env := os.Environ()
		for k, v := range req.Env {
			env = append(env, k+"="+v)
		}
		cmd.Env = env
	}
	return captureRun(cmd, req.Stdin)
}

// RunShell runs a script through a real shell. Explicit and separate from
// RunExec so its use is always intentional and visible in the audit log.
func RunShell(req protocol.ShellRequest) protocol.ExecResponse {
	if strings.TrimSpace(req.Script) == "" {
		return protocol.ExecResponse{ExitCode: -1, Error: "script is required and must be non-empty"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout(req.TimeoutSec))
	defer cancel()

	var cmd *exec.Cmd
	switch req.Shell {
	case "powershell":
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", req.Script)
	case "", "sh":
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", req.Script)
	default:
		return protocol.ExecResponse{ExitCode: -1, Error: "unsupported shell: " + req.Shell}
	}
	// cwd is intentionally unfenced — see the note in RunExec above.
	cmd.Dir = req.Cwd
	return captureRun(cmd, "")
}
