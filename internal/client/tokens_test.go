package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTokens(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tokens")
	os.WriteFile(p, []byte("# secrets\nnas2 = \"tok-aaa\"\n\nweb = 'tok-bbb'\n"), 0o600)

	tokens, err := LoadTokens(p)
	if err != nil {
		t.Fatal(err)
	}
	if tokens["nas2"] != "tok-aaa" {
		t.Fatalf("nas2=%q", tokens["nas2"])
	}
	if tokens["web"] != "tok-bbb" {
		t.Fatalf("web=%q", tokens["web"])
	}
}

func TestLoadTokensMissingFileIsEmpty(t *testing.T) {
	tokens, err := LoadTokens(filepath.Join(t.TempDir(), "nope"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(tokens) != 0 {
		t.Fatalf("expected empty map, got %v", tokens)
	}
}

func TestResolveTokenEnvWins(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tokens")
	os.WriteFile(p, []byte("nas2 = \"file-tok\"\n"), 0o600)
	t.Setenv("SPARKYCTRL_TOKENS", p)
	t.Setenv("SPARKYCTRL_TOKEN", "env-tok")

	if got := ResolveToken("nas2"); got != "env-tok" {
		t.Fatalf("env should win, got %q", got)
	}
}

func TestResolveTokenFromFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tokens")
	os.WriteFile(p, []byte("nas2 = \"file-tok\"\n"), 0o600)
	t.Setenv("SPARKYCTRL_TOKENS", p)
	t.Setenv("SPARKYCTRL_TOKEN", "")

	if got := ResolveToken("nas2"); got != "file-tok" {
		t.Fatalf("expected file-tok, got %q", got)
	}
}

func TestResolveTokenMissingHost(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tokens")
	os.WriteFile(p, []byte("nas2 = \"file-tok\"\n"), 0o600)
	t.Setenv("SPARKYCTRL_TOKENS", p)
	t.Setenv("SPARKYCTRL_TOKEN", "")

	if got := ResolveToken("ghost"); got != "" {
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
