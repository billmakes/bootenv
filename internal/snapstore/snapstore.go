// Package snapstore enumerates and resolves btrfs snapshots managed by bootenv.
package snapstore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"bootenv/internal/config"
)

// Entry represents one snapshot subvolume for a single configured target.
type Entry struct {
	Target    string    // config target name, e.g. "root", "home"
	Name      string    // snapshot basename, e.g. "2025-05-20_090132" or "before-upgrade"
	Kind      string    // "auto" or "manual"
	Path      string    // full path: <SnapshotDir>/<kind>/<name>
	Source    string    // source subvol from config (e.g. "/", "/home")
	KernelVer string    // from .bootenv-kernel marker (root target only)
	CreatedAt time.Time // mtime of the snapshot directory
}

// ListFromDir scans snapshotDir for snapshots and returns them newest-first.
//
// snapshotDir is the base directory for one target (e.g. "/@snapshots/root").
// It must contain "auto" and/or "manual" subdirectories.
// kind may be "auto", "manual", or "" for both.
// targetName and sourceSubvol populate the corresponding Entry fields.
//
// This function is the primary implementation; tests call it directly with
// temporary directories instead of the live /@snapshots paths.
func ListFromDir(snapshotDir, targetName, sourceSubvol, kind string) ([]Entry, error) {
	kinds, err := resolveKinds(kind)
	if err != nil {
		return nil, err
	}

	var entries []Entry
	for _, k := range kinds {
		dir := filepath.Join(snapshotDir, k)
		fis, err := os.ReadDir(dir)
		if err != nil {
			continue // directory not yet created — skip silently
		}
		for _, fi := range fis {
			if !fi.IsDir() {
				continue
			}
			path := filepath.Join(dir, fi.Name())
			kver := ""
			if targetName == "root" {
				kver = resolveKernel(path)
			}
			entries = append(entries, Entry{
				Target:    targetName,
				Name:      fi.Name(),
				Kind:      k,
				Path:      path,
				Source:    sourceSubvol,
				KernelVer: kver,
				CreatedAt: snapCreatedAt(path),
			})
		}
	}

	sortNewestFirst(entries)
	return entries, nil
}

// ListTargets returns snapshots across all given targets, sorted newest-first
// globally by CreatedAt. kind may be "auto", "manual", or "" for both.
func ListTargets(targets map[string]config.TargetConfig, kind string) ([]Entry, error) {
	var all []Entry
	for name, tc := range targets {
		entries, err := ListFromDir(config.SnapshotDirFor(name), name, tc.Source, kind)
		if err != nil {
			return nil, fmt.Errorf("listing target %q: %w", name, err)
		}
		all = append(all, entries...)
	}
	sortNewestFirst(all)
	return all, nil
}

// ResolveAll finds every snapshot named name across the given targets.
// Returns an error if the name is not found in any target.
func ResolveAll(targets map[string]config.TargetConfig, name string) ([]Entry, error) {
	all, err := ListTargets(targets, "")
	if err != nil {
		return nil, err
	}
	var found []Entry
	for _, e := range all {
		if e.Name == name {
			found = append(found, e)
		}
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("snapshot %q not found in any configured target", name)
	}
	return found, nil
}

// ResolveInTarget finds a snapshot by name within a specific target's snapshot
// directory. Used by restore, which always operates on the root target.
func ResolveInTarget(snapshotDir, targetName, sourceSubvol, name string) (*Entry, error) {
	entries, err := ListFromDir(snapshotDir, targetName, sourceSubvol, "")
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].Name == name {
			return &entries[i], nil
		}
	}
	return nil, fmt.Errorf("snapshot %q not found in target %q", name, targetName)
}

// SelectForPrune returns the tail of entries that should be deleted to reduce
// the pool to keep entries. entries must be sorted newest-first (as returned
// by ListFromDir / ListTargets). Returns nil if len(entries) <= keep.
func SelectForPrune(entries []Entry, keep int) []Entry {
	if keep < 0 {
		keep = 0
	}
	if len(entries) <= keep {
		return nil
	}
	return entries[keep:]
}

// resolveKinds expands a kind filter string into the slice of kinds to scan.
func resolveKinds(kind string) ([]string, error) {
	switch kind {
	case "auto", "manual":
		return []string{kind}, nil
	case "":
		return []string{"auto", "manual"}, nil
	default:
		return nil, fmt.Errorf("unknown kind %q — use auto, manual, or empty string", kind)
	}
}

// sortNewestFirst sorts entries newest-first by CreatedAt; name descending as
// a deterministic tie-break.
func sortNewestFirst(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].CreatedAt.Equal(entries[j].CreatedAt) {
			return entries[i].CreatedAt.After(entries[j].CreatedAt)
		}
		return entries[i].Name > entries[j].Name
	})
}

// snapCreatedAt returns the creation time for a snapshot at path.
//
// Priority:
//  1. .bootenv-created marker written by "bootenv snapshot" — exact wall-clock
//     time recorded at the moment the snapshot was taken.
//  2. Inode ctime via syscall — set by the kernel when the subvolume inode is
//     first created; a reliable fallback for older snapshots that pre-date the
//     marker file.
func snapCreatedAt(path string) time.Time {
	// 1. Prefer the marker file written by "bootenv snapshot".
	markerPath := filepath.Join(path, ".bootenv-created")
	if data, err := os.ReadFile(markerPath); err == nil {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(data))); err == nil {
			return t.Local()
		}
	}

	// 2. Fall back to inode ctime (reliable for btrfs snapshot subvolumes).
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err == nil {
		return time.Unix(st.Ctim.Sec, st.Ctim.Nsec)
	}

	return time.Time{}
}

// resolveKernel returns the kernel version for a root snapshot path.
func resolveKernel(rootPath string) string {
	marker := filepath.Join(rootPath, ".bootenv-kernel")
	if data, err := os.ReadFile(marker); err == nil {
		if kver := strings.TrimSpace(string(data)); kver != "" {
			return kver
		}
	}
	// Fall back to newest entry in lib/modules.
	modsDir := filepath.Join(rootPath, "lib", "modules")
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	return names[len(names)-1]
}
