package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SimpleHonors/sparkyctrl/internal/client"
	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
)

type remoteClient interface {
	Exec(protocol.ExecRequest) (protocol.ExecResponse, error)
	Shell(protocol.ShellRequest) (protocol.ExecResponse, error)
	Download(string) ([]byte, error)
	Upload(string, os.FileMode, []byte) error
	Ls(string) (protocol.LsResponse, error)
	Edit(protocol.EditRequest) (protocol.EditResponse, error)
	Info() (protocol.Info, error)
}

type clientFactory func(host string) (remoteClient, error)

// Run starts the stdio MCP server.
func Run(in io.Reader, out io.Writer) error {
	return RunWithFactory(in, out, defaultFactory)
}

// RunWithFactory starts the MCP server with a custom client factory.
func RunWithFactory(in io.Reader, out io.Writer, factory clientFactory) error {
	s := &server{
		r:       bufio.NewReader(in),
		w:       bufio.NewWriter(out),
		factory: factory,
	}
	return s.run()
}

type server struct {
	r       *bufio.Reader
	w       *bufio.Writer
	factory clientFactory
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type result struct {
	Content []content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

type content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func defaultFactory(host string) (remoteClient, error) {
	hosts, err := client.LoadHosts(hostsPath())
	if err != nil {
		return nil, err
	}
	base, err := client.Resolve(hosts, host)
	if err != nil {
		return nil, err
	}
	return client.New(base, os.Getenv("SPARKYCTRL_TOKEN")), nil
}

func hostsPath() string {
	if p := os.Getenv("SPARKYCTRL_HOSTS"); p != "" {
		return p
	}
	if _, err := os.Stat("hosts.toml"); err == nil {
		return "hosts.toml"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".sparkyctrl", "hosts.toml")
}

func (s *server) run() error {
	for {
		body, err := readFrame(s.r)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if len(body) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(body, &req); err != nil {
			if err := s.writeError(nil, -32700, "parse error", err.Error()); err != nil {
				return err
			}
			continue
		}
		if len(req.ID) == 0 {
			continue
		}
		if err := s.handle(req); err != nil {
			return err
		}
	}
}

func (s *server) handle(req rpcRequest) error {
	switch req.Method {
	case "initialize":
		return s.writeResult(req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo": map[string]any{
				"name":    "sparkyctrl",
				"version": protocol.Version,
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		})
	case "initialized":
		return nil
	case "tools/list":
		return s.writeResult(req.ID, map[string]any{
			"tools": toolList(),
		})
	case "tools/call":
		return s.handleToolCall(req.ID, req.Params)
	default:
		return s.writeError(req.ID, -32601, "method not found", req.Method)
	}
}

func (s *server) handleToolCall(id json.RawMessage, params json.RawMessage) error {
	var call struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return s.writeError(id, -32602, "invalid params", err.Error())
	}
	if call.Arguments == nil {
		call.Arguments = map[string]any{}
	}
	out, isErr, err := s.dispatch(call.Name, call.Arguments)
	if err != nil {
		return s.writeError(id, -32000, "tool call failed", err.Error())
	}
	return s.writeResult(id, outWithError(out, isErr))
}

func (s *server) dispatch(name string, args map[string]any) (result, bool, error) {
	host, err := reqString(args, "host")
	if err != nil {
		return result{}, true, err
	}
	c, err := s.factory(host)
	if err != nil {
		return result{}, true, err
	}
	switch name {
	case "exec":
		req := protocol.ExecRequest{}
		if req.Argv, err = reqStringSlice(args, "argv"); err != nil {
			return result{}, true, err
		}
		req.Cwd, _ = optString(args, "cwd")
		req.Stdin, _ = optString(args, "stdin")
		req.TimeoutSec, _ = optInt(args, "timeout_sec")
		if env, ok := args["env"]; ok {
			req.Env, err = stringMap(env)
			if err != nil {
				return result{}, true, err
			}
		}
		resp, err := c.Exec(req)
		if err != nil {
			return result{}, true, err
		}
		return jsonResult(resp), false, nil
	case "shell":
		req := protocol.ShellRequest{}
		req.Script, err = reqString(args, "script")
		if err != nil {
			return result{}, true, err
		}
		req.Shell, _ = optString(args, "shell")
		req.Cwd, _ = optString(args, "cwd")
		req.TimeoutSec, _ = optInt(args, "timeout_sec")
		resp, err := c.Shell(req)
		if err != nil {
			return result{}, true, err
		}
		return jsonResult(resp), false, nil
	case "read":
		path, err := reqString(args, "path")
		if err != nil {
			return result{}, true, err
		}
		data, err := c.Download(path)
		if err != nil {
			return result{}, true, err
		}
		return result{Content: []content{{Type: "text", Text: string(data)}}}, false, nil
	case "write":
		path, err := reqString(args, "path")
		if err != nil {
			return result{}, true, err
		}
		body, err := reqString(args, "content")
		if err != nil {
			return result{}, true, err
		}
		if err := c.Upload(path, 0o644, []byte(body)); err != nil {
			return result{}, true, err
		}
		return result{Content: []content{{Type: "text", Text: "ok"}}}, false, nil
	case "ls":
		path, err := reqString(args, "path")
		if err != nil {
			return result{}, true, err
		}
		resp, err := c.Ls(path)
		if err != nil {
			return result{}, true, err
		}
		return jsonResult(resp), false, nil
	case "edit":
		req := protocol.EditRequest{}
		if req.Path, err = reqString(args, "path"); err != nil {
			return result{}, true, err
		}
		if req.OldString, err = reqString(args, "old_string"); err != nil {
			return result{}, true, err
		}
		if req.NewString, err = reqString(args, "new_string"); err != nil {
			return result{}, true, err
		}
		req.ReplaceAll, _ = optBool(args, "replace_all")
		resp, err := c.Edit(req)
		if err != nil {
			return result{}, true, err
		}
		return jsonResult(resp), false, nil
	case "info":
		resp, err := c.Info()
		if err != nil {
			return result{}, true, err
		}
		return jsonResult(resp), false, nil
	default:
		return result{}, true, fmt.Errorf("unknown tool %q", name)
	}
}

