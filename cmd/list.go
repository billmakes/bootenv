package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"bootenv/internal/snapstore"
)

func newListCmd() *cobra.Command {
	var typeFilter string
	var target string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List bootenv snapshots",
		Long: `Prints a table of snapshots with type, name, creation time, root/home
presence, kernel version, and path. Snapshots are shown newest-first.

ROOT and HOME columns show whether each subvolume component exists (✓/✗).
An entry appears even when only one side is present.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runList(typeFilter, target)
		},
		Example: "  bootenv list\n  bootenv list --type auto\n  bootenv list --type manual\n  bootenv list --target root",
	}

	cmd.Flags().StringVarP(&typeFilter, "type", "t", "",
		`filter by snapshot type: "auto" or "manual"`)
	// For list, default is "" (show all); --target filters to entries that have
	// the specified component present.
	cmd.Flags().StringVarP(&target, "target", "T", "",
		`show only snapshots with this component present: "root", "home", or "both" (both present)`)

	return cmd
}

func runList(kind, target string) error {
	if kind != "" && kind != "auto" && kind != "manual" {
		return fmt.Errorf(`--type must be "auto" or "manual", got %q`, kind)
	}
	// Validate target (reuse parseTarget, but "" is valid here meaning "all").
	if target != "" {
		if _, _, err := parseTarget(target); err != nil {
			return err
		}
	}

	entries, err := snapstore.ListFiltered(kind)
	if err != nil {
		return fmt.Errorf("listing snapshots: %w", err)
	}

	if target != "" {
		entries = snapstore.FilterTarget(entries, target)
	}

	if len(entries) == 0 {
		switch {
		case kind != "" && target != "":
			fmt.Printf("No %s snapshots found with target %q.\n", kind, target)
		case kind != "":
			fmt.Printf("No %s snapshots found.\n", kind)
		case target != "":
			fmt.Printf("No snapshots found with target %q.\n", target)
		default:
			fmt.Println("No snapshots found.")
		}
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TYPE\tNAME\tCREATED\tROOT\tHOME\tKERNEL\tROOT PATH")
	fmt.Fprintln(w, "----\t----\t-------\t----\t----\t------\t---------")
	for _, e := range entries {
		kver := e.KernelVer
		if kver == "" {
			kver = "(unknown)"
		}
		created := "(unknown)"
		if !e.CreatedAt.IsZero() {
			created = e.CreatedAt.Format("2006-01-02 15:04:05")
		}
		rootMark, homeMark := "✗", "✗"
		if e.HasRoot {
			rootMark = "✓"
		}
		if e.HasHome {
			homeMark = "✓"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			e.Kind, e.Name, created, rootMark, homeMark, kver, e.RootPath)
	}
	return w.Flush()
}
