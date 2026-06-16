package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// DownloadAndVerify downloads rel.AssetURL into destDir, verifies it against
// the SHA256SUMS at rel.ChecksumURL, and returns the temp file path on success.
func DownloadAndVerify(client *http.Client, rel Release, destDir string) (string, error) {
	tmp := filepath.Join(destDir, rel.AssetName+".download")
	if err := fetch(client, rel.AssetURL, tmp); err != nil {
		return "", err
	}
	sums, err := fetchString(client, rel.ChecksumURL)
	if err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := verifyChecksum(tmp, rel.AssetName, sums); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return tmp, nil
}

func fetch(client *http.Client, url, dest string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func fetchString(client *http.Client, url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: status %d", url, resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

// verifyChecksum confirms file at path matches the SHA256SUMS line for name.
func verifyChecksum(path, name, sums string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))

	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			if fields[0] == got {
				return nil
			}
			return fmt.Errorf("checksum mismatch for %s: got %s want %s", name, got, fields[0])
		}
	}
	return fmt.Errorf("no checksum entry for %s", name)
}
