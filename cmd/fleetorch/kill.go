package main

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/store"
	"github.com/msnotfound/fleetorch/internal/types"
)

func newKillCmdReal() *cobra.Command {
	var purge bool
	cmd := &cobra.Command{
		Use:   "kill <task-id>",
		Short: "Stop a task and optionally remove its worktree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return doKill(args[0], purge)
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "Also remove the task's worktree")
	return cmd
}

func doKill(taskID string, purge bool) error {
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	st := store.New(paths.StateFile)
	task, err := st.GetTask(taskID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("no such task: %s", taskID)
		}
		return err
	}
	if task.Status == types.StatusDone || task.Status == types.StatusFailed {
		fmt.Printf("task %s already exited (status: %s); nothing to kill\n", taskID, task.Status)
		return nil
	}

	if task.PID > 0 && pidAlive(task.PID) {
		p, _ := os.FindProcess(task.PID)
		_ = p.Signal(syscall.SIGTERM)
		// Wait briefly for clean exit.
		for i := 0; i < 50 && pidAlive(task.PID); i++ {
			time.Sleep(100 * time.Millisecond)
		}
		if pidAlive(task.PID) {
			_ = p.Kill()
		}
	}

	zero := 0
	_ = st.UpdateTask(taskID, func(t *types.Task) {
		t.Status = types.StatusDead
		t.ExitCode = &zero
	})

	if purge && task.Worktree != "" {
		if err := os.RemoveAll(task.Worktree); err != nil {
			return fmt.Errorf("remove worktree: %w", err)
		}
	}

	fmt.Printf("killed: %s\n", taskID)
	return nil
}
