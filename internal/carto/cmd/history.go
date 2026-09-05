package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/adnoctem/gopskit/internal/carto/app"
	"github.com/adnoctem/gopskit/internal/carto/config"
	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

var _ app.CLIOpt = NewHistoryCommand // assure type compatibility

// NewHistoryCommand creates the Option which injects the 'history' subcommand into the 'carto' CLI
func NewHistoryCommand(a *app.State) *cobra.Command {
	var max int

	cmd := &cobra.Command{
		Use:              "history <chart>",
		Short:            "List a chart's release revisions",
		Args:             cobra.ExactArgs(1),
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			configPath := proc.Must(cmd.Flags().GetString("config"))

			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}

			cc, err := chartConfig(cfg, name)
			if err != nil {
				return err
			}

			hc, err := a.HelmClient(cc.Namespace)
			if err != nil {
				return err
			}

			releases, err := hc.History(name, max)
			if err != nil {
				return fmt.Errorf("could not fetch release history for %q: %w", name, err)
			}

			table := tablewriter.NewWriter(os.Stdout)
			table.Header("REVISION", "STATUS", "CHART", "APP VERSION", "DESCRIPTION")
			for _, rel := range releases {
				if err := table.Append(
					strconv.Itoa(rel.Version),
					rel.Info.Status.String(),
					fmt.Sprintf("%s-%s", rel.Chart.Metadata.Name, rel.Chart.Metadata.Version),
					rel.Chart.Metadata.AppVersion,
					rel.Info.Description,
				); err != nil {
					return err
				}
			}

			return table.Render()
		},
	}

	cmd.Flags().IntVar(&max, "max", 10, "Maximum number of revisions to list")

	return cmd
}
