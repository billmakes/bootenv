package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// addTargetFlag attaches --target / -T to cmd with a consistent description.
func addTargetFlag(cmd *cobra.Command, v *string, defaultVal string) {
	cmd.Flags().StringVarP(v, "target", "T", defaultVal,
		`subvolumes to act on: "root", "home", or "both"`)
}

// parseTarget maps a --target string to (wantRoot, wantHome).
// "both" and "" both mean operate on both sides.
func parseTarget(t string) (wantRoot, wantHome bool, err error) {
	switch t {
	case "root":
		return true, false, nil
	case "home":
		return false, true, nil
	case "both", "":
		return true, true, nil
	default:
		return false, false, fmt.Errorf(`--target must be "root", "home", or "both", got %q`, t)
	}
}
