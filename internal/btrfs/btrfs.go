// Package btrfs wraps low-level btrfs and system calls used by bootenv.
package btrfs

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DefaultTopMount is the OS path where OpenTopVol mounts the btrfs top-level.
const DefaultTopMount = "/run/bootenv/mnt"

// TopVol represents the btrfs top-level volume (subvolid=5) mounted at an OS
// path. It is needed only to access the root subvolume @ itself (e.g. to
// rename or replace it). Paths like /@snapshots/... are nested inside @ and
// are always reachable as ordinary OS paths from the running root — they do
// NOT need to go through TopVol.
//
// Obtain one with OpenTopVol; call Close when done.
type TopVol struct {
	Mount string // OS mountpoint, e.g. /run/bootenv/mnt
}

// OpenTopVol mounts subvolid=5 of the btrfs filesystem containing mountpoint
// (typically "/") at target and returns a TopVol ready for use.
// The caller must call Close when done.
func OpenTopVol(mountpoint, target string) (*TopVol, error) {
	src, err := FindmntSource(mountpoint)
	if err != nil {
		return nil, err
	}
	// Strip subvol suffix: "/dev/nvme0n1p2[/@]" → "/dev/nvme0n1p2"
	device := strings.SplitN(src, "[", 2)[0]

	if err := os.MkdirAll(target, 0o700); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", target, err)
	}
	out, err := exec.Command("mount", "-t", "btrfs", "-o", "subvolid=5", device, target).
		CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("mount btrfs top-level at %s: %w\n%s", target, err, out)
	}
	return &TopVol{Mount: target}, nil
}

// Close unmounts the top-level volume.
func (tv *TopVol) Close() error {
	out, err := exec.Command("umount", tv.Mount).CombinedOutput()
	if err != nil {
		return fmt.Errorf("umount %s: %w\n%s", tv.Mount, err, out)
	}
	return nil
}

// RootAt returns the OS path of the root subvolume (@) within the mounted
// top-level, e.g. "/run/bootenv/mnt/@".
func (tv *TopVol) RootAt() string {
	return filepath.Join(tv.Mount, "@")
}

// Snapshot creates a btrfs snapshot of src at dst.
func Snapshot(src, dst string) error {
	out, err := exec.Command("btrfs", "subvolume", "snapshot", src, dst).CombinedOutput()
	if err != nil {
		return fmt.Errorf("btrfs snapshot %s → %s: %w\n%s", src, dst, err, out)
	}
	return nil
}

// SubvolumeExists reports whether path is a btrfs subvolume that currently exists.
func SubvolumeExists(path string) bool {
	return exec.Command("btrfs", "subvolume", "show", path).Run() == nil
}



// Delete removes a btrfs subvolume.
func Delete(path string) error {
	out, err := exec.Command("btrfs", "subvolume", "delete", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("btrfs subvolume delete %s: %w\n%s", path, err, out)
	}
	return nil
}

// FindmntUUID returns the filesystem UUID of the given mountpoint.
func FindmntUUID(mountpoint string) (string, error) {
	return findmntField("-no", "UUID", mountpoint)
}

// FindmntSource returns the SOURCE field (device/subvol) of the given mountpoint.
func FindmntSource(mountpoint string) (string, error) {
	return findmntField("-no", "SOURCE", mountpoint)
}

func findmntField(opts, field, mountpoint string) (string, error) {
	out, err := exec.Command("findmnt", opts, field, mountpoint).Output()
	if err != nil {
		return "", fmt.Errorf("findmnt %s %s %s: %w", opts, field, mountpoint, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// KernelVersion returns the running kernel version string (e.g. "6.1.0-31-amd64").
func KernelVersion() (string, error) {
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return "", fmt.Errorf("uname -r: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// IsInsideSnapshot reports whether the current root mount is a snapshot subvolume.
// It checks if the SOURCE of / contains "@snapshots".
func IsInsideSnapshot() (bool, error) {
	src, err := FindmntSource("/")
	if err != nil {
		return false, err
	}
	return bytes.Contains([]byte(src), []byte("@snapshots")), nil
}
