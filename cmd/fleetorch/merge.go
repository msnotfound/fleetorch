package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// newMergeResolveCmd is a Go port of the bash-orcha-era auto_keep_both.py.
//
// For each git conflict block, replaces it with the concatenation of both
// sides (ours then theirs). Works for ~80% of conflicts where every branch
// adds a new line to a shared file (e.g. exports in packages/db/index.ts,
// nav items in Sidebar.tsx). Won't handle duplicate-line conflicts or
// non-additive changes — those need manual review.
func newMergeResolveCmd() *cobra.Command {
	var (
		dryRun  bool
		inPlace bool
	)
	cmd := &cobra.Command{
		Use:   "merge-resolve <file>...",
		Short: "Resolve git conflicts by concatenating both sides (additive only)",
		Long: `Replaces each <<<<<<<…=======…>>>>>>> block in the given files with
the concatenation of the two sides. Useful after merging waves of parallel
agent branches where every agent appends to the same shared file.

Will NOT handle:
- duplicate import/export lines (creates duplicate identifiers)
- non-additive changes (one side removes what the other adds)
- complex three-way conflicts

For those, edit by hand.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, path := range args {
				resolved, blocks, err := resolveFile(path)
				if err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				if blocks == 0 {
					fmt.Printf("%s: no conflict blocks found\n", path)
					continue
				}
				if dryRun {
					fmt.Printf("%s: would resolve %d block(s) (dry-run, no write)\n", path, blocks)
					continue
				}
				mode := os.O_WRONLY | os.O_TRUNC | os.O_CREATE
				if !inPlace {
					mode = os.O_WRONLY | os.O_CREATE | os.O_EXCL
					path = path + ".resolved"
				}
				f, err := os.OpenFile(path, mode, 0o644)
				if err != nil {
					return err
				}
				if _, err := f.WriteString(resolved); err != nil {
					_ = f.Close()
					return err
				}
				if err := f.Close(); err != nil {
					return err
				}
				fmt.Printf("%s: resolved %d block(s)\n", path, blocks)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&inPlace, "in-place", "i", true, "Rewrite the input file (default). Use --in-place=false to write <file>.resolved")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Don't write anything; just report block counts")
	return cmd
}

// resolveFile returns the resolved text and the number of blocks it merged.
//
// The state machine: in "normal" we copy lines through; on `<<<<<<<` we
// collect "ours" lines; on `=======` switch to "theirs"; on `>>>>>>>` emit
// ours then theirs and return to normal.
func resolveFile(path string) (string, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	const (
		stNormal = iota
		stOurs
		stTheirs
	)

	state := stNormal
	var out, ours, theirs strings.Builder
	blocks := 0

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "<<<<<<<") && state == stNormal:
			state = stOurs
			ours.Reset()
			theirs.Reset()
		case strings.HasPrefix(line, "=======") && state == stOurs:
			state = stTheirs
		case strings.HasPrefix(line, ">>>>>>>") && state == stTheirs:
			out.WriteString(ours.String())
			out.WriteString(theirs.String())
			blocks++
			state = stNormal
		default:
			switch state {
			case stNormal:
				out.WriteString(line)
				out.WriteByte('\n')
			case stOurs:
				ours.WriteString(line)
				ours.WriteByte('\n')
			case stTheirs:
				theirs.WriteString(line)
				theirs.WriteByte('\n')
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", 0, err
	}
	return out.String(), blocks, nil
}
