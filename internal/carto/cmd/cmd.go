package cmd

import (
	"fmt"

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
			return config.Load(app.Name, cmd)
		},
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		SilenceUsage: true,
	}

	cmd.PersistentFlags().StringVarP(&backbone, "backbone", "b", app.DefaultBackboneFile, "Path to the backbone.yaml declaring the charts")
	cmd.PersistentFlags().StringVar(&chartsDir, "charts-dir", DefaultChartsDir, "Directory containing per-chart values.yaml/values.<environment>.yaml files")
	cmd.PersistentFlags().StringVarP(&environment, "environment", "e", "dev", "The execution environment to use (dev, stage, prod)")

	config.AddFlag(cmd, app.Name)
	carto.AddKubeFlags(cmd)

	// add subcommands
	for _, opt := range Commands {
		cmd.AddCommand(opt(carto))
	}

	return cmd
}
