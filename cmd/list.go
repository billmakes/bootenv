package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"bootenv/internal/config"
	"bootenv/internal/snapstore"
)

func newListCmd() *cobra.Command {
	var typeFilter string
	var target string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List bootenv snapshots across configured targets",
		Long: `Prints a table of all snapshots across configured targets, newest-first.

Each row represents one snapshot subvolume. The TARGET column shows which
config block owns it. Use --target to restrict to one target, --type to
filter by auto or manual.`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runList(typeFilter, target)
		},
		Example: "  bootenv list\n" +
			"  bootenv list --type auto\n" +
			"  bootenv list --target root\n" +
			"  bootenv list -t manual -T home",
	}

	cmd.Flags().StringVarP(&typeFilter, "type", "t", "",
		`filter by snapshot type: "auto" or "manual"`)
	addTargetFlag(cmd, &target)
	return cmd
}

func runList(kind, targetFlag string) error {
	if kind != "" && kind != "auto" && kind != "manual" {
		return fmt.Errorf(`--type must be "auto" or "manual", got %q`, kind)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	targets, err := targetsForFlag(cfg, targetFlag)
	if err != nil {
		return err
	}

	entries, err := snapstore.ListTargets(targets, kind)
	if err != nil {
		return fmt.Errorf("listing snapshots: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("No snapshots found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TARGET\tTYPE\tNAME\tCREATED\tKERNEL\tPATH")
	fmt.Fprintln(w, "------\t----\t----\t-------\t------\t----")
	for _, e := range entries {
		kver := e.KernelVer
		if kver == "" {
			kver = "-"
		}
		created := "-"
		if !e.CreatedAt.IsZero() {
			created = e.CreatedAt.Format("2006-01-02 15:04:05")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			e.Target, e.Kind, e.Name, created, kver, e.Path)
	}
	return w.Flush()
}
