//go:build windows

package supervisor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/types"
)

// TestSpawnBareCommandNameResolvesWindows verifies that passing a bare command
// name ("cmd" rather than "C:\Windows\System32\cmd.exe") launches correctly
// on Windows via exec.LookPath. This is the direct Windows analogue of the
// Unix test in supervisor_test.go.
func TestSpawnBareCommandNameResolvesWindows(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "bare-win.log")

	m := New(config.Paths{DataDir: dir, LogDir: dir})
	task, err := m.Spawn(context.Background(), types.SpawnSpec{
		ID: "bare-cmd-windows",
		Agent: types.AgentType{
			Name:    "cmd",
			Command: "cmd", // bare name — must be resolved via %PATH%
			Args:    []string{"/Q", "/C", "exit 0"},
		},
		Log: logPath,
	})
	if err != nil {
		t.Fatalf("Spawn with bare command name on Windows: %v", err)
	}
	defer func() { _ = m.Kill("bare-cmd-windows") }()
	if task.PID == 0 {
		t.Errorf("expected non-zero PID")
	}
	if _, err := m.Wait("bare-cmd-windows"); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}
