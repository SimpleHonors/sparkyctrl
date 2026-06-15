package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"

	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
)

type fakeClient struct {
	lastExec  protocol.ExecRequest
	lastShell protocol.ShellRequest
	lastWrite string
	info      protocol.Info
}

func (f *fakeClient) Exec(req protocol.ExecRequest) (protocol.ExecResponse, error) {
	f.lastExec = req
	return protocol.ExecResponse{Stdout: "exec-ok"}, nil
}

func (f *fakeClient) Shell(req protocol.ShellRequest) (protocol.ExecResponse, error) {
	f.lastShell = req
	return protocol.ExecResponse{Stdout: "shell-ok"}, nil
}

func (f *fakeClient) Download(path string) ([]byte, error) {
	return []byte("read-ok"), nil
}

func (f *fakeClient) Upload(path string, mode os.FileMode, data []byte) error {
	f.lastWrite = path + ":" + string(data)
	return nil
}

func (f *fakeClient) Ls(path string) (protocol.LsResponse, error) {
	return protocol.LsResponse{Entries: []protocol.LsEntry{{Name: "x"}}}, nil
}

func (f *fakeClient) Edit(req protocol.EditRequest) (protocol.EditResponse, error) {
	return protocol.EditResponse{Replaced: 1, BytesWritten: 2}, nil
}

func (f *fakeClient) Info() (protocol.Info, error) {
	if f.info.Version == "" {
		f.info.Version = protocol.Version
	}
	return f.info, nil
}

func TestMCPInitializeAndToolCall(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	fc := &fakeClient{info: protocol.Info{Hostname: "box"}}
	done := make(chan error, 1)
	go func() {
		defer serverConn.Close()
		done <- RunWithFactory(serverConn, serverConn, func(host string) (remoteClient, error) {
			if host != "box" {
				return nil, fmt.Errorf("host = %q", host)
			}
			return fc, nil
		})
	}()

	sendFrame(t, clientConn, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	resp := readRPC(t, clientConn)
	if resp.Error != nil {
		t.Fatalf("initialize error: %+v", resp.Error)
	}

	sendFrame(t, clientConn, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	resp = readRPC(t, clientConn)
	if resp.Error != nil {
		t.Fatalf("tools/list error: %+v", resp.Error)
	}
	var listed struct {
		Tools []tool `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Tools) != 7 {
		t.Fatalf("tools = %d", len(listed.Tools))
	}

	sendFrame(t, clientConn, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"exec","arguments":{"host":"box","argv":["echo","hi"]}}}`)
	resp = readRPC(t, clientConn)
	if resp.Error != nil {
		t.Fatalf("tools/call error: %+v", resp.Error)
	}
	var call result
	if err := json.Unmarshal(resp.Result, &call); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(call.Content[0].Text); !strings.Contains(got, "exec-ok") {
		t.Fatalf("tool response = %q", got)
	}
	if len(fc.lastExec.Argv) != 2 || fc.lastExec.Argv[0] != "echo" {
		t.Fatalf("last exec = %+v", fc.lastExec)
	}

	clientConn.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestMCPPing(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	done := make(chan error, 1)
	go func() {
		defer serverConn.Close()
		done <- RunWithFactory(serverConn, serverConn, func(host string) (remoteClient, error) {
			return &fakeClient{}, nil
		})
	}()

	sendFrame(t, clientConn, `{"jsonrpc":"2.0","id":1,"method":"ping","params":{}}`)
	resp := readRPC(t, clientConn)
	if resp.Error != nil {
		t.Fatalf("ping returned error: %+v", resp.Error)
	}
	if len(resp.Result) == 0 {
		t.Fatal("ping result missing; MCP keepalive requires an empty result object")
	}
	var obj map[string]any
	if err := json.Unmarshal(resp.Result, &obj); err != nil {
		t.Fatalf("ping result is not a JSON object: %v", err)
	}
	if len(obj) != 0 {
		t.Fatalf("ping result must be an empty object, got %v", obj)
	}

	clientConn.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

func sendFrame(t *testing.T, conn net.Conn, body string) {
	t.Helper()
	if _, err := fmt.Fprintf(conn, "Content-Length: %d\r\n\r\n%s", len(body), body); err != nil {
		t.Fatal(err)
	}
}

func readRPC(t *testing.T, conn net.Conn) rpcEnvelope {
	t.Helper()
	reader := bufio.NewReader(conn)
	body, err := readFrame(reader)
	if err != nil {
		t.Fatal(err)
	}
	var env rpcEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	return env
}
