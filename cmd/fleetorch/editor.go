package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"unicode"
)

// resolveEditor returns the user's preferred editor, falling back to a
// platform-appropriate default.
//
// Preference order: $VISUAL → $EDITOR → platform default.
//   - $VISUAL is the POSIX convention for full-screen editors; many users
//     set it (e.g. to `code -w` or `nvim`).
//   - On Windows we default to notepad, since `vi` isn't typically on PATH.
//   - On macOS/Linux we default to vi, which is virtually always present.
func resolveEditor() string {
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v
	}
	if runtime.GOOS == "windows" {
		return "notepad"
	}
	return "vi"
}

func editorCommand(target string) (*exec.Cmd, error) {
	fields, err := splitEditor(resolveEditor())
	if err != nil {
		return nil, err
	}
	args := append([]string{}, fields[1:]...)
	args = append(args, target)
	return exec.Command(fields[0], args...), nil
}

func splitEditor(editor string) ([]string, error) {
	editor = strings.TrimSpace(editor)
	if editor == "" {
		return nil, fmt.Errorf("editor command is empty")
	}
	if binary, rest, ok := splitUnquotedWindowsEditor(editor); ok {
		args, err := shellFields(rest)
		if err != nil {
			return nil, err
		}
		return append([]string{binary}, args...), nil
	}
	fields, err := shellFields(editor)
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("editor command is empty")
	}
	return fields, nil
}

func splitUnquotedWindowsEditor(editor string) (string, string, bool) {
	lower := strings.ToLower(editor)
	for _, ext := range []string{".exe", ".cmd", ".bat", ".com"} {
		searchFrom := 0
		for {
			i := strings.Index(lower[searchFrom:], ext)
			if i < 0 {
				break
			}
			end := searchFrom + i + len(ext)
			if end == len(editor) || unicode.IsSpace(rune(editor[end])) {
				return strings.TrimSpace(editor[:end]), editor[end:], true
			}
			searchFrom = end
		}
	}
	return "", "", false
}

func shellFields(s string) ([]string, error) {
	var fields []string
	var b strings.Builder
	var quote rune
	inField := false

	flush := func() {
		if inField {
			fields = append(fields, b.String())
			b.Reset()
			inField = false
		}
	}

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\\' {
			if i+1 < len(runes) {
				next := runes[i+1]
				if next == '\\' || next == '\'' || next == '"' || unicode.IsSpace(next) {
					b.WriteRune(next)
					inField = true
					i++
					continue
				}
			}
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				inField = true
				continue
			}
			b.WriteRune(r)
			inField = true
			continue
		}
		switch {
		case r == '\'' || r == '"':
			quote = r
			inField = true
		case unicode.IsSpace(r):
			flush()
		default:
			b.WriteRune(r)
			inField = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("editor command has unterminated quote")
	}
	flush()
	return fields, nil
}
