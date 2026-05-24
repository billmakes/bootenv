// Package config loads the optional bootenv TOML configuration file.
package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// DefaultPath is the well-known location bootenv looks for its config file.
const DefaultPath = "/etc/bootenv/bootenv.toml"

// Config is the top-level configuration structure. All fields are optional;
// missing fields retain their built-in default values.
//
// Example /etc/bootenv/bootenv.toml:
//
//	[root]
//	keep_auto = 10   # keep this many auto snapshots of /
//
//	[home]
//	keep_auto = 5    # keep this many auto snapshots of /home
type Config struct {
	Root TargetConfig `toml:"root"`
	Home TargetConfig `toml:"home"`
}

// TargetConfig holds per-target (root or home) retention settings.
type TargetConfig struct {
	KeepAuto int `toml:"keep_auto"`
}

// Defaults holds the built-in values used when the config file is absent or
// a field is not specified.
var Defaults = Config{
	Root: TargetConfig{KeepAuto: 10},
	Home: TargetConfig{KeepAuto: 5},
}

// Load reads the TOML file at path and returns the resulting Config.
// If the file does not exist, Load returns Defaults with no error — the config
// file is entirely optional. A parse error is always returned as an error.
func Load(path string) (*Config, error) {
	cfg := Defaults // start from defaults so absent keys keep their values

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &cfg, nil
	}

	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	return &cfg, nil
}
