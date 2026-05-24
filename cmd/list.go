package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"bootenv/internal/snapstore"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all bootenv snapshots",
		Long:  `Prints a table of all auto and manual snapshots with their kernel version.`,
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runList()
		},
	}
}

func runList() error {
	entries, err := snapstore.List()
	if err != nil {
		return fmt.Errorf("listing snapshots: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No snapshots found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TYPE\tNAME\tKERNEL\tROOT PATH")
	fmt.Fprintln(w, "----\t----\t------\t---------")
	for _, e := range entries {
		kver := e.KernelVer
		if kver == "" {
			kver = "(unknown)"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Kind, e.Name, kver, e.RootPath)
	}
	return w.Flush()
}
