package cli

import (
	"testing"

	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
)

func TestSplitExecArgs(t *testing.T) {
	host, argv, err := splitExec([]string{"nas2", "--", "ls", "-la", "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	if host != "nas2" {
		t.Fatalf("host=%q", host)
	}
	if len(argv) != 3 || argv[0] != "ls" || argv[2] != "/tmp" {
		t.Fatalf("argv=%v", argv)
	}
}

func TestSplitExecMissingSeparator(t *testing.T) {
	_, _, err := splitExec([]string{"nas2", "ls"})
	if err == nil {
		t.Fatal("expected error when -- separator missing")
	}
}

func TestEmitExecClampsNegativeJSON(t *testing.T) {
	// A negative exit code (process couldn't start) must clamp to 1 in both modes.
	if got := emitExec(protocol.ExecResponse{ExitCode: -1, Error: "boom"}, true); got != 1 {
		t.Fatalf("json mode: got %d, want 1", got)
	}
	if got := emitExec(protocol.ExecResponse{ExitCode: -1}, false); got != 1 {
		t.Fatalf("human mode: got %d, want 1", got)
	}
	if got := emitExec(protocol.ExecResponse{ExitCode: 3}, true); got != 3 {
		t.Fatalf("json mode passthrough: got %d, want 3", got)
	}
}

func TestExtractJSONStopsAtSeparator(t *testing.T) {
	// --json before -- is OUR flag; --json after -- belongs to the command.
	args, jsonOut := extractJSON([]string{"box", "--json", "--", "mycmd", "--json"})
	if !jsonOut {
		t.Fatal("expected jsonOut true")
	}
	// the post-'--' --json must be preserved
	found := false
	for _, a := range args {
		if a == "mycmd" {
			found = true
		}
	}
	if !found {
		t.Fatalf("command args lost: %v", args)
	}
	// count remaining --json (should be exactly the one after --)
	n := 0
	for _, a := range args {
		if a == "--json" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("expected exactly one surviving --json (the command's), got %d in %v", n, args)
	}
}
