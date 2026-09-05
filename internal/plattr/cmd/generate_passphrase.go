package cmd

import (
	"fmt"

	"github.com/adnoctem/gopskit/internal/plattr/app"
	"github.com/adnoctem/gopskit/pkg/helpers"
	"github.com/spf13/cobra"
)

var _ app.CLIOpt = NewGeneratePassphraseCommand // assure type compatibility

func NewGeneratePassphraseCommand(app *app.State) *cobra.Command {
	var (
		charset string
		length  int
	)

	cmd := &cobra.Command{
		Use:              "passphrase",
		Short:            "Generate passphrases",
		Long:             "Generate passphrases for use within Kubernetes Secret",
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := make([]helpers.PassphraseOpt, 0)

			if charset != helpers.PassphraseDefaultCharset {
				opts = append(opts, helpers.WithCharSet(charset))
			}

			if length != helpers.PassphraseDefaultLength {
				opts = append(opts, helpers.WithLength(length))
			}

			fmt.Println(helpers.GeneratePassphrase(opts...))
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&charset, "charset", helpers.PassphraseDefaultCharset, "A custom charset to use as source for the passphrase")
	cmd.PersistentFlags().IntVar(&length, "length", helpers.PassphraseDefaultLength, "The length of the generated passphrase")

	return cmd
}
