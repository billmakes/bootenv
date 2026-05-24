//go:build integration

// Package btrfs integration tests exercise the REAL exec.Command paths against
// a loopback btrfs image. Run with:
//
//	sudo go test -tags=integration ./internal/btrfs/...
//
// Each test gets its own freshly-formatted image so they cannot interfere with
// each other.
package btrfs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// requireRootAndBtrfs skips the test when prerequisites are missing instead of
// failing hard, so the suite stays green on minimal CI runners.
func requireRootAndBtrfs(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("integration test requires root (mount/umount/mkfs.btrfs)")
	}
	for _, bin := range []string{"btrfs", "mkfs.btrfs", "mount", "umount", "truncate"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("integration test requires %s: %v", bin, err)
		}
	}
}

// newLoopbackBtrfs creates a 200MB btrfs filesystem in a temp image file,
// mounts it via loopback, and returns the mount point. Cleanup is registered
// on t.
func newLoopbackBtrfs(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	img := filepath.Join(dir, "fs.img")
	mnt := filepath.Join(dir, "mnt")

	if out, err := exec.Command("truncate", "-s", "200M", img).CombinedOutput(); err != nil {
		t.Fatalf("truncate: %v\n%s", err, out)
	}
	if out, err := exec.Command("mkfs.btrfs", "-q", img).CombinedOutput(); err != nil {
		t.Fatalf("mkfs.btrfs: %v\n%s", err, out)
	}
	if err := os.MkdirAll(mnt, 0o755); err != nil {
		t.Fatalf("mkdir mnt: %v", err)
	}
	if out, err := exec.Command("mount", "-o", "loop", img, mnt).CombinedOutput(); err != nil {
		t.Fatalf("mount: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("umount", mnt).Run()
	})
	return mnt
}

// makeSubvolume creates a btrfs subvolume at path via the btrfs CLI.
func makeSubvolume(t *testing.T, path string) {
	t.Helper()
	if out, err := exec.Command("btrfs", "subvolume", "create", path).CombinedOutput(); err != nil {
		t.Fatalf("subvolume create %s: %v\n%s", path, err, out)
	}
}

func TestIntegration_SnapshotCreatesSubvolume(t *testing.T) {
	requireRootAndBtrfs(t)
	mnt := newLoopbackBtrfs(t)

	src := filepath.Join(mnt, "src")
	dst := filepath.Join(mnt, "snap")
	makeSubvolume(t, src)
	// Write a known file into the source so we can verify the snapshot copied it.
	if err := os.WriteFile(filepath.Join(src, "marker"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	if err := Snapshot(src, dst); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if !SubvolumeExists(dst) {
		t.Errorf("SubvolumeExists(%q) = false, want true", dst)
	}
	got, err := os.ReadFile(filepath.Join(dst, "marker"))
	if err != nil {
		t.Fatalf("snapshot missing marker file: %v", err)
	}
	if string(got) != "hi" {
		t.Errorf("snapshot content = %q, want %q", got, "hi")
	}
}

func TestIntegration_DeleteRemovesSubvolume(t *testing.T) {
	requireRootAndBtrfs(t)
	mnt := newLoopbackBtrfs(t)

	sub := filepath.Join(mnt, "vol")
	makeSubvolume(t, sub)
	if !SubvolumeExists(sub) {
		t.Fatal("expected subvolume to exist after create")
	}
	if err := Delete(sub); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if SubvolumeExists(sub) {
		t.Errorf("SubvolumeExists(%q) = true after Delete, want false", sub)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Errorf("path still present after Delete: stat err = %v", err)
	}
}

// TestIntegration_RestoreSequence exercises the same rename-then-snapshot dance
// that cmd/restore.go performs, against a real btrfs filesystem. It does not
// invoke runRestore (which would also call regenerateGrub and require a real
// config); it pins the underlying btrfs sequence's correctness.
func TestIntegration_RestoreSequence(t *testing.T) {
	requireRootAndBtrfs(t)
	mnt := newLoopbackBtrfs(t)

	// Lay out @ (the "current root") and a snapshot under @snapshots/root/manual/foo.
	rootAt := filepath.Join(mnt, "@")
	makeSubvolume(t, rootAt)
	if err := os.WriteFile(filepath.Join(rootAt, "marker-old"), []byte("old"), 0o644); err != nil {
		t.Fatalf("seed old marker: %v", err)
	}

	snapBase := filepath.Join(mnt, "@snapshots", "root", "manual")
	if err := os.MkdirAll(filepath.Dir(snapBase), 0o755); err != nil {
		t.Fatalf("mkdir snapBase parent: %v", err)
	}
	// Take a snapshot of @ as "foo".
	if err := os.MkdirAll(snapBase, 0o755); err != nil {
		t.Fatalf("mkdir snapBase: %v", err)
	}
	snapPath := filepath.Join(snapBase, "foo")
	if err := Snapshot(rootAt, snapPath); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	// Modify @ post-snapshot so we can detect whether restore reverted it.
	if err := os.WriteFile(filepath.Join(rootAt, "marker-new"), []byte("new"), 0o644); err != nil {
		t.Fatalf("dirty rootAt: %v", err)
	}

	// Perform the restore sequence: rename @ → backup, snapshot foo → @.
	backupAt := filepath.Join(mnt, "@-pre-restore-test")
	if err := os.Rename(rootAt, backupAt); err != nil {
		t.Fatalf("rename @: %v", err)
	}
	if err := Snapshot(snapPath, rootAt); err != nil {
		t.Fatalf("restore Snapshot: %v", err)
	}

	// New @ should have the OLD marker (from the snapshot) and NOT the new one.
	if _, err := os.Stat(filepath.Join(rootAt, "marker-old")); err != nil {
		t.Errorf("restored @ missing marker-old: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootAt, "marker-new")); !os.IsNotExist(err) {
		t.Errorf("restored @ contains marker-new (snapshot wasn't promoted): err=%v", err)
	}
	// Backup must still contain marker-new.
	if _, err := os.Stat(filepath.Join(backupAt, "marker-new")); err != nil {
		t.Errorf("backup missing marker-new: %v", err)
	}
}
