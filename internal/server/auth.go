package server

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

const (
	linuxTokenPath   = "/etc/sparkyctrl/token"
	windowsTokenPath = `C:\ProgramData\sparkyctrl\token.txt`
)

// DefaultTokenPath returns the worker token path for the current OS.
func DefaultTokenPath() string {
	return TokenPathForGOOS(runtime.GOOS)
}

// TokenPathForGOOS returns the worker token path for the supplied GOOS.
func TokenPathForGOOS(goos string) string {
	if goos == "windows" {
		return windowsTokenPath
	}
	return linuxTokenPath
}

// LoadTokenFile reads and trims the worker token from disk.
func LoadTokenFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(b))
	if token == "" {
		return "", fmt.Errorf("token file %s was empty", path)
	}
	return token, nil
}

// ResolveAuth returns the effective worker token plus a short mode/source label.
// noAuth wins over every other option.
func ResolveAuth(noAuth bool, override string) (token, mode, source string, err error) {
	if noAuth {
		return "", "disabled", "", nil
	}
	if override != "" {
		return override, "token-flag", "--token", nil
	}
	path := DefaultTokenPath()
	token, err = LoadTokenFile(path)
	if err != nil {
		return "", "", "", fmt.Errorf("load token from %s: %w", path, err)
	}
	return token, "token-file", path, nil
}
