package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

	"github.com/msnotfound/fleetorch/internal/agents"
	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/types"
)

func newAgentCmdReal() *cobra.Command {
	var refreshYes bool
	var refreshDryRun bool
	refreshCmd := &cobra.Command{
		Use:   "refresh-builtins",
		Short: "Refresh installed builtin agent TOMLs from this binary",
		RunE: func(cmd *cobra.Command, args []string) error {
			return doAgentRefreshBuiltins(refreshYes, refreshDryRun)
		},
	}
	refreshCmd.Flags().BoolVar(&refreshYes, "yes", false, "Refresh changed builtins without prompting")
	refreshCmd.Flags().BoolVar(&refreshDryRun, "dry-run", false, "Show changes without writing files")

	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage agent-type plugins",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List installed agent types",
			RunE: func(cmd *cobra.Command, args []string) error {
				return doAgentList()
			},
		},
		&cobra.Command{
			Use:   "add <path-to-toml>",
			Short: "Install an agent-type descriptor",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return doAgentAdd(args[0])
			},
		},
		&cobra.Command{
			Use:   "remove <name>",
			Short: "Remove an installed agent type",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return doAgentRemove(args[0])
			},
		},
		&cobra.Command{
			Use:   "edit <name>",
			Short: "Open an installed agent TOML in $EDITOR",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return doAgentEdit(args[0])
			},
		},
		refreshCmd,
	)
	return cmd
}

func doAgentEdit(name string) error {
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
	target := filepath.Join(paths.AgentsDir, name+".toml")
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("agent %q not installed (looked for %s)", name, target)
	}
	c, err := editorCommand(target)
	if err != nil {
		return err
	}
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

func doAgentList() error {
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
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tCOMMAND\tBUDGET\tTURNS\tNOTES")
	for _, a := range reg.List() {
		fmt.Fprintf(w, "%s\t%s\t$%.2f\t%d\t%s\n",
			a.Name, a.Command, a.DefaultBudgetUSD, a.DefaultTurns, truncate(a.Notes, 50))
	}
	return w.Flush()
}

func doAgentRefreshBuiltins(yes, dryRun bool) error {
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	_, err = refreshBuiltinAgents(paths.AgentsDir, yes, dryRun, os.Stdin, os.Stdout)
	return err
}

type refreshBuiltinsSummary struct {
	refreshed int
	unchanged int
	installed int
}

func refreshBuiltinAgents(agentsDir string, yes, dryRun bool, in io.Reader, out io.Writer) (refreshBuiltinsSummary, error) {
	var summary refreshBuiltinsSummary
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return summary, fmt.Errorf("mkdir agents dir: %w", err)
	}

	builtins, err := agents.BuiltinFiles()
	if err != nil {
		return summary, err
	}

	names := make([]string, 0, len(builtins))
	for name := range builtins {
		names = append(names, name)
	}
	sort.Strings(names)

	reader := bufio.NewReader(in)
	for _, name := range names {
		shipped := builtins[name]
		target := filepath.Join(agentsDir, name+".toml")
		current, err := os.ReadFile(target)
		if err != nil {
			if !os.IsNotExist(err) {
				return summary, fmt.Errorf("read %s: %w", target, err)
			}
			fmt.Fprintf(out, "installing %s\n", name)
			summary.installed++
			if dryRun {
				continue
			}
			if err := os.WriteFile(target, shipped, 0o644); err != nil {
				return summary, fmt.Errorf("write %s: %w", target, err)
			}
			continue
		}

		if bytes.Equal(current, shipped) {
			fmt.Fprintf(out, "unchanged %s\n", name)
			summary.unchanged++
			continue
		}

		if !yes {
			fmt.Fprintf(out, "refresh %s? [y/N] ", name)
			answer, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				return summary, fmt.Errorf("read prompt: %w", err)
			}
			answer = strings.ToLower(strings.TrimSpace(answer))
			if answer != "y" && answer != "yes" {
				fmt.Fprintf(out, "skipped %s\n", name)
				continue
			}
		}

		fmt.Fprintf(out, "refreshing %s: old → new (diff: %d lines)\n", name, lineDiffCount(current, shipped))
		summary.refreshed++
		if dryRun {
			continue
		}
		if err := os.WriteFile(target, shipped, 0o644); err != nil {
			return summary, fmt.Errorf("write %s: %w", target, err)
		}
	}

	fmt.Fprintf(out, "refreshed: %d, unchanged: %d, installed: %d\n", summary.refreshed, summary.unchanged, summary.installed)
	return summary, nil
}

func lineDiffCount(oldContents, newContents []byte) int {
	oldLines := splitLines(oldContents)
	newLines := splitLines(newContents)
	common := longestCommonSubsequenceLength(oldLines, newLines)
	return len(oldLines) + len(newLines) - 2*common
}

func splitLines(contents []byte) []string {
	if len(contents) == 0 {
		return nil
	}
	text := strings.ReplaceAll(string(contents), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func longestCommonSubsequenceLength(a, b []string) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1] + 1
			} else if prev[j] > curr[j-1] {
				curr[j] = prev[j]
			} else {
				curr[j] = curr[j-1]
			}
		}
		prev, curr = curr, prev
		for j := range curr {
			curr[j] = 0
		}
	}
	return prev[len(b)]
}

func doAgentAdd(src string) error {
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	if err := paths.EnsureDirs(); err != nil {
		return err
	}

	var agent types.AgentType
	if _, err := toml.DecodeFile(src, &agent); err != nil {
		return err
	}
	agent.Name = strings.TrimSpace(agent.Name)
	if agent.Name == "" {
		return fmt.Errorf("agent TOML %s must set a non-empty name", src)
	}

	dst := filepath.Join(paths.AgentsDir, agent.Name+".toml")

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	fmt.Printf("installed: %s\n", dst)
	return nil
}

func doAgentRemove(name string) error {
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	dst := filepath.Join(paths.AgentsDir, name+".toml")
	if err := os.Remove(dst); err != nil {
		return err
	}
	fmt.Printf("removed: %s\n", dst)
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Use ASCII "..." rather than U+2026 ellipsis so legacy Windows consoles
	// (without UTF-8 codepage active) don't render the truncation as `?`.
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}
