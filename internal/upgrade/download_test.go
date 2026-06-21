package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// signStringWithTestKey signs payload (in memory) with the same minisign key
// the test has installed via SetEmbeddedPublicKeyForTest. Returns the .minisig
// text — same wire format as minisign -S produces.
//
// Tests need this so they can build a Release that DownloadAndVerify will
// accept end-to-end without holding the production private key.
func signStringWithTestKey(t *testing.T, keyFile string, payload []byte) string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "payload")
	if err := os.WriteFile(file, payload, 0600); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	cmd := exec.Command("minisign", "-S", "-s", keyFile, "-m", file, "-W")
	cmd.Stdin = strings.NewReader("\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("minisign -S: %v\n%s", err, out)
	}
	sigBytes, err := os.ReadFile(file + ".minisig")
	if err != nil {
		t.Fatalf("read .minisig: %v", err)
	}
	return string(sigBytes)
}

// installTestKey generates a fresh minisign keypair in a temp dir and
// installs the public half via SetEmbeddedPublicKeyForTest. Returns the
// private-key path (for signing test artifacts) and the restore function
// (call via t.Cleanup).
func installTestKey(t *testing.T) (string, func()) {
	t.Helper()
	if _, err := exec.LookPath("minisign"); err != nil {
		t.Skip("minisign not installed; skipping test that requires real signature roundtrip")
	}
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "test.key")
	pubFile := filepath.Join(dir, "test.pub")
	cmd := exec.Command("minisign", "-G", "-p", pubFile, "-s", keyFile, "-W")
	cmd.Stdin = strings.NewReader("\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("minisign -G: %v\n%s", err, out)
	}
	pubBytes, err := os.ReadFile(pubFile)
	if err != nil {
		t.Fatalf("read pub: %v", err)
	}
	pubLine := strings.SplitN(string(pubBytes), "\n", 2)[1]
	pubLine = strings.TrimSpace(pubLine)
	restore, err := SetEmbeddedPublicKeyForTest(pubLine)
	if err != nil {
		t.Fatalf("SetEmbeddedPublicKeyForTest: %v", err)
	}
	return keyFile, restore
}

func TestDownloadAndVerify(t *testing.T) {
	keyFile, restore := installTestKey(t)
	t.Cleanup(restore)

	payload := []byte("fake-binary-bytes")
	sum := sha256.Sum256(payload)
	sumsText := fmt.Sprintf("%s  sparkyctrl-linux-amd64\n", hex.EncodeToString(sum[:]))

	// Sign both the binary and SHA256SUMS with the test key.
	binSig := signStringWithTestKey(t, keyFile, payload)
	sumsSig := signStringWithTestKey(t, keyFile, []byte(sumsText))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bin":
			w.Write(payload)
		case "/bin.minisig":
			w.Write([]byte(binSig))
		case "/sums":
			w.Write([]byte(sumsText))
		case "/sums.minisig":
			w.Write([]byte(sumsSig))
		default:
			http.Error(w, "nope", 404)
		}
	}))
	defer srv.Close()

	rel := Release{
		AssetName:      "sparkyctrl-linux-amd64",
		AssetURL:       srv.URL + "/bin",
		AssetSigURL:    srv.URL + "/bin.minisig",
		ChecksumURL:    srv.URL + "/sums",
		ChecksumSigURL: srv.URL + "/sums.minisig",
	}
	tmp, err := DownloadAndVerify(srv.Client(), rel, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(tmp)
	if string(got) != string(payload) {
		t.Fatalf("content mismatch")
	}
}

