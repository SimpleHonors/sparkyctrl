package server

import (
	"strings"
	"testing"

	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
)

func TestBuildWindowsShellLine_TrailingBackslashSurvives(t *testing.T) {
	got := buildWindowsShellLine("cmd.exe", `dir C:\`)
	want := `cmd.exe /s /c "dir C:\"`
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	// The whole point: a trailing backslash must NOT be doubled (which is what
	// Go's default arg escaping does, breaking cmd's parser).
	if strings.Contains(got, `\\`) {
		t.Fatalf("trailing backslash was doubled: %q", got)
	}
}

func TestBuildWindowsShellLine_SpacedComspecIsQuoted(t *testing.T) {
	got := buildWindowsShellLine(`C:\Program Files\cmd.exe`, "echo hi")
	want := `"C:\Program Files\cmd.exe" /s /c "echo hi"`
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestBuildWindowsShellLine_EmptyComspecDefaults(t *testing.T) {
	got := buildWindowsShellLine("", `dir C:\share\`)
	want := `cmd.exe /s /c "dir C:\share\"`
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

// The default shell must actually run on the platform the test runs on
// (Linux in CI here: /bin/sh). This is the regression guard against the
// hardcoded-/bin/sh bug — on Windows the same RunShell call must reach cmd.
func TestRunShellDefaultRunsOnThisPlatform(t *testing.T) {
	resp := RunShell(protocol.ShellRequest{Script: "echo sparkyctrl-shell-ok"})
	if resp.ExitCode != 0 {
		t.Fatalf("exit=%d err=%q stderr=%q", resp.ExitCode, resp.Error, resp.Stderr)
	}
	if !strings.Contains(resp.Stdout, "sparkyctrl-shell-ok") {
		t.Fatalf("stdout=%q", resp.Stdout)
	}
}
