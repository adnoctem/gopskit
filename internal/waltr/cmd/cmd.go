package cmd

import (
	"fmt"

	"github.com/adnoctem/gopskit/internal/bridgecmd"
	"github.com/adnoctem/gopskit/internal/waltr/app"
	"github.com/adnoctem/gopskit/pkg/config"
	"github.com/spf13/cobra"
)

var (
	// Commands is a slice of CLIOpt options for subcommands of the 'waltr' CLI
	Commands = []app.CLIOpt{
		NewInitCommand,
		NewMountsCommand,
		NewConfigureCommand,
		NewPrepareCommand,
		NewTransitCommand,
	}

	// PrepareSubcommands is a slice of CLIOpt options for subcommands of the 'prepare' subcommand
	PrepareSubcommands = []app.CLIOpt{
		NewPrepareKeycloakCommand,
		NewPrepareGitLabCommand,
	}
)

func NewRootCommand(waltr *app.State) *cobra.Command {
	// run every persistent pre-run hook from the root down, so the configuration file still
	// loads under subcommands with hooks of their own, such as the proxy group
	cobra.EnableTraverseRunHooks = true

	var (
		environment string
		label       string
		namespace   string
	)

	cmd := &cobra.Command{
		Use:              app.Name,
		Short:            fmt.Sprintf("%s CLI", app.Name),
		Long:             "Manage HashiCorp Vault on Kubernetes",
		TraverseChildren: true,
		SilenceErrors:    true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Usage()
			}

			return nil
		},
		// every flag can also come from the environment (where bound) or the configuration file;
		// commands that need the cluster create its client themselves
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return config.Load(app.Name, cmd)
		},
		CompletionOptions: cobra.CompletionOptions{
			HiddenDefaultCmd: true,
		},
		SilenceUsage: true,
	}

	cmd.PersistentFlags().StringVarP(&environment, "environment", "e", "dev", "The execution environment to use (dev, stage, prod)")
	cmd.PersistentFlags().StringVarP(&label, "label", "l", app.DefaultLabel, "The Kubernetes label to filter resources by")
	cmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "",
		"The Kubernetes namespace to use. None equates to checking the entire cluster.")
	cmd.PersistentFlags().StringVar(&waltr.CredentialFile, "credential-file", "",
		"Where 'waltr init' stores the root token and keys, and other commands read them (default: per --environment in the cache directory)")
	config.AddFlag(cmd, app.Name)

	// add subcommands
	for _, opt := range Commands {
		cmd.AddCommand(opt(waltr))
	}

	cmd.AddCommand(bridgecmd.NewProxyCommand(bridgeOptions(waltr)))

	return cmd
}
