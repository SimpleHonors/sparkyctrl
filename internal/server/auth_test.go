package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTokenPathForGOOS(t *testing.T) {
	if got := TokenPathForGOOS("linux"); got != linuxTokenPath {
		t.Fatalf("linux token path = %q", got)
	}
	if got := TokenPathForGOOS("windows"); got != windowsTokenPath {
		t.Fatalf("windows token path = %q", got)
	}
}

func TestLoadTokenFileTrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("  secret-token \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadTokenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "secret-token" {
		t.Fatalf("token = %q", got)
	}
}

func TestResolveAuthModes(t *testing.T) {
	token, mode, source, err := ResolveAuth(true, "")
	if err != nil {
		t.Fatal(err)
	}
	if token != "" || mode != "disabled" || source != "" {
		t.Fatalf("disabled = %q %q %q", token, mode, source)
	}

	token, mode, source, err = ResolveAuth(false, "override")
	if err != nil {
		t.Fatal(err)
	}
	if token != "override" || mode != "token-flag" || source != "--token" {
		t.Fatalf("override = %q %q %q", token, mode, source)
	}
}
