package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"bootenv/internal/btrfs"
	"bootenv/internal/config"
	"bootenv/internal/snapstore"
)

func newCleanupCmd() *cobra.Command {
	var keep int
	var dryRun bool
	var target string

	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Prune old auto snapshots, keeping the N most recent per target",
		Long: `Deletes the oldest auto snapshots that exceed the keep limit for each target.

Keep limits come from each target's keep_auto in the config file.
--keep overrides the limit for every targeted pool in one shot.
Manual snapshots are never pruned automatically.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			keepOverride := -1
			if cmd.Flags().Changed("keep") {
				keepOverride = keep
			}
			return runCleanup(keepOverride, dryRun, target)
		},
		Example: "  bootenv cleanup\n" +
			"  bootenv cleanup --keep 5\n" +
			"  bootenv cleanup --target root\n" +
			"  bootenv cleanup -n",
	}

	cmd.Flags().IntVarP(&keep, "keep", "k", 10,
		"override keep limit for all targeted pools (default: from config keep_auto)")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false,
		"show what would be deleted without removing anything")
	addTargetFlag(cmd, &target)
	return cmd
}

func runCleanup(keepOverride int, dryRun bool, targetFlag string) error {
	logEvent(os.Args)
	guardSnapshot()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	targets, err := targetsForFlag(cfg, targetFlag)
	if err != nil {
		return err
	}

	rootDeleted := 0

	for _, tname := range sortedTargetNames(targets) {
		tc := targets[tname]

		keepVal := tc.GetKeepAuto()
		if keepOverride >= 0 {
			keepVal = keepOverride
		}

		entries, err := snapstore.ListFromDir(config.SnapshotDirFor(tname), tname, tc.Source, "auto")
		if err != nil {
			return fmt.Errorf("listing %s auto snapshots: %w", tname, err)
		}

		toPrune := snapstore.SelectForPrune(entries, keepVal)

		deleted, err := prunePool(tname, entries, toPrune, keepVal, dryRun)
		if err != nil {
			return err
		}
		if tname == "root" {
			rootDeleted = deleted
		}
	}

	if !dryRun && rootDeleted > 0 {
		fmt.Println()
		return regenerateGrub()
	}
	return nil
}

// prunePool prints a summary and, unless dryRun, deletes the snapshot
// subvolumes in toPrune. Returns the number of successful deletions.
func prunePool(
	label string,
	all, toPrune []snapstore.Entry,
	keep int,
	dryRun bool,
) (int, error) {
	if len(toPrune) == 0 {
		fmt.Printf("[%s] %d auto snapshot(s) — nothing to prune (keep=%d)\n",
			label, len(all), keep)
		return 0, nil
	}

	fmt.Printf("[%s] %d auto snapshot(s), keeping %d, pruning %d:\n",
		label, len(all), keep, len(toPrune))

	for _, e := range toPrune {
		created := "-"
		if !e.CreatedAt.IsZero() {
			created = e.CreatedAt.Format("2006-01-02 15:04:05")
		}
		action := "would delete"
		if !dryRun {
			action = "deleting   "
		}
		fmt.Printf("  %s  %s  (created %s)\n", action, e.Name, created)
	}

	if dryRun {
		fmt.Printf("  → dry-run, nothing removed.\n")
		return 0, nil
	}

	fmt.Println()
	deleted := 0
	for _, e := range toPrune {
		fmt.Printf("  Removing: %s\n", e.Path)
		if err := btrfs.Delete(e.Path); err != nil {
			fmt.Fprintf(os.Stderr, "  error: %v\n", err)
			continue
		}
		deleted++
	}
	fmt.Printf("  Deleted %d of %d.\n\n", deleted, len(toPrune))
	return deleted, nil
}
