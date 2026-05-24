package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"bootenv/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	// Point at a path that definitely does not exist.
	cfg, err := config.Load(filepath.Join(t.TempDir(), "nonexistent.toml"))
	if err != nil {
		t.Fatalf("unexpected error for missing file: %v", err)
	}
	if cfg.Root.KeepAuto != config.Defaults.Root.KeepAuto {
		t.Errorf("Root.KeepAuto = %d, want %d", cfg.Root.KeepAuto, config.Defaults.Root.KeepAuto)
	}
	if cfg.Home.KeepAuto != config.Defaults.Home.KeepAuto {
		t.Errorf("Home.KeepAuto = %d, want %d", cfg.Home.KeepAuto, config.Defaults.Home.KeepAuto)
	}
}

func TestLoad_ParsesFile(t *testing.T) {
	toml := `
[root]
keep_auto = 7

[home]
keep_auto = 3
`
	path := writeTempTOML(t, toml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Root.KeepAuto != 7 {
		t.Errorf("Root.KeepAuto = %d, want 7", cfg.Root.KeepAuto)
	}
	if cfg.Home.KeepAuto != 3 {
		t.Errorf("Home.KeepAuto = %d, want 3", cfg.Home.KeepAuto)
	}
}

func TestLoad_PartialFile_KeepsDefaults(t *testing.T) {
	// Only [root] is specified; [home] should retain built-in default.
	toml := `
[root]
keep_auto = 20
`
	path := writeTempTOML(t, toml)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Root.KeepAuto != 20 {
		t.Errorf("Root.KeepAuto = %d, want 20", cfg.Root.KeepAuto)
	}
	if cfg.Home.KeepAuto != config.Defaults.Home.KeepAuto {
		t.Errorf("Home.KeepAuto = %d, want default %d",
			cfg.Home.KeepAuto, config.Defaults.Home.KeepAuto)
	}
}

func TestLoad_InvalidTOML(t *testing.T) {
	path := writeTempTOML(t, "this is not [ valid toml !!!!")
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML, got nil")
	}
}

// writeTempTOML writes content to a temp file and returns its path.
func writeTempTOML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bootenv.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp toml: %v", err)
	}
	return path
}
