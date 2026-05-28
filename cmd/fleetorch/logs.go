package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/store"
)

func newLogsCmdReal() *cobra.Command {
	var full, errOnly bool
	cmd := &cobra.Command{
		Use:   "logs <task-id>",
		Short: "Print the log file for a task",
		Long: `Print the log file for a task.

By default prints the last 200 lines. --full prints the entire file.
--err prints the worker-side error sidecar (DataDir/errors/<id>.err),
which is where startup failures land when 'spawn' succeeds at the parent
but the detached worker dies before registering — i.e. the case where
'list' shows nothing despite a 'spawned: <id>' message.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return doLogs(args[0], full, errOnly)
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "Print the full log instead of the last 200 lines")
	cmd.Flags().BoolVar(&errOnly, "err", false, "Print the worker-side error sidecar instead of the agent log")
	return cmd
}

func doLogs(taskID string, full, errOnly bool) error {
	paths, err := config.Resolve()
	if err != nil {
		return err
	}

	if errOnly {
		errPath := filepath.Join(paths.DataDir, workerErrSubdir, taskID+".err")
		f, err := os.Open(errPath)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("(no worker errors recorded for this task)")
				return nil
			}
			return err
		}
		defer f.Close()
		_, err = io.Copy(os.Stdout, f)
		return err
	}

	st := store.New(paths.StateFile)
	task, err := st.GetTask(taskID)
	if err != nil {
		return err
	}
	f, err := os.Open(task.Log)
	if err != nil {
		return err
	}
	defer f.Close()

	if full {
		_, err := io.Copy(os.Stdout, f)
		return err
	}
	return tailLastLines(f, 200)
}

// tailLastLines reads the entire file and prints the last n lines.
// Fine for fleetorch's log sizes (single agent outputs, not multi-GB).
func tailLastLines(f *os.File, n int) error {
	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	start := 0
	count := 0
	for i := len(data) - 1; i >= 0; i-- {
		if data[i] == '\n' {
			count++
			if count > n {
				start = i + 1
				break
			}
		}
	}
	_, err = fmt.Fprint(os.Stdout, string(data[start:]))
	return err
}
