package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bootenv/internal/btrfs"
	"bootenv/internal/config"
)

// restoreFixture lays out a temp directory tree that stands in for a real
// btrfs system so runRestore can execute without touching the host.
type restoreFixture struct {
	dir    string // temp dir; acts as both the top-level mount and parent of @snapshots
	rootAt string // <dir>/@
}

// newRestoreFixture prepares a temp tree containing /@snapshots/root/manual/<snapName>
// and an existing /@. It points config.SnapshotBase + cfgPath at the temp tree.
// Cleanup is registered on t.
func newRestoreFixture(t *testing.T, snapName string) *restoreFixture {
	t.Helper()
	dir := t.TempDir()

	prevBase := config.SnapshotBase
	config.SnapshotBase = filepath.Join(dir, "@snapshots")
	prevCfgPath := cfgPath
	cfgPath = filepath.Join(dir, "missing.toml") // forces DefaultConfig()
	t.Cleanup(func() {
		config.SnapshotBase = prevBase
		cfgPath = prevCfgPath
	})

	snapPath := filepath.Join(config.SnapshotBase, "root", "manual", snapName)
	if err := os.MkdirAll(snapPath, 0o755); err != nil {
		t.Fatalf("create fake snapshot dir: %v", err)
	}
	rootAt := filepath.Join(dir, "@")
	if err := os.MkdirAll(rootAt, 0o755); err != nil {
		t.Fatalf("create fake @: %v", err)
	}
	return &restoreFixture{dir: dir, rootAt: rootAt}
}

// stubGrub replaces regenerateGrubFn with a no-op for the duration of the test.
func stubGrub(t *testing.T) *int {
	t.Helper()
	calls := 0
	prev := regenerateGrubFn
	regenerateGrubFn = func() error { calls++; return nil }
	t.Cleanup(func() { regenerateGrubFn = prev })
	return &calls
}

// stubConfirm makes the restore prompt read its answer from s.
func stubConfirm(t *testing.T, s string) {
	t.Helper()
	prev := confirmReader
	confirmReader = strings.NewReader(s)
	t.Cleanup(func() { confirmReader = prev })
}

// baseFakes returns btrfs.Fakes whose OpenTopVol/Close/Exists behave like a
// real filesystem rooted at fixture.dir. Snapshot is set by each test.
func baseFakes(f *restoreFixture, snap func(src, dst string) error) btrfs.Fakes {
	return btrfs.Fakes{
		OpenTopVol: func(mountpoint, target string) (*btrfs.TopVol, error) {
			return &btrfs.TopVol{Mount: f.dir}, nil
		},
		CloseTopVol:     func(tv *btrfs.TopVol) error { return nil },
		SubvolumeExists: func(p string) bool { _, err := os.Stat(p); return err == nil },
		Snapshot:        snap,
	}
}

