package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/msnotfound/fleetorch/internal/config"
)

func newConfigCmdReal() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "View or edit fleetorch config paths",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "show",
			Short: "Print resolved config paths",
			RunE: func(cmd *cobra.Command, args []string) error {
				p, err := config.Resolve()
				if err != nil {
					return err
				}
				fmt.Printf("config_dir:    %s\n", p.ConfigDir)
				fmt.Printf("data_dir:      %s\n", p.DataDir)
				fmt.Printf("agents_dir:    %s\n", p.AgentsDir)
				fmt.Printf("worktree_dir:  %s\n", p.WorktreeDir)
				fmt.Printf("log_dir:       %s\n", p.LogDir)
				fmt.Printf("socket_dir:    %s\n", p.SocketDir)
				fmt.Printf("state_file:    %s\n", p.StateFile)
				fmt.Printf("config_file:   %s\n", p.ConfigFile)
				return nil
			},
		},
		&cobra.Command{
			Use:   "edit",
			Short: "Open config file in $EDITOR",
			RunE: func(cmd *cobra.Command, args []string) error {
				p, err := config.Resolve()
				if err != nil {
					return err
				}
				if err := p.EnsureDirs(); err != nil {
					return err
				}
				if _, err := os.Stat(p.ConfigFile); os.IsNotExist(err) {
					if err := os.WriteFile(p.ConfigFile, []byte("# fleetorch config — currently empty (defaults apply)\n"), 0o644); err != nil {
						return err
					}
				}
				c, err := editorCommand(p.ConfigFile)
				if err != nil {
					return err
				}
				c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
				return c.Run()
			},
		},
	)
	return cmd
}
