package upgrade

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveTargetLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/sparkyctrl/releases/latest" {
			http.Error(w, "bad path "+r.URL.Path, http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"tag_name":"v0.1.14","assets":[
			{"name":"sparkyctrl-linux-amd64","browser_download_url":"http://x/bin"},
			{"name":"SHA256SUMS","browser_download_url":"http://x/sums"}]}`))
	}))
	defer srv.Close()

	rel, err := ResolveTarget(srv.Client(), srv.URL, "acme/sparkyctrl", "", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if rel.Version != "0.1.14" || rel.AssetName != "sparkyctrl-linux-amd64" {
		t.Fatalf("rel=%+v", rel)
	}
	if rel.AssetURL != "http://x/bin" || rel.ChecksumURL != "http://x/sums" {
		t.Fatalf("urls rel=%+v", rel)
	}
}

func TestResolveTargetMissingAsset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v0.1.14","assets":[{"name":"SHA256SUMS","browser_download_url":"http://x/sums"}]}`))
	}))
	defer srv.Close()
	if _, err := ResolveTarget(srv.Client(), srv.URL, "acme/sparkyctrl", "", "linux", "arm64"); err == nil {
		t.Fatal("expected error for missing asset")
	}
}
