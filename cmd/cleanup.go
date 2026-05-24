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
		Short: "Prune old auto snapshots, keeping the N most recent",
		Long: `Deletes the oldest auto snapshots that exceed the keep limit for each target.

Keep limits are read from the config file ([root] keep_auto and [home] keep_auto).
--keep overrides both targets with a single value when explicitly provided.

Use --target to restrict cleanup to one subvolume pool.
Use --dry-run to preview what would be removed without deleting anything.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			keepOverride := -1
			if cmd.Flags().Changed("keep") {
				keepOverride = keep
			}
			return runCleanup(keepOverride, dryRun, target)
		},
		Example: "  bootenv cleanup\n  bootenv cleanup --keep 5\n  bootenv cleanup --target root\n  bootenv cleanup -n",
	}

	cmd.Flags().IntVarP(&keep, "keep", "k", 10,
		"override keep limit for all targets (default from config)")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false,
		"show what would be deleted without removing anything")
	addTargetFlag(cmd, &target, "both")
	return cmd
}

func runCleanup(keepOverride int, dryRun bool, target string) error {
	guardSnapshot()

	wantRoot, wantHome, err := parseTarget(target)
	if err != nil {
		return err
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	rootKeep := cfg.Root.KeepAuto
	homeKeep := cfg.Home.KeepAuto
	if keepOverride >= 0 {
		rootKeep = keepOverride
		homeKeep = keepOverride
	}

	// Fetch once; filter per pool below.
	allAuto, err := snapstore.ListFiltered("auto")
	if err != nil {
		return fmt.Errorf("listing snapshots: %w", err)
	}

	rootDeleted := 0

	if wantRoot {
		rootEntries := snapstore.FilterTarget(allAuto, "root")
		toPrune := snapstore.SelectForPrune(rootEntries, rootKeep)
		rootDeleted, err = prunePool("root", rootEntries, toPrune, rootKeep, dryRun,
			func(e snapstore.Entry) string { return e.RootPath })
		if err != nil {
			return err
		}
	}

	if wantHome {
		homeEntries := snapstore.FilterTarget(allAuto, "home")
		toPrune := snapstore.SelectForPrune(homeEntries, homeKeep)
		if _, err = prunePool("home", homeEntries, toPrune, homeKeep, dryRun,
			func(e snapstore.Entry) string { return e.HomePath }); err != nil {
			return err
		}
	}

	if !dryRun && rootDeleted > 0 {
		fmt.Println()
		return regenerateGrub()
	}
	return nil
}

// prunePool prints a summary and, unless dryRun, deletes the subvolume paths
// returned by pathFn for each entry in toPrune.
// Returns the number of successful deletions.
func prunePool(
	label string,
	all, toPrune []snapstore.Entry,
	keep int,
	dryRun bool,
	pathFn func(snapstore.Entry) string,
) (int, error) {
	if len(toPrune) == 0 {
		fmt.Printf("[%s] %d auto snapshot(s) — nothing to prune (keep=%d)\n",
			label, len(all), keep)
		return 0, nil
	}

	fmt.Printf("[%s] %d auto snapshot(s), keeping %d, pruning %d:\n",
		label, len(all), keep, len(toPrune))

	for _, e := range toPrune {
		created := "(unknown)"
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
		path := pathFn(e)
		fmt.Printf("  Removing: %s\n", path)
		if err := btrfs.Delete(path); err != nil {
			fmt.Fprintf(os.Stderr, "  error: %v\n", err)
			continue
		}
		deleted++
	}
	fmt.Printf("  Deleted %d of %d.\n\n", deleted, len(toPrune))
	return deleted, nil
}
