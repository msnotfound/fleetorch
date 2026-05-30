package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/msnotfound/fleetorch/internal/config"
)

// Per-task worker-side error log. Lives at <DataDir>/errors/<id>.err.
//
// Why this exists: `fleetorch spawn` forks a detached worker whose stderr is
// nil. If the worker fails *before* it can write state.json or the agent log
// (e.g. exec.LookPath fails on the agent command, log file is unwritable,
// PTY allocation errors out), the user sees a happy "spawned: X" message
// from the parent CLI but `list` shows nothing and there is no clue where
// it went wrong. This sink fixes that by capturing such errors to a file
// the user can `fleetorch logs --err` or just open directly.
const workerErrSubdir = "errors"

// openWorkerErrLog returns an appendable file under DataDir/errors/<id>.err.
// On failure, returns a no-op closer that writes to /dev/null equivalent so
// the worker keeps running.
func openWorkerErrLog(dataDir, taskID string) io.WriteCloser {
	dir := filepath.Join(dataDir, workerErrSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nopWriteCloser{}
	}
	f, err := os.OpenFile(
		filepath.Join(dir, taskID+".err"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644,
	)
	if err != nil {
		return nopWriteCloser{}
	}
	return f
}

// writeWorkerError appends a single timestamped error line to the err log.
// Used at hard boundaries where we want to ensure something is recorded even
// if the rest of the worker fails immediately after.
func writeWorkerError(dataDir, taskID string, err error) {
	if err == nil {
		return
	}
	dir := filepath.Join(dataDir, workerErrSubdir)
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		return
	}
	f, ferr := os.OpenFile(
		filepath.Join(dir, taskID+".err"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644,
	)
	if ferr != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "[%s] %s\n", time.Now().UTC().Format(time.RFC3339), err.Error())
}

func writeWorkerStartupErr(specPath, taskID string, err error) {
	if err == nil {
		return
	}
	if taskID == "" {
		base := filepath.Base(specPath)
		if base != "." && base != string(filepath.Separator) {
			taskID = strings.TrimSuffix(base, filepath.Ext(base))
		}
	}
	if taskID == "" {
		taskID = "unknown-worker"
	}
	paths, resolveErr := config.Resolve()
	if resolveErr != nil {
		writeWorkerError(os.TempDir(), taskID, fmt.Errorf("%v (config.Resolve for worker err log failed: %w)", err, resolveErr))
		return
	}
	writeWorkerError(paths.DataDir, taskID, err)
}

func ensureWorkerErrNotEmpty(dataDir, taskID string) {
	if taskID == "" {
		taskID = "unknown-worker"
	}
	errPath := filepath.Join(dataDir, workerErrSubdir, taskID+".err")
	stat, err := os.Stat(errPath)
	if err == nil && stat.Size() > 0 {
		return
	}
	writeWorkerError(dataDir, taskID, errors.New("(unknown worker failure — task died before any output)"))
}

type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                { return nil }
