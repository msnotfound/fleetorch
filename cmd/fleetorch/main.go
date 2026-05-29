// Package main is the fleetorch CLI entrypoint.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Version is set at build time via -ldflags.
var Version = "dev"

func main() {
	root := &cobra.Command{
		Use:           "fleetorch",
		Short:         "A fleet of orchestrated AI coding agents — in one binary.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && term.IsTerminal(int(os.Stdout.Fd())) {
				return launchTour()
			}
			return cmd.Help()
		},
	}

	root.AddCommand(
		newSpawnCmdReal(),
		newListCmdReal(),
		newWatchCmdReal(),
		newAttachCmdReal(),
		newKillCmdReal(),
		newDashCmdReal(),
		newLogsCmdReal(),
		newAgentCmdReal(),
		newConfigCmdReal(),
		newLedgerCmd(),
		newMergeResolveCmd(),
		newUpgradeCmd(),
		newMonitorCmd(),
		newPruneCmd(),
		newDoctorCmd(),
		newWorkerCmd(),
		newPolicyCmdReal(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
