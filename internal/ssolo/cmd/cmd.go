package cmd

import (
	"fmt"

	"github.com/fmjstudios/gopskit/internal/ssolo/app"
	"github.com/spf13/cobra"
)

var (
	// Commands is a slice of CLIOpt options for subcommands of the 'ssolo' CLI
	Commands = []app.CLIOpt{
		NewInitCommand,
		NewGitLabCommand,
	}
)

func NewRootCommand(ssolo *app.State) *cobra.Command {
	var (
		label     string
		namespace string
	)

	cmd := &cobra.Command{
		Use:              ssolo.Name,
		Short:            fmt.Sprintf("%s CLI", ssolo.Name),
		Long:             "Manage authentication for Kubernetes applications using Keycloak",
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
	}

	// All Kubernetes Flags
	//a.KubeClient.Flags.Namespace = util.StrPtr(app.DefaultNamespace)
	//a.KubeClient.Flags.AddFlags(cmd.PersistentFlags())

	addKubernetesFlags(cmd, &label, &namespace)

	// add subcommands
	for _, opt := range Commands {
		cmd.AddCommand(opt(ssolo))
	}

	return cmd
}

func addKubernetesFlags(cmd *cobra.Command, label, namespace *string) {
	cmd.PersistentFlags().StringVarP(label, "label", "l", app.DefaultLabel, "The Kubernetes label to filter resources by")
	cmd.PersistentFlags().StringVarP(namespace, "namespace", "n", app.DefaultNamespace,
		"The Kubernetes namespace to use. None equates to checking the entire cluster.")
}
