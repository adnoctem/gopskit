package cmd

import (
	"fmt"
	"os"

	"github.com/adnoctem/gopskit/internal/carto/app"
	"github.com/adnoctem/gopskit/internal/carto/config"
	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var _ app.CLIOpt = NewRenderCommand // assure type compatibility

// NewRenderCommand creates the Option which injects the 'render' subcommand into the 'carto' CLI
func NewRenderCommand(a *app.State) *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:              "render <chart>",
		Short:            "Render a chart's merged values",
		Long:             "Merges a chart's global, base, and environment-overlay values files and prints the result",
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

			if _, err := chartConfig(cfg, name); err != nil {
				return err
			}

			vals, err := chartValues(cfg, chartsDir, name, environment)
			if err != nil {
				return err
			}

			out, err := yaml.Marshal(vals)
			if err != nil {
				return fmt.Errorf("could not render values as YAML: %w", err)
			}

			if output != "" {
				return os.WriteFile(output, out, 0644)
			}

			fmt.Print(string(out))
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "", "Write the rendered values to a file instead of stdout")

	return cmd
}
