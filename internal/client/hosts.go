package client

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// loadPairs parses a minimal `name = "value"` file. Blank lines and lines
// beginning with # are ignored, surrounding quotes are trimmed. A missing
// file yields an empty map (not an error).
func loadPairs(path string) (map[string]string, error) {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		val := strings.Trim(strings.TrimSpace(v), `"'`)
		if key != "" && val != "" {
			out[key] = val
		}
	}
	return out, sc.Err()
}

// LoadHosts parses the `name = "addr"` address book. A missing file yields an
// empty map (not an error) so raw host:port addresses still work.
func LoadHosts(path string) (map[string]string, error) {
	return loadPairs(path)
}

// LoadTokens parses the `name = "token"` per-host client token file.
func LoadTokens(path string) (map[string]string, error) {
	return loadPairs(path)
}

// Resolve turns a name (or a literal host:port) into a base URL.
func Resolve(hosts map[string]string, name string) (string, error) {
	addr, ok := hosts[name]
	if !ok {
		// Allow a raw host:port that isn't in the book.
		if strings.Contains(name, ":") {
			addr = name
		} else {
			return "", fmt.Errorf("unknown host %q (not in address book, and not a host:port)", name)
		}
	}
	return "http://" + addr, nil
}

// DefaultTokensPath returns the default client tokens file path, mirroring
// the resolution order: SPARKYCTRL_TOKENS env, ./tokens, ~/.sparkyctrl/tokens.
func DefaultTokensPath() string {
	if p := os.Getenv("SPARKYCTRL_TOKENS"); p != "" {
		return p
	}
	if _, err := os.Stat("tokens"); err == nil {
		return "tokens"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".sparkyctrl", "tokens")
}

// looseFilePerms reports whether a file mode grants any group/other access.
func looseFilePerms(mode os.FileMode) bool {
	return mode.Perm()&0o077 != 0
}

// ResolveToken returns the client auth token for the given host.
// Precedence: SPARKYCTRL_TOKEN env (override) > per-host entry in tokens file > "".
// Warns once to stderr if the tokens file is group/other-readable.
func ResolveToken(host string) string {
	if t := os.Getenv("SPARKYCTRL_TOKEN"); t != "" {
		return t
	}
	path := DefaultTokensPath()
	if info, err := os.Stat(path); err == nil && looseFilePerms(info.Mode()) {
		fmt.Fprintf(os.Stderr, "sparkyctrl: warning: %s is readable by others (chmod 600)\n", path)
	}
	tokens, err := LoadTokens(path)
	if err != nil {
		return ""
	}
	return tokens[host]
}
