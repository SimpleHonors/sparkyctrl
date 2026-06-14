package client

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
	"github.com/SimpleHonors/sparkyctrl/internal/server"
)

func TestClientExec(t *testing.T) {
	h := &server.Handler{}
	srv := httptest.NewServer(h.Mux())
	defer srv.Close()

	c := New(srv.URL, "")
	resp, err := c.Exec(protocol.ExecRequest{Argv: []string{"echo", "hello"}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ExitCode != 0 {
		t.Fatalf("exit=%d", resp.ExitCode)
	}
}

func TestClientUploadDownload(t *testing.T) {
	dir := t.TempDir()
	h := &server.Handler{Fence: dir}
	srv := httptest.NewServer(h.Mux())
	defer srv.Close()

	c := New(srv.URL, "")
	remote := dir + "/x.bin"
	payload := []byte{0, 1, 2, 3, 255}
	if err := c.Upload(remote, 0o644, payload); err != nil {
		t.Fatalf("upload: %v", err)
	}
	got, err := c.Download(remote)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("mismatch: %v", got)
	}
}

func TestClientInfo(t *testing.T) {
	h := &server.Handler{Fence: "/tmp"}
	srv := httptest.NewServer(h.Mux())
	defer srv.Close()

	c := New(srv.URL, "")
	info, err := c.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Version != protocol.Version {
		t.Fatalf("version: got %q, want %q", info.Version, protocol.Version)
	}
	if info.Fence != "/tmp" {
		t.Fatalf("fence: got %q, want %q", info.Fence, "/tmp")
	}
}

func TestClientInfoUnauthorized(t *testing.T) {
	h := &server.Handler{Token: "secret"}
	srv := httptest.NewServer(h.Mux())
	defer srv.Close()

	c := New(srv.URL, "")
	_, err := c.Info()
	if err == nil {
		t.Fatal("expected error for unauthorized request, got nil")
	}
}
