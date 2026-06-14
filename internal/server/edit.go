package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
)

// Edit performs a surgical, exact-string replacement on a fence-checked file.
//
// Matching is byte-for-byte: no regex, no shell, no whitespace normalisation.
// By default (req.ReplaceAll == false) old_string must occur exactly once —
// zero matches or more than one match both refuse and leave the file
// untouched. With ReplaceAll set, every occurrence is replaced, but zero
// matches still refuses.
//
// The write is atomic: the new content is built fully in memory and committed
// via a temp file in the same directory + os.Rename, so a reader never sees a
// half-written file and any failure before the rename leaves the original
// byte-for-byte unchanged. The original file mode is preserved.
func (h *Handler) Edit(req protocol.EditRequest) (protocol.EditResponse, error) {
	if req.OldString == "" {
		return protocol.EditResponse{}, fmt.Errorf("old_string must not be empty")
	}

	real, err := ResolveFenced(h.Fence, req.Path)
	if err != nil {
		return protocol.EditResponse{}, err
	}

	info, err := os.Stat(real)
	if err != nil {
		return protocol.EditResponse{}, err
	}
	if info.IsDir() {
		return protocol.EditResponse{}, fmt.Errorf("%s is a directory", req.Path)
	}
	mode := info.Mode().Perm()

	orig, err := os.ReadFile(real)
	if err != nil {
		return protocol.EditResponse{}, err
	}
	content := string(orig)

	count := strings.Count(content, req.OldString)
	if count == 0 {
		return protocol.EditResponse{}, fmt.Errorf("old_string not found in %s", req.Path)
	}
	if !req.ReplaceAll && count > 1 {
		return protocol.EditResponse{}, fmt.Errorf(
			"old_string is not unique (%d matches) in %s; add context or set replace_all", count, req.Path)
	}

	var updated string
	if req.ReplaceAll {
		updated = strings.ReplaceAll(content, req.OldString, req.NewString)
	} else {
		// count == 1 here.
		updated = strings.Replace(content, req.OldString, req.NewString, 1)
	}

	if err := atomicWrite(real, []byte(updated), mode); err != nil {
		return protocol.EditResponse{}, err
	}

	return protocol.EditResponse{Replaced: count, BytesWritten: int64(len(updated))}, nil
}

// atomicWrite writes data to a temp file in the same directory as dst, fsyncs
// it, then renames it over dst. On any failure before the rename, dst is left
// unchanged and the temp file is removed.
func atomicWrite(dst string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".sparkyctrl-edit-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we don't make it to a successful rename.
	committed := false
	defer func() {
		if !committed {
			os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return err
	}
	committed = true
	return nil
}
