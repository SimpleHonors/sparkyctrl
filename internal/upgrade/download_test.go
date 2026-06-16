package upgrade

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestDownloadAndVerify(t *testing.T) {
	payload := []byte("fake-binary-bytes")
	sum := sha256.Sum256(payload)
	sums := fmt.Sprintf("%s  sparkyctrl-linux-amd64\n", hex.EncodeToString(sum[:]))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bin":
			w.Write(payload)
		case "/sums":
			w.Write([]byte(sums))
		default:
			http.Error(w, "nope", 404)
		}
	}))
	defer srv.Close()

	rel := Release{AssetName: "sparkyctrl-linux-amd64", AssetURL: srv.URL + "/bin", ChecksumURL: srv.URL + "/sums"}
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sums" {
			w.Write([]byte("deadbeef  sparkyctrl-linux-amd64\n"))
		} else {
			w.Write([]byte("payload"))
		}
	}))
	defer srv.Close()
	rel := Release{AssetName: "sparkyctrl-linux-amd64", AssetURL: srv.URL + "/bin", ChecksumURL: srv.URL + "/sums"}
	if _, err := DownloadAndVerify(srv.Client(), rel, t.TempDir()); err == nil {
		t.Fatal("expected checksum mismatch error")
	}
}
