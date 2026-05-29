package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/store"
	"github.com/msnotfound/fleetorch/internal/types"
)

func newListCmdReal() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Show status of all tracked tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			if asJSON {
				return doListJSON()
			}
			return doList()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON array instead of the table")
	return cmd
}

// TaskRow is the JSON shape of a single task in `list --json` output.
// Kept separate from types.Task so we can compute derived fields (live
// status, age in seconds) without polluting the storage format.
type TaskRow struct {
	ID         string       `json:"id"`
	Agent      string       `json:"agent"`
	Status     types.Status `json:"status"`
	LiveStatus types.Status `json:"live_status"`
	AgeSeconds int64        `json:"age_seconds"`
	StartedAt  time.Time    `json:"started_at"`
	PID        int          `json:"pid"`
	BudgetUSD  float64      `json:"budget_usd"`
	ExitCode   *int         `json:"exit_code,omitempty"`
	Worktree   string       `json:"worktree"`
	Log        string       `json:"log"`
	Socket     string       `json:"socket,omitempty"`
	Repo       string       `json:"repo,omitempty"`
	Branch     string       `json:"branch,omitempty"`
}

func doListJSON() error {
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
	rows := make([]TaskRow, 0, len(tasks))
	for _, t := range tasks {
		var age int64
		if !t.StartedAt.IsZero() {
			age = int64(time.Since(t.StartedAt).Seconds())
		}
		rows = append(rows, TaskRow{
			ID:         t.ID,
			Agent:      t.Agent,
			Status:     t.Status,
			LiveStatus: liveStatus(t),
			AgeSeconds: age,
			StartedAt:  t.StartedAt,
			PID:        t.PID,
			BudgetUSD:  t.BudgetUSD,
			ExitCode:   t.ExitCode,
			Worktree:   t.Worktree,
			Log:        t.Log,
			Socket:     t.Socket,
			Repo:       t.Repo,
			Branch:     t.Branch,
		})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
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
	fmt.Fprintln(w, "TASK-ID\tAGENT\tSTATUS\tAGE\tBUDGET\tBAR\tWORKTREE")
	for _, t := range tasks {
		status := liveStatus(t)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t$%.2f\t%s\t%s\n",
			t.ID, t.Agent, status, age(t.StartedAt), t.BudgetUSD, budgetBarText(t.BudgetUSD), shortPath(t.Worktree))
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

// budgetBarText returns an ASCII budget bar safe for tabwriter alignment.
// Uses [########] notation (8 chars wide) with reference max of $20.
func budgetBarText(budgetUSD float64) string {
	const (
		barW   = 8
		maxRef = 20.0
	)
	frac := budgetUSD / maxRef
	if frac > 1 {
		frac = 1
	}
	if frac < 0 {
		frac = 0
	}
	filled := int(frac * barW)
	bar := make([]byte, barW+2)
	bar[0] = '['
	for i := 0; i < barW; i++ {
		if i < filled {
			bar[i+1] = '#'
		} else {
			bar[i+1] = '-'
		}
	}
	bar[barW+1] = ']'
	return string(bar)
}

func shortPath(p string) string {
	// On Windows we don't abbreviate. `~` isn't expanded by cmd.exe, and
	// mixing `~/` with the rest of the Windows backslash path produces
	// visually broken output like `~/AppData\Local\fleetorch\worktrees\...`.
	if runtime.GOOS == "windows" {
		return p
	}
	home, err := os.UserHomeDir()
	if err == nil {
		if rel, err := filepath.Rel(home, p); err == nil && len(rel) < len(p) {
			return "~/" + rel
		}
	}
	return p
}
