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
		Short:            "Show a unified diff from the deployed release to the local chart render",
		Long:             "Performs a client-only dry-run render of the chart and compares it against the currently deployed release manifest",
		Args:             cobra.ExactArgs(1),
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			configPath := proc.Must(cmd.Flags().GetString("backbone"))
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
			switch {
			case apihelm.IsReleaseNotFound(err):
				a.Log.Infof("%q is not installed yet; showing everything it would create", name)
				current = ""
			case err != nil:
				return fmt.Errorf("could not fetch the current manifest of %q: %w", name, err)
			}

			d := apihelm.ManifestDiff(current, proposed)
			if d == "" {
				fmt.Println("no differences")
				return nil
			}

			fmt.Print(d)
			return nil
		},
	}

	return cmd
}
