package snapstore_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"bootenv/internal/config"
	"bootenv/internal/snapstore"
)

// makeSnap creates a fake snapshot directory at <base>/<kind>/<name>.
func makeSnap(t *testing.T, base, kind, name string) string {
	t.Helper()
	dir := filepath.Join(base, kind, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll %s: %v", dir, err)
	}
	return dir
}

// setMtime sets the mtime of path to mtime.
func setMtime(t *testing.T, path string, mtime time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("Chtimes %s: %v", path, err)
	}
}

// -----------------------------------------------------------------------
// ListFromDir — sort order
// -----------------------------------------------------------------------

func TestListFromDir_SortNewestFirst(t *testing.T) {
	base := t.TempDir()
	now := time.Now().Truncate(time.Second)

	for _, name := range []string{"snap-a", "snap-b", "snap-c"} {
		makeSnap(t, base, "auto", name)
	}
	setMtime(t, filepath.Join(base, "auto", "snap-a"), now.Add(-2*time.Hour))
	setMtime(t, filepath.Join(base, "auto", "snap-b"), now.Add(-1*time.Hour))
	setMtime(t, filepath.Join(base, "auto", "snap-c"), now)

	entries, err := snapstore.ListFromDir(base, "root", "/", "auto")
	if err != nil {
		t.Fatalf("ListFromDir: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	want := []string{"snap-c", "snap-b", "snap-a"}
	for i, w := range want {
		if entries[i].Name != w {
			t.Errorf("entries[%d].Name = %q, want %q", i, entries[i].Name, w)
		}
	}
}

// -----------------------------------------------------------------------
// ListFromDir — kind filter
// -----------------------------------------------------------------------

func TestListFromDir_KindFilter(t *testing.T) {
	base := t.TempDir()
	makeSnap(t, base, "auto", "auto-1")
	makeSnap(t, base, "auto", "auto-2")
	makeSnap(t, base, "manual", "my-snap")

	t.Run("auto", func(t *testing.T) {
		entries, err := snapstore.ListFromDir(base, "root", "/", "auto")
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 auto entries, got %d", len(entries))
		}
		for _, e := range entries {
			if e.Kind != "auto" {
				t.Errorf("got kind %q, want auto", e.Kind)
			}
		}
	})

	t.Run("manual", func(t *testing.T) {
		entries, err := snapstore.ListFromDir(base, "root", "/", "manual")
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].Name != "my-snap" {
			t.Errorf("unexpected manual entries: %v", entries)
		}
	})

	t.Run("all (empty)", func(t *testing.T) {
		entries, err := snapstore.ListFromDir(base, "root", "/", "")
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 3 {
			t.Errorf("expected 3 total entries, got %d", len(entries))
		}
	})
}

// -----------------------------------------------------------------------
// ListFromDir — target and source fields are populated correctly
// -----------------------------------------------------------------------

