package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"bootenv/internal/btrfs"
	"bootenv/internal/config"
	"bootenv/internal/snapstore"
)

func newRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore [name]",
		Short: "Promote a root snapshot to the live root subvolume (/@)",
		Long: `Replaces the current /@ subvolume with the named root snapshot.

If no name is given, the currently booted snapshot is used. This is useful
when you have booted into a snapshot via the GRUB menu and want to make it
the permanent root. If you are already running from the real root (/@) and
no name is given, an error is returned.

Steps:
  1. Mount the btrfs top-level volume at /run/bootenv/mnt
  2. Rename the current /@ to /@-pre-restore-<timestamp> (the safety backup)
  3. Snapshot /@snapshots/root/<kind>/<name> → /@
  4. Regenerate GRUB menu

The renamed old root (/@-pre-restore-<timestamp>) remains at the btrfs
top-level. To remove it later:
  sudo mount -o subvolid=5 /dev/<device> /run/bootenv/mnt
  sudo btrfs subvolume delete /run/bootenv/mnt/@-pre-restore-<timestamp>
  sudo umount /run/bootenv/mnt

A reboot is required for the restored environment to take effect.
/home and other non-root targets are left untouched.`,
		Args:    cobra.RangeArgs(0, 1),
		RunE:    runRestore,
		Example: "  bootenv restore before-upgrade\n  bootenv restore",
	}
}

func runRestore(_ *cobra.Command, args []string) error {
	logEvent(os.Args)

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	rootTC, ok := cfg.Targets["root"]
	if !ok {
		return fmt.Errorf("root target not found in config")
	}

	var name string

	if len(args) == 1 {
		name = args[0]
	} else {
		// No name given — use the currently booted snapshot.
		inside, err := btrfs.IsInsideSnapshot()
		if err != nil {
			return fmt.Errorf("detecting current boot environment: %w", err)
		}
		if !inside {
			return fmt.Errorf("currently booted from the real root (/@), not a snapshot\n" +
				"  specify a snapshot name: bootenv restore <name>")
		}

		subvol, err := btrfs.CurrentRootSubvol()
		if err != nil {
			return fmt.Errorf("reading current root subvolume: %w", err)
		}
		name = filepath.Base(subvol)
		fmt.Printf("Auto-detected currently booted snapshot: %s\n\n", name)
	}

	entry, err := snapstore.ResolveInTarget(config.SnapshotDirFor("root"), "root", rootTC.Source, name)
	if err != nil {
		return err
	}

	// Confirmation prompt — this is a destructive operation.
	fmt.Printf("About to promote snapshot %q to the live root (/@).\n", name)
	fmt.Printf("The current /@ will be renamed to /@-pre-restore-<timestamp> as a safety backup.\n\n")
	fmt.Print("Are you sure you want to continue? [y/N] ")

	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading confirmation: %w", err)
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		fmt.Println("Aborted.")
		return nil
	}
	fmt.Println()

	// Step 1 — mount the btrfs top-level so /@ is reachable as an OS path.
	fmt.Printf("Step 1/4 — Mounting btrfs top-level at %s\n", btrfs.DefaultTopMount)
	tv, err := btrfs.OpenTopVol("/", btrfs.DefaultTopMount)
	if err != nil {
		return fmt.Errorf("cannot access btrfs top-level: %w", err)
	}
	defer func() {
		if err := tv.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: unmount top-level: %v\n", err)
		}
	}()

	rootAt := tv.RootAt() // /run/bootenv/mnt/@

	// Step 2 — rename the current @ out of the way.
	// We rename rather than delete because @ may contain nested subvolumes
	// (like @snapshots) and btrfs refuses to delete a subvolume that has them.
	ts := time.Now().Format("2006-01-02_150405")
	backupName := fmt.Sprintf("@-pre-restore-%s", ts)
	backupAt := filepath.Join(tv.Mount, backupName)

	fmt.Printf("Step 2/4 — Renaming current /@ → /%s (safety backup)\n", backupName)
	if btrfs.SubvolumeExists(rootAt) {
		if err := os.Rename(rootAt, backupAt); err != nil {
			return fmt.Errorf("rename /@ → /%s failed: %w", backupName, err)
		}
	} else {
		fmt.Println("  (/@ already absent — resuming partial restore, skipping rename)")
	}

	// Step 3 — create the new @ from the chosen snapshot.
	// entry.Path (e.g. /@snapshots/root/manual/before-upgrade) is a normal OS
	// path inside the currently-mounted root subvolume.
	fmt.Printf("Step 3/4 — Restoring: %s → /@\n", entry.Path)
	if err := btrfs.Snapshot(entry.Path, rootAt); err != nil {
		return fmt.Errorf("restore snapshot failed: %w\n"+
			"  old root is preserved at: /%s\n"+
			"  to recover manually: btrfs subvolume snapshot %s/%s %s",
			err, backupName, tv.Mount, backupName, rootAt)
	}

	// Step 4 — regenerate GRUB.
	fmt.Println("Step 4/4 — Regenerating GRUB")
	if err := regenerateGrub(); err != nil {
		return err
	}

	fmt.Println()
	fmt.Printf("✓ Restored %q to /@\n", name)
	fmt.Printf("  Old root preserved at: /%s\n", backupName)
	fmt.Printf("  To delete it after confirming the restore works:\n")
	fmt.Printf("    sudo mount -o subvolid=5 $(findmnt -no SOURCE / | cut -d'[' -f1) %s\n", btrfs.DefaultTopMount)
	fmt.Printf("    sudo btrfs subvolume delete %s\n", backupAt)
	fmt.Printf("    sudo umount %s\n", btrfs.DefaultTopMount)
	fmt.Println("  Reboot to enter the restored environment.")
	return nil
}
