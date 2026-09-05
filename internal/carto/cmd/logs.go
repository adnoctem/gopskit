package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/fmjstudios/gopskit/internal/carto/app"
	"github.com/fmjstudios/gopskit/internal/carto/config"
	"github.com/fmjstudios/gopskit/pkg/proc"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
)

var _ app.CLIOpt = NewLogsCommand // assure type compatibility

// NewLogsCommand creates the Option which injects the 'logs' subcommand into the 'carto' CLI
func NewLogsCommand(a *app.State) *cobra.Command {
	var (
		follow    bool
		container string
	)

	cmd := &cobra.Command{
		Use:              "logs <chart>",
		Short:            "Stream logs from a chart's release pod",
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

			pod, hasPod := releasePod(a, cc.Namespace, name)
			if !hasPod {
				return fmt.Errorf("could not find a release pod for chart %q in namespace %q", name, cc.Namespace)
			}

			stream, err := a.Kube.Logs(pod.Namespace, pod.Name, corev1.PodLogOptions{
				Container: container,
				Follow:    follow,
			})
			if err != nil {
				return fmt.Errorf("could not stream logs for pod %q: %w", pod.Name, err)
			}
			defer stream.Close()

			_, err = io.Copy(os.Stdout, bufio.NewReader(stream))
			return err
		},
	}

	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Stream logs continuously")
	cmd.Flags().StringVarP(&container, "container", "C", "", "Container name (defaults to the pod's only container)")

	return cmd
}
