package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"bootenv/internal/btrfs"
	"bootenv/internal/grubgen"
	"bootenv/internal/snapstore"
)

var rootCmd = &cobra.Command{
	Use:   "bootenv",
	Short: "Manage btrfs boot environment snapshots",
	Long: `bootenv creates, lists, and manages btrfs subvolume snapshots
and keeps the GRUB menu in sync with them.`,
}

// Execute is the entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(
		newSnapshotCmd(),
		newGrubCmd(),
		newCleanupCmd(),
		newListCmd(),
		newDeleteCmd(),
		newRestoreCmd(),
	)
}

// guardSnapshot exits 0 with a message when running inside a snapshot environment.
func guardSnapshot() {
	inside, err := btrfs.IsInsideSnapshot()
	if err != nil {
		// Can't determine; proceed with a warning.
		fmt.Fprintln(os.Stderr, "warning: could not determine snapshot state:", err)
		return
	}
	if inside {
		fmt.Fprintln(os.Stderr, "Running inside snapshot environment; skipping.")
		os.Exit(0)
	}
}

// regenerateGrub regenerates the grub snippet and runs update-grub.
func regenerateGrub() error {
	rootUUID, err := btrfs.FindmntUUID("/")
	if err != nil {
		return fmt.Errorf("get root UUID: %w", err)
	}

	distro := grubgen.ReadDistro()
	snaps := grubgen.SnapInfoFromDir("/@snapshots/root")
	entries := grubgen.BuildEntries(snaps, distro, rootUUID)

	if err := grubgen.Generate(entries); err != nil {
		return err
	}

	fmt.Println("Running update-grub...")
	out, err := exec.Command("update-grub").CombinedOutput()
	if err != nil {
		return fmt.Errorf("update-grub: %w\n%s", err, out)
	}
	return nil
}

// snapshotEntries converts snapstore.Entry slice to grubgen.SnapInfo slice.
func snapstoreToGrubSnaps(entries []snapstore.Entry) []grubgen.SnapInfo {
	out := make([]grubgen.SnapInfo, len(entries))
	for i, e := range entries {
		out[i] = grubgen.SnapInfo{
			Name:      e.Name,
			Kind:      e.Kind,
			KernelVer: e.KernelVer,
			RootPath:  e.RootPath,
		}
	}
	return out
}
