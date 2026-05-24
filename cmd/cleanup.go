package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
)

func newCleanupCmd() *cobra.Command {
	var keep int

	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Show which auto snapshots would be pruned (dry-run)",
		Long: `Dry-run: lists the oldest auto snapshots that exceed the --keep limit.
No snapshots are actually deleted. This mirrors the original bootenv-cleanup
behaviour which also did not delete anything.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runCleanup(keep)
		},
	}

	cmd.Flags().IntVarP(&keep, "keep", "k", 10, "number of auto snapshots to keep")
	return cmd
}

func runCleanup(keep int) error {
	guardSnapshot()

	pairs := []struct{ label, dir string }{
		{"root auto", "/@snapshots/root/auto"},
		{"home auto", "/@snapshots/home/auto"},
	}

	totalWouldDelete := 0

	for _, p := range pairs {
		entries, err := readSortedDirs(p.dir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("[%s] directory does not exist, skipping\n", p.label)
				continue
			}
			return fmt.Errorf("reading %s: %w", p.dir, err)
		}

		if len(entries) <= keep {
			fmt.Printf("[%s] %d snapshots — nothing to prune (keep=%d)\n",
				p.label, len(entries), keep)
			continue
		}

		excess := len(entries) - keep
		toRemove := entries[:excess]
		fmt.Printf("[%s] %d snapshots, would delete %d (keep=%d):\n",
			p.label, len(entries), excess, keep)
		for _, name := range toRemove {
			fmt.Printf("  would delete: %s\n", filepath.Join(p.dir, name))
		}
		totalWouldDelete += excess
	}

	if totalWouldDelete > 0 {
		fmt.Printf("\n%d snapshot(s) would be deleted (dry-run — nothing was removed).\n",
			totalWouldDelete)
		fmt.Println("Use `bootenv delete <name>` to remove individual snapshots.")
	}
	return nil
}

// readSortedDirs returns directory entry names sorted ascending.
func readSortedDirs(dir string) ([]string, error) {
	fis, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, fi := range fis {
		if fi.IsDir() {
			names = append(names, fi.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}