func TestDownloadAndVerifyBadChecksum(t *testing.T) {
	keyFile, restore := installTestKey(t)
	t.Cleanup(restore)

	payload := []byte("payload")
	// Real signature over payload, but SHA256SUMS line is bogus — checksum
	// verify must fail after sig verify passes.
	binSig := signStringWithTestKey(t, keyFile, payload)
	bogusSums := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef  sparkyctrl-linux-amd64\n"
	sumsSig := signStringWithTestKey(t, keyFile, []byte(bogusSums))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bin":
			w.Write(payload)
		case "/bin.minisig":
			w.Write([]byte(binSig))
		case "/sums":
			w.Write([]byte(bogusSums))
		case "/sums.minisig":
			w.Write([]byte(sumsSig))
		default:
			http.Error(w, "nope", 404)
		}
	}))
	defer srv.Close()
	rel := Release{
		AssetName:      "sparkyctrl-linux-amd64",
		AssetURL:       srv.URL + "/bin",
		AssetSigURL:    srv.URL + "/bin.minisig",
		ChecksumURL:    srv.URL + "/sums",
		ChecksumSigURL: srv.URL + "/sums.minisig",
	}
	_, err := DownloadAndVerify(srv.Client(), rel, t.TempDir())
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch error, got: %v", err)
	}
}

func TestDownloadAndVerifyMissingSignature(t *testing.T) {
	payload := []byte("payload")
	sum := sha256.Sum256(payload)
	sumsText := fmt.Sprintf("%s  sparkyctrl-linux-amd64\n", hex.EncodeToString(sum[:]))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Deliberately no /bin.minisig — the release is unsigned.
		switch r.URL.Path {
		case "/bin":
			w.Write(payload)
		case "/sums":
			w.Write([]byte(sumsText))
		default:
			http.Error(w, "nope", 404)
		}
	}))
	defer srv.Close()
	rel := Release{
		AssetName:      "sparkyctrl-linux-amd64",
		AssetURL:       srv.URL + "/bin",
		ChecksumURL:    srv.URL + "/sums",
	}
	_, err := DownloadAndVerify(srv.Client(), rel, t.TempDir())
	if err == nil {
		t.Fatal("expected refusal for unsigned release")
	}
	if !strings.Contains(err.Error(), "no signature") {
		t.Fatalf("expected 'no signature' error, got: %v", err)
	}
}

func TestDownloadAndVerifyTamperedBinary(t *testing.T) {
	keyFile, restore := installTestKey(t)
	t.Cleanup(restore)

	original := []byte("original-binary-bytes")
	tampered := []byte("tampered-binary-bytes")
	// Sign the ORIGINAL bytes — the server delivers TAMPERED bytes. The sig
	// verify must catch this.
	binSig := signStringWithTestKey(t, keyFile, original)
	sum := sha256.Sum256(tampered)
	sumsText := fmt.Sprintf("%s  sparkyctrl-linux-amd64\n", hex.EncodeToString(sum[:]))
	sumsSig := signStringWithTestKey(t, keyFile, []byte(sumsText))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bin":
			w.Write(tampered)
		case "/bin.minisig":
			w.Write([]byte(binSig))
		case "/sums":
			w.Write([]byte(sumsText))
		case "/sums.minisig":
			w.Write([]byte(sumsSig))
		default:
			http.Error(w, "nope", 404)
		}
	}))
	defer srv.Close()
	rel := Release{
		AssetName:      "sparkyctrl-linux-amd64",
		AssetURL:       srv.URL + "/bin",
		AssetSigURL:    srv.URL + "/bin.minisig",
		ChecksumURL:    srv.URL + "/sums",
		ChecksumSigURL: srv.URL + "/sums.minisig",
	}
	_, err := DownloadAndVerify(srv.Client(), rel, t.TempDir())
	if err == nil {
		t.Fatal("expected signature mismatch error")
	}
	if !strings.Contains(err.Error(), "signature") {
		t.Fatalf("expected signature error, got: %v", err)
	}
}

