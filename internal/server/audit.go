package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// Auditor appends one JSON object per request to a log file. Concurrency-safe.
//
// Tamper evidence: when constructed with a non-nil key, every record carries
// an HMAC-SHA256 over its canonical bytes plus the previous record's hash,
// chaining the log end-to-end. Deletions, edits, and reorderings are
// detectable with `Verify`; the chain only proves tampering happened, not
// what the original content was. Off-box shipping (so an attacker with
// shell on the worker can't suppress the sink) is a separate, larger feature.
type Auditor struct {
	mu       sync.Mutex
	f        *os.File
	key      []byte        // nil disables chaining (legacy format)
	prevHash [32]byte      // hash of the last record's body; zero on a fresh log
}

// NewAuditor opens (or creates) the audit log for appending. A path of ""
// returns a nil Auditor that silently no-ops (logging disabled).
//
// When key is nil the auditor writes the legacy un-chained JSONL format
// (compatibility with readers written before the chain was added). When key
// is non-nil every record gets prev_hash + hmac fields and `Verify` can
// prove the log was not modified between writes.
func NewAuditor(path string, key []byte) (*Auditor, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	a := &Auditor{f: f, key: key}
	if key != nil {
		// Resume the chain from the tail of any existing log so reopens
		// don't break verification.
		last, err := lastRecordHash(path)
		if err != nil {
			f.Close()
			return nil, err
		}
		a.prevHash = last
	}
	return a, nil
}

// Log writes one timestamped, tamper-evident record. Safe to call on a nil
// Auditor.
func (a *Auditor) Log(op string, fields map[string]any) {
	if a == nil {
		return
	}
	rec := map[string]any{"ts": time.Now().UTC().Format(time.RFC3339), "op": op}
	for k, v := range fields {
		rec[k] = v
	}
	body, err := json.Marshal(rec)
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	var line []byte
	if a.key == nil {
		// Legacy un-chained format: just the JSON object + newline.
		line = append(body, '\n')
	} else {
		// Chained format: include prev_hash, then HMAC over the body.
		rec["prev_hash"] = hex.EncodeToString(a.prevHash[:])
		rec["hmac"] = "" // placeholder so json.Marshal keeps the field order stable
		chained, err := json.Marshal(rec)
		if err != nil {
			return
		}
		mac := hmac.New(sha256.New, a.key)
		mac.Write(chained)
		sum := mac.Sum(nil)
		// Splice the hex HMAC into the placeholder we reserved above.
		chained, err = spliceHMAC(chained, hex.EncodeToString(sum))
		if err != nil {
			return
		}
		line = append(chained, '\n')
		a.prevHash = sha256.Sum256(chained)
	}
	a.f.Write(line)
}

// Close closes the underlying file. Safe on a nil Auditor.
func (a *Auditor) Close() error {
	if a == nil {
		return nil
	}
	return a.f.Close()
}

// GenerateAuditKey returns a fresh 32-byte HMAC key as hex. Operators can
// pass the output to `--audit-key` or persist it to `<log>.key` for reload.
func GenerateAuditKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// lastRecordHash returns sha256 of the final record's bytes (without trailing
// newline) in path, or the zero hash if the file is empty / missing. Used to
// resume the chain when the worker restarts.
func lastRecordHash(path string) ([32]byte, error) {
	var zero [32]byte
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return zero, nil
		}
		return zero, err
	}
	if len(data) == 0 {
		return zero, nil
	}
	// Strip trailing newline(s).
	for len(data) > 0 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}
	if len(data) == 0 {
		return zero, nil
	}
	// Find the start of the last line.
	start := len(data) - 1
	for start > 0 && data[start-1] != '\n' {
		start--
	}
	return sha256.Sum256(data[start:]), nil
}

