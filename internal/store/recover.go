// Package store - orphan recovery via socket scan.
package store

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/types"
)

// RecoverOrphans scans SocketDir for .sock files that have no matching task
// in state.json. For each socket that accepts a connection (live listener),
// it creates a synthetic Task with status="recovered" and minimal metadata,
// then persists it to the store. Returns the recovered tasks.
func RecoverOrphans(paths config.Paths) ([]types.Task, error) {
	st := New(paths.StateFile)
	tasks, err := st.ListTasks()
	if err != nil {
		return nil, err
	}

	// Index known socket paths and IDs so we can skip already-tracked workers.
	knownSock := make(map[string]bool, len(tasks))
	knownID := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		knownID[t.ID] = true
		if t.Socket != "" {
			knownSock[filepath.Clean(t.Socket)] = true
		}
	}

	entries, err := os.ReadDir(paths.SocketDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read socket dir: %w", err)
	}

	var recovered []types.Task
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sock") {
			continue
		}

		sockPath := filepath.Join(paths.SocketDir, e.Name())
		if knownSock[filepath.Clean(sockPath)] {
			continue
		}

		// Derive task ID from filename: "<taskID>.sock" → "<taskID>".
		taskID := strings.TrimSuffix(e.Name(), ".sock")
		if knownID[taskID] {
			continue
		}

		// Only recover sockets whose listener is still alive.
		if !socketListening(sockPath) {
			continue
		}

		info, err := e.Info()
		if err != nil {
			continue
		}

		// Log file is conventionally <LogDir>/<taskID>.log.
		logPath := filepath.Join(paths.LogDir, taskID+".log")
		if _, err := os.Stat(logPath); err != nil {
			logPath = ""
		}

		pid := socketPID(sockPath)

		task := types.Task{
			ID:        taskID,
			Agent:     "unknown",
			Socket:    sockPath,
			Log:       logPath,
			PID:       pid,
			StartedAt: info.ModTime(),
			Status:    types.Status("recovered"),
		}

		if err := st.AddTask(&task); err != nil {
			continue // best-effort; don't abort the whole scan
		}
		knownID[taskID] = true
		recovered = append(recovered, task)
	}

	return recovered, nil
}

// socketListening returns true if a listener is accepting connections on the
// given Unix-domain socket path. The connection is closed immediately; this
// only tests liveness.
func socketListening(path string) bool {
	conn, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
