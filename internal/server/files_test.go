package server

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadDownloadRoundTrip(t *testing.T) {
	root := t.TempDir()
	h := &Handler{Fence: root}
	dst := filepath.Join(root, "data.bin")
	payload := []byte{0, 1, 2, 255, 254, '\n', 'x'} // binary, includes NUL

	if err := h.Upload(dst, 0o644, bytes.NewReader(payload)); err != nil {
		t.Fatalf("upload: %v", err)
	}
	var got bytes.Buffer
	if err := h.Download(dst, &got); err != nil {
		t.Fatalf("download: %v", err)
	}
	if !bytes.Equal(got.Bytes(), payload) {
		t.Fatalf("round-trip not byte-exact: %v", got.Bytes())
	}
}

func TestUploadOutsideFenceRejected(t *testing.T) {
	root := t.TempDir()
	h := &Handler{Fence: root}
	err := h.Upload("/tmp/escape.bin", 0o644, bytes.NewReader([]byte("x")))
	if err == nil {
		t.Fatal("expected fence rejection")
	}
}

// TestUploadDanglingSymlinkEscapeRejected reproduces bug #18: a dangling
// symlink planted inside the fence pointing OUTSIDE it must not let a write
// escape the fence. os.OpenFile(O_CREATE) follows the symlink and would create
// the file at the outside target; the write must be refused before that.
func TestUploadDanglingSymlinkEscapeRejected(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "evil") // does NOT exist yet
	link := filepath.Join(root, "evil")      // inside the fence
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	h := &Handler{Fence: root}
	err := h.Upload(link, 0o644, bytes.NewReader([]byte("* * * * * root payload\n")))
	if err == nil {
		t.Fatal("expected fence rejection for write through dangling symlink escaping the fence")
	}
	if _, statErr := os.Lstat(target); statErr == nil {
		t.Fatalf("write escaped the fence: file was created at outside target %q", target)
	}
}

// TestUploadDanglingSymlinkChainEscapeRejected covers a symlink -> symlink
// chain whose final target lands outside the fence.
func TestUploadDanglingSymlinkChainEscapeRejected(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "evil") // does NOT exist yet
	mid := filepath.Join(root, "mid")
	link := filepath.Join(root, "evil")
	if err := os.Symlink(target, mid); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if err := os.Symlink(mid, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	h := &Handler{Fence: root}
	err := h.Upload(link, 0o644, bytes.NewReader([]byte("x")))
	if err == nil {
		t.Fatal("expected fence rejection for write through symlink chain escaping the fence")
	}
	if _, statErr := os.Lstat(target); statErr == nil {
		t.Fatalf("write escaped the fence via chain: file created at %q", target)
	}
}

// TestUploadInFenceSymlinkAllowed: a symlink pointing to another in-fence
// location is allowed (least-surprising semantics: symlinks are fine as long as
// the resolved target stays inside the fence).
func TestUploadInFenceSymlinkAllowed(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real.txt") // in-fence, not-yet-existing
	link := filepath.Join(root, "alias.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	h := &Handler{Fence: root}
	if err := h.Upload(link, 0o644, bytes.NewReader([]byte("hello"))); err != nil {
		t.Fatalf("in-fence symlinked write should succeed: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read through symlink target: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("content mismatch: %q", got)
	}
}

// TestUploadNewFileInFenceAllowed: writing a not-yet-existing regular file
// inside the fence (no symlink) still creates it.
func TestUploadNewFileInFenceAllowed(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "nested", "new.txt")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	h := &Handler{Fence: root}
	if err := h.Upload(dst, 0o644, bytes.NewReader([]byte("data"))); err != nil {
		t.Fatalf("new in-fence file write should succeed: %v", err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("expected file created: %v", err)
	}
}

func TestLs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := &Handler{Fence: root}
	resp, err := h.Ls(root)
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(resp.Entries))
	}
	if resp.Entries[0].Name != "a.txt" {
		t.Errorf("entries[0].Name = %q, want %q", resp.Entries[0].Name, "a.txt")
	}
	if resp.Entries[1].Name != "sub" {
		t.Errorf("entries[1].Name = %q, want %q", resp.Entries[1].Name, "sub")
	}
	if !resp.Entries[1].IsDir {
		t.Errorf("entries[1].IsDir = false, want true")
	}
}

func TestDownloadOutsideFenceRejected(t *testing.T) {
	root := t.TempDir()
	h := &Handler{Fence: root}
	var buf bytes.Buffer
	if err := h.Download("/etc/hostname", &buf); err == nil {
		t.Fatal("expected fence rejection")
	}
}
