package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveFileMergesAdditiveBlocks(t *testing.T) {
	input := `import a
<<<<<<< HEAD
export b
=======
export c
>>>>>>> branch
import d
`
	dir := t.TempDir()
	path := filepath.Join(dir, "x.ts")
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}
	got, blocks, err := resolveFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if blocks != 1 {
		t.Errorf("blocks = %d, want 1", blocks)
	}
	want := "import a\nexport b\nexport c\nimport d\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestResolveFileNoConflicts(t *testing.T) {
	input := "no\nconflicts\nhere\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	_ = os.WriteFile(path, []byte(input), 0o644)
	got, blocks, err := resolveFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if blocks != 0 {
		t.Errorf("blocks = %d, want 0", blocks)
	}
	if got != input {
		t.Errorf("got %q, want %q", got, input)
	}
}

func TestResolveFileMultipleBlocks(t *testing.T) {
	input := strings.Join([]string{
		"<<<<<<< HEAD", "a", "=======", "b", ">>>>>>> x",
		"middle",
		"<<<<<<< HEAD", "c", "=======", "d", ">>>>>>> y",
	}, "\n") + "\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	_ = os.WriteFile(path, []byte(input), 0o644)
	_, blocks, err := resolveFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if blocks != 2 {
		t.Errorf("blocks = %d, want 2", blocks)
	}
}
