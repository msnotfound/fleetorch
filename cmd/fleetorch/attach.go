package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/store"
)

func newAttachCmdReal() *cobra.Command {
	return &cobra.Command{
		Use:   "attach <task-id>",
		Short: "Follow a task's output (read-only in v0.1)",
		Long: `Follow a task's output as it streams to disk. Press Ctrl-C to detach.

Note: v0.1 attach is read-only. Bidirectional PTY attach (sending input
to the agent) is planned for v0.2 — it requires a per-task Unix socket
that the spawn worker exposes.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return doAttach(args[0])
		},
	}
}

func doAttach(taskID string) error {
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	st := store.New(paths.StateFile)
	task, err := st.GetTask(taskID)
	if err != nil {
		return err
	}

	f, err := os.Open(task.Log)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(os.Stdout, f); err != nil {
		return err
	}

	for {
		if !pidAlive(task.PID) {
			fmt.Fprintln(os.Stderr, "\n[task exited]")
			return nil
		}
		if _, err := io.Copy(os.Stdout, f); err != nil {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}
}
