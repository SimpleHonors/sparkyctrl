package upgrade

import "strings"

// AssetName returns the release asset filename for a GOOS/GOARCH pair.
func AssetName(goos, goarch string) string {
	name := "sparkyctrl-" + goos + "-" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// NormalizeVersion strips a leading "v" so tags and binary versions compare equal.
func NormalizeVersion(s string) string {
	return strings.TrimPrefix(s, "v")
}
