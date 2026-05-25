package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bootenv/internal/btrfs"
	"bootenv/internal/config"
)

// newSnapshotFixture points SnapshotBase + cfgPath at a temp tree and stubs
// guardSnapshot (via IsInsideSnapshot=false). Returns the temp dir.
func newSnapshotFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prevBase := config.SnapshotBase
	config.SnapshotBase = filepath.Join(dir, "@snapshots")
	prevCfgPath := cfgPath
	cfgPath = filepath.Join(dir, "missing.toml")
	t.Cleanup(func() {
		config.SnapshotBase = prevBase
		cfgPath = prevCfgPath
	})
	return dir
}

// ----------------------------- snapshot --------------------------------------

func TestSnapshot_Auto_Root_WritesMarkersAndRegeneratesGrub(t *testing.T) {
	newSnapshotFixture(t)
	grubCalls := stubGrub(t)

	snapshotCalls := 0
	defer btrfs.WithFakes(btrfs.Fakes{
		IsInsideSnapshot: func() (bool, error) { return false, nil },
		KernelVersion:    func() (string, error) { return "6.1.0-test-amd64", nil },
		Snapshot: func(src, dst string) error {
			snapshotCalls++
			if src != "/" {
				t.Errorf("Snapshot src = %q, want %q", src, "/")
			}
			// Pretend the btrfs snapshot created the destination directory.
			return os.MkdirAll(dst, 0o755)
		},
	})()

	if err := runSnapshot([]string{"auto"}, "root"); err != nil {
		t.Fatalf("runSnapshot: %v", err)
	}
	if snapshotCalls != 1 {
		t.Fatalf("Snapshot called %d times, want 1", snapshotCalls)
	}
	if *grubCalls != 1 {
		t.Errorf("regenerateGrub called %d times, want 1", *grubCalls)
	}

	// One snapshot dir under root/auto with marker files.
	autoDir := filepath.Join(config.SnapshotBase, "root", "auto")
	entries, err := os.ReadDir(autoDir)
	if err != nil {
		t.Fatalf("read autoDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(entries))
	}
	snapPath := filepath.Join(autoDir, entries[0].Name())

	tsData, err := os.ReadFile(filepath.Join(snapPath, ".bootenv-created"))
	if err != nil {
		t.Errorf(".bootenv-created missing: %v", err)
	} else if _, err := time.Parse(time.RFC3339, strings.TrimSpace(string(tsData))); err != nil {
		t.Errorf(".bootenv-created not RFC3339: %v", err)
	}

	kver, err := os.ReadFile(filepath.Join(snapPath, ".bootenv-kernel"))
	if err != nil {
		t.Errorf(".bootenv-kernel missing: %v", err)
	} else if strings.TrimSpace(string(kver)) != "6.1.0-test-amd64" {
		t.Errorf(".bootenv-kernel = %q, want stub value", string(kver))
	}
}

func TestSnapshot_BtrfsFailure_BailsOutAndSkipsGrub(t *testing.T) {
	newSnapshotFixture(t)
	grubCalls := stubGrub(t)

	defer btrfs.WithFakes(btrfs.Fakes{
		IsInsideSnapshot: func() (bool, error) { return false, nil },
		KernelVersion:    func() (string, error) { return "6.1.0-test-amd64", nil },
		Snapshot: func(src, dst string) error {
			return fmt.Errorf("simulated failure")
		},
	})()

	err := runSnapshot([]string{"manual", "before-upgrade"}, "root")
	if err == nil {
		t.Fatal("expected error from failed snapshot, got nil")
	}
	if *grubCalls != 0 {
		t.Errorf("grub should not regen on failure; got %d calls", *grubCalls)
	}
}

// ----------------------------- cleanup ---------------------------------------

// seedAutoSnapshots creates n auto snapshot dirs under root/auto with .bootenv-created
// markers spaced 1 hour apart (oldest first) so SelectForPrune orders them deterministically.
func seedAutoSnapshots(t *testing.T, n int) []string {
	t.Helper()
	autoDir := filepath.Join(config.SnapshotBase, "root", "auto")
	if err := os.MkdirAll(autoDir, 0o755); err != nil {
		t.Fatalf("mkdir autoDir: %v", err)
	}
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	var paths []string
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("2025-01-%02d_000000", i+1)
		p := filepath.Join(autoDir, name)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir snap: %v", err)
		}
		ts := base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339)
		if err := os.WriteFile(filepath.Join(p, ".bootenv-created"), []byte(ts+"\n"), 0o644); err != nil {
			t.Fatalf("write marker: %v", err)
		}
		paths = append(paths, p)
	}
	return paths
}

func TestCleanup_ContinuesAfterPartialDeleteFailure(t *testing.T) {
	newSnapshotFixture(t)
	grubCalls := stubGrub(t)
	paths := seedAutoSnapshots(t, 5)
	_ = paths

	// keep=2 → 3 oldest will be selected for prune. Fail the 2nd of those.
	deleteCalls := 0
	var deletedPaths []string
	defer btrfs.WithFakes(btrfs.Fakes{
		IsInsideSnapshot: func() (bool, error) { return false, nil },
		Delete: func(p string) error {
			deleteCalls++
			if deleteCalls == 2 {
				return fmt.Errorf("simulated delete failure for %s", p)
			}
			deletedPaths = append(deletedPaths, p)
			return os.RemoveAll(p)
		},
	})()

	if err := runCleanup(2, false, "root"); err != nil {
		t.Fatalf("runCleanup returned err: %v", err)
	}
	if deleteCalls != 3 {
		t.Errorf("Delete called %d times, want 3 (loop must not abort on individual failure)", deleteCalls)
	}
	if len(deletedPaths) != 2 {
		t.Errorf("expected 2 successful deletes, got %d: %v", len(deletedPaths), deletedPaths)
	}
	// At least one root snapshot was deleted → grub should regen.
	if *grubCalls != 1 {
		t.Errorf("regenerateGrub called %d times, want 1", *grubCalls)
	}
}

