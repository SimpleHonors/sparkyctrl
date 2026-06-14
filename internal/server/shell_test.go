package server

import (
	"runtime"
	"testing"

	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
)

func TestShellExpandsPipes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix shell test")
	}
	// Unlike exec, the shell path MUST interpret a pipe.
	resp := RunShell(protocol.ShellRequest{Script: "printf 'a\\nb\\nc\\n' | grep b"})
	if resp.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", resp.ExitCode, resp.Stderr)
	}
	if resp.Stdout != "b\n" {
		t.Fatalf("pipe not interpreted: %q", resp.Stdout)
	}
}

func TestShellEmptyScriptIsError(t *testing.T) {
	resp := RunShell(protocol.ShellRequest{Script: ""})
	if resp.Error == "" {
		t.Fatal("expected error for empty script")
	}
}
