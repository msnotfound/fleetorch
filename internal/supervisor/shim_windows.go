//go:build windows

package supervisor

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// maybeWrapShim rewrites argv to invoke a Windows .cmd / .bat wrapper through
// `cmd.exe /C` so paths containing spaces are quoted correctly.
//
// The bug it fixes: many CLI agents (codex, gemini, claude, agy) install on
// Windows via npm as `<name>.cmd` shims under `%APPDATA%\npm\`. When a user's
// profile path contains spaces (e.g. `C:\Users\MAYANK SAHU\…`), invoking the
// shim directly produces:
//
//   'C:\Users\MAYANK' is not recognized as an internal or external command,
//   operable program or batch file.
//
// Root cause: Windows resolves `.cmd` via cmd.exe and builds the command line
// without quoting the shim path. cmd.exe then parses up to the first space.
//
// The fix: explicitly prepend `cmd.exe /C` and let Go's own arg-quoting wrap
// the shim path in quotes. cmd.exe then parses the quoted token as a single
// command, regardless of spaces.
func maybeWrapShim(argv []string) []string {
	if len(argv) == 0 {
		return argv
	}
	ext := strings.ToLower(filepath.Ext(argv[0]))
	if ext != ".cmd" && ext != ".bat" {
		return argv
	}
	cmdExe, err := exec.LookPath("cmd.exe")
	if err != nil {
		// Fall back to the canonical Windows location. cmd.exe is effectively
		// always installed here on every supported Windows version.
		cmdExe = `C:\Windows\System32\cmd.exe`
	}
	wrapped := make([]string, 0, len(argv)+2)
	wrapped = append(wrapped, cmdExe, "/C", argv[0])
	wrapped = append(wrapped, argv[1:]...)
	return wrapped
}