func TestCleanup_NothingToDelete_SkipsGrub(t *testing.T) {
	newSnapshotFixture(t)
	grubCalls := stubGrub(t)
	seedAutoSnapshots(t, 2)

	defer btrfs.WithFakes(btrfs.Fakes{
		IsInsideSnapshot: func() (bool, error) { return false, nil },
		Delete: func(p string) error {
			t.Errorf("Delete should not be called; got %s", p)
			return nil
		},
	})()

	if err := runCleanup(5, false, "root"); err != nil {
		t.Fatalf("runCleanup: %v", err)
	}
	if *grubCalls != 0 {
		t.Errorf("grub should not regen when nothing deleted; got %d calls", *grubCalls)
	}
}

// ----------------------------- delete ----------------------------------------

func TestDelete_RootSnapshot_RegeneratesGrub(t *testing.T) {
	newSnapshotFixture(t)
	grubCalls := stubGrub(t)

	// Seed one manual snapshot under root.
	snapPath := filepath.Join(config.SnapshotBase, "root", "manual", "before-upgrade")
	if err := os.MkdirAll(snapPath, 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}

	deleteCalls := 0
	defer btrfs.WithFakes(btrfs.Fakes{
		Delete: func(p string) error {
			deleteCalls++
			if p != snapPath {
				t.Errorf("Delete path = %q, want %q", p, snapPath)
			}
			return os.RemoveAll(p)
		},
	})()

	if err := runDelete("before-upgrade", "root"); err != nil {
		t.Fatalf("runDelete: %v", err)
	}
	if deleteCalls != 1 {
		t.Errorf("Delete called %d times, want 1", deleteCalls)
	}
	if *grubCalls != 1 {
		t.Errorf("regenerateGrub called %d times, want 1", *grubCalls)
	}
}

func TestDelete_NameNotFound(t *testing.T) {
	newSnapshotFixture(t)
	stubGrub(t)

	defer btrfs.WithFakes(btrfs.Fakes{
		Delete: func(p string) error {
			t.Errorf("Delete should not be called; got %s", p)
			return nil
		},
	})()

	err := runDelete("nope", "root")
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
}

// ----------------------------- findUpdateGrub --------------------------------

// withSearchPaths installs ps as the active updateGrubSearchPaths for the test.
func withSearchPaths(t *testing.T, ps []string) {
	t.Helper()
	prev := updateGrubSearchPaths
	updateGrubSearchPaths = ps
	t.Cleanup(func() { updateGrubSearchPaths = prev })
}

func TestFindUpdateGrub_PrefersStandardSbinOverPath(t *testing.T) {
	dir := t.TempDir()
	standard := filepath.Join(dir, "standard-update-grub")
	if err := os.WriteFile(standard, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake standard binary: %v", err)
	}
	pathDir := t.TempDir()
	pathBin := filepath.Join(pathDir, "update-grub")
	if err := os.WriteFile(pathBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake PATH binary: %v", err)
	}
	t.Setenv("PATH", pathDir)
	withSearchPaths(t, []string{standard})

	got, err := findUpdateGrub()
	if err != nil {
		t.Fatalf("findUpdateGrub: %v", err)
	}
	if got != standard {
		t.Errorf("got %q, want %q (sbin path must win over PATH)", got, standard)
	}
}

func TestFindUpdateGrub_FallsBackToPath(t *testing.T) {
	pathDir := t.TempDir()
	pathBin := filepath.Join(pathDir, "update-grub")
	if err := os.WriteFile(pathBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake PATH binary: %v", err)
	}
	t.Setenv("PATH", pathDir)
	withSearchPaths(t, []string{"/nonexistent-bootenv-test/update-grub"})

	got, err := findUpdateGrub()
	if err != nil {
		t.Fatalf("findUpdateGrub: %v", err)
	}
	if got != pathBin {
		t.Errorf("got %q, want %q (PATH fallback)", got, pathBin)
	}
}

func TestFindUpdateGrub_NotFoundAnywhere(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty dir
	withSearchPaths(t, []string{"/nonexistent-bootenv-test/update-grub"})

	if _, err := findUpdateGrub(); err == nil {
		t.Fatal("expected error when update-grub is nowhere, got nil")
	}
}

// TestFindUpdateGrub_SkipsDirectoryNamedUpdateGrub guards against the case
// where some path coincidentally contains a directory named "update-grub".
func TestFindUpdateGrub_SkipsDirectoryNamedUpdateGrub(t *testing.T) {
	dir := t.TempDir()
	bogus := filepath.Join(dir, "bogus-update-grub")
	if err := os.Mkdir(bogus, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	pathDir := t.TempDir()
	pathBin := filepath.Join(pathDir, "update-grub")
	if err := os.WriteFile(pathBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake: %v", err)
	}
	t.Setenv("PATH", pathDir)
	withSearchPaths(t, []string{bogus})

	got, err := findUpdateGrub()
	if err != nil {
		t.Fatalf("findUpdateGrub: %v", err)
	}
	if got != pathBin {
		t.Errorf("got %q, want %q (directory should not match)", got, pathBin)
	}
}
