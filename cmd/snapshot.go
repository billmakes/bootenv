package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"bootenv/internal/btrfs"
	"bootenv/internal/config"
)

func newSnapshotCmd() *cobra.Command {
	var target string

	cmd := &cobra.Command{
		Use:   "snapshot <auto|manual> [name]",
		Short: "Create a btrfs snapshot for configured targets",
		Long: `Create a read-write btrfs snapshot for each configured target.

Without --target all configured targets are snapshotted.
Each target must reference a pre-existing btrfs subvolume (set via the
"source" field in the config file). Snapshot directories are created
automatically when they do not exist yet.

The "root" target receives a .bootenv-kernel marker file recording the
running kernel version. The GRUB menu is regenerated automatically when
the root target is snapshotted.`,
		Args: cobra.RangeArgs(1, 2),
		Example: "  bootenv snapshot auto\n" +
			"  bootenv snapshot auto --target root\n" +
			"  bootenv snapshot manual before-upgrade\n" +
			"  bootenv snapshot manual before-upgrade -T home",
		RunE: func(_ *cobra.Command, args []string) error {
			return runSnapshot(args, target)
		},
	}

	addTargetFlag(cmd, &target)
	return cmd
}

func runSnapshot(args []string, targetFlag string) error {
	guardSnapshot()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	targets, err := targetsForFlag(cfg, targetFlag)
	if err != nil {
		return err
	}

	mode := args[0]
	var snapName string

	switch mode {
	case "auto":
		// Includes seconds: names sort lexically in creation order and
		// back-to-back snapshots never produce the same name.
		snapName = time.Now().Format("2006-01-02_150405")
	case "manual":
		if len(args) < 2 || args[1] == "" {
			return fmt.Errorf("usage: bootenv snapshot manual <name>")
		}
		snapName = args[1]
	default:
		return fmt.Errorf("unknown mode %q — use auto or manual", mode)
	}

	rootSnapped := false

	for _, tname := range sortedTargetNames(targets) {
		tc := targets[tname]

		// Ensure the kind subdirectory exists (auto-creates on first use).
		kindDir := filepath.Join(config.SnapshotDirFor(tname), mode)
		if err := os.MkdirAll(kindDir, 0755); err != nil {
			return fmt.Errorf("create snapshot dir %s: %w", kindDir, err)
		}

		dst := filepath.Join(kindDir, snapName)
		fmt.Printf("Snapshotting %s → %s\n", tc.Source, dst)

		if err := btrfs.Snapshot(tc.Source, dst); err != nil {
			return err
		}

		// Record the exact creation time for every snapshot so that
		// "bootenv list" can display it correctly. (btrfs snapshot inherits
		// the source subvolume's mtime, not the wall-clock time.)
		now := time.Now()
		tsMarker := filepath.Join(dst, ".bootenv-created")
		if err := os.WriteFile(tsMarker, []byte(now.UTC().Format(time.RFC3339)+"\n"), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not write .bootenv-created: %v\n", err)
		}

		// Write the kernel version marker into root snapshots so grub and
		// list can display it without scanning lib/modules.
		if tname == "root" {
			kver, err := btrfs.KernelVersion()
			if err != nil {
				fmt.Fprintln(os.Stderr, "warning: could not determine kernel version:", err)
			} else {
				marker := filepath.Join(dst, ".bootenv-kernel")
				if err := os.WriteFile(marker, []byte(kver+"\n"), 0644); err != nil {
					fmt.Fprintf(os.Stderr, "warning: could not write .bootenv-kernel: %v\n", err)
				}
			}
			rootSnapped = true
		}

		fmt.Println(" ", dst)
	}

	if rootSnapped {
		return regenerateGrub()
	}
	return nil
}
