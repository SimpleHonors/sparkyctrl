package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
)

// Client talks to one worker.
type Client struct {
	base  string
	token string
	http  *http.Client
}

// New builds a client for a base URL (e.g. http://192.0.2.5:7766).
func New(base, token string) *Client {
	return &Client{base: base, token: token, http: &http.Client{}}
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	if c.token != "" {
		req.Header.Set(protocol.TokenHeader, c.token)
	}
	return c.http.Do(req)
}

func (c *Client) postJSON(path string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", c.base+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e protocol.ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("worker error (%d): %s", resp.StatusCode, e.Error)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *Client) Exec(req protocol.ExecRequest) (protocol.ExecResponse, error) {
	var out protocol.ExecResponse
	err := c.postJSON("/v1/exec", req, &out)
	return out, err
}

func (c *Client) Shell(req protocol.ShellRequest) (protocol.ExecResponse, error) {
	var out protocol.ExecResponse
	err := c.postJSON("/v1/shell", req, &out)
	return out, err
}

func (c *Client) Ls(path string) (protocol.LsResponse, error) {
	var out protocol.LsResponse
	err := c.postJSON("/v1/ls", protocol.LsRequest{Path: path}, &out)
	return out, err
}

// Edit performs a surgical exact-string replacement on a remote file.
func (c *Client) Edit(req protocol.EditRequest) (protocol.EditResponse, error) {
	var out protocol.EditResponse
	err := c.postJSON("/v1/edit", req, &out)
	return out, err
}

func (c *Client) Info() (protocol.Info, error) {
	var out protocol.Info
	req, err := http.NewRequest("GET", c.base+"/v1/info", nil)
	if err != nil {
		return out, err
	}
	resp, err := c.do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e protocol.ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return out, fmt.Errorf("info failed (%d): %s", resp.StatusCode, e.Error)
	}
	err = json.NewDecoder(resp.Body).Decode(&out)
	return out, err
}

// Upload sends bytes to a remote path.
func (c *Client) Upload(path string, mode os.FileMode, data []byte) error {
	q := url.Values{}
	q.Set("path", path)
	q.Set("mode", strconv.FormatUint(uint64(mode), 8))
	req, err := http.NewRequest("POST", c.base+"/v1/upload?"+q.Encode(), bytes.NewReader(data))
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e protocol.ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("upload failed (%d): %s", resp.StatusCode, e.Error)
	}
	return nil
}

// Download fetches a remote path's bytes.
func (c *Client) Download(path string) ([]byte, error) {
	q := url.Values{}
	q.Set("path", path)
	req, err := http.NewRequest("POST", c.base+"/v1/download?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e protocol.ErrorResponse
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return nil, fmt.Errorf("download failed (%d): %s", resp.StatusCode, e.Error)
	}
	return io.ReadAll(resp.Body)
}
