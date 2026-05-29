package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/store"
	"github.com/msnotfound/fleetorch/internal/supervisor"
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

	cputimeDir := filepath.Join(paths.DataDir, "cputime")
	_ = os.MkdirAll(cputimeDir, 0o755)

	rows := make([]TaskRow, 0, len(tasks))
	for _, t := range tasks {
		var ageVal int64
		if !t.StartedAt.IsZero() {
			ageVal = int64(time.Since(t.StartedAt).Seconds())
		}
		rows = append(rows, TaskRow{
			ID:         t.ID,
			Agent:      t.Agent,
			Status:     t.Status,
			LiveStatus: liveStatus(t, cputimeDir),
			AgeSeconds: ageVal,
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

	// Recover orphaned workers whose state.json rows were lost but whose
	// sockets are still live on disk.
	if hasOrphanSockets(paths, tasks) {
		if recovered, recErr := store.RecoverOrphans(paths); recErr == nil && len(recovered) > 0 {
			fmt.Fprintf(os.Stderr, "recovered %d orphaned task(s)\n", len(recovered))
			tasks, err = st.ListTasks()
			if err != nil {
				return err
			}
		}
	}

	if len(tasks) == 0 {
		fmt.Println("no tasks. spawn one: fleetorch spawn <agent> <id> \"<prompt>\"")
		return nil
	}

	cputimeDir := filepath.Join(paths.DataDir, "cputime")
	_ = os.MkdirAll(cputimeDir, 0o755)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TASK-ID\tAGENT\tSTATUS\tAGE\tBUDGET\tBAR\tWORKTREE")
	for _, t := range tasks {
		status := liveStatus(t, cputimeDir)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t$%.2f\t%s\t%s\n",
			t.ID, t.Agent, status, age(t.StartedAt), t.BudgetUSD, budgetBarText(t.BudgetUSD), shortPath(t.Worktree))
	}
	return w.Flush()
}

// hasOrphanSockets returns true if any .sock file in SocketDir lacks a
// matching entry in the provided task list.
func hasOrphanSockets(paths config.Paths, tasks []*types.Task) bool {
	known := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		if t.Socket != "" {
			known[filepath.Base(t.Socket)] = true
		}
	}
	entries, err := os.ReadDir(paths.SocketDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sock") && !known[e.Name()] {
			return true
		}
	}
	return false
}

// liveStatus reports a derived status accounting for the on-disk record being
// stale (the process may have died without the worker updating state.json).
// Pass cputimeDir (DataDir/cputime) to enable CPU-time liveness; omit or pass
// "" to fall back to log-mtime only (used by dash.go, monitor.go).
func liveStatus(t *types.Task, cputimeDirs ...string) types.Status {
	cputimeDir := ""
	if len(cputimeDirs) > 0 {
		cputimeDir = cputimeDirs[0]
	}
	_ = cputimeDir // used below; suppress unused-variable lint for the branch
	if t.Status == types.StatusDone || t.Status == types.StatusFailed {
		return t.Status
	}
	if t.PID > 0 && !pidAlive(t.PID) {
		return types.StatusDead
	}

	// Try CPU-time liveness: if cumulative CPU grew since the last sample the
	// process is active regardless of stdout silence.
	if t.PID > 0 {
		if s, ok := cpuLiveness(t, cputimeDir); ok {
			return s
		}
	}

	// Fall back to log-mtime check (existing behaviour on platforms without /proc).
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

// cpuLiveness probes whether the process consumed new CPU since the last list
// call. Returns (status, true) when a determination can be made; (_, false)
// to signal the caller should fall back to the log-mtime check.
func cpuLiveness(t *types.Task, cacheDir string) (types.Status, bool) {
	current, err := supervisor.ProcessCPUTime(t.PID)
	if err != nil {
		return "", false // /proc not available (macOS, BSD, exotic platform)
	}

	cacheFile := filepath.Join(cacheDir, t.ID)
	prev, prevTime, readErr := readCPUSample(cacheFile)

	// Always persist the latest sample so the next call has a baseline.
	_ = writeCPUSample(cacheFile, current)

	if readErr != nil {
		return "", false // no previous sample; can't compare yet
	}
	if time.Since(prevTime) < 5*time.Second {
		return "", false // too soon; delta unreliable
	}

	if current > prev {
		return types.StatusActive, true
	}
	return types.StatusIdle, true
}

// readCPUSample reads a cached CPU sample written by writeCPUSample.
func readCPUSample(path string) (cpu time.Duration, sampledAt time.Time, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, time.Time{}, err
	}
	parts := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	if len(parts) != 2 {
		return 0, time.Time{}, fmt.Errorf("malformed cpu sample")
	}
	ns, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, time.Time{}, err
	}
	ts, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, time.Time{}, err
	}
	return time.Duration(ns), time.Unix(ts, 0), nil
}

// writeCPUSample persists a CPU sample for the next list call to compare against.
func writeCPUSample(path string, cpu time.Duration) error {
	content := fmt.Sprintf("%d\n%d\n", int64(cpu), time.Now().Unix())
	return os.WriteFile(path, []byte(content), 0o644)
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
