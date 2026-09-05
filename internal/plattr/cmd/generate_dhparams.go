package cmd

import (
	"fmt"

	"github.com/adnoctem/gopskit/internal/plattr/app"
	"github.com/adnoctem/gopskit/pkg/helpers"
	"github.com/spf13/cobra"
)

var _ app.CLIOpt = NewGenerateDHParamsCommand // assure type compatibility

func NewGenerateDHParamsCommand(app *app.State) *cobra.Command {
	var (
		bits int
	)

	cmd := &cobra.Command{
		Use:              "dhparams",
		Short:            "Generate Diffie-Hellman parameters",
		Long:             "Generate Diffie-Hellman parameters for use within Kubernetes Secret",
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := make([]helpers.DiffieHellmanOpt, 0)

			if bits != helpers.DHParamDefaultBits {
				opts = append(opts, helpers.WithBits(bits))
			}

			params, err := helpers.GenerateDiffieHellmanParams(opts...)
			if err != nil {
				return err
			}

			fmt.Println(params)
			return nil
		},
	}

	cmd.PersistentFlags().IntVar(&bits, "bits", helpers.DHParamDefaultBits, "The default amount of bits to generate parameters for")

	return cmd
}
