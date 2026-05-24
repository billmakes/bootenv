// Package snapstore enumerates and resolves btrfs snapshots managed by bootenv.
package snapstore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	snapshotRootBase = "/@snapshots/root"
	snapshotHomeBase = "/@snapshots/home"
)

// Entry represents one snapshot (root and/or home component).
type Entry struct {
	Name      string    // basename, e.g. "2025-05-20_090132" or "before-upgrade"
	Kind      string    // "auto" or "manual"
	RootPath  string    // /@snapshots/root/<kind>/<name>
	HomePath  string    // /@snapshots/home/<kind>/<name>
	HasRoot   bool      // root subvolume directory exists
	HasHome   bool      // home subvolume directory exists
	KernelVer string    // from .bootenv-kernel or lib/modules (root only)
	CreatedAt time.Time // directory mtime; root preferred, falls back to home
}

// List returns all known snapshots (both kinds, both targets) newest-first.
func List() ([]Entry, error) {
	return ListFromDirs(snapshotRootBase, snapshotHomeBase, "")
}

// ListFiltered returns snapshots filtered to one kind.
// Pass "auto" or "manual"; empty string returns all kinds.
func ListFiltered(kind string) ([]Entry, error) {
	return ListFromDirs(snapshotRootBase, snapshotHomeBase, kind)
}

// ListFromDirs is the real implementation. rootBase and homeBase are the
// directories containing "auto" and "manual" subdirectories (e.g.
// "/@snapshots/root" and "/@snapshots/home"). kind may be "auto", "manual",
// or "" for both. Tests pass t.TempDir()-based paths here.
func ListFromDirs(rootBase, homeBase, kind string) ([]Entry, error) {
	var kinds []string
	switch kind {
	case "auto", "manual":
		kinds = []string{kind}
	case "":
		kinds = []string{"auto", "manual"}
	default:
		return nil, fmt.Errorf("unknown kind %q — use auto, manual, or empty string", kind)
	}

	var entries []Entry
	for _, k := range kinds {
		rootDir := filepath.Join(rootBase, k)
		homeDir := filepath.Join(homeBase, k)

		for _, name := range unionDirNames(rootDir, homeDir) {
			rootPath := filepath.Join(rootDir, name)
			homePath := filepath.Join(homeDir, name)
			hasRoot := dirExists(rootPath)
			hasHome := dirExists(homePath)

			createdAt := dirMtime(rootPath)
			if createdAt.IsZero() {
				createdAt = dirMtime(homePath)
			}

			kver := ""
			if hasRoot {
				kver = resolveKernel(rootPath)
			}

			entries = append(entries, Entry{
				Name:      name,
				Kind:      k,
				RootPath:  rootPath,
				HomePath:  homePath,
				HasRoot:   hasRoot,
				HasHome:   hasHome,
				KernelVer: kver,
				CreatedAt: createdAt,
			})
		}
	}

	// Newest first; tie-break by name descending for determinism.
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].CreatedAt.Equal(entries[j].CreatedAt) {
			return entries[i].CreatedAt.After(entries[j].CreatedAt)
		}
		return entries[i].Name > entries[j].Name
	})

	return entries, nil
}

// Resolve finds a snapshot by name across both kinds and both targets.
// Returns an error if not found.
func Resolve(name string) (*Entry, error) {
	entries, err := List()
	if err != nil {
		return nil, err
	}
	for i := range entries {
		if entries[i].Name == name {
			return &entries[i], nil
		}
	}
	return nil, fmt.Errorf("snapshot %q not found", name)
}

// FilterTarget filters entries by which component(s) are present.
//   - "root"  → only entries where HasRoot is true
//   - "home"  → only entries where HasHome is true
//   - "both"  → only entries where both HasRoot and HasHome are true
//   - ""      → all entries unchanged
func FilterTarget(entries []Entry, target string) []Entry {
	if target == "" {
		return entries
	}
	var out []Entry
	for _, e := range entries {
		switch target {
		case "root":
			if e.HasRoot {
				out = append(out, e)
			}
		case "home":
			if e.HasHome {
				out = append(out, e)
			}
		case "both":
			if e.HasRoot && e.HasHome {
				out = append(out, e)
			}
		}
	}
	return out
}

// SelectForPrune returns the entries that should be deleted to bring the pool
// down to keep entries. entries must be sorted newest-first (as returned by
// ListFiltered/ListFromDirs); the tail (oldest) is returned.
// Returns nil if len(entries) <= keep.
func SelectForPrune(entries []Entry, keep int) []Entry {
	if keep < 0 {
		keep = 0
	}
	if len(entries) <= keep {
		return nil
	}
	return entries[keep:]
}

// unionDirNames returns the sorted, deduplicated set of sub-directory names
// found in dir1 and dir2. Missing directories are silently ignored.
func unionDirNames(dir1, dir2 string) []string {
	seen := map[string]struct{}{}
	for _, dir := range []string{dir1, dir2} {
		fis, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, fi := range fis {
			if fi.IsDir() {
				seen[fi.Name()] = struct{}{}
			}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// dirExists reports whether path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// dirMtime returns the modification time of path, or the zero time on error.
func dirMtime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

// resolveKernel returns the kernel version for a root snapshot path.
func resolveKernel(rootPath string) string {
	marker := filepath.Join(rootPath, ".bootenv-kernel")
	if data, err := os.ReadFile(marker); err == nil {
		kver := strings.TrimSpace(string(data))
		if kver != "" {
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
