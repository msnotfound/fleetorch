package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/msnotfound/fleetorch/internal/agents"
	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/types"
)

func newSpawnCmdReal() *cobra.Command {
	var (
		repo    string
		budget  float64
		turns   int
		model   string
		fg      bool
	)
	cmd := &cobra.Command{
		Use:   "spawn <agent-type> <task-id> <prompt>",
		Short: "Spawn an agent in an isolated worktree",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return doSpawn(args[0], args[1], args[2], repo, budget, turns, model, fg)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "Path to git repo to worktree from (empty = scratch dir)")
	cmd.Flags().Float64Var(&budget, "budget-usd", 0, "USD budget ceiling (overrides agent default)")
	cmd.Flags().IntVar(&turns, "turns", 0, "Max turns (overrides agent default; claude-* only)")
	cmd.Flags().StringVar(&model, "model", "", "Override model")
	cmd.Flags().BoolVar(&fg, "foreground", false, "Run in the foreground, attached to this terminal (no detach)")
	return cmd
}

func doSpawn(agentName, taskID, prompt, repo string, budget float64, turns int, model string, foreground bool) error {
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	if err := agents.SeedDefaults(paths.AgentsDir); err != nil {
		return err
	}

	reg, err := agents.Load(paths.AgentsDir)
	if err != nil {
		return err
	}
	agent, err := reg.Get(agentName)
	if err != nil {
		return err
	}

	taskID, err = uniqueTaskID(paths, taskID)
	if err != nil {
		return err
	}

	worktree := filepath.Join(paths.WorktreeDir, taskID)
	branch := ""
	if repo != "" {
		repoAbs, err := filepath.Abs(repo)
		if err != nil {
			return err
		}
		branch = "agent/" + taskID
		if err := runGit(repoAbs, "worktree", "add", "-b", branch, worktree, "HEAD"); err != nil {
			return fmt.Errorf("git worktree: %w", err)
		}
	} else {
		if err := os.MkdirAll(worktree, 0o755); err != nil {
			return err
		}
	}

	log := filepath.Join(paths.LogDir, taskID+".log")
	sock := filepath.Join(paths.SocketDir, taskID+".sock")

	spec := types.SpawnSpec{
		ID:        taskID,
		Agent:     *applyOverrides(agent, budget, turns, model),
		Prompt:    resolvePrompt(prompt),
		Worktree:  worktree,
		Log:       log,
		Socket:    sock,
		BudgetUSD: pickBudget(agent, budget),
		Turns:     pickTurns(agent, turns),
		Model:     model,
	}

	if foreground {
		return runWorker(spec)
	}
	return forkWorker(spec, paths)
}

func uniqueTaskID(p config.Paths, base string) (string, error) {
	id := base
	for n := 2; n < 100; n++ {
		_, err := os.Stat(filepath.Join(p.WorktreeDir, id))
		if errors.Is(err, os.ErrNotExist) {
			return id, nil
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return "", fmt.Errorf("could not find unique id starting with %q", base)
}

func runGit(repo string, args ...string) error {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func resolvePrompt(arg string) string {
	if len(arg) > 1 && arg[0] == '@' {
		data, err := os.ReadFile(arg[1:])
		if err == nil {
			return string(data)
		}
	}
	if data, err := os.ReadFile(arg); err == nil {
		return string(data)
	}
	return arg
}

func applyOverrides(a *types.AgentType, budget float64, turns int, model string) *types.AgentType {
	out := *a
	if budget > 0 {
		out.DefaultBudgetUSD = budget
	}
	if turns > 0 {
		out.DefaultTurns = turns
	}
	if model != "" {
		// Replace any --model <m> arg.
		for i, arg := range out.Args {
			if arg == "--model" && i+1 < len(out.Args) {
				out.Args[i+1] = model
			}
		}
	}
	return &out
}

func pickBudget(a *types.AgentType, override float64) float64 {
	if override > 0 {
		return override
	}
	return a.DefaultBudgetUSD
}

func pickTurns(a *types.AgentType, override int) int {
	if override > 0 {
		return override
	}
	return a.DefaultTurns
}

// forkWorker writes the SpawnSpec to a temp file, re-execs fleetorch in
// `worker` mode detached from the parent's stdio, and prints a summary.
func forkWorker(spec types.SpawnSpec, paths config.Paths) error {
	specDir := filepath.Join(paths.DataDir, "specs")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		return err
	}
	specPath := filepath.Join(specDir, spec.ID+".json")

	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(specPath, data, 0o644); err != nil {
		return err
	}

	self, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(self, "worker", "--spec", specPath)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()

	// Give the worker a moment to register the task.
	time.Sleep(200 * time.Millisecond)

	fmt.Printf("spawned: %s\n", spec.ID)
	fmt.Printf("  agent:    %s\n", spec.Agent.Name)
	fmt.Printf("  worktree: %s\n", spec.Worktree)
	fmt.Printf("  log:      %s\n", spec.Log)
	fmt.Printf("\n  follow output: fleetorch attach %s\n", spec.ID)
	return nil
}
