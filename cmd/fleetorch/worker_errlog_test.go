package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteWorkerStartupErrInfersTaskIDFromSpecPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLEETORCH_HOME", home)

	specPath := filepath.Join(home, "specs", "startup-fail.json")
	writeWorkerStartupErr(specPath, "", errors.New("read spec: permission denied"))

	data, err := os.ReadFile(filepath.Join(home, workerErrSubdir, "startup-fail.err"))
	if err != nil {
		t.Fatalf("read worker error sidecar: %v", err)
	}
	if !strings.Contains(string(data), "read spec: permission denied") {
		t.Fatalf("worker error sidecar missing startup error: %q", string(data))
	}
}