func TestDownloadAndVerifyTamperedSums(t *testing.T) {
	keyFile, restore := installTestKey(t)
	t.Cleanup(restore)

	payload := []byte("payload")
	binSig := signStringWithTestKey(t, keyFile, payload)
	// Sign a legitimate SHA256SUMS, then deliver a tampered one with matching
	// hash for the payload — the SHA256SUMS sig verify must catch the swap.
	realSum := sha256.Sum256(payload)
	realSums := fmt.Sprintf("%s  sparkyctrl-linux-amd64\n", hex.EncodeToString(realSum[:]))
	sumsSig := signStringWithTestKey(t, keyFile, []byte(realSums))
	tamperedSums := realSums // start identical, then swap to invalid sig target

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bin":
			w.Write(payload)
		case "/bin.minisig":
			w.Write([]byte(binSig))
		case "/sums":
			w.Write([]byte(tamperedSums + "garbage line that breaks the sig\n"))
		case "/sums.minisig":
			w.Write([]byte(sumsSig))
		default:
			http.Error(w, "nope", 404)
		}
	}))
	defer srv.Close()
	rel := Release{
		AssetName:      "sparkyctrl-linux-amd64",
		AssetURL:       srv.URL + "/bin",
		AssetSigURL:    srv.URL + "/bin.minisig",
		ChecksumURL:    srv.URL + "/sums",
		ChecksumSigURL: srv.URL + "/sums.minisig",
	}
	_, err := DownloadAndVerify(srv.Client(), rel, t.TempDir())
	if err == nil {
		t.Fatal("expected SHA256SUMS signature mismatch")
	}
	if !strings.Contains(err.Error(), "SHA256SUMS") {
		t.Fatalf("expected SHA256SUMS sig error, got: %v", err)
	}
}

func TestDownloadAndVerifyForeignKey(t *testing.T) {
	// Sign with key A but install key B as the embedded pin — signature
	// verify must reject.
	keyA, _ := installTestKey(t)
	// Generate a SECOND key and install it as the "pinned" key.
	if _, err := exec.LookPath("minisign"); err != nil {
		t.Skip("minisign not installed")
	}
	dir := t.TempDir()
	_, pubFile := filepath.Join(dir, "b.key"), filepath.Join(dir, "b.pub")
	cmd := exec.Command("minisign", "-G", "-p", pubFile, "-s", filepath.Join(dir, "b.key"), "-W")
	cmd.Stdin = strings.NewReader("\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("minisign -G (b): %v\n%s", err, out)
	}
	pubBytes, err := os.ReadFile(pubFile)
	if err != nil {
		t.Fatal(err)
	}
	pubLine := strings.TrimSpace(strings.SplitN(string(pubBytes), "\n", 2)[1])
	restoreB, err := SetEmbeddedPublicKeyForTest(pubLine)
	if err != nil {
		t.Fatal(err)
	}
	defer restoreB()

	payload := []byte("payload")
	binSig := signStringWithTestKey(t, keyA, payload) // signed with key A, verified against key B
	sum := sha256.Sum256(payload)
	sumsText := fmt.Sprintf("%s  sparkyctrl-linux-amd64\n", hex.EncodeToString(sum[:]))
	sumsSig := signStringWithTestKey(t, keyA, []byte(sumsText))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bin":
			w.Write(payload)
		case "/bin.minisig":
			w.Write([]byte(binSig))
		case "/sums":
			w.Write([]byte(sumsText))
		case "/sums.minisig":
			w.Write([]byte(sumsSig))
		default:
			http.Error(w, "nope", 404)
		}
	}))
	defer srv.Close()
	rel := Release{
		AssetName:      "sparkyctrl-linux-amd64",
		AssetURL:       srv.URL + "/bin",
		AssetSigURL:    srv.URL + "/bin.minisig",
		ChecksumURL:    srv.URL + "/sums",
		ChecksumSigURL: srv.URL + "/sums.minisig",
	}
	_, err = DownloadAndVerify(srv.Client(), rel, t.TempDir())
	if err == nil {
		t.Fatal("expected foreign-key rejection")
	}
}
