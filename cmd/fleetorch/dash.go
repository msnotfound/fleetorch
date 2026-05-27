package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

func newDashCmdReal() *cobra.Command {
	return &cobra.Command{
		Use:   "dash",
		Short: "Auto-refreshing dashboard (v0.1: simple table refresh; bubbletea TUI in v0.2)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return doDash()
		},
	}
}

func doDash() error {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	clear := func() { fmt.Print("\033[H\033[2J") }
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()

	for {
		clear()
		fmt.Println("fleetorch dash — press Ctrl-C to exit")
		fmt.Println()
		if err := doList(); err != nil {
			fmt.Fprintln(os.Stderr, "list error:", err)
		}
		select {
		case <-tick.C:
		case <-sigCh:
			fmt.Println()
			return nil
		}
	}
}
