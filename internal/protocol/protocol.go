// Package protocol defines the wire contract shared by the sparkyctrl
// worker (server) and the CLI client. It is the single source of truth
// for request/response shapes.
package protocol

const (
	// Version is the build/protocol version reported by /v1/info.
	Version = "0.1.12"
	// DefaultPort is the worker's default listen port.
	DefaultPort = 7766
	// TokenHeader carries the optional shared secret.
	TokenHeader = "X-Sparkyctrl-Token"
	// DefaultTimeoutSec applies when a request omits timeout_sec.
	DefaultTimeoutSec = 60
)

// ExecRequest runs argv directly via the OS exec call. No shell is
// involved, so quoting/glob/redirect characters are passed literally.
type ExecRequest struct {
	Argv       []string          `json:"argv"`
	Cwd        string            `json:"cwd,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	Stdin      string            `json:"stdin,omitempty"`
	TimeoutSec int               `json:"timeout_sec,omitempty"`
}

// ShellRequest runs a script through a real shell. Use only when shell
// features (pipes, globs, redirects) are genuinely required.
type ShellRequest struct {
	Script     string `json:"script"`
	Shell      string `json:"shell,omitempty"` // "sh" (default) | "powershell"
	Cwd        string `json:"cwd,omitempty"`
	TimeoutSec int    `json:"timeout_sec,omitempty"`
}

// ExecResponse is returned by both exec and shell. Error is set only for
// setup/transport failures (e.g. binary not found), not for a non-zero
// exit of the command itself — that is reported via ExitCode.
type ExecResponse struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

// LsEntry is one directory entry.
type LsEntry struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Mode  string `json:"mode"`
	IsDir bool   `json:"is_dir"`
	Mtime int64  `json:"mtime"` // unix seconds
}

// LsResponse is the result of a directory listing.
type LsResponse struct {
	Entries []LsEntry `json:"entries"`
}

// LsRequest asks for a directory listing.
type LsRequest struct {
	Path string `json:"path"`
}

// EditRequest replaces a literal substring in a remote file. OldString is
// matched byte-for-byte (no regex, no shell, no whitespace normalisation). By
// default it must occur exactly once; set ReplaceAll to change every
// occurrence. Zero matches always refuses.
type EditRequest struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

// EditResponse is returned by edit. Error is set (with a non-2xx status) for
// any refusal: no match, ambiguous match (when ReplaceAll is unset), fence
// violation, or write failure. On success Error is "".
type EditResponse struct {
	Replaced     int    `json:"replaced"`      // number of occurrences changed
	BytesWritten int64  `json:"bytes_written"` // size of the file after the edit
	Error        string `json:"error,omitempty"`
}

// Info is the /v1/info payload.
type Info struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Hostname string `json:"hostname"`
	Version  string `json:"version"`
	Fence    string `json:"fence"` // "" when no fence is configured
}

// ErrorResponse is returned with non-2xx HTTP status for request errors.
type ErrorResponse struct {
	Error string `json:"error"`
}
