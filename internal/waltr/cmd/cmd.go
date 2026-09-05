package cmd

import (
	"fmt"

	"github.com/fmjstudios/gopskit/internal/waltr/app"
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
	var (
		environment string
		label       string
		namespace   string
	)

	cmd := &cobra.Command{
		Use:              app.Name,
		Short:            fmt.Sprintf("%s CLI", app.Name),
		Long:             "Manage HashCorp Vault on Kubernetes",
		TraverseChildren: true,
		SilenceErrors:    true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				cmd.Usage()
			}

			return nil
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

	// add subcommands
	for _, opt := range Commands {
		cmd.AddCommand(opt(waltr))
	}

	return cmd
}
