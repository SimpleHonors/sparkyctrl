package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
)

// shellScriptArgLimit is the max script size (in bytes) that we'll pass as a
// shell -c argument. Beyond this the script is written to a temp file and the
// shell runs that file instead, avoiding OS argument-length limits.
const shellScriptArgLimit = 4096

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
//
// Large scripts are written to a temp file and executed from there instead of
// being passed as a -c argument, avoiding OS command-line length limits.
func RunShell(req protocol.ShellRequest) protocol.ExecResponse {
	if strings.TrimSpace(req.Script) == "" {
		return protocol.ExecResponse{ExitCode: -1, Error: "script is required and must be non-empty"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout(req.TimeoutSec))
	defer cancel()

	useFile := len(req.Script) > shellScriptArgLimit
	var cmd *exec.Cmd
	var cleanup func()

	switch req.Shell {
	case "powershell":
		if useFile {
			path, c, err := writeTempScript(req.Script, ".ps1")
			if err != nil {
				return protocol.ExecResponse{ExitCode: -1, Error: err.Error()}
			}
			cleanup = c
			cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-File", path)
		} else {
			cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", req.Script)
		}
	case "", "sh":
		if useFile {
			path, c, err := writeTempScript(req.Script, ".sh")
			if err != nil {
				return protocol.ExecResponse{ExitCode: -1, Error: err.Error()}
			}
			cleanup = c
			cmd = fileShellCommand(ctx, path)
		} else {
			cmd = defaultShellCommand(ctx, req.Script)
		}
	default:
		return protocol.ExecResponse{ExitCode: -1, Error: "unsupported shell: " + req.Shell}
	}
	// cwd is intentionally unfenced — see the note in RunExec above.
	cmd.Dir = req.Cwd
	resp := captureRun(cmd, "")
	if cleanup != nil {
		cleanup()
	}
	return resp
}

// writeTempScript writes script to a temp file with the given extension,
// makes it executable, and returns the path plus a cleanup function.
func writeTempScript(script, ext string) (string, func(), error) {
	dir := os.TempDir()
	f, err := os.CreateTemp(dir, "sparkyctrl-shell-*"+ext)
	if err != nil {
		return "", nil, fmt.Errorf("temp file: %w", err)
	}
	path := f.Name()
	if _, err := f.WriteString(script); err != nil {
		f.Close()
		os.Remove(path)
		return "", nil, fmt.Errorf("write temp script: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", nil, fmt.Errorf("close temp script: %w", err)
	}
	// Make executable on Unix; no-op on Windows (extension handles it).
	os.Chmod(path, 0o755)
	cleanup := func() { os.Remove(path) }
	return path, cleanup, nil
}
