package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"bootenv/internal/btrfs"
	"bootenv/internal/snapstore"
)

func newDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a snapshot by name",
		Long: `Deletes both the root and home subvolumes for the named snapshot,
then regenerates the GRUB menu.`,
		Args:    cobra.ExactArgs(1),
		RunE:    runDelete,
		Example: "  bootenv delete 2025-05-20_0901\n  bootenv delete before-upgrade",
	}
}

func runDelete(_ *cobra.Command, args []string) error {
	name := args[0]

	entry, err := snapstore.Resolve(name)
	if err != nil {
		return err
	}

	fmt.Printf("Deleting root snapshot: %s\n", entry.RootPath)
	if err := btrfs.Delete(entry.RootPath); err != nil {
		return err
	}

	fmt.Printf("Deleting home snapshot: %s\n", entry.HomePath)
	if err := btrfs.Delete(entry.HomePath); err != nil {
		// Home counterpart may not exist in all setups — warn but continue.
		fmt.Fprintf(os.Stderr, "warning: could not delete home snapshot: %v\n", err)
	}

	return regenerateGrub()
}
