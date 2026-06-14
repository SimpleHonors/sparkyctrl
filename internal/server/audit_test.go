package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
)

// newAuditTestServer wires a Handler with an Auditor writing to logPath and
// returns the running test server. The caller is responsible for srv.Close();
// the auditor is closed via t.Cleanup.
func newAuditTestServer(t *testing.T, fence string, logPath string) *httptest.Server {
	t.Helper()
	a, err := NewAuditor(logPath)
	if err != nil {
		t.Fatalf("new auditor: %v", err)
	}
	t.Cleanup(func() { a.Close() })
	h := &Handler{Fence: fence, Audit: a}
	return httptest.NewServer(h.Mux())
}

// readAuditRecords parses the JSONL audit file into decoded records.
func readAuditRecords(t *testing.T, logPath string) []map[string]any {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	var recs []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("bad audit line %q: %v", line, err)
		}
		recs = append(recs, rec)
	}
	return recs
}

func TestAuditDeniedFenceWriteIsLogged(t *testing.T) {
	dir := t.TempDir()
	fence := filepath.Join(dir, "fence")
	if err := os.Mkdir(fence, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "audit.log")
	srv := newAuditTestServer(t, fence, logPath)
	defer srv.Close()

	// Upload to a path OUTSIDE the fence -> must be denied.
	outside := filepath.Join(dir, "escape.txt")
	resp, err := http.Post(srv.URL+"/v1/upload?path="+outside, "application/octet-stream", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for fence violation, got %d", resp.StatusCode)
	}

	recs := readAuditRecords(t, logPath)
	if len(recs) != 1 {
		t.Fatalf("want 1 audit record for denied write, got %d: %+v", len(recs), recs)
	}
	rec := recs[0]
	if rec["op"] != "upload" {
		t.Errorf("op = %v, want upload", rec["op"])
	}
	if rec["outcome"] != "denied" {
		t.Errorf("outcome = %v, want denied", rec["outcome"])
	}
	if ip, _ := rec["src_ip"].(string); ip == "" {
		t.Errorf("src_ip empty, want populated source IP; rec=%+v", rec)
	}
}

func TestAuditSuccessfulExecRecordsOutcomeAndExitCode(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")
	srv := newAuditTestServer(t, "", logPath)
	defer srv.Close()

	body, _ := json.Marshal(protocol.ExecRequest{Argv: []string{"true"}})
	resp, err := http.Post(srv.URL+"/v1/exec", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	recs := readAuditRecords(t, logPath)
	if len(recs) != 1 {
		t.Fatalf("want 1 audit record, got %d: %+v", len(recs), recs)
	}
	rec := recs[0]
	if rec["op"] != "exec" {
		t.Errorf("op = %v, want exec", rec["op"])
	}
	if rec["outcome"] != "success" {
		t.Errorf("outcome = %v, want success", rec["outcome"])
	}
	// JSON numbers decode to float64.
	if code, ok := rec["exit_code"].(float64); !ok || code != 0 {
		t.Errorf("exit_code = %v, want 0", rec["exit_code"])
	}
	if ip, _ := rec["src_ip"].(string); ip == "" {
		t.Errorf("src_ip empty, want populated source IP; rec=%+v", rec)
	}
}

func TestAuditFailedExecRecordsErrorOutcomeWithExitCode(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")
	srv := newAuditTestServer(t, "", logPath)
	defer srv.Close()

	// `false` exits non-zero.
	body, _ := json.Marshal(protocol.ExecRequest{Argv: []string{"false"}})
	resp, err := http.Post(srv.URL+"/v1/exec", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	recs := readAuditRecords(t, logPath)
	if len(recs) != 1 {
		t.Fatalf("want 1 audit record, got %d: %+v", len(recs), recs)
	}
	rec := recs[0]
	if rec["outcome"] != "error" {
		t.Errorf("outcome = %v, want error for non-zero exit", rec["outcome"])
	}
	if code, ok := rec["exit_code"].(float64); !ok || code != 1 {
		t.Errorf("exit_code = %v, want 1", rec["exit_code"])
	}
}

func TestAuditWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")
	a, err := NewAuditor(logPath)
	if err != nil {
		t.Fatalf("new auditor: %v", err)
	}
	defer a.Close()

	a.Log("exec", map[string]any{"argv": []string{"ls", "-l"}})
	a.Log("shell", map[string]any{"script": "ps | grep x"})

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 log lines, got %d", len(lines))
	}
	if !strings.Contains(lines[1], `"op":"shell"`) {
		t.Fatalf("shell op not recorded distinctly: %s", lines[1])
	}
}

func TestNilAuditorNoOps(t *testing.T) {
	var a *Auditor // nil
	// Must not panic.
	a.Log("exec", map[string]any{"argv": []string{"true"}})
	if err := a.Close(); err != nil {
		t.Fatalf("nil Close should be nil: %v", err)
	}
}

func TestNewAuditorEmptyPathDisabled(t *testing.T) {
	a, err := NewAuditor("")
	if err != nil {
		t.Fatalf("empty path should not error: %v", err)
	}
	if a != nil {
		t.Fatal("empty path should yield a nil (disabled) auditor")
	}
}
