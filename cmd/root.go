package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"bootenv/internal/btrfs"
	"bootenv/internal/config"
	"bootenv/internal/grubgen"
)

// cfgPath is set by the persistent --config flag and read by all subcommands.
var cfgPath string

var rootCmd = &cobra.Command{
	Use:   "bootenv",
	Short: "Manage btrfs boot environment snapshots",
	Long: `bootenv creates, lists, and manages btrfs subvolume snapshots
and keeps the GRUB menu in sync with them.

Targets are defined in the config file. The "root" target is always present.
Additional targets (e.g. [home]) can be added to the config.`,
}

// Execute is the entry point called from main.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", config.DefaultPath,
		"path to bootenv TOML config file")

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
		fmt.Fprintln(os.Stderr, "warning: could not determine snapshot state:", err)
		return
	}
	if inside {
		fmt.Fprintln(os.Stderr, "Running inside snapshot environment; skipping.")
		os.Exit(0)
	}
}

// regenerateGrubFn is the seam called by every command that needs to refresh
// the GRUB menu. Tests swap it for a no-op to avoid invoking update-grub.
var regenerateGrubFn = realRegenerateGrub

// regenerateGrub regenerates the GRUB snippet and runs update-grub.
func regenerateGrub() error { return regenerateGrubFn() }

// realRegenerateGrub reads the root target's SnapshotDir from the config so the
// path is always consistent with what the config defines.
func realRegenerateGrub() error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if _, ok := cfg.Targets["root"]; !ok {
		return fmt.Errorf("root target not found in config")
	}

	rootUUID, err := btrfs.FindmntUUID("/")
	if err != nil {
		return fmt.Errorf("get root UUID: %w", err)
	}

	distro := grubgen.ReadDistro()
	snaps := grubgen.SnapInfoFromDir(config.SnapshotDirFor("root"))
	entries := grubgen.BuildEntries(snaps, distro, rootUUID)

	if err := grubgen.Generate(entries); err != nil {
		return err
	}

	grubBin, err := findUpdateGrub()
	if err != nil {
		return fmt.Errorf("locating update-grub: %w", err)
	}
	fmt.Printf("Running %s...\n", grubBin)
	out, err := exec.Command(grubBin).CombinedOutput()
	if err != nil {
		return fmt.Errorf("update-grub: %w\n%s", err, out)
	}
	return nil
}

// updateGrubSearchPaths lists absolute locations checked before falling back
// to $PATH. Needed because cron strips $PATH down to /usr/bin:/bin, which
// excludes /usr/sbin where update-grub actually lives on Debian-likes.
var updateGrubSearchPaths = []string{
	"/usr/local/sbin/update-grub",
	"/usr/sbin/update-grub",
	"/sbin/update-grub",
}

// findUpdateGrub returns an absolute path to update-grub, preferring the
// standard sbin locations over $PATH.
func findUpdateGrub() (string, error) {
	for _, p := range updateGrubSearchPaths {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return exec.LookPath("update-grub")
}
