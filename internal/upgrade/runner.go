package upgrade

import (
	"fmt"
	"os/exec"
	"strings"
)

// Runner runs an external command and returns combined stdout.
type Runner interface {
	Run(name string, args ...string) (string, error)
}

// OSRunner is the production Runner.
type OSRunner struct{}

func (OSRunner) Run(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}

// CheckVersion runs `<binPath> version` and confirms it reports wantVersion.
func CheckVersion(r Runner, binPath, wantVersion string) error {
	out, err := r.Run(binPath, "version")
	if err != nil {
		return fmt.Errorf("new binary failed to run: %w", err)
	}
	got := NormalizeVersion(strings.TrimSpace(out))
	if got != NormalizeVersion(wantVersion) {
		return fmt.Errorf("new binary reports version %q, expected %q", got, wantVersion)
	}
	return nil
}
