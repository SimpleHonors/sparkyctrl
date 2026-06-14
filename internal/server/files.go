package server

import (
	"io"
	"os"
	"sort"

	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
)

// Handler holds worker configuration and implements the operations.
type Handler struct {
	Fence string
	Token string
	Audit *Auditor
}

// Upload writes the stream to dst (fence-checked), creating/truncating it.
func (h *Handler) Upload(dst string, mode os.FileMode, src io.Reader) error {
	real, err := ResolveFenced(h.Fence, dst)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(real, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, src)
	return err
}

// Download streams the fence-checked file at path to w.
func (h *Handler) Download(path string, w io.Writer) error {
	real, err := ResolveFenced(h.Fence, path)
	if err != nil {
		return err
	}
	f, err := os.Open(real)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

// Ls lists a fence-checked directory.
func (h *Handler) Ls(path string) (protocol.LsResponse, error) {
	real, err := ResolveFenced(h.Fence, path)
	if err != nil {
		return protocol.LsResponse{}, err
	}
	ents, err := os.ReadDir(real)
	if err != nil {
		return protocol.LsResponse{}, err
	}
	out := protocol.LsResponse{Entries: make([]protocol.LsEntry, 0, len(ents))}
	for _, e := range ents {
		entry := protocol.LsEntry{Name: e.Name(), IsDir: e.IsDir()}
		if info, ierr := e.Info(); ierr == nil {
			entry.Size = info.Size()
			entry.Mode = info.Mode().String()
			entry.Mtime = info.ModTime().Unix()
		} else {
			entry.Size = -1
			entry.Mode = "?"
			entry.Mtime = 0
		}
		out.Entries = append(out.Entries, entry)
	}
	sort.Slice(out.Entries, func(i, j int) bool { return out.Entries[i].Name < out.Entries[j].Name })
	return out, nil
}
