package main

import (
	"os"
	"runtime"
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
