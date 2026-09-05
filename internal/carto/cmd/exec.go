package cmd

import (
	"fmt"

	"github.com/adnoctem/gopskit/internal/carto/app"
	"github.com/adnoctem/gopskit/internal/carto/config"
	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/spf13/cobra"
)

var _ app.CLIOpt = NewExecCommand // assure type compatibility

// NewExecCommand creates the Option which injects the 'exec' subcommand into the 'carto' CLI
func NewExecCommand(a *app.State) *cobra.Command {
	var container string

	cmd := &cobra.Command{
		Use:              "exec <chart> -- <command> [args...]",
		Short:            "Open an interactive command in a chart's release pod",
		Long:             "Attaches an interactive TTY to a command run inside a chart's release pod. Live terminal resizing (SIGWINCH) is not tracked yet - only the initial size is sent.",
		Args:             cobra.MinimumNArgs(2),
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			command := args[1:]

			configPath := proc.Must(cmd.Flags().GetString("config"))

			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}

			cc, err := chartConfig(cfg, name)
			if err != nil {
				return err
			}

			pod, hasPod := releasePod(a, cc.Namespace, name)
			if !hasPod {
				return fmt.Errorf("could not find a release pod for chart %q in namespace %q", name, cc.Namespace)
			}

			if container == "" {
				container = pod.Spec.Containers[0].Name
			}

			return a.Kube.ExecTTY(pod, container, command)
		},
	}

	cmd.Flags().StringVarP(&container, "container", "C", "", "Container name (defaults to the pod's only container)")

	return cmd
}
