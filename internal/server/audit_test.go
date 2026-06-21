package server

import (
	"bytes"
	"encoding/hex"
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
	a, err := NewAuditor(logPath, nil)
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
	a, err := NewAuditor(logPath, nil)
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
	a, err := NewAuditor("", nil)
	if err != nil {
		t.Fatalf("empty path should not error: %v", err)
	}
	if a != nil {
		t.Fatal("empty path should yield a nil (disabled) auditor")
	}
}

// ----- tamper-evident chain tests -----

func testKey(t *testing.T) []byte {
	t.Helper()
	key, err := GenerateAuditKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	raw, err := hex.DecodeString(key)
	if err != nil {
		t.Fatalf("decode generated key: %v", err)
	}
	return raw
}

func TestChainVerifyPassesOnCleanLog(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")
	key := testKey(t)
	a, err := NewAuditor(logPath, key)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		a.Log("exec", map[string]any{"i": i})
	}
	a.Close()

	line, err := Verify(logPath, key)
	if err != nil || line != 0 {
		t.Fatalf("clean log should verify; got line=%d err=%v", line, err)
	}
}

func TestChainVerifyDetectsTruncatedTail(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")
	key := testKey(t)
	a, _ := NewAuditor(logPath, key)
	a.Log("exec", map[string]any{"i": 0})
	a.Log("exec", map[string]any{"i": 1})
	a.Log("exec", map[string]any{"i": 2})
	a.Close()

	// Truncate the file to half its length — last record is now corrupt.
	data, _ := os.ReadFile(logPath)
	if err := os.WriteFile(logPath, data[:len(data)/2], 0o600); err != nil {
		t.Fatal(err)
	}

	// Verify should either fail (bad JSON / bad HMAC) on the partial line,
	// or detect a prev_hash mismatch from the surviving-but-orphaned record.
	line, err := Verify(logPath, key)
	if err == nil && line == 0 {
		t.Fatalf("truncated tail must fail verification; got line=%d err=%v", line, err)
	}
}

func TestChainVerifyDetectsMutatedRecord(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")
	key := testKey(t)
	a, _ := NewAuditor(logPath, key)
	a.Log("exec", map[string]any{"i": 0})
	a.Log("exec", map[string]any{"i": 1})
	a.Log("exec", map[string]any{"i": 2})
	a.Close()

	// Flip one byte in the middle of the second record's body (its
	// "op":"exec" → "op":"EXEC" edit). The HMAC over the second record
	// no longer matches, and the third record's prev_hash also fails.
	data, _ := os.ReadFile(logPath)
	lines := bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d", len(lines))
	}
	// Pick a byte inside the op field of line 1 and flip it.
	mutated := append([]byte{}, lines[1]...)
	idx := bytes.Index(mutated, []byte(`"op":"exec"`))
	if idx < 0 {
		t.Fatalf("could not locate op field in line 1: %s", lines[1])
	}
	mutated[idx+6] = 'E' // "exec" -> "ExEc" — defaces op value
	out := append(append(bytes.Join(lines[:1], []byte("\n")), '\n'), mutated...)
	out = append(out, '\n')
	out = append(out, lines[2]...)
	out = append(out, '\n')
	if err := os.WriteFile(logPath, out, 0o600); err != nil {
		t.Fatal(err)
	}

	line, err := Verify(logPath, key)
	if err == nil && line == 0 {
		t.Fatalf("mutated record must fail verification; got line=%d err=%v", line, err)
	}
}

func TestChainVerifyDetectsRemovedRecord(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")
	key := testKey(t)
	a, _ := NewAuditor(logPath, key)
	a.Log("exec", map[string]any{"i": 0})
	a.Log("exec", map[string]any{"i": 1})
	a.Log("exec", map[string]any{"i": 2})
	a.Close()

	// Drop the middle record. The third record's prev_hash now points at
	// record #1's hash, but the chain expects it to point at #2's hash.
	data, _ := os.ReadFile(logPath)
	lines := bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n"))
	if len(lines) != 3 {
		t.Fatalf("want 3 lines, got %d", len(lines))
	}
	out := append(append(bytes.Join(lines[:1], []byte("\n")), '\n'), lines[2]...)
	out = append(out, '\n')
	if err := os.WriteFile(logPath, out, 0o600); err != nil {
		t.Fatal(err)
	}

	line, err := Verify(logPath, key)
	if err == nil && line == 0 {
		t.Fatalf("removed record must fail verification; got line=%d err=%v", line, err)
	}
}

func TestChainResumesAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")
	key := testKey(t)

	// First writer appends three records.
	a1, _ := NewAuditor(logPath, key)
	a1.Log("exec", map[string]any{"i": 0})
	a1.Log("exec", map[string]any{"i": 1})
	a1.Log("exec", map[string]any{"i": 2})
	a1.Close()

	// Second writer reopens with the same key — must resume the chain so the
	// final log still verifies end-to-end.
	a2, _ := NewAuditor(logPath, key)
	a2.Log("exec", map[string]any{"i": 3})
	a2.Log("exec", map[string]any{"i": 4})
	a2.Close()

	line, err := Verify(logPath, key)
	if err != nil || line != 0 {
		t.Fatalf("reopened log should still verify; got line=%d err=%v", line, err)
	}
}

func TestChainVerifyRequiresKey(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")
	key := testKey(t)
	a, _ := NewAuditor(logPath, key)
	a.Log("exec", nil)
	a.Close()

	if _, err := Verify(logPath, nil); err == nil {
		t.Fatal("verify without key should fail")
	}
	if _, err := Verify(logPath, []byte{}); err == nil {
		t.Fatal("verify with empty key should fail")
	}
}

func TestChainVerifyRejectsWrongKey(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")
	key := testKey(t)
	a, _ := NewAuditor(logPath, key)
	a.Log("exec", nil)
	a.Close()

	wrongKey := testKey(t)
	line, err := Verify(logPath, wrongKey)
	if err == nil && line == 0 {
		t.Fatal("verify with wrong key must fail")
	}
}

func TestLegacyLogReturnsLineMinusOne(t *testing.T) {
	// Records written without prev_hash/hmac (legacy format) should report
	// "line = -1, no error" so callers can distinguish un-chained history
	// from a real tamper.
	dir := t.TempDir()
	logPath := filepath.Join(dir, "audit.log")
	a, _ := NewAuditor(logPath, nil) // nil key → legacy format
	a.Log("exec", map[string]any{"x": 1})
	a.Close()

	key := testKey(t)
	line, err := Verify(logPath, key)
	if err != nil {
		t.Fatalf("legacy log: want nil err, got %v", err)
	}
	if line != -1 {
		t.Fatalf("legacy log: want line=-1, got %d", line)
	}
}
