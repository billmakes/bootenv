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
	var target string

	cmd := &cobra.Command{
		Use:   "snapshot <auto|manual> [name]",
		Short: "Create a btrfs snapshot of / and/or /home",
		Long: `Create a read-write btrfs snapshot of the root and/or home subvolumes.

  bootenv snapshot auto                     Both root and home (default)
  bootenv snapshot auto --target root       Root only
  bootenv snapshot auto --target home       Home only
  bootenv snapshot manual <name>            Both root and home (default)
  bootenv snapshot manual <name> -T root    Root only`,
		Args:    cobra.RangeArgs(1, 2),
		Example: "  bootenv snapshot auto\n  bootenv snapshot auto --target root\n  bootenv snapshot manual before-upgrade",
		RunE: func(_ *cobra.Command, args []string) error {
			return runSnapshot(args, target)
		},
	}

	addTargetFlag(cmd, &target, "both")
	return cmd
}

func runSnapshot(args []string, target string) error {
	wantRoot, wantHome, err := parseTarget(target)
	if err != nil {
		return err
	}

	guardSnapshot()

	mode := args[0]

	var rootTarget, homeTarget string
	switch mode {
	case "auto":
		// Format includes seconds: names are sortable in creation order and
		// back-to-back snapshots never collide.
		ts := time.Now().Format("2006-01-02_150405")
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

	if wantRoot {
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
	}

	if wantHome {
		fmt.Printf("Snapshotting /home → %s\n", homeTarget)
		if err := btrfs.Snapshot("/home", homeTarget); err != nil {
			return err
		}
	}

	fmt.Println("Created:")
	if wantRoot {
		fmt.Println(" ", rootTarget)
	}
	if wantHome {
		fmt.Println(" ", homeTarget)
	}

	if wantRoot {
		return regenerateGrub()
	}
	return nil
}