// spliceHMAC rewrites the empty `hmac` placeholder in chained with the given
// hex digest, preserving the JSON shape. Returns an error if the placeholder
// is missing (which means the caller's Marshal ordering changed).
func spliceHMAC(chained []byte, hmacHex string) ([]byte, error) {
	marker := []byte(`"hmac":""`)
	i := indexOf(chained, marker)
	if i < 0 {
		return nil, fmt.Errorf("audit: hmac placeholder missing from chained record")
	}
	out := make([]byte, 0, len(chained)+len(hmacHex))
	out = append(out, chained[:i]...)
	out = append(out, []byte(`"hmac":"`+hmacHex+`"`)...)
	out = append(out, chained[i+len(marker):]...)
	return out, nil
}

func indexOf(haystack, needle []byte) int {
	if len(needle) == 0 {
		return 0
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if hmac.Equal(haystack[i:i+len(needle)], needle) {
			return i
		}
	}
	return -1
}

// Verify walks the audit log at path under key and returns the 1-based line
// number of the first tampered record, or 0 if the chain is intact. Records
// without prev_hash/hmac fields are reported as legacy (line = -1, nil err)
// so callers can distinguish "no chain was ever started" from "chain broke".
//
// Returns ErrNoAuditKey if key is empty (caller didn't supply one).
func Verify(path string, key []byte) (int, error) {
	if len(key) == 0 {
		return 0, ErrNoAuditKey
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var (
		prevHash [32]byte
		lineNo   int
	)
	for _, line := range splitLines(data) {
		lineNo++
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			return lineNo, fmt.Errorf("line %d: not valid JSON: %w", lineNo, err)
		}
		gotPrev, _ := rec["prev_hash"].(string)
		gotMAC, _ := rec["hmac"].(string)
		if gotPrev == "" || gotMAC == "" {
			// Legacy record — can't verify anything past this point.
			return -1, nil
		}
		prevBytes, err := hex.DecodeString(gotPrev)
		if err != nil || len(prevBytes) != 32 {
			return lineNo, fmt.Errorf("line %d: malformed prev_hash", lineNo)
		}
		if [32]byte(prevBytes) != prevHash {
			return lineNo, fmt.Errorf("line %d: prev_hash mismatch", lineNo)
		}
		// Recompute the HMAC over the canonical body (line minus the trailing
		// newline) but with the hmac field cleared, exactly like Log() writes.
		body, err := canonicalBodyForVerify(line)
		if err != nil {
			return lineNo, err
		}
		mac := hmac.New(sha256.New, key)
		mac.Write(body)
		wantMAC := hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(wantMAC), []byte(gotMAC)) {
			return lineNo, fmt.Errorf("line %d: hmac mismatch", lineNo)
		}
		prevHash = sha256.Sum256(line)
	}
	return 0, nil
}

// ErrNoAuditKey is returned by Verify when no key was supplied.
var ErrNoAuditKey = errors.New("audit: verify requires a non-empty key")

// splitLines yields each newline-terminated record without the trailing \n.
// Empty trailing fragments are skipped.
func splitLines(data []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				out = append(out, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		out = append(out, data[start:])
	}
	return out
}

// canonicalBodyForVerify rebuilds the bytes Log() HMAC'd: the line as written
// but with the `hmac` field replaced by the empty placeholder Log() reserved.
// This avoids re-marshalling the parsed map (which could reorder keys and
// invalidate the HMAC).
func canonicalBodyForVerify(line []byte) ([]byte, error) {
	const open = `"hmac":`
	i := indexOf(line, []byte(open))
	if i < 0 {
		return nil, fmt.Errorf("audit: hmac field missing")
	}
	// Find the closing quote of the value (skip escaped quotes).
	j := i + len(open)
	if j >= len(line) || line[j] != '"' {
		return nil, fmt.Errorf("audit: hmac value not a string")
	}
	j++
	for j < len(line) {
		if line[j] == '\\' && j+1 < len(line) {
			j += 2
			continue
		}
		if line[j] == '"' {
			break
		}
		j++
	}
	if j >= len(line) {
		return nil, fmt.Errorf("audit: hmac value unterminated")
	}
	out := make([]byte, 0, len(line))
	out = append(out, line[:i]...)
	out = append(out, []byte(`"hmac":""`)...)
	out = append(out, line[j+1:]...)
	return out, nil
}