func toolList() []tool {
	return []tool{
		{
			Name:        "exec",
			Description: "Run argv directly on a remote host.",
			InputSchema: objectSchema(map[string]any{
				"host":        stringProp("host name or host:port"),
				"argv":        arrayProp("command argv", "string"),
				"cwd":         stringProp("working directory"),
				"stdin":       stringProp("stdin payload"),
				"timeout_sec": numberProp("timeout in seconds"),
				"env":         objectStringMapProp("environment variables"),
			}, []string{"host", "argv"}),
		},
		{
			Name:        "shell",
			Description: "Run a script through the remote shell.",
			InputSchema: objectSchema(map[string]any{
				"host":        stringProp("host name or host:port"),
				"script":      stringProp("shell script"),
				"shell":       stringProp(`shell to use, such as "sh" or "powershell"`),
				"cwd":         stringProp("working directory"),
				"timeout_sec": numberProp("timeout in seconds"),
			}, []string{"host", "script"}),
		},
		{
			Name:        "read",
			Description: "Read a remote file as text.",
			InputSchema: objectSchema(map[string]any{
				"host": stringProp("host name or host:port"),
				"path": stringProp("remote file path"),
			}, []string{"host", "path"}),
		},
		{
			Name:        "write",
			Description: "Write text to a remote file.",
			InputSchema: objectSchema(map[string]any{
				"host":    stringProp("host name or host:port"),
				"path":    stringProp("remote file path"),
				"content": stringProp("file content"),
			}, []string{"host", "path", "content"}),
		},
		{
			Name:        "edit",
			Description: "Perform an exact-string remote file edit.",
			InputSchema: objectSchema(map[string]any{
				"host":        stringProp("host name or host:port"),
				"path":        stringProp("remote file path"),
				"old_string":  stringProp("literal text to replace"),
				"new_string":  stringProp("replacement text"),
				"replace_all": boolProp("replace every match"),
			}, []string{"host", "path", "old_string", "new_string"}),
		},
		{
			Name:        "ls",
			Description: "List a remote directory.",
			InputSchema: objectSchema(map[string]any{
				"host": stringProp("host name or host:port"),
				"path": stringProp("remote directory path"),
			}, []string{"host", "path"}),
		},
		{
			Name:        "info",
			Description: "Fetch worker metadata.",
			InputSchema: objectSchema(map[string]any{
				"host": stringProp("host name or host:port"),
			}, []string{"host"}),
		},
	}
}

func objectSchema(props map[string]any, required []string) map[string]any {
	out := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func stringProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func boolProp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

func numberProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

func arrayProp(desc, itemType string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": desc,
		"items":       map[string]any{"type": itemType},
	}
}

func objectStringMapProp(desc string) map[string]any {
	return map[string]any{
		"type":        "object",
		"description": desc,
		"additionalProperties": map[string]any{
			"type": "string",
		},
	}
}

func jsonResult(v any) result {
	b, _ := json.MarshalIndent(v, "", "  ")
	return result{Content: []content{{Type: "text", Text: string(b)}}}
}

func outWithError(res result, isErr bool) result {
	res.IsError = isErr
	return res
}

func reqString(args map[string]any, key string) (string, error) {
	v, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing required argument %q", key)
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", fmt.Errorf("argument %q must be a non-empty string", key)
	}
	return s, nil
}

func optString(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return s, true
}

func optBool(args map[string]any, key string) (bool, bool) {
	v, ok := args[key]
	if !ok || v == nil {
		return false, false
	}
	b, ok := v.(bool)
	if !ok {
		return false, false
	}
	return b, true
}

func optInt(args map[string]any, key string) (int, bool) {
	v, ok := args[key]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

func reqStringSlice(args map[string]any, key string) ([]string, error) {
	v, ok := args[key]
	if !ok {
		return nil, fmt.Errorf("missing required argument %q", key)
	}
	raw, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("argument %q must be an array of strings", key)
	}
	out := make([]string, 0, len(raw))
	for i, item := range raw {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("argument %q item %d must be a string", key, i)
		}
		out = append(out, s)
	}
	return out, nil
}

func stringMap(v any) (map[string]string, error) {
	raw, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("env must be an object of string values")
	}
	out := make(map[string]string, len(raw))
	for k, item := range raw {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("env value for %q must be a string", k)
		}
		out[k] = s
	}
	return out, nil
}

func readFrame(r *bufio.Reader) ([]byte, error) {
	var length int
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "content-length") {
			n, err := strconv.Atoi(strings.TrimSpace(val))
			if err != nil {
				return nil, err
			}
			length = n
		}
	}
	if length <= 0 {
		return nil, io.EOF
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

func writeFrame(w *bufio.Writer, body []byte) error {
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	return w.Flush()
}

func (s *server) writeResult(id json.RawMessage, v any) error {
	body, err := json.Marshal(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  v,
	})
	if err != nil {
		return err
	}
	return writeFrame(s.w, body)
}

func (s *server) writeError(id json.RawMessage, code int, msg string, data any) error {
	body, err := json.Marshal(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: msg,
			Data:    data,
		},
	})
	if err != nil {
		return err
	}
	return writeFrame(s.w, body)
}
