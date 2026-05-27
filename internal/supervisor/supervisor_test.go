//go:build !windows

package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/types"
)

func TestSpawnEchoCapturesOutput(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "echo.log")

	m := New(config.Paths{DataDir: dir, LogDir: dir})
	task, err := m.Spawn(context.Background(), types.SpawnSpec{
		ID: "echo-test",
		Agent: types.AgentType{
			Name:    "sh",
			Command: "sh",
			Args:    []string{"-c", "echo hello-from-pty"},
		},
		Log: logPath,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if task.PID == 0 {
		t.Errorf("expected non-zero PID")
	}

	if _, err := m.Wait("echo-test"); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	// Allow any final IO to flush to the log.
	time.Sleep(50 * time.Millisecond)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "hello-from-pty") {
		t.Errorf("log missing expected output. got: %q", string(data))
	}
}

func TestKillStopsLongRunningProcess(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "sleep.log")

	m := New(config.Paths{DataDir: dir, LogDir: dir})
	_, err := m.Spawn(context.Background(), types.SpawnSpec{
		ID: "sleep-test",
		Agent: types.AgentType{
			Name:    "sh",
			Command: "sh",
			Args:    []string{"-c", "sleep 60"},
		},
		Log: logPath,
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	if !m.IsAlive("sleep-test") {
		t.Fatalf("expected task to be alive immediately after spawn")
	}

	if err := m.Kill("sleep-test"); err != nil {
		t.Fatalf("Kill: %v", err)
	}
	if m.IsAlive("sleep-test") {
		t.Errorf("expected task to be dead after Kill")
	}
}

func TestKillUnknownReturnsErrNotFound(t *testing.T) {
	m := New(config.Paths{})
	err := m.Kill("nope")
	if err == nil || !strings.Contains(err.Error(), "task not found") {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}
