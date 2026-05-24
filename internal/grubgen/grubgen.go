// Package grubgen generates the /etc/grub.d/42_bootenv_snapshots snippet.
package grubgen

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const outputPath = "/etc/grub.d/42_bootenv_snapshots"

// Entry is a single grub menu entry to generate.
type Entry struct {
	Distro    string // e.g. "Devuan GNU/Linux 5 (daedalus)"
	Name      string // snapshot basename
	Kind      string // "auto" or "manual"
	KernelVer string
	RootUUID  string
}

const grubTemplate = `#!/bin/sh
exec tail -n +3 $0
{{range .}}
menuentry "{{.Distro}} Snapshot - {{.Name}} ({{.KernelVer}})" {
    insmod part_gpt
    insmod btrfs

    search --no-floppy --fs-uuid --set=root {{.RootUUID}}

    linux /@/boot/vmlinuz-{{.KernelVer}} root=UUID={{.RootUUID}} rootflags=subvol=@snapshots/root/{{.Kind}}/{{.Name}} rw quiet
    initrd /@/boot/initrd.img-{{.KernelVer}}
}
{{end}}`

// Generate writes the grub snippet for the given entries and makes it executable.
// It skips entries where the kernel or initrd files are missing from /boot.
func Generate(entries []Entry) error {
	// Filter entries where kernel files actually exist on disk.
	var valid []Entry
	for _, e := range entries {
		kernel := fmt.Sprintf("/boot/vmlinuz-%s", e.KernelVer)
		initrd := fmt.Sprintf("/boot/initrd.img-%s", e.KernelVer)
		if _, err := os.Stat(kernel); err != nil {
			fmt.Fprintf(os.Stderr, "Skipping %s: missing %s\n", e.Name, kernel)
			continue
		}
		if _, err := os.Stat(initrd); err != nil {
			fmt.Fprintf(os.Stderr, "Skipping %s: missing %s\n", e.Name, initrd)
			continue
		}
		valid = append(valid, e)
	}

	tmpl, err := template.New("grub").Parse(grubTemplate)
	if err != nil {
		return fmt.Errorf("parse grub template: %w", err)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outputPath, err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, valid); err != nil {
		return fmt.Errorf("render grub template: %w", err)
	}

	if err := os.Chmod(outputPath, 0755); err != nil {
		return fmt.Errorf("chmod %s: %w", outputPath, err)
	}

	fmt.Printf("Generated %s (%d entries)\n", outputPath, len(valid))
	return nil
}

// ReadDistro reads PRETTY_NAME (falling back to NAME) from /etc/os-release.
func ReadDistro() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return "Linux"
	}
	defer f.Close()

	vars := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if k, v, ok := strings.Cut(line, "="); ok {
			vars[k] = strings.Trim(v, `"`)
		}
	}
	if v := vars["PRETTY_NAME"]; v != "" {
		return v
	}
	if v := vars["NAME"]; v != "" {
		return v
	}
	return "Linux"
}

// BuildEntries converts snapstore entries + rootUUID into grubgen Entries.
// Caller passes a snapstore-style list; this package avoids importing snapstore
// to keep the dependency graph acyclic — callers bridge the types.
func BuildEntries(snaps []SnapInfo, distro, rootUUID string) []Entry {
	var out []Entry
	for _, s := range snaps {
		if s.KernelVer == "" {
			continue
		}
		out = append(out, Entry{
			Distro:    distro,
			Name:      s.Name,
			Kind:      s.Kind,
			KernelVer: s.KernelVer,
			RootUUID:  rootUUID,
		})
	}
	return out
}

// SnapInfo is a minimal snapshot descriptor used to avoid circular imports.
type SnapInfo struct {
	Name      string
	Kind      string
	KernelVer string
	RootPath  string
}

// SnapInfoFromDir scans a snapshot root directory and returns entries ready
// for BuildEntries.  dir is e.g. "/@snapshots/root".
func SnapInfoFromDir(dir string) []SnapInfo {
	var out []SnapInfo
	for _, kind := range []string{"auto", "manual"} {
		base := filepath.Join(dir, kind)
		fis, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, fi := range fis {
			if !fi.IsDir() {
				continue
			}
			snapPath := filepath.Join(base, fi.Name())
			kver := resolveKernel(snapPath)
			out = append(out, SnapInfo{
				Name:      fi.Name(),
				Kind:      kind,
				KernelVer: kver,
				RootPath:  snapPath,
			})
		}
	}
	return out
}

func resolveKernel(rootPath string) string {
	marker := filepath.Join(rootPath, ".bootenv-kernel")
	if data, err := os.ReadFile(marker); err == nil {
		if kver := strings.TrimSpace(string(data)); kver != "" {
			return kver
		}
	}
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
	// lexical sort is fine for kernel version strings
	last := names[0]
	for _, n := range names[1:] {
		if n > last {
			last = n
		}
	}
	return last
}
