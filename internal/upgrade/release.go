package upgrade

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// DefaultRepo is the only source upgrade will ever pull from.
const DefaultRepo = "SimpleHonors/sparkyctrl"

// DefaultAPIBase is the GitHub API base; overridable in tests.
const DefaultAPIBase = "https://api.github.com"

// Release is the resolved upgrade target for this host.
type Release struct {
	Tag             string
	Version         string
	AssetName       string
	AssetURL        string
	ChecksumURL     string
	AssetSigURL     string // <AssetName>.minisig — detached minisign signature of the binary
	ChecksumSigURL  string // SHA256SUMS.minisig — detached minisign signature of SHA256SUMS
}

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

// ResolveTarget fetches release metadata and locates the asset + SHA256SUMS
// for this host. requested "" means latest; otherwise the tag (v-prefixed).
func ResolveTarget(client *http.Client, apiBase, repo, requested, goos, goarch string) (Release, error) {
	url := apiBase + "/repos/" + repo + "/releases/latest"
	if requested != "" {
		tag := requested
		if tag[0] != 'v' {
			tag = "v" + tag
		}
		url = apiBase + "/repos/" + repo + "/releases/tags/" + tag
	}
	resp, err := client.Get(url)
	if err != nil {
		return Release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("release lookup: %s returned %d", url, resp.StatusCode)
	}
	var gr ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		return Release{}, err
	}
	want := AssetName(goos, goarch)
	rel := Release{Tag: gr.TagName, Version: NormalizeVersion(gr.TagName), AssetName: want}
	for _, a := range gr.Assets {
		switch a.Name {
		case want:
			rel.AssetURL = a.URL
		case want + ".minisig":
			rel.AssetSigURL = a.URL
		case "SHA256SUMS":
			rel.ChecksumURL = a.URL
		case "SHA256SUMS.minisig":
			rel.ChecksumSigURL = a.URL
		}
	}
	if rel.AssetURL == "" {
		return Release{}, fmt.Errorf("release %s has no asset %q", gr.TagName, want)
	}
	return rel, nil
}
