package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"bootenv/internal/config"
)

func TestLoad_MissingFile_ReturnsDefaultConfig(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "nonexistent.toml"))
	if err != nil {
		t.Fatalf("unexpected error for missing file: %v", err)
	}

	root, ok := cfg.Targets["root"]
	if !ok {
		t.Fatal("root target missing from default config")
	}
	if root.Source != "/" {
		t.Errorf("root.Source = %q, want /", root.Source)
	}
	if root.GetKeepAuto() != config.DefaultKeepAuto {
		t.Errorf("root.GetKeepAuto() = %d, want %d", root.GetKeepAuto(), config.DefaultKeepAuto)
	}
	if got := config.SnapshotDirFor("root"); got != "/@snapshots/root" {
		t.Errorf("SnapshotDirFor(root) = %q, want /@snapshots/root", got)
	}
}

func TestLoad_OnlyRootTarget(t *testing.T) {
	path := writeTOML(t, `
[root]
keep_auto = 7
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	root := cfg.Targets["root"]
	if root.GetKeepAuto() != 7 {
		t.Errorf("root.GetKeepAuto() = %d, want 7", root.GetKeepAuto())
	}
	if root.Source != "/" {
		t.Errorf("root.Source = %q, want /", root.Source)
	}
}

func TestLoad_MultipleTargets(t *testing.T) {
	path := writeTOML(t, `
[root]
keep_auto = 10

[home]
keep_auto = 3

[var]
source    = "/var"
keep_auto = 2
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := cfg.Targets["root"]; !ok {
		t.Error("root target missing")
	}

	home, ok := cfg.Targets["home"]
	if !ok {
		t.Fatal("home target missing")
	}
	if home.GetKeepAuto() != 3 {
		t.Errorf("home.GetKeepAuto() = %d, want 3", home.GetKeepAuto())
	}
	if home.Source != "/home" {
		t.Errorf("home.Source = %q, want /home", home.Source)
	}
	if got := config.SnapshotDirFor("home"); got != "/@snapshots/home" {
		t.Errorf("SnapshotDirFor(home) = %q, want /@snapshots/home", got)
	}

	v := cfg.Targets["var"]
	if v.Source != "/var" {
		t.Errorf("var.Source = %q, want /var", v.Source)
	}
	if v.GetKeepAuto() != 2 {
		t.Errorf("var.GetKeepAuto() = %d, want 2", v.GetKeepAuto())
	}
}

func TestLoad_RootInjectedWhenAbsent(t *testing.T) {
	// A config file that only defines [home] must still get [root] injected.
	path := writeTOML(t, `
[home]
keep_auto = 5
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := cfg.Targets["root"]; !ok {
		t.Error("root target should be injected even when not in the file")
	}
}

func TestLoad_KeepAutoZeroIsRespected(t *testing.T) {
	// keep_auto = 0 means "keep nothing" and must not be treated as "unset".
	path := writeTOML(t, `
[root]
keep_auto = 0
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Targets["root"].GetKeepAuto() != 0 {
		t.Errorf("expected GetKeepAuto()=0, got %d", cfg.Targets["root"].GetKeepAuto())
	}
}

func TestLoad_InvalidTOML(t *testing.T) {
	path := writeTOML(t, "this is not [ valid ] toml !!!!")
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML, got nil")
	}
}

func TestGetKeepAuto_NilUsesDefault(t *testing.T) {
	tc := config.TargetConfig{} // KeepAuto is nil
	if got := tc.GetKeepAuto(); got != config.DefaultKeepAuto {
		t.Errorf("GetKeepAuto() = %d, want %d", got, config.DefaultKeepAuto)
	}
}

// writeTOML writes content to a temp file and returns its path.
func writeTOML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bootenv.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp toml: %v", err)
	}
	return path
}
