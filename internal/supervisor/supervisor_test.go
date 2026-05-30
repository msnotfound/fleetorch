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

// TestSpawnBareCommandNameResolves verifies that passing a bare command name
// (e.g. "sh" rather than "/bin/sh") works correctly via exec.LookPath.
// This is the Unix counterpart of the Windows bare-name fix; on Windows the
// equivalent test uses "cmd" (see supervisor_windows_test.go).
func TestSpawnBareCommandNameResolves(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "bare.log")

	m := New(config.Paths{DataDir: dir, LogDir: dir})
	task, err := m.Spawn(context.Background(), types.SpawnSpec{
		ID: "bare-cmd-test",
		Agent: types.AgentType{
			Name:    "sh",
			Command: "sh", // bare name — must be resolved via PATH
			Args:    []string{"-c", "exit 0"},
		},
		Log: logPath,
	})
	if err != nil {
		t.Fatalf("Spawn with bare command name: %v", err)
	}
	if task.PID == 0 {
		t.Errorf("expected non-zero PID")
	}
	if _, err := m.Wait("bare-cmd-test"); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}

func TestSpawnReturnsSocketListenError(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "socket-listen.log")
	socketPath := filepath.Join(dir, "missing", "task.sock")

	m := New(config.Paths{DataDir: dir, LogDir: dir})
	_, err := m.Spawn(context.Background(), types.SpawnSpec{
		ID: "socket-listen-test",
		Agent: types.AgentType{
			Name:    "sh",
			Command: "sh",
			Args:    []string{"-c", "sleep 60"},
		},
		Log:    logPath,
		Socket: socketPath,
	})
	if err == nil {
		_ = m.Kill("socket-listen-test")
		t.Fatalf("expected Spawn to return socket listen error")
	}
	if !strings.Contains(err.Error(), "listen socket") {
		t.Fatalf("expected listen socket error, got: %v", err)
	}
	if m.IsAlive("socket-listen-test") {
		_ = m.Kill("socket-listen-test")
		t.Fatalf("task still alive after socket listen failure")
	}
}
