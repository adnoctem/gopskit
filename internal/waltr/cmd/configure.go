package cmd

import (
	"context"
	"fmt"

	"github.com/adnoctem/gopskit/internal/waltr/app"
	"github.com/adnoctem/gopskit/internal/waltr/util"
	apivault "github.com/adnoctem/gopskit/pkg/api/vault"
	"github.com/adnoctem/gopskit/pkg/core"
	"github.com/adnoctem/gopskit/pkg/helpers"
	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/spf13/cobra"
)

var _ app.CLIOpt = NewConfigureCommand

func NewConfigureCommand(app *app.State) *cobra.Command {
	var (
		token     string // Vault token
		overwrite bool
	)

	cmd := &cobra.Command{
		Use:              "configure",
		Short:            "Configure Vault",
		Aliases:          []string{"conf", "config"},
		Long:             "Configure ACL and password policies within Vault",
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			envF := proc.Must(cmd.Flags().GetString("environment"))
			label := proc.Must(cmd.Flags().GetString("label"))
			environment := proc.Must(core.EnvFromString(envF))

			pods, err := util.Pods(app, "", label)
			if err != nil {
				return fmt.Errorf("could not retrieve Vault pods for label: %s. Error: %v", label, err)
			}

			if token == "" {
				creds, err := util.ReadCredentials(app, environment)
				if err != nil {
					msg := fmt.Errorf("token option is unset and could not read credentials: %w", err)
					app.Log.Error(msg)
					return err
				}

				token = creds.Token
			}

			// port-forward the (leader)
			app.Log.Infof("Port-forwarding Vault instance: %s", pods[0].Name)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			readyChan := make(chan struct{})
			go func(rc chan struct{}) {
				err := app.Kube.PortForward(ctx, pods[0], rc)
				if err != nil {
					panic(err)
				}
			}(readyChan)
			<-readyChan

			// add token
			if err := app.Vault.SetToken(&apivault.Credentials{Token: token}); err != nil {
				return fmt.Errorf("could not set token: %v", err)
			}

			// get current policies
			pol, err := util.Policies(app)
			if err != nil {
				return err
			}

			// get current password policies
			ppol, err := util.PasswordPolicies(app)
			if err != nil {
				return err
			}

			// current policies for Helm releases
			for _, v := range util.Releases {
				if !helpers.SliceContains(pol, v) || overwrite {
					err := app.Vault.PoliciesWriteAclPolicy(context.Background(), v, fmt.Sprintf(util.ConfigReleasePolicyTemplate, v))
					if err != nil {
						return err
					}

					app.Log.Infof("configured Vault ACL policy for Helm release %s ", v)
				} else {
					app.Log.Infof("skipped configuration of Vault ACL policy for Helm release %s ", v)
				}
			}

			// configure Vault policies to use for authenticated users
			for k, v := range util.ConfigAclPolicies {
				if !helpers.SliceContains(pol, k) {
					err := app.Vault.PoliciesWriteAclPolicy(context.Background(), k, v)
					if err != nil {
						return err
					}

					app.Log.Infof("configured Vault ACL policy %s ", k)
				} else {
					app.Log.Infof("skipped configuration of Vault ACL policy %s ", k)
				}
			}

			// configure Vault password policies to generate secure secrets later on
			for k, v := range util.ConfigPasswordPolicies {
				if !helpers.SliceContains(ppol, k) {
					err := app.Vault.PoliciesWritePasswordPolicy(context.Background(), k, v)
					if err != nil {
						return err
					}

					app.Log.Infof("configured Vault password policy %s ", k)
				} else {
					app.Log.Infof("skipped configuration of Vault password policy %s ", k)
				}
			}

			return nil
		},
	}

	cmd.PersistentFlags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing configuration")
	cmd.PersistentFlags().StringVarP(&token, "token", "t", "", "The Vault root token")

	return cmd
}
