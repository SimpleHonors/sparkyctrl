package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTokenEnvWins(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tokens")
	os.WriteFile(p, []byte("nas2 = \"file-tok\"\n"), 0o600)
	t.Setenv("SPARKYCTRL_TOKENS", p)
	t.Setenv("SPARKYCTRL_TOKEN", "env-tok")

	if got := resolveToken("nas2"); got != "env-tok" {
		t.Fatalf("env should win, got %q", got)
	}
}

func TestResolveTokenFromFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tokens")
	os.WriteFile(p, []byte("nas2 = \"file-tok\"\n"), 0o600)
	t.Setenv("SPARKYCTRL_TOKENS", p)
	t.Setenv("SPARKYCTRL_TOKEN", "")

	if got := resolveToken("nas2"); got != "file-tok" {
		t.Fatalf("expected file-tok, got %q", got)
	}
}

func TestResolveTokenMissingHost(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tokens")
	os.WriteFile(p, []byte("nas2 = \"file-tok\"\n"), 0o600)
	t.Setenv("SPARKYCTRL_TOKENS", p)
	t.Setenv("SPARKYCTRL_TOKEN", "")

	if got := resolveToken("ghost"); got != "" {
		t.Fatalf("unknown host should be empty, got %q", got)
	}
}

func TestLooseFilePerms(t *testing.T) {
	if !looseFilePerms(0o644) {
		t.Fatal("0o644 should be flagged loose")
	}
	if looseFilePerms(0o600) {
		t.Fatal("0o600 should be fine")
	}
}
