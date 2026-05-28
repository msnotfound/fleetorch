package main

import (
	"github.com/spf13/cobra"
)

func newWatchCmdReal() *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "watch <task-id>",
		Short: "Print the recent output of a task (alias of `logs`; --follow tails)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if follow {
				return doAttach(args[0], true)
			}
			return doLogs(args[0], false, false)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Tail the log (same as `attach`)")
	return cmd
}
