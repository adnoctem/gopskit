package cmd

import (
	"fmt"

	"github.com/adnoctem/gopskit/internal/carto/app"
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
		config      string
		chartsDir   string
		environment string
	)

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
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		SilenceUsage: true,
	}

	cmd.PersistentFlags().StringVarP(&config, "config", "c", app.DefaultBackboneFile, "Path to the backbone.yaml configuration file")
	cmd.PersistentFlags().StringVar(&chartsDir, "charts-dir", DefaultChartsDir, "Directory containing per-chart values.yaml/values.<environment>.yaml files")
	cmd.PersistentFlags().StringVarP(&environment, "environment", "e", "dev", "The execution environment to use (dev, stage, prod)")

	// add subcommands
	for _, opt := range Commands {
		cmd.AddCommand(opt(carto))
	}

	return cmd
}
