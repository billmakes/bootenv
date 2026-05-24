package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"bootenv/internal/config"
)

// addTargetFlag attaches --target / -T to cmd.
// The default is "" which means "all configured targets".
func addTargetFlag(cmd *cobra.Command, v *string) {
	cmd.Flags().StringVarP(v, "target", "T", "",
		`config target to act on (e.g. "root", "home"); default is all configured targets`)
}

// targetsForFlag returns the subset of config targets to operate on.
// If flag is "", all targets are returned unchanged.
// If flag names a target not in cfg, an error is returned listing what is available.
func targetsForFlag(cfg *config.Config, flag string) (map[string]config.TargetConfig, error) {
	if flag == "" {
		return cfg.Targets, nil
	}
	tc, ok := cfg.Targets[flag]
	if !ok {
		available := strings.Join(sortedTargetNames(cfg.Targets), ", ")
		return nil, fmt.Errorf("target %q not in config; available: %s", flag, available)
	}
	return map[string]config.TargetConfig{flag: tc}, nil
}

// sortedTargetNames returns target names with "root" always first, then the
// remaining names in alphabetical order.
func sortedTargetNames(targets map[string]config.TargetConfig) []string {
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == "root" {
			return true
		}
		if names[j] == "root" {
			return false
		}
		return names[i] < names[j]
	})
	return names
}
