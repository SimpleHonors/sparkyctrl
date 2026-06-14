package cli

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SimpleHonors/sparkyctrl/internal/server"
)

// setupEditHost wires a fenced worker + hosts file and returns the fence dir.
func setupEditHost(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	h := &server.Handler{Fence: dir}
	srv := httptest.NewServer(h.Mux())
	t.Cleanup(srv.Close)
	addr := strings.TrimPrefix(srv.URL, "http://")
	hostsFile := filepath.Join(dir, "hosts.toml")
	if err := os.WriteFile(hostsFile, []byte("box = \""+addr+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPARKYCTRL_HOSTS", hostsFile)
	return dir
}

func TestEditVerbExactOnce(t *testing.T) {
	dir := setupEditHost(t)
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("hello WORLD\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code := Run([]string{"edit", "box", p, "--old", "WORLD", "--new", "there"})
	if code != 0 {
		t.Fatalf("edit exit=%d", code)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "hello there\n" {
		t.Fatalf("file=%q", got)
	}
}

func TestEditVerbNonUniqueFails(t *testing.T) {
	dir := setupEditHost(t)
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("a a a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"edit", "box", p, "--old", "a", "--new", "b"}); code == 0 {
		t.Fatal("expected non-zero exit for non-unique match")
	}
	got, _ := os.ReadFile(p)
	if string(got) != "a a a\n" {
		t.Fatalf("file changed on refused edit: %q", got)
	}
}

func TestEditVerbAllFlag(t *testing.T) {
	dir := setupEditHost(t)
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("a a a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"edit", "box", p, "--old", "a", "--new", "b", "--all"}); code != 0 {
		t.Fatalf("edit --all exit=%d", code)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "b b b\n" {
		t.Fatalf("file=%q", got)
	}
}

func TestEditVerbOldFileNewFileMultiline(t *testing.T) {
	dir := setupEditHost(t)
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("start\nMID1\nMID2\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldF := filepath.Join(dir, "old.txt")
	newF := filepath.Join(dir, "new.txt")
	if err := os.WriteFile(oldF, []byte("MID1\nMID2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newF, []byte("REPLACED\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code := Run([]string{"edit", "box", p, "--old-file", oldF, "--new-file", newF})
	if code != 0 {
		t.Fatalf("edit exit=%d", code)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "start\nREPLACED\nend\n" {
		t.Fatalf("file=%q", got)
	}
}

func TestEditVerbMissingFlags(t *testing.T) {
	dir := setupEditHost(t)
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"edit", "box", p}); code == 0 {
		t.Fatal("expected non-zero exit when --old is empty")
	}
}
