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
		return protocol.EditResponse{}, fmt.Errorf("old_string not found in %s%s",
			req.Path, diagnoseNoMatch(content, req.OldString))
	}
	if !req.ReplaceAll && count > 1 {
		return protocol.EditResponse{}, fmt.Errorf(
			"old_string is not unique (%d matches) in %s; add context or set replace_all%s",
			count, req.Path, diagnoseAmbiguous(content, req.OldString, count))
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

// diagnoseNoMatch builds a diagnostic suffix for a zero-match edit failure.
// It shows a preview of the searched-for string and the beginning of the file
// so the caller can spot common mistakes like CRLF/LF mismatch, missing
// whitespace, or wrong target file without having to re-read the file and
// diff byte-by-byte.
//
// Note: fixed-byte slicing may bisect multi-byte UTF-8 runes at the cut
// boundary. Since this is an error path and the result is formatted with %q
// (which escapes rather than panics), this is cosmetic — the caller still
// sees the line-ending / encoding mismatch that matters.
func diagnoseNoMatch(content, old string) string {
	var b strings.Builder
	if n := len(old); n > 0 {
		preview := old
		if len(preview) > 40 {
			preview = preview[:40] + "..."
		}
		b.WriteString(fmt.Sprintf("\n  searched for (%d bytes): %q", n, preview))
	}
	if len(content) > 0 {
		filePreview := content
		if len(filePreview) > 80 {
			filePreview = filePreview[:80] + "..."
		}
		b.WriteString(fmt.Sprintf("\n  file starts with (%d bytes): %q", len(content), filePreview))
	}
	// If line endings differ, call it out explicitly so the caller doesn't
	// have to visually compare \r\n against \n in the previews.
	if hint := lineEndingHint(content, old); hint != "" {
		b.WriteString("\n  " + hint)
	}
	b.WriteString("\n  matching is byte-for-byte: no regex, no whitespace normalisation, no shell")
	return b.String()
}

// diagnoseAmbiguous builds a diagnostic suffix for an ambiguous edit where
// old_string matched multiple times. It shows one match location with
// surrounding context to help the caller add enough context for uniqueness.
//
// The matched string is truncated if it exceeds 80 bytes to avoid bloating
// the error message when old_string is very large.
func diagnoseAmbiguous(content, old string, count int) string {
	idx := strings.Index(content, old)
	if idx < 0 {
		return ""
	}
	start := idx - 20
	if start < 0 {
		start = 0
	}
	end := idx + len(old) + 20
	if end > len(content) {
		end = len(content)
	}

	// Build the context with the match highlighted.
	prefix := content[start:idx]
	match := content[idx : idx+len(old)]
	suffix := content[idx+len(old) : end]

	// Truncate the matched portion if it's very large (e.g. a 10KB block).
	// We show only the first 40 and last 40 bytes with a marker between.
	const maxMatch = 80
	if len(match) > maxMatch {
		match = match[:40] + "…" + match[len(match)-40:]
	}

	ctx := prefix + ">>>" + match + "<<<" + suffix
	return fmt.Sprintf("\n  context around first match (offset %d, total matches: %d): %q", idx, count, ctx)
}

// lineEndingHint returns a human-readable description of line-ending
// mismatch between the search text and the file content, so the caller
// doesn't have to eyeball \r\n vs \n in the raw previews.
func lineEndingHint(content, old string) string {
	fCRLF := strings.Contains(content, "\r\n")
	fCR := strings.Contains(content, "\r") && !fCRLF
	oCRLF := strings.Contains(old, "\r\n")
	oCR := strings.Contains(old, "\r") && !oCRLF
	oLF := strings.Contains(old, "\n") && !oCRLF && !oCR
	fLF := strings.Contains(content, "\n") && !fCRLF && !fCR

	// Only surface a hint when both contain newlines and they differ.
	switch {
	case oLF && fCRLF:
		return "line endings differ: search text uses LF, but the file uses CRLF"
	case oCRLF && fLF:
		return "line endings differ: search text uses CRLF, but the file uses LF"
	case oCR && !fCR && !fCRLF:
		return "line endings differ: search text uses CR, but the file does not"
	default:
		return ""
	}
}
