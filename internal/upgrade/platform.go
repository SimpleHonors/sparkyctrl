package upgrade

import (
	"os"
)

// flashDir is the Unraid flash location that survives reboot.
const flashDir = "/boot/config/sparkyctrl"

const flashBinary = flashDir + "/sparkyctrl"

// Platform installs a new binary and restarts the worker for one host type.
type Platform interface {
	Name() string
	Place(newBinary string) error
	Restart() (restarted bool, err error)
}

// Detect picks the installer for this host.
func Detect(goos, currentBinary, service string, r Runner, exists func(string) bool) Platform {
	if goos == "windows" {
		return &windowsPlatform{current: currentBinary, task: service, r: r}
	}
	if exists(flashDir) {
		return &unraidPlatform{r: r, flashPath: flashBinary}
	}
	if exists("/run/systemd/system") {
		return &systemdPlatform{current: currentBinary, service: service, r: r}
	}
	return &manualPlatform{current: currentBinary}
}

type systemdPlatform struct {
	current, service string
	r                Runner
}

func (p *systemdPlatform) Name() string { return "systemd" }
func (p *systemdPlatform) Place(newBinary string) error {
	if err := copyFile(p.current, p.current+".bak"); err != nil {
		return err
	}
	if err := os.Rename(newBinary, p.current); err != nil {
		return err
	}
	return os.Chmod(p.current, 0o755)
}
func (p *systemdPlatform) Restart() (bool, error) {
	_, err := p.r.Run("systemctl", "restart", p.service)
	return err == nil, err
}

type unraidPlatform struct {
	r         Runner
	flashPath string
}

func (p *unraidPlatform) Name() string { return "unraid" }
func (p *unraidPlatform) path() string {
	if p.flashPath != "" {
		return p.flashPath
	}
	return flashBinary
}
func (p *unraidPlatform) Place(newBinary string) error {
	_ = copyFile(p.path(), p.path()+".bak")
	return copyFile(newBinary, p.path())
}
func (p *unraidPlatform) Restart() (bool, error) {
	_, err := p.r.Run("killall", "sparkyctrl")
	return err == nil, err
}

type windowsPlatform struct {
	current, task string
	r             Runner
}

func (p *windowsPlatform) Name() string { return "windows" }
func (p *windowsPlatform) Place(newBinary string) error {
	if err := os.Rename(p.current, p.current+".old"); err != nil {
		return err
	}
	return os.Rename(newBinary, p.current)
}
func (p *windowsPlatform) Restart() (bool, error) {
	cmd := "Stop-ScheduledTask -TaskName '" + p.task + "' -ErrorAction SilentlyContinue; Start-ScheduledTask -TaskName '" + p.task + "'"
	_, err := p.r.Run("powershell", "-NoProfile", "-Command", cmd)
	return err == nil, err
}

type manualPlatform struct{ current string }

func (p *manualPlatform) Name() string { return "manual" }
func (p *manualPlatform) Place(newBinary string) error {
	if err := copyFile(p.current, p.current+".bak"); err != nil {
		return err
	}
	if err := os.Rename(newBinary, p.current); err != nil {
		return err
	}
	return os.Chmod(p.current, 0o755)
}
func (p *manualPlatform) Restart() (bool, error) { return false, nil }
