package server

import (
	"runtime"
	"testing"

	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
)

func TestExecPassesArgsVerbatim(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /usr/bin/printf")
	}
	// Every dangerous character in one argument. With no shell, printf %s
	// must echo it back byte-for-byte and nothing may be expanded.
	nasty := "a b\t$(rm -rf /) `whoami` ; | & > < * ? \" ' \\ \n 日本語 café 🔥 end"
	resp := RunExec(protocol.ExecRequest{Argv: []string{"printf", "%s", nasty}})
	if resp.Error != "" {
		t.Fatalf("unexpected setup error: %s", resp.Error)
	}
	if resp.ExitCode != 0 {
		t.Fatalf("exit=%d stderr=%q", resp.ExitCode, resp.Stderr)
	}
	if resp.Stdout != nasty {
		t.Fatalf("argument was altered:\n got: %q\nwant: %q", resp.Stdout, nasty)
	}
}

func TestExecDoesNotGlob(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /usr/bin/echo")
	}
	// `echo *` via exec must print a literal asterisk, proving no shell glob.
	resp := RunExec(protocol.ExecRequest{Argv: []string{"echo", "*"}})
	if resp.Stdout != "*\n" {
		t.Fatalf("glob expansion leaked: %q", resp.Stdout)
	}
}

func TestExecExitCodeAndStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh false")
	}
	resp := RunExec(protocol.ExecRequest{Argv: []string{"false"}})
	if resp.ExitCode != 1 {
		t.Fatalf("exit=%d, want 1", resp.ExitCode)
	}
}

func TestExecEmptyArgvIsError(t *testing.T) {
	resp := RunExec(protocol.ExecRequest{Argv: nil})
	if resp.Error == "" {
		t.Fatal("expected error for empty argv")
	}
}

func TestExecStdin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/cat")
	}
	resp := RunExec(protocol.ExecRequest{Argv: []string{"cat"}, Stdin: "hello stdin"})
	if resp.Stdout != "hello stdin" {
		t.Fatalf("stdin not piped: %q", resp.Stdout)
	}
}
