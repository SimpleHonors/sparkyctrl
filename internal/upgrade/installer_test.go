package upgrade

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestSystemdPlaceAndRestart(t *testing.T) {
	dir := t.TempDir()
	cur := filepath.Join(dir, "sparkyctrl")
	writeFile(t, cur, "OLD")
	newbin := filepath.Join(dir, "new")
	writeFile(t, newbin, "NEW")

	r := &fakeRunner{}
	p := &systemdPlatform{current: cur, service: "sparkyctrl", r: r}
	if err := p.Place(newbin); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(cur)
	if string(got) != "NEW" {
		t.Fatalf("current=%q want NEW", got)
	}
	bak, _ := os.ReadFile(cur + ".bak")
	if string(bak) != "OLD" {
		t.Fatalf("backup=%q want OLD", bak)
	}
	restarted, err := p.Restart()
	if err != nil || !restarted {
		t.Fatalf("restart=%v err=%v", restarted, err)
	}
	if got := r.last; len(got) < 3 || got[0] != "systemctl" || got[1] != "restart" || got[2] != "sparkyctrl" {
		t.Fatalf("ran %v", r.last)
	}
}

func TestUnraidPlaceWritesFlashAndKills(t *testing.T) {
	dir := t.TempDir()
	flash := filepath.Join(dir, "flash-sparkyctrl")
	writeFile(t, flash, "OLD")
	newbin := filepath.Join(dir, "new")
	writeFile(t, newbin, "NEW")

	r := &fakeRunner{}
	p := &unraidPlatform{r: r, flashPath: flash}
	if err := p.Place(newbin); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(flash)
	if string(got) != "NEW" {
		t.Fatalf("flash=%q want NEW", got)
	}
	restarted, err := p.Restart()
	if err != nil || !restarted {
		t.Fatalf("restart=%v err=%v", restarted, err)
	}
	if r.last[0] != "killall" || r.last[1] != "sparkyctrl" {
		t.Fatalf("ran %v", r.last)
	}
}

func TestWindowsPlaceRenamesAndRestartsTask(t *testing.T) {
	dir := t.TempDir()
	cur := filepath.Join(dir, "sparkyctrl.exe")
	writeFile(t, cur, "OLD")
	newbin := filepath.Join(dir, "new")
	writeFile(t, newbin, "NEW")

	r := &fakeRunner{}
	p := &windowsPlatform{current: cur, task: "sparkyctrl", r: r}
	if err := p.Place(newbin); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(cur)
	if string(got) != "NEW" {
		t.Fatalf("current=%q want NEW", got)
	}
	old, _ := os.ReadFile(cur + ".old")
	if string(old) != "OLD" {
		t.Fatalf("old=%q want OLD", old)
	}
	restarted, err := p.Restart()
	if err != nil || !restarted {
		t.Fatalf("restart=%v err=%v", restarted, err)
	}
	if r.last[0] != "powershell" {
		t.Fatalf("ran %v", r.last)
	}
}

func TestManualRestartReportsFalse(t *testing.T) {
	dir := t.TempDir()
	cur := filepath.Join(dir, "sparkyctrl")
	writeFile(t, cur, "OLD")
	newbin := filepath.Join(dir, "new")
	writeFile(t, newbin, "NEW")
	p := &manualPlatform{current: cur}
	if err := p.Place(newbin); err != nil {
		t.Fatal(err)
	}
	restarted, err := p.Restart()
	if err != nil || restarted {
		t.Fatalf("manual restart should be (false,nil); got (%v,%v)", restarted, err)
	}
}
