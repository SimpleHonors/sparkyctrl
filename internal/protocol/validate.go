package protocol

import (
	"errors"
	"fmt"
	"strings"
)

// MaxTimeoutSec is the upper bound accepted for a request's timeout_sec.
// 24 hours is far beyond any legitimate command while still bounding the
// value so an absurd timeout cannot pin a worker goroutine effectively
// forever. Requests above this are rejected with a 400.
const MaxTimeoutSec = 24 * 60 * 60

// ValidateTimeoutSec checks a request's timeout_sec field.
//
// Semantics of 0: "use the server default" (DefaultTimeoutSec). This is the
// safer, clearer choice — omitting the field and sending 0 behave identically,
// and the worker never runs with an unbounded/zero timeout. Negative values are
// rejected (they are nonsensical and were previously silently treated as the
// default), and values above MaxTimeoutSec are rejected as absurd.
func ValidateTimeoutSec(sec int) error {
	if sec < 0 {
		return fmt.Errorf("timeout_sec must not be negative (got %d)", sec)
	}
	if sec > MaxTimeoutSec {
		return fmt.Errorf("timeout_sec exceeds maximum of %d seconds (got %d)", MaxTimeoutSec, sec)
	}
	return nil
}

// ValidateEnv rejects env keys that would corrupt the child process
// environment. An empty key, a key containing '=' (which splits the
// key/value), or a key containing a newline or NUL (a newline splits one
// entry into two env vars) is refused.
func ValidateEnv(env map[string]string) error {
	for k := range env {
		if k == "" {
			return errors.New("env key must not be empty")
		}
		if strings.ContainsAny(k, "=\n\x00") {
			return fmt.Errorf("env key %q contains an illegal character ('=', newline, or NUL)", k)
		}
	}
	return nil
}
