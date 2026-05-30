//go:build windows

package supervisor

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// maybeWrapShim rewrites argv to invoke a Windows .cmd / .bat wrapper through
// cmd.exe so paths containing spaces work. See buildShimCmdLine for the deeper
// /S /C bulletproofing that is also required when arguments themselves contain
// spaces (e.g. a multi-word prompt to gemini -p "…").
//
// The bug this whole file fights: many CLI agents (codex, gemini, claude, agy)
// install on Windows via npm as `<name>.cmd` shims under `%APPDATA%\npm\`.
// When a user's profile path contains spaces (e.g. `C:\Users\MAYANK SAHU\…`),
// invoking the shim directly produces:
//
//	'C:\Users\MAYANK' is not recognized as an internal or external command,
//	operable program or batch file.
//
// Calling cmd.exe and letting Go's syscall.EscapeArg quote the shim path was
// the v0.4.7 mitigation. It works when the *only* quoted token is the shim
// path. But cmd.exe applies special rules when /C sees more than two quotes on
// its tail: it strips the outer pair, mangling the shim path. So whenever an
// agent argument itself contains spaces and is quoted, the v0.4.7 path breaks.
//
// The v0.6.8 fix: use `cmd.exe /S /C "<entire command line>"` and override
// SysProcAttr.CmdLine in the caller, bypassing Go's automatic argv → cmdline
// quoting. /S tells cmd.exe to strip only the *outer* pair and use whatever is
// inside verbatim, so inner quotes around individual arguments are preserved.
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
	wrapped := make([]string, 0, len(argv)+3)
	wrapped = append(wrapped, cmdExe, "/S", "/C", argv[0])
	wrapped = append(wrapped, argv[1:]...)
	return wrapped
}

// buildShimCmdLine returns the literal Windows command line to launch a wrapped
// shim, or "" if argv is not a shim wrap. The caller assigns this to
// cmd.SysProcAttr.CmdLine, which go-pty and the standard Go runtime both honour
// in preference to constructing one from c.Args.
//
// The shape produced is exactly:
//
//	cmd.exe /S /C "\"<shim>\" <quoted args...>"
//
// where each argument that requires quoting is wrapped in its own inner pair.
// /S strips only the outer pair, leaving the shim path and each quoted arg
// intact for cmd.exe's parser.
func buildShimCmdLine(argv []string) string {
	// argv was already passed through maybeWrapShim, so on Windows the layout
	// is [cmd.exe, /S, /C, <shim>, ...args]. If anything else, no override.
	if len(argv) < 4 {
		return ""
	}
	if strings.ToLower(filepath.Base(argv[0])) != "cmd.exe" || argv[1] != "/S" || argv[2] != "/C" {
		return ""
	}
	shim := argv[3]
	rest := argv[4:]

	var inner strings.Builder
	inner.WriteString(quoteArg(shim))
	for _, a := range rest {
		inner.WriteString(" ")
		inner.WriteString(quoteArg(a))
	}

	// Outer wrapper: cmd.exe /S /C "<inner>"
	return quoteArg(argv[0]) + " /S /C \"" + inner.String() + "\""
}

// quoteArg returns s wrapped in double quotes if it contains characters that
// would otherwise be parsed as token separators by the Windows command line
// processor or cmd.exe (spaces, tabs, &, <, >, |, ^, (, ), ", @). Internal
// double quotes are escaped as \". This mirrors syscall.EscapeArg but is
// duplicated here to keep the platform split clean and so we can guarantee
// consistent behavior regardless of Go version.
func quoteArg(s string) string {
	if s == "" {
		return `""`
	}
	needsQuote := false
	for _, r := range s {
		switch r {
		case ' ', '\t', '"', '&', '<', '>', '|', '^', '(', ')', '@':
			needsQuote = true
		}
		if needsQuote {
			break
		}
	}
	if !needsQuote {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		if r == '"' {
			b.WriteString(`\"`)
		} else {
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
