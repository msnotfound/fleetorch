package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/store"
	"github.com/msnotfound/fleetorch/internal/types"
)

func newListCmdReal() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show status of all tracked tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return doList()
		},
	}
}

func doList() error {
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	if err := paths.EnsureDirs(); err != nil {
		return err
	}

	st := store.New(paths.StateFile)
	tasks, err := st.ListTasks()
	if err != nil {
		return err
	}

	if len(tasks) == 0 {
		fmt.Println("no tasks. spawn one: fleetorch spawn <agent> <id> \"<prompt>\"")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TASK-ID\tAGENT\tSTATUS\tAGE\tBUDGET\tWORKTREE")
	for _, t := range tasks {
		status := liveStatus(t)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t$%.2f\t%s\n",
			t.ID, t.Agent, status, age(t.StartedAt), t.BudgetUSD, shortPath(t.Worktree))
	}
	return w.Flush()
}

// liveStatus reports a derived status accounting for the on-disk record being
// stale (the process may have died without the worker updating state.json).
func liveStatus(t *types.Task) types.Status {
	if t.Status == types.StatusDone || t.Status == types.StatusFailed {
		return t.Status
	}
	if t.PID > 0 && !pidAlive(t.PID) {
		return types.StatusDead
	}
	if t.Log != "" {
		if info, err := os.Stat(t.Log); err == nil {
			if time.Since(info.ModTime()) > 3*time.Minute {
				return types.StatusIdle
			}
			return types.StatusActive
		}
	}
	return t.Status
}

func age(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t).Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func shortPath(p string) string {
	home, err := os.UserHomeDir()
	if err == nil {
		if rel, err := filepath.Rel(home, p); err == nil && len(rel) < len(p) {
			return "~/" + rel
		}
	}
	return p
}
