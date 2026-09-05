package cmd

import (
	"fmt"

	"github.com/adnoctem/gopskit/internal/plattr/app"
	"github.com/adnoctem/gopskit/pkg/tools"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var _ app.CLIOpt = NewGenerateSmallstepValues // assure type compatibility

func NewGenerateSmallstepValues(app *app.State) *cobra.Command {
	var (
		name           string
		hostname       string
		address        string
		provisioner    string
		deploymentType string
		secretFile     string
	)

	cmd := &cobra.Command{
		Use:   "smallstep",
		Short: "Generate secrets for use with Smallstep's private CA",
		// Long:             "Generate secrets for use with Smallstep's private CA",
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			values, passphrase, err := tools.GenerateStepValues(
				tools.WithName(name),
				tools.WithHostname(hostname),
				tools.WithAddress(address),
				tools.WithProvisioner(provisioner),
				tools.WithDeploymentType(deploymentType),
			)

			if err != nil {
				return err
			}

			if secretFile != "" {
				_, err := tools.AddSecretStepValues(values, passphrase, secretFile)
				if err != nil {
					return err
				}

				app.Log.Infof("Added Smallstep values to Helm secret file: %s", secretFile)
				return nil
			}

			data, err := yaml.Marshal(values)
			if err != nil {
				return err
			}

			fmt.Printf(`Smallstep Values generated:
Passphrase: %s,
Values:
%s
`, passphrase, data)

			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&name, "name", tools.SmallstepDefaultName, "The name to give the private CA")
	cmd.PersistentFlags().StringVar(&hostname, "hostname", tools.SmallstepDefaultHostname, "The hostname for the private CA")
	cmd.PersistentFlags().StringVar(&address, "address", tools.SmallstepDefaultAddress, "The address to bind the private CA to")
	cmd.PersistentFlags().StringVar(&provisioner, "provisioner", tools.SmallstepDefaultProvisioner, "The provisioner for the private CA")
	cmd.PersistentFlags().StringVar(&deploymentType, "deployment-type", tools.SmallstepDefaultDeploymentType, "The deployment type for the private CA")
	cmd.PersistentFlags().StringVar(&secretFile, "secret-file", "", "A secret Helm-secrets file to inject values into")

	return cmd
}
