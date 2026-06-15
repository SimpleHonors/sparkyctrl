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
