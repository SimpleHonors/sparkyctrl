package upgrade

import "testing"

func existsSet(paths ...string) func(string) bool {
	set := map[string]bool{}
	for _, p := range paths {
		set[p] = true
	}
	return func(p string) bool { return set[p] }
}

func TestDetect(t *testing.T) {
	r := &fakeRunner{}
	cases := []struct {
		name   string
		goos   string
		exists func(string) bool
		want   string
	}{
		{"unraid", "linux", existsSet(flashDir), "unraid"},
		{"systemd", "linux", existsSet("/run/systemd/system"), "systemd"},
		{"manual-linux", "linux", existsSet(), "manual"},
		{"windows", "windows", existsSet(), "windows"},
	}
	for _, c := range cases {
		p := Detect(c.goos, "/usr/local/bin/sparkyctrl", "sparkyctrl", r, c.exists)
		if p.Name() != c.want {
			t.Errorf("%s: Detect=%q want %q", c.name, p.Name(), c.want)
		}
	}
}
