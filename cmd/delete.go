package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"bootenv/internal/btrfs"
	"bootenv/internal/snapstore"
)

func newDeleteCmd() *cobra.Command {
	var target string

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a snapshot by name",
		Long: `Deletes the root and/or home subvolumes for the named snapshot,
then regenerates the GRUB menu (only when the root subvolume is removed).

Use --target to delete only one side of the pair.`,
		Args:    cobra.ExactArgs(1),
		Example: "  bootenv delete 2025-05-20_090132\n  bootenv delete before-upgrade\n  bootenv delete before-upgrade --target root",
		RunE: func(_ *cobra.Command, args []string) error {
			return runDelete(args[0], target)
		},
	}

	addTargetFlag(cmd, &target, "both")
	return cmd
}

func runDelete(name, target string) error {
	wantRoot, wantHome, err := parseTarget(target)
	if err != nil {
		return err
	}

	entry, err := snapstore.Resolve(name)
	if err != nil {
		return err
	}

	rootDeleted := false

	if wantRoot {
		if entry.HasRoot {
			fmt.Printf("Deleting root snapshot: %s\n", entry.RootPath)
			if err := btrfs.Delete(entry.RootPath); err != nil {
				return err
			}
			rootDeleted = true
		} else {
			fmt.Printf("Root snapshot %s does not exist, skipping.\n", entry.RootPath)
		}
	}

	if wantHome {
		if entry.HasHome {
			fmt.Printf("Deleting home snapshot: %s\n", entry.HomePath)
			if err := btrfs.Delete(entry.HomePath); err != nil {
				// Home counterpart may not exist in all setups — warn but continue.
				fmt.Fprintf(os.Stderr, "warning: could not delete home snapshot: %v\n", err)
			}
		} else {
			fmt.Printf("Home snapshot %s does not exist, skipping.\n", entry.HomePath)
		}
	}

	// Only regenerate GRUB if the root subvolume was removed, since home
	// snapshots are not referenced in the GRUB menu.
	if rootDeleted {
		return regenerateGrub()
	}
	return nil
}
