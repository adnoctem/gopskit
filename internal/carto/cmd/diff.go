package cmd

import (
	"fmt"

	"github.com/adnoctem/gopskit/internal/carto/app"
	"github.com/adnoctem/gopskit/internal/carto/config"
	apihelm "github.com/adnoctem/gopskit/pkg/helm"
	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/spf13/cobra"
)

var _ app.CLIOpt = NewDiffCommand // assure type compatibility

// NewDiffCommand creates the Option which injects the 'diff' subcommand into the 'carto' CLI
func NewDiffCommand(a *app.State) *cobra.Command {
	cmd := &cobra.Command{
		Use:              "diff <chart>",
		Short:            "Show the semantic diff between the local chart render and the cluster",
		Long:             "Performs a client-only dry-run render of the chart and compares it against the currently deployed release manifest",
		Args:             cobra.ExactArgs(1),
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			configPath := proc.Must(cmd.Flags().GetString("config"))
			chartsDir := proc.Must(cmd.Flags().GetString("charts-dir"))
			environment := proc.Must(cmd.Flags().GetString("environment"))

			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}

			cc, err := chartConfig(cfg, name)
			if err != nil {
				return err
			}

			vals, err := chartValues(cfg, chartsDir, name, environment)
			if err != nil {
				return err
			}

			hc, err := a.HelmClient(cc.Namespace)
			if err != nil {
				return err
			}

			ch, err := hc.LoadChart(name, cc.Repository, cc.Version)
			if err != nil {
				return err
			}

			proposed, err := hc.RenderManifest(name, ch, vals)
			if err != nil {
				return err
			}

			current, err := hc.CurrentManifest(name)
			if err != nil {
				a.Log.Warnf("could not fetch current release manifest for %q (likely not installed yet): %v", name, err)
				current = ""
			}

			d := apihelm.ManifestDiff(current, proposed)
			if d == "" {
				fmt.Println("no differences")
				return nil
			}

			fmt.Printf("Infrastructure Changes:\n%s\n", d)
			return nil
		},
	}

	return cmd
}
