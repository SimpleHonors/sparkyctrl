package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
)

// Audit outcome values.
const (
	outcomeSuccess = "success"
	outcomeDenied  = "denied"
	outcomeError   = "error"
)

// MaxBodyBytes bounds the size of any request body the worker will read.
// argv, env, and stdin are all carried in the JSON body; without a cap a
// client could stream an unbounded multi-MB (or multi-GB) body and exhaust
// worker memory. 32 MiB is generous for legitimate stdin/argv while keeping
// the worker bounded. Over-cap bodies are rejected with HTTP 413.
const MaxBodyBytes int64 = 32 << 20

// srcIP extracts the remote source address from an HTTP request, stripping the
// port. Falls back to the raw RemoteAddr if it has no host:port form.
func srcIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// auditExec maps an ExecResponse onto an audit outcome + exit code. A non-zero
// exit code (including the -1 setup-failure sentinel) is recorded as an error.
func auditExec(r *http.Request, a *Auditor, op string, fields map[string]any, resp protocol.ExecResponse) {
	outcome := outcomeSuccess
	if resp.ExitCode != 0 {
		outcome = outcomeError
	}
	fields["src_ip"] = srcIP(r)
	fields["outcome"] = outcome
	fields["exit_code"] = resp.ExitCode
	a.Log(op, fields)
}

// Version mirrors the protocol version for the serve banner and elsewhere.
var Version = protocol.Version

// Mux builds the HTTP routes with the token middleware applied.
func (h *Handler) Mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/info", h.handleInfo)
	mux.HandleFunc("/v1/exec", h.handleExec)
	mux.HandleFunc("/v1/shell", h.handleShell)
	mux.HandleFunc("/v1/ls", h.handleLs)
	mux.HandleFunc("/v1/upload", h.handleUpload)
	mux.HandleFunc("/v1/download", h.handleDownload)
	mux.HandleFunc("/v1/edit", h.handleEdit)
	return h.withToken(mux)
}

func (h *Handler) withToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.Token != "" {
			got := []byte(r.Header.Get(protocol.TokenHeader))
			want := []byte(h.Token)
			if len(got) != len(want) || subtle.ConstantTimeCompare(got, want) != 1 {
				writeErr(w, http.StatusUnauthorized, "invalid or missing token")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, protocol.ErrorResponse{Error: msg})
}

// requirePOST enforces that an endpoint is only reached via POST. On any other
// method it sets the Allow header and writes a 405, returning false so the
// caller can stop. This keeps each handler from acting on unintended methods.
func requirePOST(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return false
	}
	return true
}

// decodeBody reads and JSON-decodes the request body into dst with two
// hardening measures:
//
//   - the body is wrapped in http.MaxBytesReader so an over-cap body is
//     rejected (HTTP 413) instead of being read unbounded into memory;
//   - decode failures return a generic "invalid request body" message to the
//     client. Go's default JSON errors leak internal type/field names (e.g.
//     "protocol.ExecRequest", "ExecRequest.argv of type []string"); we keep the
//     detail server-side (best-effort log) and never echo it to the caller.
//
// It returns false (and has already written the HTTP error) when decoding fails.
func (h *Handler) decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			h.Audit.Log("decode", map[string]any{"src_ip": srcIP(r), "outcome": outcomeDenied, "error": err.Error()})
			writeErr(w, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		// Preserve the real decode error in the audit log; return a generic
		// message so Go type/field names never reach the client.
		h.Audit.Log("decode", map[string]any{"src_ip": srcIP(r), "outcome": outcomeDenied, "error": err.Error()})
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func (h *Handler) handleInfo(w http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	writeJSON(w, http.StatusOK, protocol.Info{
		OS: runtime.GOOS, Arch: runtime.GOARCH, Hostname: host,
		Version: protocol.Version, Fence: h.Fence,
	})
}

func (h *Handler) handleExec(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req protocol.ExecRequest
	if !h.decodeBody(w, r, &req) {
		return
	}
	if err := protocol.ValidateTimeoutSec(req.TimeoutSec); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := protocol.ValidateEnv(req.Env); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	resp := RunExec(req)
	auditExec(r, h.Audit, "exec", map[string]any{"argv": req.Argv, "cwd": req.Cwd}, resp)
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleShell(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req protocol.ShellRequest
	if !h.decodeBody(w, r, &req) {
		return
	}
	if err := protocol.ValidateTimeoutSec(req.TimeoutSec); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	resp := RunShell(req)
	auditExec(r, h.Audit, "shell", map[string]any{"script": req.Script, "cwd": req.Cwd}, resp)
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleLs(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req protocol.LsRequest
	if !h.decodeBody(w, r, &req) {
		return
	}
	resp, err := h.Ls(req.Path)
	if err != nil {
		h.Audit.Log("ls", map[string]any{"path": req.Path, "src_ip": srcIP(r), "outcome": outcomeDenied, "error": err.Error()})
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	h.Audit.Log("ls", map[string]any{"path": req.Path, "src_ip": srcIP(r), "outcome": outcomeSuccess})
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	path := r.URL.Query().Get("path")
	mode := os.FileMode(0o644)
	if m := r.URL.Query().Get("mode"); m != "" {
		if parsed, err := strconv.ParseUint(m, 8, 32); err == nil {
			mode = os.FileMode(parsed) & 0o7777
		}
	}
	if err := h.Upload(path, mode, r.Body); err != nil {
		h.Audit.Log("upload", map[string]any{"path": path, "src_ip": srcIP(r), "outcome": outcomeDenied, "error": err.Error()})
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeErr(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	h.Audit.Log("upload", map[string]any{"path": path, "src_ip": srcIP(r), "outcome": outcomeSuccess})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleDownload(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	path := r.URL.Query().Get("path")
	w.Header().Set("Content-Type", "application/octet-stream")
	if err := h.Download(path, w); err != nil {
		h.Audit.Log("download", map[string]any{"path": path, "src_ip": srcIP(r), "outcome": outcomeDenied, "error": err.Error()})
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	h.Audit.Log("download", map[string]any{"path": path, "src_ip": srcIP(r), "outcome": outcomeSuccess})
}

func (h *Handler) handleEdit(w http.ResponseWriter, r *http.Request) {
	if !requirePOST(w, r) {
		return
	}
	var req protocol.EditRequest
	if !h.decodeBody(w, r, &req) {
		return
	}
	resp, err := h.Edit(req)
	if err != nil {
		h.Audit.Log("edit", map[string]any{"path": req.Path, "src_ip": srcIP(r), "outcome": outcomeDenied, "error": err.Error()})
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	h.Audit.Log("edit", map[string]any{"path": req.Path, "src_ip": srcIP(r), "outcome": outcomeSuccess, "replaced": resp.Replaced})
	writeJSON(w, http.StatusOK, resp)
}

// Serve starts the worker. addr is host:port (e.g. "0.0.0.0:7766" or a LAN IP).
func (h *Handler) Serve(addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h.Mux(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.ListenAndServe()
}
