package cmd

import (
	"context"
	"fmt"

	"github.com/adnoctem/gopskit/internal/waltr/app"
	"github.com/adnoctem/gopskit/internal/waltr/util"
	"github.com/adnoctem/gopskit/pkg/helpers"
	"github.com/spf13/cobra"
)

var _ app.CLIOpt = NewConfigureCommand

func NewConfigureCommand(app *app.State) *cobra.Command {
	var (
		vf        vaultFlags
		overwrite bool
	)

	cmd := &cobra.Command{
		Use:              "configure",
		Short:            "Configure Vault",
		Aliases:          []string{"conf", "config"},
		Long:             "Configure ACL and password policies within Vault",
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			environment, err := environmentOf(cmd)
			if err != nil {
				return err
			}

			release, err := vf.connect(cmd.Context(), app, vf.addr, environment)
			if err != nil {
				return err
			}
			defer release()

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

	vf.add(cmd, true)
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing configuration")

	return cmd
}
