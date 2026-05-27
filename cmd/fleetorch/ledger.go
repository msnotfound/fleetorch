package main

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/store"
)

func newLedgerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ledger",
		Short: "Show cumulative spawn counts per agent type",
		RunE: func(cmd *cobra.Command, args []string) error {
			return doLedger()
		},
	}
}

func doLedger() error {
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	st := store.New(paths.StateFile)
	state, err := st.Load()
	if err != nil {
		return err
	}

	if len(state.Ledger) == 0 {
		fmt.Println("no spawns yet.")
		return nil
	}

	keys := make([]string, 0, len(state.Ledger))
	for k := range state.Ledger {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "AGENT\tSPAWNS")
	total := 0
	for _, k := range keys {
		v := state.Ledger[k]
		total += v
		fmt.Fprintf(w, "%s\t%d\n", k, v)
	}
	fmt.Fprintln(w, "---\t---")
	fmt.Fprintf(w, "TOTAL\t%d\n", total)
	return w.Flush()
}
