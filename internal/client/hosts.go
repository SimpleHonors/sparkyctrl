package client

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// LoadHosts parses a minimal `name = "addr"` file. Blank lines and lines
// beginning with # are ignored. A missing file yields an empty map (not an
// error) so raw host:port addresses still work.
func LoadHosts(path string) (map[string]string, error) {
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
