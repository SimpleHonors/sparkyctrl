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

// DownloadAndVerify downloads rel.AssetURL into destDir, verifies the
// cryptographic signature of both the asset and SHA256SUMS against the
// pinned release key (sigverify.MinisignPublicKey), then verifies the asset
// hash against the SHA256SUMS body, and returns the temp file path on
// success. Any failure removes the downloaded temp file and returns an error.
//
// Signature verification is the load-bearing security check here. SHA256SUMS
// alone proves only that the artifact matches what was hashed at release time
// — if an attacker compromises the release (or the sums file itself) the
// checksum verify still passes. A minisign signature binds the artifact to
// the pinned release key, which lives in the repo as deploy/sparkyctrl-release.pub
// and is also compiled into this binary (sigverify.MinisignPublicKey).
//
// The checksum verify is kept because it gives us a second independent path to
// trust the artifact, AND because it lets us surface a more specific error
// (mismatch vs. untrusted) when something goes wrong.
//
// Signature pre-requisites (REQUIRED for any release the upgrader will accept):
//   - rel.AssetSigURL  must resolve (asset's .minisig is present in the release)
//   - rel.ChecksumSigURL must resolve (SHA256SUMS.minisig is present)
//
// A release that lacks either signature is rejected outright. There is no
// opt-out flag — silent downgrade to checksum-only would defeat the whole
// point of the signature path (and that gap is what 87dd3eb1 / af4487db were
// opened to fix).
func DownloadAndVerify(client *http.Client, rel Release, destDir string) (string, error) {
	tmp := filepath.Join(destDir, rel.AssetName+".download")
	if err := fetch(client, rel.AssetURL, tmp); err != nil {
		return "", err
	}
	if rel.AssetSigURL == "" {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("release %s has no signature for %s (refusing unverified install)", rel.Tag, rel.AssetName)
	}
	assetSig, err := fetchString(client, rel.AssetSigURL)
	if err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("fetch %s signature: %w (refusing unverified install)", rel.AssetName, err)
	}
	if err := VerifyFileSignature(tmp, assetSig); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("signature verification failed for %s: %w", rel.AssetName, err)
	}

	if rel.ChecksumSigURL == "" {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("release %s has no SHA256SUMS.minisig (refusing unverified install)", rel.Tag)
	}
	sumsSig, err := fetchString(client, rel.ChecksumSigURL)
	if err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("fetch SHA256SUMS signature: %w (refusing unverified install)", err)
	}
	sums, err := fetchString(client, rel.ChecksumURL)
	if err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	// Verify the signature of SHA256SUMS BEFORE trusting any hash inside it.
	// Otherwise a checksum-valid attack is possible: attacker replaces both
	// SHA256SUMS and the binary with self-consistent garbage.
	if err := verifyStringSignature(sums, sumsSig); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("signature verification failed for SHA256SUMS: %w", err)
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
