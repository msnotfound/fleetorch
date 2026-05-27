package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Stub commands. Wired up in Phase 4 once internal packages land.

func newSpawnCmd() *cobra.Command {
	var (
		repo     string
		budget   float64
		turns    int
		model    string
	)
	cmd := &cobra.Command{
		Use:   "spawn <agent-type> <task-id> <prompt>",
		Short: "Spawn an agent in an isolated worktree",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("spawn not yet wired (phase 4)")
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "Path to git repo to worktree from (empty = scratch dir)")
	cmd.Flags().Float64Var(&budget, "budget-usd", 0, "USD budget ceiling")
	cmd.Flags().IntVar(&turns, "turns", 0, "Max turns (claude only)")
	cmd.Flags().StringVar(&model, "model", "", "Override model")
	return cmd
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show status of all tracked tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("list not yet wired (phase 4)")
		},
	}
}

func newWatchCmd() *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "watch <task-id>",
		Short: "Show the recent output of a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("watch not yet wired (phase 4)")
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Tail the log")
	return cmd
}

func newAttachCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "attach <task-id>",
		Short: "Drop into the live PTY of a running task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("attach not yet wired (phase 3)")
		},
	}
}

func newKillCmd() *cobra.Command {
	var purge bool
	cmd := &cobra.Command{
		Use:   "kill <task-id>",
		Short: "Stop a task and optionally remove its worktree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("kill not yet wired (phase 4)")
		},
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "Also remove worktree")
	return cmd
}

func newDashCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dash",
		Short: "Open the TUI dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("dash not yet wired (phase 3)")
		},
	}
}

func newLogsCmd() *cobra.Command {
	var full bool
	cmd := &cobra.Command{
		Use:   "logs <task-id>",
		Short: "Print the log file for a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("logs not yet wired (phase 4)")
		},
	}
	cmd.Flags().BoolVar(&full, "full", false, "Print full log instead of last 200 lines")
	return cmd
}

func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agent-type plugins",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List installed agent types",
			RunE: func(cmd *cobra.Command, args []string) error {
				return fmt.Errorf("agent list not yet wired (phase 4)")
			},
		},
		&cobra.Command{
			Use:   "add <path-to-toml>",
			Short: "Install an agent-type descriptor",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return fmt.Errorf("agent add not yet wired (phase 4)")
			},
		},
		&cobra.Command{
			Use:   "remove <name>",
			Short: "Remove an installed agent type",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return fmt.Errorf("agent remove not yet wired (phase 4)")
			},
		},
	)
	return cmd
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "View or edit fleetorch config",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "show",
			Short: "Print resolved config",
			RunE: func(cmd *cobra.Command, args []string) error {
				return fmt.Errorf("config show not yet wired (phase 4)")
			},
		},
		&cobra.Command{
			Use:   "edit",
			Short: "Open config in $EDITOR",
			RunE: func(cmd *cobra.Command, args []string) error {
				return fmt.Errorf("config edit not yet wired (phase 4)")
			},
		},
	)
	return cmd
}
