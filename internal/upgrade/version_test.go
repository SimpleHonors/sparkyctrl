package upgrade

import "testing"

func TestAssetName(t *testing.T) {
	cases := []struct{ goos, goarch, want string }{
		{"linux", "amd64", "sparkyctrl-linux-amd64"},
		{"linux", "arm64", "sparkyctrl-linux-arm64"},
		{"windows", "amd64", "sparkyctrl-windows-amd64.exe"},
	}
	for _, c := range cases {
		if got := AssetName(c.goos, c.goarch); got != c.want {
			t.Errorf("AssetName(%q,%q)=%q want %q", c.goos, c.goarch, got, c.want)
		}
	}
}

func TestNormalizeVersion(t *testing.T) {
	for in, want := range map[string]string{"v0.1.14": "0.1.14", "0.1.14": "0.1.14", "": ""} {
		if got := NormalizeVersion(in); got != want {
			t.Errorf("NormalizeVersion(%q)=%q want %q", in, got, want)
		}
	}
}
