package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/adnoctem/gopskit/internal/carto/app"
	"github.com/adnoctem/gopskit/pkg/config"
	"github.com/spf13/cobra"
)

var (
	// Commands is a slice of CLIOpt options for subcommands of the 'carto' CLI
	Commands = []app.CLIOpt{
		NewRenderCommand,
		NewDiffCommand,
		NewApplyCommand,
		NewHistoryCommand,
		NewRollbackCommand,
		NewLogsCommand,
		NewExecCommand,
	}
)

func NewRootCommand(carto *app.State) *cobra.Command {
	var (
		backbone    string
		chartsDir   string
		environment string
	)

	// run every persistent pre-run hook from the root down, so the configuration file still
	// loads under subcommands with hooks of their own
	cobra.EnableTraverseRunHooks = true

	cmd := &cobra.Command{
		Use:              app.Name,
		Short:            fmt.Sprintf("%s CLI", app.Name),
		Long:             "Manage Kubernetes backbone components with a deterministic Helm values merge strategy",
		TraverseChildren: true,
		SilenceErrors:    true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Usage()
			}

			return nil
		},
		// every flag can also come from the configuration file, carto.yaml
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return loadSettings(cmd)
		},
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		SilenceUsage: true,
	}

	cmd.PersistentFlags().StringVarP(&backbone, "backbone", "b", app.DefaultBackboneFile, "Path to the backbone.yaml declaring the charts")
	cmd.PersistentFlags().StringVar(&chartsDir, "charts-dir", DefaultChartsDir, "Directory containing per-chart values.yaml/values.<environment>.yaml files")
	cmd.PersistentFlags().StringVarP(&environment, "environment", "e", "dev", "The environment, which selects each chart's values.<environment>.yaml overlay")

	config.AddFlag(cmd, app.Name)
	carto.AddKubeFlags(cmd)

	// add subcommands
	for _, opt := range Commands {
		cmd.AddCommand(opt(carto))
	}

	return cmd
}

// loadSettings resolves only explicitly configured Carto input paths against the
// selected settings file. Capture CLI provenance before Load: the shared loader
// also marks flags Changed when it supplies their values from the settings file.
func loadSettings(cmd *cobra.Command) error {
	var candidates []string
	for _, name := range []string{"backbone", "charts-dir"} {
		if !cmd.Flags().Changed(name) {
			candidates = append(candidates, name)
		}
	}
	if err := config.Load(app.Name, cmd); err != nil {
		return err
	}
	path := config.Path(cmd)
	if path == "" {
		return nil
	}
	for _, name := range candidates {
		flag := cmd.Flags().Lookup(name)
		// Carto has no environment bindings for these flags. A newly Changed
		// flag came from the file, including a command-scoped setting.
		if !flag.Changed || filepath.IsAbs(flag.Value.String()) {
			continue
		}
		if err := cmd.Flags().Set(name, filepath.Join(filepath.Dir(path), flag.Value.String())); err != nil {
			return fmt.Errorf("could not resolve %s from settings %q: %w", name, path, err)
		}
	}
	return nil
}
