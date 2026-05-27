package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/msnotfound/fleetorch/internal/agents"
	"github.com/msnotfound/fleetorch/internal/config"
)

func newAgentCmdReal() *cobra.Command {
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
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	c := exec.Command(editor, target)
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

func doAgentAdd(src string) error {
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	if err := paths.EnsureDirs(); err != nil {
		return err
	}
	dst := filepath.Join(paths.AgentsDir, filepath.Base(src))

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
	return s[:n-1] + "…"
}
