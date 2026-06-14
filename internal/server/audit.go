package server

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// TODO(tamper-resistance, task #16/#19): the audit log is still trivially
// tamperable — an attacker with exec/shell access can `truncate -s0` or rewrite
// this file via the same channel it audits. Harden by HMAC-chaining each record
// (so deletions/edits are detectable) and/or offloading records to a remote
// sink (syslog/append-only collector) the worker cannot reach to rewrite.
// Deliberately out of scope for this focused change; see report.

// Auditor appends one JSON object per request to a log file. Concurrency-safe.
type Auditor struct {
	mu sync.Mutex
	f  *os.File
}

// NewAuditor opens (or creates) the audit log for appending. A path of ""
// returns a nil Auditor that silently no-ops (logging disabled).
func NewAuditor(path string) (*Auditor, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	return &Auditor{f: f}, nil
}

// Log writes one timestamped record. Safe to call on a nil Auditor.
func (a *Auditor) Log(op string, fields map[string]any) {
	if a == nil {
		return
	}
	rec := map[string]any{"ts": time.Now().UTC().Format(time.RFC3339), "op": op}
	for k, v := range fields {
		rec[k] = v
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.f.Write(append(b, '\n'))
}

// Close closes the underlying file. Safe on a nil Auditor.
func (a *Auditor) Close() error {
	if a == nil {
		return nil
	}
	return a.f.Close()
}
