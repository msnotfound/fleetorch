// Package main is the fleetorch CLI entrypoint.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
	}

	root.AddCommand(
		newSpawnCmd(),
		newListCmd(),
		newWatchCmd(),
		newAttachCmd(),
		newKillCmd(),
		newDashCmd(),
		newLogsCmd(),
		newAgentCmd(),
		newConfigCmd(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
