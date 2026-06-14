package cli

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SimpleHonors/sparkyctrl/internal/server"
)

func TestEndToEnd(t *testing.T) {
	dir := t.TempDir()
	h := &server.Handler{Fence: dir}
	srv := httptest.NewServer(h.Mux())
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	hostsFile := filepath.Join(dir, "hosts.toml")
	if err := os.WriteFile(hostsFile, []byte("box = \""+addr+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPARKYCTRL_HOSTS", hostsFile)

	if code := Run([]string{"exec", "box", "--", "echo", "hi"}); code != 0 {
		t.Fatalf("exec exit=%d", code)
	}
	local := filepath.Join(dir, "local.txt")
	if err := os.WriteFile(local, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	remote := filepath.Join(dir, "remote.txt")
	if code := Run([]string{"push", "box", local, remote}); code != 0 {
		t.Fatalf("push exit=%d", code)
	}
	back := filepath.Join(dir, "back.txt")
	if code := Run([]string{"pull", "box", remote, back}); code != 0 {
		t.Fatalf("pull exit=%d", code)
	}
	got, _ := os.ReadFile(back)
	if string(got) != "payload" {
		t.Fatalf("round-trip mismatch: %q", got)
	}
}
