package cmd

import (
	"github.com/spf13/cobra"
)

func newGrubCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "grub",
		Short: "Regenerate /etc/grub.d/42_bootenv_snapshots and run update-grub",
		Long: `Scans all snapshots under /@snapshots/root/{auto,manual} and writes
a GRUB script with one menuentry per snapshot, then calls update-grub.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return regenerateGrub()
		},
	}
}
