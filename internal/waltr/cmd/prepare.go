package cmd

import (
	"github.com/fmjstudios/gopskit/internal/waltr/app"
	"github.com/spf13/cobra"
)

var _ app.CLIOpt = NewPrepareCommand // assure type compatibility

func NewPrepareCommand(app *app.State) *cobra.Command {
	cmd := &cobra.Command{
		Use:              "prepare",
		Short:            "Prepare Vault for various applications",
		Long:             "Prepare Vault for various applications like GitLab, AWX or Keycloak",
		TraverseChildren: true,
	}

	// subcommands
	for _, subc := range PrepareSubcommands {
		cmd.AddCommand(subc(app))
	}

	return cmd
}

func addPrepareFlags(cmd *cobra.Command, overwrite *bool, token *string) {
	cmd.PersistentFlags().BoolVar(overwrite, "overwrite", false, "Overwrite existing configuration")
	cmd.PersistentFlags().StringVarP(token, "token", "t", "", "The Vault root token")
}
