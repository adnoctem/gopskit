package cmd

import (
	"fmt"

	"github.com/adnoctem/gopskit/internal/bridgecmd"
	"github.com/adnoctem/gopskit/internal/ssolo/app"
	"github.com/adnoctem/gopskit/pkg/config"
	"github.com/spf13/cobra"
)

var (
	// Commands is a slice of CLIOpt options for subcommands of the 'ssolo' CLI
	Commands = []app.CLIOpt{
		NewInitCommand,
		NewClientCommand,
	}
)

func NewRootCommand(ssolo *app.State) *cobra.Command {
	// run every persistent pre-run hook from the root down, so the configuration file still
	// loads under subcommands with hooks of their own, such as the proxy group
	cobra.EnableTraverseRunHooks = true

	cmd := &cobra.Command{
		Use:   ssolo.Name,
		Short: fmt.Sprintf("%s CLI", ssolo.Name),
		Long: `Manage single sign-on with Keycloak.

ssolo reaches Keycloak at --url: directly, e.g. through an Ingress, or through the gopskit bridge
for cluster-internal addresses such as https://keycloak.keycloak.svc:8443 - see 'ssolo proxy'.`,
		TraverseChildren: true,
		SilenceErrors:    true,
		SilenceUsage:     true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Usage()
			}

			return nil
		},
		// every flag can also come from the environment (where bound) or the configuration file
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return config.Load(app.Name, cmd)
		},
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
	}

	config.AddFlag(cmd, app.Name)

	// add subcommands
	for _, opt := range Commands {
		cmd.AddCommand(opt(ssolo))
	}

	cmd.AddCommand(bridgecmd.NewProxyCommand(bridgeOptions(ssolo)))

	return cmd
}
