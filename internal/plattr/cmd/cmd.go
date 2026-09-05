package cmd

import (
	"fmt"

	"github.com/adnoctem/gopskit/internal/plattr/app"
	"github.com/spf13/cobra"
)

var (
	// Commands is a slice of CLIOpt options for subcommands of the 'plattr' CLI
	Commands = []app.CLIOpt{
		NewAWSCommand,
		NewCloudflareCommand,
		NewHetznerEncryptionCommand,
		NewLonghornEncryptionCommand,
		NewNginxCommand,
		NewGenerateCommand,
	}

	// GenerateSubcommands is a slice of CLIOpt options for subcommands of the 'generate' subcommand
	GenerateSubcommands = []app.CLIOpt{
		NewGeneratePassphraseCommand,
		NewGenerateDHParamsCommand,
		NewGenerateSmallstepValues,
	}
)

func NewRootCommand(plattr *app.State) *cobra.Command {
	var (
		namespace string
		reflect   bool
	)

	cmd := &cobra.Command{
		Use:              app.Name,
		Short:            fmt.Sprintf("%s CLI", app.Name),
		Long:             "Integrate Kubernetes with various Platforms",
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

	cmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", app.DefaultNamespace, "The Kubernetes namespace to use. None equates to checking the entire cluster.")
	cmd.PersistentFlags().BoolVar(&reflect, "reflect", false, "Enable reflection for the created resources via Reflector.")

	// add subcommands
	for _, opt := range Commands {
		cmd.AddCommand(opt(plattr))
	}

	return cmd
}
