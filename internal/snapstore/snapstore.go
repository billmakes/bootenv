// Package snapstore enumerates and resolves btrfs snapshots managed by bootenv.
package snapstore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	rootAuto   = "/@snapshots/root/auto"
	rootManual = "/@snapshots/root/manual"
	homeAuto   = "/@snapshots/home/auto"
	homeManual = "/@snapshots/home/manual"
)

// Entry represents one snapshot pair (root + home).
type Entry struct {
	Name      string // basename, e.g. "2025-05-20_0901" or "before-upgrade"
	Kind      string // "auto" or "manual"
	RootPath  string // /@snapshots/root/<kind>/<name>
	HomePath  string // /@snapshots/home/<kind>/<name>
	KernelVer string // contents of .bootenv-kernel, or detected from lib/modules
}

// List returns all known snapshots sorted by kind then name.
func List() ([]Entry, error) {
	var entries []Entry

	pairs := []struct{ kind, rootDir, homeDir string }{
		{"auto", rootAuto, homeAuto},
		{"manual", rootManual, homeManual},
	}

	for _, p := range pairs {
		dirs, err := readDir(p.rootDir)
		if err != nil {
			// Directory may not exist yet — treat as empty.
			continue
		}
		for _, d := range dirs {
			rootPath := filepath.Join(p.rootDir, d)
			homePath := filepath.Join(p.homeDir, d)
			kver := resolveKernel(rootPath)
			entries = append(entries, Entry{
				Name:      d,
				Kind:      p.kind,
				RootPath:  rootPath,
				HomePath:  homePath,
				KernelVer: kver,
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

// Resolve finds a snapshot by name, searching auto first then manual.
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

// readDir returns sorted directory entry names under dir (only directories).
func readDir(dir string) ([]string, error) {
	fis, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, fi := range fis {
		if fi.IsDir() {
			names = append(names, fi.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// resolveKernel returns the kernel version for a root snapshot path.
// It first checks the .bootenv-kernel marker file, then falls back to
// scanning lib/modules inside the snapshot.
func resolveKernel(rootPath string) string {
	marker := filepath.Join(rootPath, ".bootenv-kernel")
	if data, err := os.ReadFile(marker); err == nil {
		kver := strings.TrimSpace(string(data))
		if kver != "" {
			return kver
		}
	}

	// Fall back: newest entry in lib/modules
	modsDir := filepath.Join(rootPath, "lib", "modules")
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		return ""
	}
	// Sort version strings lexically descending; the last one is newest.
	names := make([]string, 0, len(entries))
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
