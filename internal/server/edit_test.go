package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
)

// writeFile is a small helper that creates a file with content under the fence.
func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// postEdit issues an /v1/edit request and returns the response.
func postEdit(t *testing.T, url string, req protocol.EditRequest) *http.Response {
	t.Helper()
	body, _ := json.Marshal(req)
	resp, err := http.Post(url+"/v1/edit", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestEditReplacesExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	h := &Handler{Fence: dir}
	srv := httptest.NewServer(h.Mux())
	defer srv.Close()

	p := filepath.Join(dir, "f.txt")
	writeFile(t, p, "alpha BETA gamma\n", 0o644)

	resp := postEdit(t, srv.URL, protocol.EditRequest{Path: p, OldString: "BETA", NewString: "delta"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out protocol.EditResponse
	json.NewDecoder(resp.Body).Decode(&out)
	if out.Replaced != 1 {
		t.Fatalf("replaced=%d want 1", out.Replaced)
	}
	if got := readFile(t, p); got != "alpha delta gamma\n" {
		t.Fatalf("file=%q", got)
	}
	if out.BytesWritten != int64(len("alpha delta gamma\n")) {
		t.Fatalf("bytes_written=%d", out.BytesWritten)
	}
}

func TestEditZeroMatchRefused(t *testing.T) {
	dir := t.TempDir()
	h := &Handler{Fence: dir}
	srv := httptest.NewServer(h.Mux())
	defer srv.Close()

	p := filepath.Join(dir, "f.txt")
	const orig = "nothing to see here\n"
	writeFile(t, p, orig, 0o644)

	resp := postEdit(t, srv.URL, protocol.EditRequest{Path: p, OldString: "MISSING", NewString: "x"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for zero matches, got %d", resp.StatusCode)
	}
	if got := readFile(t, p); got != orig {
		t.Fatalf("file changed on refused edit: %q", got)
	}
}

func TestEditMultipleMatchRefusedWithoutReplaceAll(t *testing.T) {
	dir := t.TempDir()
	h := &Handler{Fence: dir}
	srv := httptest.NewServer(h.Mux())
	defer srv.Close()

	p := filepath.Join(dir, "f.txt")
	const orig = "foo foo foo\n"
	writeFile(t, p, orig, 0o644)

	resp := postEdit(t, srv.URL, protocol.EditRequest{Path: p, OldString: "foo", NewString: "bar"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for non-unique match, got %d", resp.StatusCode)
	}
	if got := readFile(t, p); got != orig {
		t.Fatalf("file changed on refused edit: %q", got)
	}
}

func TestEditReplaceAllReplacesAll(t *testing.T) {
	dir := t.TempDir()
	h := &Handler{Fence: dir}
	srv := httptest.NewServer(h.Mux())
	defer srv.Close()

	p := filepath.Join(dir, "f.txt")
	writeFile(t, p, "foo foo foo\n", 0o644)

	resp := postEdit(t, srv.URL, protocol.EditRequest{Path: p, OldString: "foo", NewString: "bar", ReplaceAll: true})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var out protocol.EditResponse
	json.NewDecoder(resp.Body).Decode(&out)
	if out.Replaced != 3 {
		t.Fatalf("replaced=%d want 3", out.Replaced)
	}
	if got := readFile(t, p); got != "bar bar bar\n" {
		t.Fatalf("file=%q", got)
	}
}

func TestEditReplaceAllZeroMatchStillRefused(t *testing.T) {
	dir := t.TempDir()
	h := &Handler{Fence: dir}
	srv := httptest.NewServer(h.Mux())
	defer srv.Close()

	p := filepath.Join(dir, "f.txt")
	const orig = "abc\n"
	writeFile(t, p, orig, 0o644)

	resp := postEdit(t, srv.URL, protocol.EditRequest{Path: p, OldString: "zzz", NewString: "y", ReplaceAll: true})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for replace_all zero matches, got %d", resp.StatusCode)
	}
	if got := readFile(t, p); got != orig {
		t.Fatalf("file changed: %q", got)
	}
}

func TestEditFenceViolationRefused(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	h := &Handler{Fence: dir}
	srv := httptest.NewServer(h.Mux())
	defer srv.Close()

	p := filepath.Join(outside, "secret.txt")
	const orig = "secret data\n"
	writeFile(t, p, orig, 0o644)

	resp := postEdit(t, srv.URL, protocol.EditRequest{Path: p, OldString: "secret", NewString: "leaked"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for fence violation, got %d", resp.StatusCode)
	}
	if got := readFile(t, p); got != orig {
		t.Fatalf("out-of-fence file changed: %q", got)
	}
}

func TestEditPreservesFileMode(t *testing.T) {
	dir := t.TempDir()
	h := &Handler{Fence: dir}
	srv := httptest.NewServer(h.Mux())
	defer srv.Close()

	p := filepath.Join(dir, "f.txt")
	writeFile(t, p, "one two\n", 0o640)

	resp := postEdit(t, srv.URL, protocol.EditRequest{Path: p, OldString: "two", NewString: "three"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0o640 {
		t.Fatalf("perm=%o want 640", fi.Mode().Perm())
	}
}

func TestEditRefusedLeavesFileByteIdentical(t *testing.T) {
	dir := t.TempDir()
	h := &Handler{Fence: dir}
	srv := httptest.NewServer(h.Mux())
	defer srv.Close()

	p := filepath.Join(dir, "f.txt")
	const orig = "line1\nline2\nline2\n"
	writeFile(t, p, orig, 0o644)
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}

	// non-unique -> refused
	resp := postEdit(t, srv.URL, protocol.EditRequest{Path: p, OldString: "line2", NewString: "X"})
	resp.Body.Close()
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("file mutated on refused edit: before=%q after=%q", before, after)
	}
}

func TestEditRejectsWrongMethod(t *testing.T) {
	dir := t.TempDir()
	h := &Handler{Fence: dir}
	srv := httptest.NewServer(h.Mux())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/edit")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", resp.StatusCode)
	}
	if allow := resp.Header.Get("Allow"); allow != "POST" {
		t.Fatalf("want Allow: POST, got %q", allow)
	}
}

func TestEditNoTempFileLeftBehind(t *testing.T) {
	dir := t.TempDir()
	h := &Handler{Fence: dir}
	srv := httptest.NewServer(h.Mux())
	defer srv.Close()

	p := filepath.Join(dir, "f.txt")
	writeFile(t, p, "keep KEEP keep\n", 0o644)

	resp := postEdit(t, srv.URL, protocol.EditRequest{Path: p, OldString: "KEEP", NewString: "kept"})
	resp.Body.Close()

	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 || ents[0].Name() != "f.txt" {
		var names []string
		for _, e := range ents {
			names = append(names, e.Name())
		}
		t.Fatalf("expected only f.txt to remain, got %v", names)
	}
}