func TestRestore_HappyPath(t *testing.T) {
	f := newRestoreFixture(t, "before-upgrade")
	grubCalls := stubGrub(t)
	stubConfirm(t, "y\n")

	snapshotCalls := 0
	defer btrfs.WithFakes(baseFakes(f, func(src, dst string) error {
		snapshotCalls++
		return os.MkdirAll(dst, 0o755)
	}))()

	if err := runRestore(nil, []string{"before-upgrade"}); err != nil {
		t.Fatalf("runRestore: %v", err)
	}
	if snapshotCalls != 1 {
		t.Errorf("Snapshot called %d times, want 1", snapshotCalls)
	}
	if *grubCalls != 1 {
		t.Errorf("regenerateGrub called %d times, want 1", *grubCalls)
	}
	if _, err := os.Stat(f.rootAt); err != nil {
		t.Errorf("expected /@ to exist after restore: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(f.dir, "@-pre-restore-*"))
	if len(matches) != 1 {
		t.Errorf("expected 1 backup, found %d: %v", len(matches), matches)
	}
}

func TestRestore_SnapshotFailsAfterRename(t *testing.T) {
	f := newRestoreFixture(t, "before-upgrade")
	stubGrub(t)
	stubConfirm(t, "y\n")

	defer btrfs.WithFakes(baseFakes(f, func(src, dst string) error {
		return fmt.Errorf("simulated btrfs failure")
	}))()

	err := runRestore(nil, []string{"before-upgrade"})
	if err == nil {
		t.Fatal("expected error from failed snapshot, got nil")
	}
	msg := err.Error()
	// The error must direct the user to the safety backup.
	if !strings.Contains(msg, "old root is preserved at") {
		t.Errorf("error missing recovery hint: %q", msg)
	}
	if !strings.Contains(msg, "@-pre-restore-") {
		t.Errorf("error missing backup name: %q", msg)
	}
	if !strings.Contains(msg, "btrfs subvolume snapshot") {
		t.Errorf("error missing recovery command: %q", msg)
	}

	// The backup must exist on disk — that's the entire point of the rename.
	matches, _ := filepath.Glob(filepath.Join(f.dir, "@-pre-restore-*"))
	if len(matches) != 1 {
		t.Errorf("expected backup to remain after failure, found %d: %v", len(matches), matches)
	}
	// And @ should NOT exist (rename moved it; snapshot failed to recreate it).
	if _, err := os.Stat(f.rootAt); !os.IsNotExist(err) {
		t.Errorf("expected /@ to be absent after failed snapshot, got err=%v", err)
	}
}

func TestRestore_ResumePartialRestore(t *testing.T) {
	f := newRestoreFixture(t, "before-upgrade")
	stubGrub(t)
	stubConfirm(t, "y\n")

	// Simulate a prior crashed restore: @ is gone, the safety backup remains.
	if err := os.Rename(f.rootAt, filepath.Join(f.dir, "@-pre-restore-old")); err != nil {
		t.Fatalf("setup partial state: %v", err)
	}

	snapshotCalls := 0
	defer btrfs.WithFakes(baseFakes(f, func(src, dst string) error {
		snapshotCalls++
		return os.MkdirAll(dst, 0o755)
	}))()

	if err := runRestore(nil, []string{"before-upgrade"}); err != nil {
		t.Fatalf("runRestore: %v", err)
	}
	if snapshotCalls != 1 {
		t.Errorf("Snapshot should still be called once; got %d", snapshotCalls)
	}
	// Only the OLD backup should remain — no new one created since rename was skipped.
	matches, _ := filepath.Glob(filepath.Join(f.dir, "@-pre-restore-*"))
	if len(matches) != 1 {
		t.Errorf("expected 1 backup (the pre-existing one), found %d: %v", len(matches), matches)
	}
}

func TestRestore_DeclinedPrompt(t *testing.T) {
	f := newRestoreFixture(t, "before-upgrade")
	stubGrub(t)
	stubConfirm(t, "n\n")

	snapshotCalls := 0
	defer btrfs.WithFakes(baseFakes(f, func(src, dst string) error {
		snapshotCalls++
		return nil
	}))()

	if err := runRestore(nil, []string{"before-upgrade"}); err != nil {
		t.Fatalf("expected no error on decline, got: %v", err)
	}
	if snapshotCalls != 0 {
		t.Errorf("Snapshot must not run when user declines; got %d calls", snapshotCalls)
	}
	matches, _ := filepath.Glob(filepath.Join(f.dir, "@-pre-restore-*"))
	if len(matches) != 0 {
		t.Errorf("no backup must be made when user declines; got %v", matches)
	}
}

func TestRestore_SnapshotNameNotFound(t *testing.T) {
	f := newRestoreFixture(t, "before-upgrade")
	stubGrub(t)
	stubConfirm(t, "y\n")

	snapshotCalls := 0
	defer btrfs.WithFakes(baseFakes(f, func(src, dst string) error {
		snapshotCalls++
		return nil
	}))()

	err := runRestore(nil, []string{"does-not-exist"})
	if err == nil {
		t.Fatal("expected error for missing snapshot, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
	if snapshotCalls != 0 {
		t.Errorf("Snapshot must not run when name lookup fails; got %d calls", snapshotCalls)
	}
	matches, _ := filepath.Glob(filepath.Join(f.dir, "@-pre-restore-*"))
	if len(matches) != 0 {
		t.Errorf("no rename must happen when name lookup fails; got %v", matches)
	}
}

func TestRestore_AutoDetectFromBootedSnapshot(t *testing.T) {
	f := newRestoreFixture(t, "2025-05-20_090132")
	stubGrub(t)
	stubConfirm(t, "y\n")

	snapshotCalls := 0
	bf := baseFakes(f, func(src, dst string) error {
		snapshotCalls++
		// src must be the resolved snapshot path, not "/@".
		if !strings.Contains(src, "2025-05-20_090132") {
			return fmt.Errorf("unexpected src %q", src)
		}
		return os.MkdirAll(dst, 0o755)
	})
	bf.IsInsideSnapshot = func() (bool, error) { return true, nil }
	bf.CurrentRootSubvol = func() (string, error) {
		return "/@snapshots/root/auto/2025-05-20_090132", nil
	}
	defer btrfs.WithFakes(bf)()

	if err := runRestore(nil, nil); err != nil {
		t.Fatalf("runRestore: %v", err)
	}
	if snapshotCalls != 1 {
		t.Errorf("Snapshot called %d times, want 1", snapshotCalls)
	}
}

func TestRestore_AutoDetectRefusesOnRealRoot(t *testing.T) {
	f := newRestoreFixture(t, "before-upgrade")
	stubGrub(t)
	stubConfirm(t, "y\n")

	snapshotCalls := 0
	bf := baseFakes(f, func(src, dst string) error {
		snapshotCalls++
		return nil
	})
	bf.IsInsideSnapshot = func() (bool, error) { return false, nil }
	defer btrfs.WithFakes(bf)()

	err := runRestore(nil, nil)
	if err == nil {
		t.Fatal("expected error when on real root with no name, got nil")
	}
	if !strings.Contains(err.Error(), "real root") {
		t.Errorf("expected mention of 'real root' in error, got: %v", err)
	}
	if snapshotCalls != 0 {
		t.Errorf("no mutation must occur on the refused path; got %d Snapshot calls", snapshotCalls)
	}
	matches, _ := filepath.Glob(filepath.Join(f.dir, "@-pre-restore-*"))
	if len(matches) != 0 {
		t.Errorf("no rename must happen; got %v", matches)
	}
}

func TestReadConfirm(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"y\n", true},
		{"yes\n", true},
		{"YES\n", true},
		{"Y\n", true},
		{"  y  \n", true},
		{"n\n", false},
		{"no\n", false},
		{"\n", false},
		{"maybe\n", false},
	}
	for _, c := range cases {
		got, err := readConfirm(strings.NewReader(c.in))
		if err != nil {
			t.Errorf("readConfirm(%q) unexpected error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("readConfirm(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
