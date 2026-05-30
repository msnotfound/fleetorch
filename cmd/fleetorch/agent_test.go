package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/msnotfound/fleetorch/internal/agents"
)

func TestRefreshBuiltinAgentsRefreshesStaleAndInstallsMissing(t *testing.T) {
	dir := t.TempDir()
	builtins, err := agents.BuiltinFiles()
	if err != nil {
		t.Fatalf("BuiltinFiles: %v", err)
	}
	stale := []byte(`name = "gemini"
command = "old-gemini"
`)
	if err := os.WriteFile(filepath.Join(dir, "gemini.toml"), stale, 0o644); err != nil {
		t.Fatalf("write stale gemini: %v", err)
	}

	var out strings.Builder
	summary, err := refreshBuiltinAgents(dir, true, false, strings.NewReader(""), &out)
	if err != nil {
		t.Fatalf("refreshBuiltinAgents: %v", err)
	}

	if summary.refreshed != 1 {
		t.Fatalf("refreshed = %d, want 1", summary.refreshed)
	}
	if summary.installed != len(builtins)-1 {
		t.Fatalf("installed = %d, want %d", summary.installed, len(builtins)-1)
	}
	gotGemini, err := os.ReadFile(filepath.Join(dir, "gemini.toml"))
	if err != nil {
		t.Fatalf("read gemini: %v", err)
	}
	if string(gotGemini) != string(builtins["gemini"]) {
		t.Fatalf("gemini.toml was not refreshed")
	}
	if !strings.Contains(out.String(), "refreshing gemini: old → new (diff: ") {
		t.Fatalf("output = %q, want refreshing line", out.String())
	}
}

func TestRefreshBuiltinAgentsDryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	stale := []byte(`name = "gemini"
command = "old-gemini"
`)
	if err := os.WriteFile(filepath.Join(dir, "gemini.toml"), stale, 0o644); err != nil {
		t.Fatalf("write stale gemini: %v", err)
	}

	var out strings.Builder
	summary, err := refreshBuiltinAgents(dir, true, true, strings.NewReader(""), &out)
	if err != nil {
		t.Fatalf("refreshBuiltinAgents: %v", err)
	}

	if summary.refreshed != 1 {
		t.Fatalf("refreshed = %d, want 1", summary.refreshed)
	}
	got, err := os.ReadFile(filepath.Join(dir, "gemini.toml"))
	if err != nil {
		t.Fatalf("read gemini: %v", err)
	}
	if string(got) != string(stale) {
		t.Fatalf("dry run changed gemini.toml")
	}
	if !strings.Contains(out.String(), "refreshed: 1, unchanged: 0, installed: ") {
		t.Fatalf("output = %q, want summary", out.String())
	}
}
