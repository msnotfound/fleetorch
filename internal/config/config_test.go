package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveHonorsFleetorchHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLEETORCH_HOME", dir)

	p, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if p.ConfigDir != dir || p.DataDir != dir {
		t.Errorf("expected both dirs under FLEETORCH_HOME, got %+v", p)
	}
	if p.StateFile != filepath.Join(dir, "state.json") {
		t.Errorf("unexpected state file: %s", p.StateFile)
	}
}

func TestEnsureDirsCreatesAll(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLEETORCH_HOME", dir)

	p, err := Resolve()
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	for _, d := range []string{p.AgentsDir, p.WorktreeDir, p.LogDir} {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("expected %s to exist: %v", d, err)
		}
	}
}
