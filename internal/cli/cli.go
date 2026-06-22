// Package cli parses arguments and dispatches to the client or the worker.
package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/SimpleHonors/sparkyctrl/internal/client"
	"github.com/SimpleHonors/sparkyctrl/internal/mcp"
	"github.com/SimpleHonors/sparkyctrl/internal/protocol"
	"github.com/SimpleHonors/sparkyctrl/internal/server"
	"github.com/SimpleHonors/sparkyctrl/internal/upgrade"
)

const usage = `sparkyctrl - mangle-proof remote sysadmin for AI agents

WORKER (run on a target machine):
  sparkyctrl serve [--addr 0.0.0.0:7766] [--fence /path] [--token X] [--no-auth]
                   [--audit /path/audit.log]

CLIENT (run from the agent side):
  sparkyctrl exec  <host> -- <argv...>     run a command (no shell, mangle-proof)
   sparkyctrl shell <host> <script>         run a script through a real shell
                                           (or pipe stdin: shell <host> < script)
                                           --shell sh|powershell (default: platform)
  sparkyctrl ls    <host> <path>           list a directory
  sparkyctrl read  <host> <remote>         print a remote file to stdout
  sparkyctrl write <host> <remote>         write stdin to a remote file
  sparkyctrl push  <host> <local> <remote> upload a local file
  sparkyctrl pull  <host> <remote> <local> download to a local file
  sparkyctrl edit  <host> <remote> --old S --new S [--all]
                                           exact-string edit of a remote file
                                           (--old-file/--new-file for multiline)
  sparkyctrl info  <host>                  show worker info
  sparkyctrl mcp                           stdio MCP server wrapping the client
  sparkyctrl upgrade [--version vX] [--check] [--no-restart]   self-update from the official release

Host is a name from ~/.sparkyctrl/hosts.toml (or ./hosts.toml), or a literal host:port.
Add --json to any client verb for raw JSON output.
Env: SPARKYCTRL_HOSTS (hosts file path), SPARKYCTRL_TOKENS (tokens file path),
     SPARKYCTRL_TOKEN (token override). Per-host tokens: ~/.sparkyctrl/tokens (name = "token").

  sparkyctrl --version                 print version`

// splitExec splits "<host> -- <argv...>" into host and argv.
func splitExec(args []string) (string, []string, error) {
	if len(args) < 1 {
		return "", nil, fmt.Errorf("exec requires <host> -- <argv...>")
	}
	host := args[0]
	sep := -1
	for i, a := range args {
		if a == "--" {
			sep = i
			break
		}
	}
	if sep == -1 || sep == len(args)-1 {
		return "", nil, fmt.Errorf("exec requires '--' followed by the command, e.g. exec %s -- ls -la", host)
	}
	return host, args[sep+1:], nil
}

// extractJSON removes a leading "--json" flag but STOPS at the "--" command
// separator, so a --json that is part of an exec command is preserved.
func extractJSON(args []string) ([]string, bool) {
	out := make([]string, 0, len(args))
	jsonOut := false
	for i, a := range args {
		if a == "--" {
			out = append(out, args[i:]...)
			return out, jsonOut
		}
		if a == "--json" {
			jsonOut = true
			continue
		}
		out = append(out, a)
	}
	return out, jsonOut
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

func mkClient(host string) (*client.Client, error) {
	hosts, err := client.LoadHosts(hostsPath())
	if err != nil {
		return nil, err
	}
	base, err := client.Resolve(hosts, host)
	if err != nil {
		return nil, err
	}
	return client.New(base, client.ResolveToken(host)), nil
}

func fail(msg string) int {
	fmt.Fprintln(os.Stderr, "sparkyctrl: "+msg)
	return 1
}

// printJSON marshals v; v is always JSON-serializable so the error is ignored.
func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

// Run is the entrypoint. Returns the process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Println(usage)
		return 0
	}
	verb := args[0]
	rest := args[1:]

	switch verb {
	case "serve":
		return runServe(rest)
	case "mcp":
		return runMCP(rest)
	case "upgrade":
		return runUpgrade(args[1:])
	case "-h", "--help", "help":
		fmt.Println(usage)
		return 0
	case "--version", "-v", "version":
		fmt.Println(protocol.Version)
		return 0
	}

	rest, jsonOut := extractJSON(rest)
	switch verb {
	case "exec":
		return runExec(rest, jsonOut)
	case "shell":
		return runShell(rest, jsonOut)
	case "ls":
		return runLs(rest, jsonOut)
	case "read":
		return runRead(rest)
	case "write":
		return runWrite(rest)
	case "push":
		return runPush(rest)
	case "pull":
		return runPull(rest)
	case "edit":
		return runEdit(rest, jsonOut)
	case "info":
		return runInfo(rest, jsonOut)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s\n", verb, usage)
		return 2
	}
}