func TestListFromDir_FieldsPopulated(t *testing.T) {
	base := t.TempDir()
	makeSnap(t, base, "auto", "snap-1")

	entries, err := snapstore.ListFromDir(base, "home", "/home", "auto")
	if err != nil {
		t.Fatalf("ListFromDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Target != "home" {
		t.Errorf("Target = %q, want home", e.Target)
	}
	if e.Source != "/home" {
		t.Errorf("Source = %q, want /home", e.Source)
	}
	if e.Kind != "auto" {
		t.Errorf("Kind = %q, want auto", e.Kind)
	}
	wantPath := filepath.Join(base, "auto", "snap-1")
	if e.Path != wantPath {
		t.Errorf("Path = %q, want %q", e.Path, wantPath)
	}
}

// -----------------------------------------------------------------------
// ListTargets — merges multiple targets
// -----------------------------------------------------------------------

func TestListTargets_MergesTargets(t *testing.T) {
	// Redirect SnapshotBase to a temp directory so ListTargets reads from there
	// instead of the live /@snapshots path.
	base := t.TempDir()
	orig := config.SnapshotBase
	config.SnapshotBase = base
	defer func() { config.SnapshotBase = orig }()

	now := time.Now().Truncate(time.Second)

	// config.SnapshotDirFor("root") == base+"/root", etc.
	makeSnap(t, filepath.Join(base, "root"), "auto", "snap-2025")
	makeSnap(t, filepath.Join(base, "home"), "auto", "snap-2025")
	makeSnap(t, filepath.Join(base, "home"), "auto", "snap-2024")
	setMtime(t, filepath.Join(base, "root", "auto", "snap-2025"), now)
	setMtime(t, filepath.Join(base, "home", "auto", "snap-2025"), now.Add(-1*time.Minute))
	setMtime(t, filepath.Join(base, "home", "auto", "snap-2024"), now.Add(-1*time.Hour))

	targets := map[string]config.TargetConfig{
		"root": {Source: "/"},
		"home": {Source: "/home"},
	}

	entries, err := snapstore.ListTargets(targets, "auto")
	if err != nil {
		t.Fatalf("ListTargets: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries total, got %d", len(entries))
	}
	// Global newest-first: root/snap-2025, home/snap-2025, home/snap-2024.
	if entries[0].Target != "root" || entries[0].Name != "snap-2025" {
		t.Errorf("entries[0] = {%s %s}, want {root snap-2025}", entries[0].Target, entries[0].Name)
	}
}

// -----------------------------------------------------------------------
// ResolveAll
// -----------------------------------------------------------------------

func TestResolveAll_FindsAcrossTargets(t *testing.T) {
	base := t.TempDir()
	orig := config.SnapshotBase
	config.SnapshotBase = base
	defer func() { config.SnapshotBase = orig }()

	makeSnap(t, filepath.Join(base, "root"), "manual", "before-upgrade")
	makeSnap(t, filepath.Join(base, "home"), "manual", "before-upgrade")
	makeSnap(t, filepath.Join(base, "root"), "manual", "other")

	targets := map[string]config.TargetConfig{
		"root": {Source: "/"},
		"home": {Source: "/home"},
	}

	found, err := snapstore.ResolveAll(targets, "before-upgrade")
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	if len(found) != 2 {
		t.Errorf("expected 2 matches, got %d", len(found))
	}
	seen := map[string]bool{}
	for _, e := range found {
		seen[e.Target] = true
	}
	if !seen["root"] || !seen["home"] {
		t.Errorf("expected both root and home, got targets: %v", seen)
	}
}

func TestResolveAll_NotFound(t *testing.T) {
	base := t.TempDir()
	orig := config.SnapshotBase
	config.SnapshotBase = base
	defer func() { config.SnapshotBase = orig }()

	targets := map[string]config.TargetConfig{
		"root": {Source: "/"},
	}
	_, err := snapstore.ResolveAll(targets, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent snapshot, got nil")
	}
}

// -----------------------------------------------------------------------
// ResolveInTarget
// -----------------------------------------------------------------------

func TestResolveInTarget_Found(t *testing.T) {
	base := t.TempDir()
	makeSnap(t, base, "manual", "before-upgrade")

	entry, err := snapstore.ResolveInTarget(base, "root", "/", "before-upgrade")
	if err != nil {
		t.Fatalf("ResolveInTarget: %v", err)
	}
	if entry.Name != "before-upgrade" {
		t.Errorf("Name = %q, want before-upgrade", entry.Name)
	}
}

func TestResolveInTarget_NotFound(t *testing.T) {
	base := t.TempDir()
	_, err := snapstore.ResolveInTarget(base, "root", "/", "ghost")
	if err == nil {
		t.Fatal("expected error for missing snapshot, got nil")
	}
}

// -----------------------------------------------------------------------
// SelectForPrune
// -----------------------------------------------------------------------

func TestSelectForPrune(t *testing.T) {
	entries := []snapstore.Entry{
		{Name: "newest"},
		{Name: "middle"},
		{Name: "oldest"},
	}

	cases := []struct {
		keep    int
		wantLen int
		wantOld string
	}{
		{10, 0, ""},      // keep more than available → nil
		{3, 0, ""},       // keep exactly available → nil
		{2, 1, "oldest"}, // keep 2, prune 1
		{0, 3, "newest"}, // keep 0, prune all
		{-1, 3, "newest"}, // negative treated as 0
	}

	for _, tc := range cases {
		got := snapstore.SelectForPrune(entries, tc.keep)
		if len(got) != tc.wantLen {
			t.Errorf("keep=%d: len=%d, want %d", tc.keep, len(got), tc.wantLen)
			continue
		}
		if tc.wantLen > 0 && got[0].Name != tc.wantOld {
			t.Errorf("keep=%d: first pruned = %q, want %q", tc.keep, got[0].Name, tc.wantOld)
		}
	}
}
