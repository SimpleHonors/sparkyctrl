package server

import "strings"

// buildWindowsShellLine builds the raw lpCommandLine for running script through
// cmd.exe as `<comspec> /s /c "<script>"`. With /s, cmd strips only the outermost
// pair of quotes and runs the remainder verbatim, so trailing backslashes and
// shell metacharacters in the script survive intact — unlike Go's default
// argument escaping, which doubles a trailing backslash and breaks cmd's parser.
func buildWindowsShellLine(comspec, script string) string {
	if comspec == "" {
		comspec = "cmd.exe"
	}
	prog := comspec
	if strings.ContainsAny(prog, " \t") {
		prog = `"` + prog + `"`
	}
	return prog + ` /s /c "` + script + `"`
}