func emitExec(resp protocol.ExecResponse, jsonOut bool) int {
	if jsonOut {
		printJSON(resp)
		if resp.ExitCode < 0 {
			return 1
		}
		return resp.ExitCode
	}
	if resp.Error != "" {
		fmt.Fprintln(os.Stderr, "error: "+resp.Error)
	}
	fmt.Print(resp.Stdout)
	if resp.Stderr != "" {
		fmt.Fprint(os.Stderr, resp.Stderr)
	}
	if resp.ExitCode < 0 {
		return 1
	}
	return resp.ExitCode
}

func runExec(args []string, jsonOut bool) int {
	host, argv, err := splitExec(args)
	if err != nil {
		return fail(err.Error())
	}
	c, err := mkClient(host)
	if err != nil {
		return fail(err.Error())
	}
	resp, err := c.Exec(protocol.ExecRequest{Argv: argv})
	if err != nil {
		return fail(err.Error())
	}
	return emitExec(resp, jsonOut)
}

func runShell(args []string, jsonOut bool) int {
	if len(args) < 1 {
		return fail("usage: shell <host> [--shell sh|powershell] <script>  (or pipe stdin)")
	}
	host := args[0]

	// Parse flags from the remaining args.
	fs := flag.NewFlagSet("shell", flag.ContinueOnError)
	shell := fs.String("shell", "", "shell to use: sh (default on Unix), powershell")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	rest := fs.Args()

	var script string
	if len(rest) >= 1 {
		script = strings.Join(rest, " ")
	} else {
		// Read script from stdin. Refuse to block on a terminal.
		fi, err := os.Stdin.Stat()
		if err != nil {
			return fail(err.Error())
		}
		if fi.Mode()&os.ModeCharDevice != 0 {
			return fail("shell requires <script> or piped stdin (stdin is a terminal)")
		}
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fail(err.Error())
		}
		script = strings.TrimSpace(string(data))
		if script == "" {
			return fail("shell: stdin was empty")
		}
	}
	c, err := mkClient(host)
	if err != nil {
		return fail(err.Error())
	}
	resp, err := c.Shell(protocol.ShellRequest{Script: script, Shell: *shell})
	if err != nil {
		return fail(err.Error())
	}
	return emitExec(resp, jsonOut)
}

func runLs(args []string, jsonOut bool) int {
	if len(args) < 2 {
		return fail("usage: ls <host> <path>")
	}
	c, err := mkClient(args[0])
	if err != nil {
		return fail(err.Error())
	}
	resp, err := c.Ls(args[1])
	if err != nil {
		return fail(err.Error())
	}
	if jsonOut {
		printJSON(resp)
		return 0
	}
	for _, e := range resp.Entries {
		tag := "-"
		if e.IsDir {
			tag = "d"
		}
		fmt.Printf("%s %10d  %s\n", tag, e.Size, e.Name)
	}
	return 0
}

