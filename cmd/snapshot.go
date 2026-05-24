package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"bootenv/internal/btrfs"
)

func newSnapshotCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot <auto|manual> [name]",
		Short: "Create a btrfs snapshot of / and /home",
		Long: `Create a read-write btrfs snapshot of the root and home subvolumes.

  bootenv snapshot auto            Timestamp-named snapshot in the auto pool
  bootenv snapshot manual <name>   Named snapshot in the manual pool`,
		Args:    cobra.RangeArgs(1, 2),
		RunE:    runSnapshot,
		Example: "  bootenv snapshot auto\n  bootenv snapshot manual before-upgrade",
	}
	return cmd
}

func runSnapshot(cmd *cobra.Command, args []string) error {
	guardSnapshot()

	mode := args[0]

	var rootTarget, homeTarget string

	switch mode {
	case "auto":
		ts := time.Now().Format("2006-01-02_1504")
		rootTarget = filepath.Join("/@snapshots/root/auto", ts)
		homeTarget = filepath.Join("/@snapshots/home/auto", ts)

	case "manual":
		if len(args) < 2 || args[1] == "" {
			return fmt.Errorf("usage: bootenv snapshot manual <name>")
		}
		name := args[1]
		rootTarget = filepath.Join("/@snapshots/root/manual", name)
		homeTarget = filepath.Join("/@snapshots/home/manual", name)

	default:
		return fmt.Errorf("unknown mode %q — use auto or manual", mode)
	}

	fmt.Printf("Snapshotting / → %s\n", rootTarget)
	if err := btrfs.Snapshot("/", rootTarget); err != nil {
		return err
	}

	kver, err := btrfs.KernelVersion()
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not determine kernel version:", err)
	} else {
		marker := filepath.Join(rootTarget, ".bootenv-kernel")
		if err := os.WriteFile(marker, []byte(kver+"\n"), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not write .bootenv-kernel: %v\n", err)
		}
	}

	fmt.Printf("Snapshotting /home → %s\n", homeTarget)
	if err := btrfs.Snapshot("/home", homeTarget); err != nil {
		return err
	}

	fmt.Println("Created:")
	fmt.Println(" ", rootTarget)
	fmt.Println(" ", homeTarget)

	return regenerateGrub()
}
