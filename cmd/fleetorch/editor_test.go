package main

import (
	"strings"
	"testing"
)

func TestEditorCommandSplitsEditorArgs(t *testing.T) {
	t.Setenv("VISUAL", "code -w")
	t.Setenv("EDITOR", "")

	cmd, err := editorCommand("agent.toml")
	if err != nil {
		t.Fatal(err)
	}

	wantArgs := []string{"code", "-w", "agent.toml"}
	if strings.Join(cmd.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("Args = %#v, want %#v", cmd.Args, wantArgs)
	}
}

func TestEditorCommandAllowsQuotedPathWithSpaces(t *testing.T) {
	t.Setenv("VISUAL", `"C:\Users\Mayank Sahu\AppData\Local\Programs\Microsoft VS Code\Code.exe" -w`)
	t.Setenv("EDITOR", "")

	cmd, err := editorCommand("agent.toml")
	if err != nil {
		t.Fatal(err)
	}

	wantPath := `C:\Users\Mayank Sahu\AppData\Local\Programs\Microsoft VS Code\Code.exe`
	if cmd.Path != wantPath {
		t.Fatalf("Path = %q, want %q", cmd.Path, wantPath)
	}
	wantArgs := []string{wantPath, "-w", "agent.toml"}
	if strings.Join(cmd.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("Args = %#v, want %#v", cmd.Args, wantArgs)
	}
}

func TestEditorCommandAllowsUnquotedWindowsExePathWithSpaces(t *testing.T) {
	t.Setenv("VISUAL", `C:\Users\Mayank Sahu\AppData\Local\Programs\Microsoft VS Code\Code.exe -w`)
	t.Setenv("EDITOR", "")

	cmd, err := editorCommand("agent.toml")
	if err != nil {
		t.Fatal(err)
	}

	wantPath := `C:\Users\Mayank Sahu\AppData\Local\Programs\Microsoft VS Code\Code.exe`
	if cmd.Path != wantPath {
		t.Fatalf("Path = %q, want %q", cmd.Path, wantPath)
	}
	wantArgs := []string{wantPath, "-w", "agent.toml"}
	if strings.Join(cmd.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("Args = %#v, want %#v", cmd.Args, wantArgs)
	}
}

func TestEditorCommandRejectsEmptyEditor(t *testing.T) {
	t.Setenv("VISUAL", "   ")
	t.Setenv("EDITOR", "")

	_, err := editorCommand("agent.toml")
	if err == nil {
		t.Fatal("editorCommand returned nil error, want non-nil")
	}
	if !strings.Contains(err.Error(), "editor command is empty") {
		t.Fatalf("error = %q, want empty editor message", err)
	}
}

func TestEditorCommandRejectsInteractiveEditorWithoutTTY(t *testing.T) {
	t.Setenv("VISUAL", "vim")
	t.Setenv("EDITOR", "")

	_, err := editorCommand("agent.toml")
	if err == nil {
		t.Fatal("editorCommand returned nil error, want non-nil")
	}
	if !strings.Contains(err.Error(), `editor "vim" is interactive but stdin/stdout is not a TTY`) {
		t.Fatalf("error = %q, want interactive non-TTY message", err)
	}
}

func TestEditorCommandAllowsNonInteractiveEditorWithoutTTY(t *testing.T) {
	t.Setenv("VISUAL", "cat")
	t.Setenv("EDITOR", "")

	cmd, err := editorCommand("agent.toml")
	if err != nil {
		t.Fatal(err)
	}

	wantArgs := []string{"cat", "agent.toml"}
	if strings.Join(cmd.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("Args = %#v, want %#v", cmd.Args, wantArgs)
	}
}
