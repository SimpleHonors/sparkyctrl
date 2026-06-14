package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
)

// post is a small helper that issues a POST and returns the response.
func post(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return string(b)
}

func TestExecRejectsNegativeTimeout(t *testing.T) {
	srv := newTestServer(t, "", "")
	defer srv.Close()
	body, _ := json.Marshal(protocol.ExecRequest{Argv: []string{"echo", "hi"}, TimeoutSec: -1})
	resp := post(t, srv.URL+"/v1/exec", string(body))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for negative timeout, got %d", resp.StatusCode)
	}
}

func TestExecRejectsHugeTimeout(t *testing.T) {
	srv := newTestServer(t, "", "")
	defer srv.Close()
	body, _ := json.Marshal(protocol.ExecRequest{Argv: []string{"echo", "hi"}, TimeoutSec: protocol.MaxTimeoutSec + 1})
	resp := post(t, srv.URL+"/v1/exec", string(body))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for over-cap timeout, got %d", resp.StatusCode)
	}
}

func TestExecZeroTimeoutAccepted(t *testing.T) {
	srv := newTestServer(t, "", "")
	defer srv.Close()
	body, _ := json.Marshal(protocol.ExecRequest{Argv: []string{"echo", "hi"}, TimeoutSec: 0})
	resp := post(t, srv.URL+"/v1/exec", string(body))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 for zero (server-default) timeout, got %d", resp.StatusCode)
	}
}

func TestShellRejectsNegativeTimeout(t *testing.T) {
	srv := newTestServer(t, "", "")
	defer srv.Close()
	body, _ := json.Marshal(protocol.ShellRequest{Script: "echo hi", TimeoutSec: -5})
	resp := post(t, srv.URL+"/v1/shell", string(body))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for negative timeout, got %d", resp.StatusCode)
	}
}

func TestExecBodyOverCapRejected(t *testing.T) {
	srv := newTestServer(t, "", "")
	defer srv.Close()
	// Build a valid-prefix JSON body larger than the cap so the reader trips.
	big := strings.Repeat("a", int(MaxBodyBytes)+1024)
	body := `{"argv":["echo","` + big + `"]}`
	resp := post(t, srv.URL+"/v1/exec", body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413 for over-cap body, got %d", resp.StatusCode)
	}
}

func TestExecBodyUnderCapAccepted(t *testing.T) {
	srv := newTestServer(t, "", "")
	defer srv.Close()
	body, _ := json.Marshal(protocol.ExecRequest{Argv: []string{"echo", strings.Repeat("a", 1024)}})
	resp := post(t, srv.URL+"/v1/exec", string(body))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 for under-cap body, got %d", resp.StatusCode)
	}
}

func TestExecRejectsWrongMethod(t *testing.T) {
	srv := newTestServer(t, "", "")
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/exec")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("want 405 for GET on POST endpoint, got %d", resp.StatusCode)
	}
	if allow := resp.Header.Get("Allow"); allow != "POST" {
		t.Fatalf("want Allow: POST, got %q", allow)
	}
}

func TestDecodeErrorDoesNotLeakGoTypes(t *testing.T) {
	srv := newTestServer(t, "", "")
	defer srv.Close()
	// argv must be []string; feeding a number forces a Go *json.UnmarshalTypeError
	// whose default message names protocol.ExecRequest and []string.
	resp := post(t, srv.URL+"/v1/exec", `{"argv": 123}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
	bodyStr := readBody(t, resp)
	for _, leak := range []string{"protocol.", "ExecRequest", "[]string", "json:"} {
		if strings.Contains(bodyStr, leak) {
			t.Fatalf("decode error leaked Go internals %q in response: %s", leak, bodyStr)
		}
	}
}

func TestExecRejectsNewlineInEnvKey(t *testing.T) {
	srv := newTestServer(t, "", "")
	defer srv.Close()
	body, _ := json.Marshal(protocol.ExecRequest{
		Argv: []string{"echo", "hi"},
		Env:  map[string]string{"FOO\nBAR": "x"},
	})
	resp := post(t, srv.URL+"/v1/exec", string(body))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 for newline-in-env-key, got %d", resp.StatusCode)
	}
}
