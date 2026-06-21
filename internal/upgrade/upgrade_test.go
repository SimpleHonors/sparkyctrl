package upgrade

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newReleaseServer(t *testing.T, payload []byte) *httptest.Server {
	t.Helper()
	keyFile, restore := installTestKey(t)
	t.Cleanup(restore)
	sum := sha256.Sum256(payload)
	sumsText := fmt.Sprintf("%s  sparkyctrl-linux-amd64\n", hex.EncodeToString(sum[:]))
	binSig := signStringWithTestKey(t, keyFile, payload)
	sumsSig := signStringWithTestKey(t, keyFile, []byte(sumsText))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			fmt.Fprintf(w, `{"tag_name":"v0.1.14","assets":[
				{"name":"sparkyctrl-linux-amd64","browser_download_url":%q},
				{"name":"sparkyctrl-linux-amd64.minisig","browser_download_url":%q},
				{"name":"SHA256SUMS","browser_download_url":%q},
				{"name":"SHA256SUMS.minisig","browser_download_url":%q}]}`,
				"http://"+r.Host+"/bin",
				"http://"+r.Host+"/bin.minisig",
				"http://"+r.Host+"/sums",
				"http://"+r.Host+"/sums.minisig")
		case r.URL.Path == "/bin":
			w.Write(payload)
		case r.URL.Path == "/bin.minisig":
			w.Write([]byte(binSig))
		case r.URL.Path == "/sums":
			fmt.Fprintf(w, "%s  sparkyctrl-linux-amd64\n", hex.EncodeToString(sum[:]))
		case r.URL.Path == "/sums.minisig":
			w.Write([]byte(sumsSig))
		default:
			http.Error(w, "no", 404)
		}
	}))
	return srv
}

func TestRunUpgradesAndRestarts(t *testing.T) {
	payload := []byte("NEWBIN")
	srv := newReleaseServer(t, payload)
	defer srv.Close()

	dir := t.TempDir()
	cur := filepath.Join(dir, "sparkyctrl")
	writeFile(t, cur, "OLDBIN")

	r := &fakeRunner{out: map[string]string{filepath.Join(dir, "sparkyctrl-linux-amd64.download") + " version": "0.1.14\n"}}
	var out bytes.Buffer
	deps := Deps{
		Client: srv.Client(), APIBase: srv.URL, Runner: r,
		GOOS: "linux", GOARCH: "amd64",
		CurrentBinary: cur, CurrentVersion: "0.1.13",
		Exists: existsSet("/run/systemd/system"), Stdout: &out,
	}
	if err := Run(deps, Options{Repo: "acme/sparkyctrl", Service: "sparkyctrl"}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(cur)
	if string(got) != "NEWBIN" {
		t.Fatalf("binary not replaced: %q", got)
	}
	if !strings.Contains(out.String(), "0.1.14") {
		t.Fatalf("output missing version: %q", out.String())
	}
}

func TestRunAlreadyCurrent(t *testing.T) {
	srv := newReleaseServer(t, []byte("x"))
	defer srv.Close()
	var out bytes.Buffer
	deps := Deps{
		Client: srv.Client(), APIBase: srv.URL, Runner: &fakeRunner{},
		GOOS: "linux", GOARCH: "amd64",
		CurrentBinary: "/nonexistent", CurrentVersion: "0.1.14",
		Exists: existsSet(), Stdout: &out,
	}
	if err := Run(deps, Options{Repo: "acme/sparkyctrl"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "already") {
		t.Fatalf("expected already-current message, got %q", out.String())
	}
}
