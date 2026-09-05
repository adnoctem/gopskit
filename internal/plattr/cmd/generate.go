package cmd

import (
	"github.com/adnoctem/gopskit/internal/plattr/app"
	"github.com/spf13/cobra"
)

var _ app.CLIOpt = NewGenerateCommand // assure type compatibility

func NewGenerateCommand(app *app.State) *cobra.Command {
	cmd := &cobra.Command{
		Use:              "generate",
		Short:            "Generate passphrases and Diffie-Hellman parameters",
		Long:             "Generate passphrases and Diffie-Hellman parameters for use within Kubernetes Secret",
		TraverseChildren: true,
	}

	// subcommands
	for _, subc := range GenerateSubcommands {
		cmd.AddCommand(subc(app))
	}

	return cmd
}