func runRead(args []string) int {
	if len(args) < 2 {
		return fail("usage: read <host> <remote>")
	}
	c, err := mkClient(args[0])
	if err != nil {
		return fail(err.Error())
	}
	data, err := c.Download(args[1])
	if err != nil {
		return fail(err.Error())
	}
	os.Stdout.Write(data)
	return 0
}

func runWrite(args []string) int {
	if len(args) < 2 {
		return fail("usage: write <host> <remote>  (content from stdin)")
	}
	c, err := mkClient(args[0])
	if err != nil {
		return fail(err.Error())
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fail(err.Error())
	}
	if err := c.Upload(args[1], 0o644, data); err != nil {
		return fail(err.Error())
	}
	return 0
}

func runPush(args []string) int {
	if len(args) < 3 {
		return fail("usage: push <host> <local> <remote>")
	}
	c, err := mkClient(args[0])
	if err != nil {
		return fail(err.Error())
	}
	data, err := os.ReadFile(args[1])
	if err != nil {
		return fail(err.Error())
	}
	if err := c.Upload(args[2], 0o644, data); err != nil {
		return fail(err.Error())
	}
	return 0
}

func runPull(args []string) int {
	if len(args) < 3 {
		return fail("usage: pull <host> <remote> <local>")
	}
	c, err := mkClient(args[0])
	if err != nil {
		return fail(err.Error())
	}
	data, err := c.Download(args[1])
	if err != nil {
		return fail(err.Error())
	}
	if err := os.WriteFile(args[2], data, 0o644); err != nil {
		return fail(err.Error())
	}
	return 0
}

// runEdit handles `sparkyctrl edit <host> <remote> --old S --new S [--all]`.
//
// host and remote are the first two positional args; the strings are supplied
// via flags so the no-shell argv path keeps them mangle-free. For multi-line
// or binary-ish strings the flags --old-file / --new-file read the value from
// a local file instead (cleanest minimal option for multiline; the flag value
// is never interpreted by a shell either way).
func runEdit(args []string, jsonOut bool) int {
	fs := flag.NewFlagSet("edit", flag.ContinueOnError)
	oldStr := fs.String("old", "", "exact string to replace (must match once unless --all)")
	newStr := fs.String("new", "", "replacement string")
	oldFile := fs.String("old-file", "", "read old string from this local file (for multiline)")
	newFile := fs.String("new-file", "", "read new string from this local file (for multiline)")
	all := fs.Bool("all", false, "replace every occurrence instead of requiring a unique match")

	// host and remote are positional and come before the flags.
	if len(args) < 2 {
		return fail("usage: edit <host> <remote> --old S --new S [--all]  (or --old-file/--new-file)")
	}
	host, remote := args[0], args[1]
	if err := fs.Parse(args[2:]); err != nil {
		return 2
	}

	old, ok := resolveEditStr(*oldStr, *oldFile, "old")
	if !ok {
		return 1
	}
	newS, ok := resolveEditStr(*newStr, *newFile, "new")
	if !ok {
		return 1
	}
	if old == "" {
		return fail("--old (or --old-file) must be non-empty")
	}

	c, err := mkClient(host)
	if err != nil {
		return fail(err.Error())
	}
	resp, err := c.Edit(protocol.EditRequest{
		Path: remote, OldString: old, NewString: newS, ReplaceAll: *all,
	})
	if err != nil {
		return fail(err.Error())
	}
	if jsonOut {
		printJSON(resp)
	} else {
		fmt.Printf("edited %s: replaced %d occurrence(s), %d bytes\n", remote, resp.Replaced, resp.BytesWritten)
	}
	return 0
}

// resolveEditStr returns the string for an edit flag, reading from file when
// the *-file variant is set. Setting both the literal and file form is an
// error. Returns (value, true) on success or (_, false) after reporting.
func resolveEditStr(lit, file, name string) (string, bool) {
	if file != "" {
		if lit != "" {
			fail(fmt.Sprintf("--%s and --%s-file are mutually exclusive", name, name))
			return "", false
		}
		b, err := os.ReadFile(file)
		if err != nil {
			fail(err.Error())
			return "", false
		}
		return string(b), true
	}
	return lit, true
}

