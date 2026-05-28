package agents

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadReadsTomls(t *testing.T) {
	dir := t.TempDir()
	writeAgentTOML(t, dir, "codex.toml", `name = "codex"
command = "codex"
`)
	writeAgentTOML(t, dir, "gemini.toml", `name = "gemini"
command = "gemini"
`)

	registry, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if _, err := registry.Get("codex"); err != nil {
		t.Fatalf("Get(codex): %v", err)
	}
	if _, err := registry.Get("gemini"); err != nil {
		t.Fatalf("Get(gemini): %v", err)
	}
	if got := len(registry.List()); got != 2 {
		t.Fatalf("List length = %d, want 2", got)
	}
}

func TestLoadSkipsInvalid(t *testing.T) {
	dir := t.TempDir()
	writeAgentTOML(t, dir, "valid.toml", `name = "codex"
command = "codex"
`)
	writeAgentTOML(t, dir, "missing-name.toml", `command = "codex"
`)

	registry, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := len(registry.List()); got != 1 {
		t.Fatalf("List length = %d, want 1", got)
	}
	if _, err := registry.Get("codex"); err != nil {
		t.Fatalf("Get(codex): %v", err)
	}
}

func TestRenderSubstitutesPrompt(t *testing.T) {
	agent := AgentType{
		Args:      []string{"--task"},
		PromptArg: "{prompt}",
	}

	got := agent.Render("hello")
	want := []string{"--task", "hello"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Render() = %#v, want %#v", got, want)
	}
}

func TestSeedDefaultsPopulatesEmptyDir(t *testing.T) {
	dir := t.TempDir()

	if err := SeedDefaults(dir); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}

	for _, name := range []string{
		"agy.toml",
		"codex.toml",
		"gemini.toml",
		"claude-haiku.toml",
		"claude-sonnet.toml",
		"claude-opus.toml",
	} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("expected %s to be written: %v", name, err)
		}
	}
}

func TestGetUnknownReturnsErr(t *testing.T) {
	registry := &Registry{}

	_, err := registry.Get("missing")
	if !errors.Is(err, ErrUnknownAgent) {
		t.Fatalf("Get(missing) error = %v, want ErrUnknownAgent", err)
	}
}

func writeAgentTOML(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
