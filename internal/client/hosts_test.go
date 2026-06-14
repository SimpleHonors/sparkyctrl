package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseHosts(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hosts.toml")
	os.WriteFile(p, []byte("# comment\nnas2 = \"192.0.2.5:7766\"\n\nweb = '192.0.2.6:7766'\n"), 0o644)

	hosts, err := LoadHosts(p)
	if err != nil {
		t.Fatal(err)
	}
	if hosts["nas2"] != "192.0.2.5:7766" {
		t.Fatalf("nas2=%q", hosts["nas2"])
	}
	if hosts["web"] != "192.0.2.6:7766" {
		t.Fatalf("web=%q", hosts["web"])
	}
}

func TestResolveAddsScheme(t *testing.T) {
	hosts := map[string]string{"nas2": "192.0.2.5:7766"}
	url, err := Resolve(hosts, "nas2")
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://192.0.2.5:7766" {
		t.Fatalf("url=%q", url)
	}
}

func TestResolveUnknownHost(t *testing.T) {
	_, err := Resolve(map[string]string{}, "ghost")
	if err == nil {
		t.Fatal("expected error for unknown host")
	}
}

func TestResolveRawAddress(t *testing.T) {
	// A host:port not in the book is used directly.
	url, err := Resolve(map[string]string{}, "192.0.2.9:7766")
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://192.0.2.9:7766" {
		t.Fatalf("url=%q", url)
	}
}