func runInfo(args []string, jsonOut bool) int {
	if len(args) < 1 {
		return fail("usage: info <host>")
	}
	c, err := mkClient(args[0])
	if err != nil {
		return fail(err.Error())
	}
	info, err := c.Info()
	if err != nil {
		return fail(err.Error())
	}
	if jsonOut {
		printJSON(info)
	} else {
		fenceStr := info.Fence
		if fenceStr == "" {
			fenceStr = "none (FULL access)"
		}
		fmt.Printf("%s/%s  host=%s  version=%s  fence=%q\n", info.OS, info.Arch, info.Hostname, info.Version, fenceStr)
	}
	return 0
}

func runServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", "0.0.0.0:"+strconv.Itoa(protocol.DefaultPort), "listen address host:port")
	fence := fs.String("fence", "", "restrict file ops to this directory")
	token := fs.String("token", "", "explicit shared token override")
	noAuth := fs.Bool("no-auth", false, "disable token authentication")
	auditPath := fs.String("audit", "", "audit log file path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	auditor, err := server.NewAuditor(*auditPath)
	if err != nil {
		return fail("audit log: " + err.Error())
	}
	effectiveToken, authMode, authSource, err := server.ResolveAuth(*noAuth, *token)
	if err != nil {
		return fail(err.Error())
	}
	if authMode == "token-flag" {
		fmt.Fprintln(os.Stderr, "sparkyctrl: warning: --token places the secret in the process command line (visible via ps). Prefer the token file or SPARKYCTRL_TOKEN env var.")
	}
	h := &server.Handler{Fence: *fence, Token: effectiveToken, Audit: auditor}
	if authMode == "token-file" {
		fmt.Printf("sparkyctrl %s listening on %s (fence=%q auth=%s path=%q audit=%q)\n",
			protocol.Version, *addr, *fence, authMode, authSource, *auditPath)
	} else {
		fmt.Printf("sparkyctrl %s listening on %s (fence=%q auth=%s audit=%q)\n",
			protocol.Version, *addr, *fence, authMode, *auditPath)
	}
	if strings.HasPrefix(*addr, "0.0.0.0:") || strings.HasPrefix(*addr, ":") {
		fmt.Println("  note: listening on ALL interfaces (0.0.0.0) — intended for trusted LANs only, do not expose to the internet")
	}
	if err := h.Serve(*addr); err != nil {
		return fail(err.Error())
	}
	return 0
}

func runMCP(args []string) int {
	if len(args) != 0 {
		return fail("usage: mcp")
	}
	if err := mcp.Run(os.Stdin, os.Stdout); err != nil {
		return fail(err.Error())
	}
	return 0
}

func runUpgrade(args []string) int {
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	version := fs.String("version", "", "target version (default: latest release)")
	check := fs.Bool("check", false, "report current vs available and exit")
	noRestart := fs.Bool("no-restart", false, "install the binary but do not restart the worker")
	service := fs.String("service", "sparkyctrl", "systemd unit / Windows task name")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	self, err := os.Executable()
	if err != nil {
		return fail("cannot resolve own path: " + err.Error())
	}
	deps := upgrade.Deps{
		Client:         &http.Client{Timeout: 60 * time.Second},
		APIBase:        upgrade.DefaultAPIBase,
		Runner:         upgrade.OSRunner{},
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		CurrentBinary:  self,
		CurrentVersion: protocol.Version,
		Exists:         func(p string) bool { _, e := os.Stat(p); return e == nil },
		Stdout:         os.Stdout,
	}
	opts := upgrade.Options{
		Repo:          upgrade.DefaultRepo,
		TargetVersion: *version,
		Service:       *service,
		CheckOnly:     *check,
		NoRestart:     *noRestart,
	}
	if err := upgrade.Run(deps, opts); err != nil {
		return fail(err.Error())
	}
	return 0
}
