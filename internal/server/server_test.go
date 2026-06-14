package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
)

func newTestServer(t *testing.T, fence, token string) *httptest.Server {
	t.Helper()
	h := &Handler{Fence: fence, Token: token}
	return httptest.NewServer(h.Mux())
}

func TestExecEndpoint(t *testing.T) {
	srv := newTestServer(t, "", "")
	defer srv.Close()

	body, _ := json.Marshal(protocol.ExecRequest{Argv: []string{"echo", "hi"}})
	resp, err := http.Post(srv.URL+"/v1/exec", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out protocol.ExecResponse
	json.NewDecoder(resp.Body).Decode(&out)
	if strings.TrimSpace(out.Stdout) != "hi" {
		t.Fatalf("stdout=%q", out.Stdout)
	}
}

func TestInfoEndpoint(t *testing.T) {
	srv := newTestServer(t, "/tmp", "")
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/info")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var info protocol.Info
	json.NewDecoder(resp.Body).Decode(&info)
	if info.Version != protocol.Version || info.Fence != "/tmp" {
		t.Fatalf("info=%+v", info)
	}
}

func TestTokenRequiredWhenSet(t *testing.T) {
	srv := newTestServer(t, "", "secret")
	defer srv.Close()

	body, _ := json.Marshal(protocol.ExecRequest{Argv: []string{"echo", "hi"}})
	// no token -> 401
	resp, err := http.Post(srv.URL+"/v1/exec", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
	// correct token -> 200
	req, _ := http.NewRequest("POST", srv.URL+"/v1/exec", bytes.NewReader(body))
	req.Header.Set(protocol.TokenHeader, "secret")
	req.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp2.StatusCode)
	}
}

func TestTokenWrongSameLengthRejected(t *testing.T) {
	srv := newTestServer(t, "", "secret")
	defer srv.Close()
	body, _ := json.Marshal(protocol.ExecRequest{Argv: []string{"echo", "hi"}})
	req, _ := http.NewRequest("POST", srv.URL+"/v1/exec", bytes.NewReader(body))
	req.Header.Set(protocol.TokenHeader, "secres") // same length, wrong
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestUploadModeMaskedToUnixBits(t *testing.T) {
	dir := t.TempDir()
	h := &Handler{Fence: dir}
	srv := httptest.NewServer(h.Mux())
	defer srv.Close()
	target := dir + "/m.txt"
	resp, err := http.Post(srv.URL+"/v1/upload?path="+target+"&mode=40755", "application/octet-stream", strings.NewReader("hi"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload status=%d", resp.StatusCode)
	}
	fi, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Fatalf("perm=%o want 755", fi.Mode().Perm())
	}
}
