// Package config loads the optional bootenv TOML configuration file.
package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// DefaultPath is the well-known location bootenv looks for its config file.
const DefaultPath = "/etc/bootenv/bootenv.toml"

// DefaultKeepAuto is the keep_auto value used when none is specified.
const DefaultKeepAuto = 10

// SnapshotBase is the root of the snapshot tree on the btrfs filesystem.
// It is a variable rather than a constant so tests can redirect it to a
// temporary directory without touching the live /@snapshots path.
var SnapshotBase = "/@snapshots"

// SnapshotDirFor returns the snapshot directory for a target named name.
// It is always <SnapshotBase>/<name> — the TOML block header is the
// directory name.
func SnapshotDirFor(name string) string {
	return SnapshotBase + "/" + name
}

// TargetConfig defines one snapshot target.
//
// The snapshot directory is not a field — it is always /@snapshots/<name>
// where <name> is the TOML block header (see SnapshotDirFor).
//
// KeepAuto is a pointer so that an explicit "keep_auto = 0" (keep nothing) is
// distinguishable from an absent key (which uses the built-in default of 10).
type TargetConfig struct {
	Source   string `toml:"source"`    // subvol to snapshot; default "/" for root, "/<name>" otherwise
	KeepAuto *int   `toml:"keep_auto"` // auto snapshots to keep; nil → DefaultKeepAuto
}

// GetKeepAuto returns the effective keep_auto value, applying the default when
// the field was not set in the config file.
func (tc TargetConfig) GetKeepAuto() int {
	if tc.KeepAuto == nil {
		return DefaultKeepAuto
	}
	return *tc.KeepAuto
}

// Config holds all configured snapshot targets keyed by their TOML block name.
// The "root" target is always present; it is injected with built-in defaults
// if the config file omits it or does not exist.
type Config struct {
	Targets map[string]TargetConfig
}

// sourceDefault returns the default source subvolume for a named target.
func sourceDefault(name string) string {
	if name == "root" {
		return "/"
	}
	return "/" + name
}

// DefaultConfig returns a Config containing only the root target with
// built-in defaults. This is what the tool behaves like when no config file
// is present.
func DefaultConfig() *Config {
	return &Config{
		Targets: map[string]TargetConfig{
			"root": {Source: sourceDefault("root")},
		},
	}
}

// Load reads the TOML config at path.
//
//   - If the file does not exist, DefaultConfig() is returned with no error.
//   - If the file exists but cannot be parsed, an error is returned.
//   - Any target with an empty Source gets a name-based default ("/" for root,
//     "/<name>" for anything else).
//   - The "root" target is always present; it is injected if the file omits it.
func Load(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	var raw map[string]TargetConfig
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if raw == nil {
		raw = make(map[string]TargetConfig)
	}

	// Fill in the source default for any target that omitted it.
	for name, tc := range raw {
		if tc.Source == "" {
			tc.Source = sourceDefault(name)
			raw[name] = tc
		}
	}

	// root is always present.
	if _, ok := raw["root"]; !ok {
		raw["root"] = TargetConfig{Source: sourceDefault("root")}
	}

	return &Config{Targets: raw}, nil
}
