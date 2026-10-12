package cmd

import (
	"fmt"

	"github.com/adnoctem/gopskit/internal/bridgecmd"
	"github.com/adnoctem/gopskit/internal/steppa/app"
	"github.com/adnoctem/gopskit/pkg/config"
	"github.com/spf13/cobra"
)

var (
	// Commands is a slice of CLIOpt options for subcommands of the 'steppa' CLI
	Commands = []app.CLIOpt{
		NewInitCommand,
	}
)

func NewRootCommand(steppa *app.State) *cobra.Command {
	// run every persistent pre-run hook from the root down, so the configuration file still
	// loads under subcommands with hooks of their own, such as the proxy group
	cobra.EnableTraverseRunHooks = true

	cmd := &cobra.Command{
		Use:   app.Name,
		Short: fmt.Sprintf("%s CLI", app.Name),
		Long: `Create a private certificate authority for Smallstep's step-ca and its Helm chart,
smallstep/step-certificates.`,
		TraverseChildren: true,
		SilenceErrors:    true,
		SilenceUsage:     true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Usage()
			}

			return nil
		},
		// every flag can also come from the environment (where bound) or steppa.yaml
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return config.Load(app.Name, cmd)
		},
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
	}

	config.AddFlag(cmd, app.Name)
	steppa.AddKubeFlags(cmd)

	for _, opt := range Commands {
		cmd.AddCommand(opt(steppa))
	}

	cmd.AddCommand(bridgecmd.NewProxyCommand(bridgeOptions(steppa)))

	return cmd
}

// bridgeOptions describes steppa to the bridge commands and helpers.
func bridgeOptions(s *app.State) bridgecmd.Options {
	return bridgecmd.Options{
		CLI:     app.Name,
		Version: s.Stamp.Version,
		Kube:    s.KubeClient,
		Log:     s.Log,
	}
}
