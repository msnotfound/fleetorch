package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/store"
	"github.com/msnotfound/fleetorch/internal/types"
)

func newPruneCmd() *cobra.Command {
	var (
		dryRun          bool
		olderThan       time.Duration
		keepWorktrees   bool
		keepSockets     bool
		keepErrors      bool
		includeRunning  bool
		recoverOrphans  bool
	)
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove finished tasks from state.json (optionally their worktrees, sockets, error logs)",
		Long: `Garbage-collect dead tasks.

A task is eligible for pruning when its status is done, failed, or dead.
Running/active tasks are skipped unless --include-running is set (use that
only when you know the recorded PIDs are stale, e.g. after a crash).

By default, prune removes the matching task rows from state.json AND
deletes the corresponding worktree, control socket, and worker error log.
Use --keep-worktrees / --keep-sockets / --keep-errors to retain any of
those on disk. Use --dry-run to preview without deleting anything.

This is the main 'free up disk' lever — pnpm worktrees, npm installs,
and large clones can quickly add gigabytes per agent run.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doPrune(dryRun, olderThan, keepWorktrees, keepSockets, keepErrors, includeRunning, recoverOrphans)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without deleting anything")
	cmd.Flags().DurationVar(&olderThan, "older-than", 0, "Only prune tasks that started before now-D (e.g. 24h, 7d, 168h)")
	cmd.Flags().BoolVar(&keepWorktrees, "keep-worktrees", false, "Don't delete worktree directories")
	cmd.Flags().BoolVar(&keepSockets, "keep-sockets", false, "Don't delete control socket files")
	cmd.Flags().BoolVar(&keepErrors, "keep-errors", false, "Don't delete worker error sidecar files")
	cmd.Flags().BoolVar(&includeRunning, "include-running", false, "Also prune tasks marked running/active (recorded PIDs assumed stale)")
	cmd.Flags().BoolVar(&recoverOrphans, "recover-orphans", false, "Scan SocketDir for live sockets absent from state.json and add them back first")
	return cmd
}

type pruneCandidate struct {
	task    *types.Task
	reason  string
	size    int64 // approximate worktree size in bytes
	actions []string
}

func doPrune(dryRun bool, olderThan time.Duration, keepWorktrees, keepSockets, keepErrors, includeRunning, recoverOrphans bool) error {
	paths, err := config.Resolve()
	if err != nil {
		return err
	}

	if recoverOrphans {
		recovered, recErr := store.RecoverOrphans(paths)
		if recErr != nil {
			fmt.Fprintf(os.Stderr, "warning: orphan recovery failed: %v\n", recErr)
		} else if len(recovered) > 0 {
			fmt.Printf("orphans recovered: %d\n", len(recovered))
		}
	}

	st := store.New(paths.StateFile)
	tasks, err := st.ListTasks()
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		fmt.Println("no tasks to prune.")
		return nil
	}

	cutoff := time.Time{}
	if olderThan > 0 {
		cutoff = time.Now().Add(-olderThan)
	}

	var candidates []pruneCandidate
	for _, t := range tasks {
		if !includeRunning {
			if t.Status == types.StatusRunning || t.Status == types.StatusActive || t.Status == types.StatusIdle {
				// Cross-check with PID liveness; many "running" rows are actually stale.
				if t.PID > 0 && pidAlive(t.PID) {
					continue
				}
			}
		}
		if !cutoff.IsZero() && !t.StartedAt.IsZero() && t.StartedAt.After(cutoff) {
			continue
		}

		c := pruneCandidate{task: t, reason: string(t.Status)}
		if !includeRunning && t.PID > 0 && !pidAlive(t.PID) && (t.Status == types.StatusRunning || t.Status == types.StatusActive || t.Status == types.StatusIdle) {
			c.reason = string(t.Status) + " (stale PID)"
		}
		if !keepWorktrees && t.Worktree != "" {
			c.actions = append(c.actions, "worktree")
			if info := dirSize(t.Worktree); info > 0 {
				c.size = info
			}
		}
		if !keepSockets && t.Socket != "" {
			c.actions = append(c.actions, "socket")
		}
		if !keepErrors {
			errPath := filepath.Join(paths.DataDir, workerErrSubdir, t.ID+".err")
			if _, err := os.Stat(errPath); err == nil {
				c.actions = append(c.actions, "err-log")
			}
		}
		c.actions = append(c.actions, "state-row")
		candidates = append(candidates, c)
	}

	if len(candidates) == 0 {
		fmt.Println("nothing to prune (all tasks are running or younger than --older-than).")
		return nil
	}

	// Show what we're about to do.
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TASK-ID\tAGENT\tSTATUS\tAGE\tSIZE\tACTIONS")
	var totalSize int64
	for _, c := range candidates {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			c.task.ID, c.task.Agent, c.reason, age(c.task.StartedAt), humanBytes(c.size), strings.Join(c.actions, ","))
		totalSize += c.size
	}
	_ = w.Flush()
	fmt.Printf("\n%d task(s) selected, ~%s reclaimable\n", len(candidates), humanBytes(totalSize))

	if dryRun {
		fmt.Println("(dry-run: nothing deleted)")
		return nil
	}

	// Actually prune.
	var pruned int
	for _, c := range candidates {
		if !keepWorktrees && c.task.Worktree != "" {
			_ = os.RemoveAll(c.task.Worktree)
		}
		if !keepSockets && c.task.Socket != "" {
			_ = os.Remove(c.task.Socket)
		}
		if !keepErrors {
			errPath := filepath.Join(paths.DataDir, workerErrSubdir, c.task.ID+".err")
			_ = os.Remove(errPath)
		}
		if err := st.RemoveTask(c.task.ID); err == nil {
			pruned++
		}
	}

	// Also sweep orphan sockets: .sock files in SocketDir not referenced by any
	// remaining task. Common after worker crashes.
	if !keepSockets {
		swept := sweepOrphanSockets(paths.SocketDir, st)
		if swept > 0 {
			fmt.Printf("swept %d orphan socket(s)\n", swept)
		}
	}

	fmt.Printf("pruned %d task(s)\n", pruned)
	return nil
}

func sweepOrphanSockets(socketDir string, st *store.Store) int {
	entries, err := os.ReadDir(socketDir)
	if err != nil {
		return 0
	}
	tasks, err := st.ListTasks()
	if err != nil {
		return 0
	}
	live := make(map[string]bool, len(tasks))
	for _, t := range tasks {
		if t.Socket != "" {
			live[filepath.Base(t.Socket)] = true
		}
	}
	swept := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sock") {
			continue
		}
		if !live[e.Name()] {
			if err := os.Remove(filepath.Join(socketDir, e.Name())); err == nil {
				swept++
			}
		}
	}
	return swept
}

// dirSize walks a directory and returns the total byte count. Returns 0
// silently if the dir is missing or unreadable.
func dirSize(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	suffix := []string{"K", "M", "G", "T"}[exp]
	return fmt.Sprintf("%.1f%sB", float64(n)/float64(div), suffix)
}

// Compile-time check that we use sort (kept for stable diffs in future refactor).
var _ = sort.Strings
