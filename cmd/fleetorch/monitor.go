package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/store"
	"github.com/msnotfound/fleetorch/internal/types"
)

func newMonitorCmd() *cobra.Command {
	var (
		interval time.Duration
		dryRun   bool
		budget   float64
	)
	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Foreground narrator: polls the fleet and summarizes stuck/failed tasks via claude-haiku",
		Long: `Polls state.json every --interval. Tasks idle for 5+ minutes or in
'failed'/'dead' state are summarized by a short claude-haiku call.
Runs until Ctrl-C. ~$0.05/hour at full activity.

Use --dry-run to skip the claude call and just print the trigger conditions.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doMonitor(interval, dryRun, budget)
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", 60*time.Second, "How often to poll")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Skip claude-haiku call; just print trigger conditions")
	cmd.Flags().Float64Var(&budget, "max-budget-usd", 0.02, "Max budget per claude-haiku narration call")
	return cmd
}

func doMonitor(interval time.Duration, dryRun bool, budget float64) error {
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	st := store.New(paths.StateFile)

	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() { <-sigCh; cancel() }()

	hasClaude := false
	if !dryRun {
		if _, err := exec.LookPath("claude"); err == nil {
			hasClaude = true
		} else {
			fmt.Fprintln(os.Stderr, "warning: `claude` not on PATH — narration disabled (use --dry-run to silence)")
		}
	}

	fmt.Printf("monitor: polling every %s (Ctrl-C to stop)\n", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		runMonitorTick(st, hasClaude && !dryRun, budget)
		select {
		case <-ctx.Done():
			fmt.Println()
			return nil
		case <-ticker.C:
		}
	}
}

func runMonitorTick(st *store.Store, callClaude bool, budget float64) {
	tasks, err := st.ListTasks()
	if err != nil {
		fmt.Fprintln(os.Stderr, "monitor: list error:", err)
		return
	}

	var (
		active   []*types.Task
		stuck    []*types.Task
		failed   []*types.Task
	)
	for _, t := range tasks {
		live := liveStatus(t)
		switch live {
		case types.StatusActive, types.StatusRunning:
			active = append(active, t)
		case types.StatusIdle:
			if !t.StartedAt.IsZero() && time.Since(t.StartedAt) > 5*time.Minute {
				stuck = append(stuck, t)
			}
		case types.StatusFailed, types.StatusDead:
			failed = append(failed, t)
		}
	}

	ts := time.Now().Format("15:04:05")
	fmt.Printf("[%s] active=%d stuck=%d failed=%d\n", ts, len(active), len(stuck), len(failed))

	if len(stuck) == 0 && len(failed) == 0 {
		return
	}

	for _, t := range stuck {
		fmt.Printf("  STUCK   %s (%s) — idle since %s\n", t.ID, t.Agent, t.StartedAt.Format("15:04"))
	}
	for _, t := range failed {
		ec := "?"
		if t.ExitCode != nil {
			ec = fmt.Sprintf("%d", *t.ExitCode)
		}
		fmt.Printf("  FAILED  %s (%s) — exit=%s\n", t.ID, t.Agent, ec)
	}

	if !callClaude {
		return
	}
	narrate(stuck, failed, budget)
}

// narrate shells out to a single claude-haiku call. Cheap (~$0.001 per call).
func narrate(stuck, failed []*types.Task, budget float64) {
	var sb strings.Builder
	sb.WriteString("You are watching a parallel-agent fleet. In one or two short sentences, summarize the situation and suggest the next action. Be terse.\n\n")
	if len(stuck) > 0 {
		sb.WriteString("Stuck tasks (idle 5+ minutes):\n")
		for _, t := range stuck {
			sb.WriteString(fmt.Sprintf("- %s (%s) running since %s\n", t.ID, t.Agent, t.StartedAt.Format("15:04")))
		}
	}
	if len(failed) > 0 {
		sb.WriteString("Failed tasks:\n")
		for _, t := range failed {
			ec := "?"
			if t.ExitCode != nil {
				ec = fmt.Sprintf("%d", *t.ExitCode)
			}
			sb.WriteString(fmt.Sprintf("- %s (%s) exit=%s\n", t.ID, t.Agent, ec))
		}
	}

	args := []string{
		"-p", sb.String(),
		"--model", "haiku",
		"--max-budget-usd", fmt.Sprintf("%.3f", budget),
	}
	cmd := exec.Command("claude", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "  narrate error:", err)
		return
	}
	line := strings.TrimSpace(out.String())
	if line != "" {
		fmt.Println("  narrate:", line)
	}
}
