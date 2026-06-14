package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFenceAllowsInside(t *testing.T) {
	root := t.TempDir()
	got, err := ResolveFenced(root, filepath.Join(root, "sub", "file.txt"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, root) {
		t.Fatalf("resolved path %q escaped fence %q", got, root)
	}
}

func TestFenceAllowsExactFenceRoot(t *testing.T) {
	root := t.TempDir()
	got, err := ResolveFenced(root, root)
	if err != nil {
		t.Fatalf("fence root itself should be allowed: %v", err)
	}
	if got == "" {
		t.Fatal("expected resolved path")
	}
}

func TestFenceBlocksTraversal(t *testing.T) {
	root := t.TempDir()
	_, err := ResolveFenced(root, filepath.Join(root, "..", "escape.txt"))
	if err == nil {
		t.Fatal("expected fence to block .. traversal")
	}
}

func TestFenceBlocksSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	_, err := ResolveFenced(root, filepath.Join(link, "file.txt"))
	if err == nil {
		t.Fatal("expected fence to block symlink escape")
	}
}

func TestNoFenceAllowsAnything(t *testing.T) {
	got, err := ResolveFenced("", "/etc/hostname")
	if err != nil || got == "" {
		t.Fatalf("no-fence should allow: got=%q err=%v", got, err)
	}
}

func TestFenceRequiresAbsolute(t *testing.T) {
	root := t.TempDir()
	_, err := ResolveFenced(root, "relative/path")
	if err == nil {
		t.Fatal("expected error for relative path")
	}
}
