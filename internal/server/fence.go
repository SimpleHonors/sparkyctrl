package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// maxSymlinkHops bounds symlink-chain resolution to avoid loops.
const maxSymlinkHops = 40

// ResolveFenced validates that an absolute path stays within fence (after
// resolving `..` and symlinks) and returns the real resolved path. When
// fence is "" the path is returned resolved but unrestricted.
func ResolveFenced(fence, p string) (string, error) {
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("path must be absolute: %q", p)
	}
	clean := filepath.Clean(p)

	// Bug #18 fix: if the leaf itself is a symlink (possibly dangling, possibly
	// a chain), os.OpenFile(O_CREATE) on the write path follows it. The walk-up
	// below only resolves the *parent*, so a dangling symlink whose target is
	// outside the fence would slip through. Resolve the leaf's symlink target(s)
	// here — including not-yet-existing targets and chains — so the fence check
	// below applies to where the write would actually land.
	for i := 0; ; i++ {
		if i > maxSymlinkHops {
			return "", fmt.Errorf("too many symlink hops resolving %q", p)
		}
		fi, lerr := os.Lstat(clean)
		if lerr != nil || fi.Mode()&os.ModeSymlink == 0 {
			break
		}
		dest, rerr := os.Readlink(clean)
		if rerr != nil {
			return "", fmt.Errorf("readlink %q: %w", clean, rerr)
		}
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(filepath.Dir(clean), dest)
		}
		clean = filepath.Clean(dest)
	}

	// Resolve symlinks on the deepest existing ancestor so that writing a
	// not-yet-existing file still gets a real, traversal-free path.
	real := clean
	if r, err := filepath.EvalSymlinks(clean); err == nil {
		real = r
	} else {
		// Walk up to find the deepest ancestor that exists, then reconstruct
		// the remaining path suffix beneath it.
		cur := clean
		suffix := ""
		for {
			parent := filepath.Dir(cur)
			if parent == cur {
				// hit filesystem root with no existing ancestor
				return "", fmt.Errorf("resolve parent of %q: path has no existing ancestor", p)
			}
			base := filepath.Base(cur)
			if suffix == "" {
				suffix = base
			} else {
				suffix = filepath.Join(base, suffix)
			}
			if rdir, derr := filepath.EvalSymlinks(parent); derr == nil {
				real = filepath.Join(rdir, suffix)
				break
			}
			cur = parent
		}
	}

	if fence == "" {
		return real, nil
	}
	rfence, err := filepath.EvalSymlinks(fence)
	if err != nil {
		return "", fmt.Errorf("resolve fence %q: %w", fence, err)
	}
	if real != rfence && !strings.HasPrefix(real, rfence+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q is outside fence %q", p, fence)
	}
	return real, nil
}
