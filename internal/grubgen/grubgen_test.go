package grubgen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bootenv/internal/grubgen"
)

// makeFakeKernel creates dummy vmlinuz-<ver> and initrd.img-<ver> files
// under bootDir so GenerateTo treats the entry as valid.
func makeFakeKernel(t *testing.T, bootDir, ver string) {
	t.Helper()
	for _, name := range []string{"vmlinuz-" + ver, "initrd.img-" + ver} {
		path := filepath.Join(bootDir, name)
		if err := os.WriteFile(path, []byte("dummy"), 0644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

// -----------------------------------------------------------------------
// GenerateTo — submenu structure
// -----------------------------------------------------------------------

func TestGenerateTo_Submenu(t *testing.T) {
	bootDir := t.TempDir()
	outFile := filepath.Join(t.TempDir(), "42_bootenv_snapshots")

	makeFakeKernel(t, bootDir, "6.1.0-31-amd64")
	makeFakeKernel(t, bootDir, "6.1.0-30-amd64")

	entries := []grubgen.Entry{
		{Distro: "TestOS", Name: "snap-new", Kind: "auto", KernelVer: "6.1.0-31-amd64", RootUUID: "aaaa"},
		{Distro: "TestOS", Name: "snap-old", Kind: "auto", KernelVer: "6.1.0-30-amd64", RootUUID: "aaaa"},
	}

	if err := grubgen.GenerateTo(entries, outFile, bootDir); err != nil {
		t.Fatalf("GenerateTo: %v", err)
	}

	content, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	body := string(content)

	if !strings.Contains(body, `submenu "Bootenv Snapshots"`) {
		t.Error("output missing submenu declaration")
	}
	if count := strings.Count(body, "menuentry"); count != 2 {
		t.Errorf("expected 2 menuentry blocks, got %d", count)
	}
	// Newest entry should appear before oldest.
	posNew := strings.Index(body, "snap-new")
	posOld := strings.Index(body, "snap-old")
	if posNew < 0 || posOld < 0 {
		t.Error("entry names not found in output")
	} else if posNew > posOld {
		t.Error("snap-new should appear before snap-old (newest first)")
	}
}

// -----------------------------------------------------------------------
// GenerateTo — skips entries with missing kernel/initrd
// -----------------------------------------------------------------------

func TestGenerateTo_SkipsMissingKernel(t *testing.T) {
	bootDir := t.TempDir()
	outFile := filepath.Join(t.TempDir(), "42_bootenv_snapshots")

	// Only provide kernel files for the first entry.
	makeFakeKernel(t, bootDir, "6.1.0-31-amd64")

	entries := []grubgen.Entry{
		{Distro: "TestOS", Name: "good-snap", Kind: "auto", KernelVer: "6.1.0-31-amd64", RootUUID: "aaaa"},
		{Distro: "TestOS", Name: "bad-snap", Kind: "auto", KernelVer: "9.9.9-missing", RootUUID: "aaaa"},
	}

	if err := grubgen.GenerateTo(entries, outFile, bootDir); err != nil {
		t.Fatalf("GenerateTo: %v", err)
	}

	content, _ := os.ReadFile(outFile)
	body := string(content)

	if !strings.Contains(body, "good-snap") {
		t.Error("good-snap should be in output")
	}
	if strings.Contains(body, "bad-snap") {
		t.Error("bad-snap should be skipped (missing kernel)")
	}
}

// -----------------------------------------------------------------------
// BuildEntries — skips entries with empty KernelVer
// -----------------------------------------------------------------------

func TestBuildEntries_SkipsEmptyKernel(t *testing.T) {
	snaps := []grubgen.SnapInfo{
		{Name: "with-kernel", Kind: "auto", KernelVer: "6.1.0-31-amd64"},
		{Name: "no-kernel", Kind: "auto", KernelVer: ""},
		{Name: "also-kernel", Kind: "manual", KernelVer: "6.1.0-30-amd64"},
	}

	entries := grubgen.BuildEntries(snaps, "TestOS", "test-uuid")
	if len(entries) != 2 {
		t.Errorf("expected 2 entries (skipping no-kernel), got %d", len(entries))
	}
	for _, e := range entries {
		if e.KernelVer == "" {
			t.Errorf("entry %q has empty KernelVer", e.Name)
		}
	}
}

// -----------------------------------------------------------------------
// SnapInfoFromDir — sort order
// -----------------------------------------------------------------------

func TestSnapInfoFromDir_SortNewestFirst(t *testing.T) {
	base := t.TempDir()
	now := time.Now().Truncate(time.Second)

	names := []string{"snap-a", "snap-b", "snap-c"}
	for _, name := range names {
		dir := filepath.Join(base, "auto", name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}

	// snap-c newest, snap-a oldest
	os.Chtimes(filepath.Join(base, "auto", "snap-a"), now.Add(-2*time.Hour), now.Add(-2*time.Hour))
	os.Chtimes(filepath.Join(base, "auto", "snap-b"), now.Add(-1*time.Hour), now.Add(-1*time.Hour))
	os.Chtimes(filepath.Join(base, "auto", "snap-c"), now, now)

	snaps := grubgen.SnapInfoFromDir(base)
	if len(snaps) != 3 {
		t.Fatalf("expected 3 snaps, got %d", len(snaps))
	}

	want := []string{"snap-c", "snap-b", "snap-a"}
	for i, w := range want {
		if snaps[i].Name != w {
			t.Errorf("snaps[%d].Name = %q, want %q", i, snaps[i].Name, w)
		}
	}
}

// -----------------------------------------------------------------------
// GenerateTo — output file is executable (mode 0755)
// -----------------------------------------------------------------------

func TestGenerateTo_FileIsExecutable(t *testing.T) {
	bootDir := t.TempDir()
	outFile := filepath.Join(t.TempDir(), "42_bootenv_snapshots")
	makeFakeKernel(t, bootDir, "6.1.0-31-amd64")

	entries := []grubgen.Entry{
		{Distro: "TestOS", Name: "snap", Kind: "auto", KernelVer: "6.1.0-31-amd64", RootUUID: "aaaa"},
	}
	if err := grubgen.GenerateTo(entries, outFile, bootDir); err != nil {
		t.Fatalf("GenerateTo: %v", err)
	}

	info, err := os.Stat(outFile)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode()&0111 == 0 {
		t.Errorf("output file not executable: mode %v", info.Mode())
	}
}
