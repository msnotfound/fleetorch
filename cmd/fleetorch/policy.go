package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/msnotfound/fleetorch/internal/config"
	"github.com/msnotfound/fleetorch/internal/store"
	"github.com/msnotfound/fleetorch/internal/types"
)

func newPolicyCmdReal() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Manage spawn policy caps",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Print current policy caps and live usage",
		RunE:  runPolicyShow,
	})
	return cmd
}

func runPolicyShow(_ *cobra.Command, _ []string) error {
	paths, err := config.Resolve()
	if err != nil {
		return err
	}
	cfg, err := paths.LoadConfig()
	if err != nil {
		return err
	}
	pol := cfg.Policy

	st, err := store.New(paths.StateFile).Load()
	if err != nil {
		return err
	}

	running := 0
	perAgent := map[string]int{}
	spendLastHour := 0.0
	spendLastDay := 0.0
	now := time.Now()

	for _, t := range st.Tasks {
		isRunning := t.Status == types.StatusRunning ||
			t.Status == types.StatusActive ||
			t.Status == types.StatusIdle
		if isRunning {
			running++
			perAgent[t.Agent]++
		}
		age := now.Sub(t.StartedAt)
		if age <= time.Hour {
			spendLastHour += t.BudgetUSD
		}
		if age <= 24*time.Hour {
			spendLastDay += t.BudgetUSD
		}
	}

	if pol.MaxConcurrentTotal == 0 {
		fmt.Printf("Concurrent:      %d / unlimited\n", running)
	} else {
		fmt.Printf("Concurrent:      %d / %d\n", running, pol.MaxConcurrentTotal)
	}

	for ag, cnt := range perAgent {
		if pol.MaxConcurrentPerAgent == 0 {
			fmt.Printf("Per-agent %-12s %d / unlimited\n", ag+":", cnt)
		} else {
			fmt.Printf("Per-agent %-12s %d / %d\n", ag+":", cnt, pol.MaxConcurrentPerAgent)
		}
	}

	if pol.MaxSpendUSDPerHour == 0 {
		fmt.Printf("Spend last  1h:  $%.2f / unlimited\n", spendLastHour)
	} else {
		fmt.Printf("Spend last  1h:  $%.2f / $%.2f\n", spendLastHour, pol.MaxSpendUSDPerHour)
	}

	if pol.MaxSpendUSDPerDay == 0 {
		fmt.Printf("Spend last 24h:  $%.2f / unlimited\n", spendLastDay)
	} else {
		fmt.Printf("Spend last 24h:  $%.2f / $%.2f\n", spendLastDay, pol.MaxSpendUSDPerDay)
	}

	return nil
}
