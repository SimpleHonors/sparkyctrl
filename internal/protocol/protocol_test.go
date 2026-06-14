package protocol

import (
	"encoding/json"
	"testing"
)

func TestExecRequestRoundTrip(t *testing.T) {
	in := ExecRequest{Argv: []string{"echo", "hi"}, Cwd: "/tmp", TimeoutSec: 5}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out ExecRequest
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Argv) != 2 || out.Argv[0] != "echo" || out.Cwd != "/tmp" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

func TestEditRequestRoundTrip(t *testing.T) {
	in := EditRequest{Path: "/etc/x", OldString: "a", NewString: "b", ReplaceAll: true}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out EditRequest
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Path != "/etc/x" || out.OldString != "a" || out.NewString != "b" || !out.ReplaceAll {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

func TestConstants(t *testing.T) {
	if DefaultPort != 7766 {
		t.Fatalf("DefaultPort = %d, want 7766", DefaultPort)
	}
	if Version == "" {
		t.Fatal("Version must be set")
	}
}
