package cmd

import (
	"fmt"

	"github.com/adnoctem/gopskit/internal/carto/app"
	"github.com/adnoctem/gopskit/internal/carto/config"
	apihelm "github.com/adnoctem/gopskit/pkg/helm"
	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/spf13/cobra"
	"helm.sh/helm/v3/pkg/chart"
)

var _ app.CLIOpt = NewDiffCommand // assure type compatibility

// NewDiffCommand creates the Option which injects the 'diff' subcommand into the 'carto' CLI
func NewDiffCommand(a *app.State) *cobra.Command {
	return newDiffCommand(a, func(namespace string) (diffHelm, error) { return a.HelmClient(namespace) })
}

type diffHelm interface {
	LoadChart(name, repository, version string) (*chart.Chart, error)
	RenderManifest(name string, ch *chart.Chart, vals map[string]interface{}) (string, error)
	CurrentManifest(name string) (string, error)
}

func newDiffCommand(a *app.State, helmClient func(string) (diffHelm, error)) *cobra.Command {
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

			hc, err := helmClient(cc.Namespace)
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

			if err := previewNamespaces(cmd.OutOrStdout(), cfg.Namespaces); err != nil {
				return err
			}
			d := apihelm.ManifestDiff(current, proposed)
			if d == "" {
				message := "no differences"
				if len(cfg.Namespaces) > 0 {
					message = "no chart manifest differences"
				}
				_, err := fmt.Fprintln(cmd.OutOrStdout(), message)
				return err
			}

			_, err = fmt.Fprint(cmd.OutOrStdout(), d)
			return err
		},
	}

	return cmd
}
