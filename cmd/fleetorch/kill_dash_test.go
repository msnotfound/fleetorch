package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/store"
	"github.com/msnotfound/fleetorch/internal/types"
)

func TestDoKillAlreadyExitedTaskIsNoop(t *testing.T) {
	cases := []types.Status{types.StatusDone, types.StatusFailed}

	for _, status := range cases {
		t.Run(string(status), func(t *testing.T) {
			t.Setenv("FLEETORCH_HOME", t.TempDir())
			paths, err := config.Resolve()
			if err != nil {
				t.Fatal(err)
			}
			if err := paths.EnsureDirs(); err != nil {
				t.Fatal(err)
			}

			exitCode := 7
			st := store.New(paths.StateFile)
			if err := st.Save(&types.State{
				Tasks: []*types.Task{
					{
						ID:        "done-task",
						Agent:     "codex",
						Worktree:  filepath.Join(t.TempDir(), "worktree"),
						Log:       filepath.Join(t.TempDir(), "task.log"),
						StartedAt: time.Now(),
						Status:    status,
						ExitCode:  &exitCode,
					},
				},
			}); err != nil {
				t.Fatal(err)
			}

			output := captureStdout(t, func() {
				if err := doKill("done-task", false); err != nil {
					t.Fatal(err)
				}
			})

			wantMessage := "task done-task already exited (status: " + string(status) + "); nothing to kill"
			if !strings.Contains(output, wantMessage) {
				t.Fatalf("output = %q, want message containing %q", output, wantMessage)
			}

			got, err := st.GetTask("done-task")
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != status {
				t.Fatalf("status = %q, want %q", got.Status, status)
			}
			if got.ExitCode == nil || *got.ExitCode != exitCode {
				t.Fatalf("exit code = %v, want %d", got.ExitCode, exitCode)
			}
		})
	}
}

func TestDoDashTUIRejectsNonTerminalStdout(t *testing.T) {
	t.Setenv("FLEETORCH_HOME", t.TempDir())

	stderr := captureStderr(t, func() {
		err := doDashTUI()
		if err == nil {
			t.Fatal("doDashTUI returned nil error, want non-nil")
		}
	})

	want := "dash requires a terminal. Use --plain for a non-interactive table."
	if !strings.Contains(stderr, want) {
		t.Fatalf("stderr = %q, want message containing %q", stderr, want)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() {
		os.Stderr = old
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
