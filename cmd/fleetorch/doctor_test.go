package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/msnotfound/fleetorch/internal/agents"
)

func TestStaleBuiltinAgentWarningsOnlyWarnsForChangedBuiltins(t *testing.T) {
	dir := t.TempDir()
	builtins, err := agents.BuiltinFiles()
	if err != nil {
		t.Fatalf("BuiltinFiles: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "gemini.toml"), []byte(`name = "gemini"
command = "old-gemini"
`), 0o644); err != nil {
		t.Fatalf("write stale builtin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "codex.toml"), builtins["codex"], 0o644); err != nil {
		t.Fatalf("write unchanged builtin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "custom.toml"), []byte(`name = "custom"
command = "custom"
`), 0o644); err != nil {
		t.Fatalf("write custom agent: %v", err)
	}

	warnings := staleBuiltinAgentWarnings(dir, []string{"codex", "custom", "gemini"})
	if len(warnings) != 1 {
		t.Fatalf("warnings = %#v, want one stale builtin warning", warnings)
	}
	want := "warn: builtin agent \"gemini\" on disk differs from shipped builtin. Run `fleetorch agent refresh-builtins` to update, or `agent edit \"gemini\"` to inspect."
	if warnings[0] != want {
		t.Fatalf("warning = %q, want %q", warnings[0], want)
	}
}
