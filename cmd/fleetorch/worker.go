package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/store"
	"github.com/msnotfound/fleetorch/internal/supervisor"
	"github.com/msnotfound/fleetorch/internal/types"
)

// newWorkerCmd is the hidden subprocess that actually supervises a single
// spawn. `fleetorch spawn` forks itself with this command so the user-facing
// process can return immediately.
func newWorkerCmd() *cobra.Command {
	var specPath string
	cmd := &cobra.Command{
		Use:    "worker",
		Short:  "Internal: supervise a single spawn (forked by `spawn`)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(specPath)
			if err != nil {
				err = fmt.Errorf("read spec: %w", err)
				writeWorkerStartupErr(specPath, "", err)
				return err
			}
			var spec types.SpawnSpec
			if err := json.Unmarshal(data, &spec); err != nil {
				err = fmt.Errorf("decode spec: %w", err)
				writeWorkerStartupErr(specPath, spec.ID, err)
				return err
			}
			return runWorker(spec)
		},
	}
	cmd.Flags().StringVar(&specPath, "spec", "", "path to JSON SpawnSpec file")
	_ = cmd.MarkFlagRequired("spec")
	return cmd
}

func runWorker(spec types.SpawnSpec) (retErr error) {
	paths, err := config.Resolve()
	if err != nil {
		// Can't even resolve paths — write to a best-effort temp location.
		writeWorkerError(os.TempDir(), spec.ID, fmt.Errorf("config.Resolve: %w", err))
		return err
	}

	// Always-on error sidecar. The worker is detached (stderr=nil), so any
	// error it prints is invisible without redirection. Capturing startup
	// errors here means `spawn` failures stop being silent.
	errSink := openWorkerErrLog(paths.DataDir, spec.ID)
	defer func() {
		if retErr != nil {
			ensureWorkerErrNotEmpty(paths.DataDir, spec.ID)
		}
		_ = errSink.Close()
	}()

	debug := os.Getenv("FLEETORCH_DEBUG") == "1"
	if debug {
		debugLog, openErr := os.OpenFile(
			filepath.Join(paths.DataDir, "debug-"+spec.ID+".log"),
			os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644,
		)
		if openErr == nil {
			defer debugLog.Close()
			os.Stderr = debugLog
			log.SetOutput(debugLog)
			log.Printf("[fleetorch-debug] worker %s: started pid=%d", spec.ID, os.Getpid())
		}
	} else {
		// Route the log package into the err sidecar so any late warning is
		// captured. We do NOT reassign os.Stderr — direct writes to it from
		// other packages would attempt to call methods on something that
		// might not be a real *os.File (when the sidecar couldn't open).
		log.SetOutput(errSink)
	}

	st := store.New(paths.StateFile)
	mgr := supervisor.New(paths)

	task, err := mgr.Spawn(context.Background(), spec)
	if err != nil {
		writeWorkerError(paths.DataDir, spec.ID, fmt.Errorf("supervisor.Spawn: %w", err))
		return err
	}
	if err := st.AddTask(task); err != nil {
		// Don't kill the spawn — log to stderr and continue. The PTY is alive.
		fmt.Fprintf(os.Stderr, "warning: AddTask failed: %v\n", err)
	}

	if debug {
		log.Printf("[fleetorch-debug] worker %s: task registered, waiting", spec.ID)
	}
	exitCode, _ := mgr.Wait(spec.ID)
	if debug {
		log.Printf("[fleetorch-debug] worker %s: agent exited code=%d", spec.ID, exitCode)
	}
	finalStatus := types.StatusDone
	if exitCode != 0 {
		finalStatus = types.StatusFailed
	}
	_ = st.UpdateTask(spec.ID, func(t *types.Task) {
		t.Status = finalStatus
		t.ExitCode = &exitCode
	})

	_ = mgr.Kill(spec.ID) // cleans up PTY/log handles
	// Process exit code mirrors the agent's so log inspection can show it.
	if exitCode != 0 {
		os.Exit(exitCode)
	}
	return nil
}
