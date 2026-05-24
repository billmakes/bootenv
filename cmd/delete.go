package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"bootenv/internal/btrfs"
	"bootenv/internal/config"
	"bootenv/internal/snapstore"
)

func newDeleteCmd() *cobra.Command {
	var target string

	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a snapshot by name",
		Long: `Deletes the named snapshot from all configured targets (or just --target).

The GRUB menu is regenerated only when the root target's snapshot is removed.`,
		Args: cobra.ExactArgs(1),
		Example: "  bootenv delete 2025-05-20_090132\n" +
			"  bootenv delete before-upgrade\n" +
			"  bootenv delete before-upgrade --target root",
		RunE: func(_ *cobra.Command, args []string) error {
			return runDelete(args[0], target)
		},
	}

	addTargetFlag(cmd, &target)
	return cmd
}

func runDelete(name, targetFlag string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	targets, err := targetsForFlag(cfg, targetFlag)
	if err != nil {
		return err
	}

	found, err := snapstore.ResolveAll(targets, name)
	if err != nil {
		return err
	}

	rootDeleted := false
	for _, e := range found {
		fmt.Printf("Deleting [%s] %s\n", e.Target, e.Path)
		if err := btrfs.Delete(e.Path); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			continue
		}
		if e.Target == "root" {
			rootDeleted = true
		}
	}

	if rootDeleted {
		return regenerateGrub()
	}
	return nil
}
