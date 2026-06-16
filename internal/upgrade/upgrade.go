package upgrade

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Options are the user-facing knobs.
type Options struct {
	Repo          string
	TargetVersion string
	Service       string
	CheckOnly     bool
	NoRestart     bool
}

// Deps are injected so the orchestrator is fully testable.
type Deps struct {
	Client         *http.Client
	APIBase        string
	Runner         Runner
	GOOS, GOARCH   string
	CurrentBinary  string
	CurrentVersion string
	Exists         func(string) bool
	Stdout         io.Writer
}

// Run performs the upgrade end to end.
func Run(d Deps, o Options) error {
	repo := o.Repo
	if repo == "" {
		repo = DefaultRepo
	}
	service := o.Service
	if service == "" {
		service = "sparkyctrl"
	}

	rel, err := ResolveTarget(d.Client, d.APIBase, repo, o.TargetVersion, d.GOOS, d.GOARCH)
	if err != nil {
		return err
	}

	if o.TargetVersion == "" && NormalizeVersion(d.CurrentVersion) == rel.Version {
		fmt.Fprintf(d.Stdout, "already on %s (latest)\n", rel.Version)
		return nil
	}
	if o.CheckOnly {
		fmt.Fprintf(d.Stdout, "current %s, available %s\n", NormalizeVersion(d.CurrentVersion), rel.Version)
		return nil
	}

	tmp, err := DownloadAndVerify(d.Client, rel, filepath.Dir(d.CurrentBinary))
	if err != nil {
		return err
	}
	defer os.Remove(tmp)

	if err := CheckVersion(d.Runner, tmp, rel.Version); err != nil {
		return err
	}

	plat := Detect(d.GOOS, d.CurrentBinary, service, d.Runner, d.Exists)
	if err := plat.Place(tmp); err != nil {
		return fmt.Errorf("install new binary: %w", err)
	}
	fmt.Fprintf(d.Stdout, "installed %s via %s\n", rel.Version, plat.Name())

	if o.NoRestart {
		fmt.Fprintln(d.Stdout, "skipped restart (--no-restart); restart the worker to apply")
		return nil
	}
	restarted, err := plat.Restart()
	if err != nil {
		return fmt.Errorf("restart: %w", err)
	}
	if restarted {
		fmt.Fprintf(d.Stdout, "restarted worker on %s\n", rel.Version)
	} else {
		fmt.Fprintf(d.Stdout, "binary updated to %s — restart the worker yourself on this platform\n", rel.Version)
	}
	return nil
}
