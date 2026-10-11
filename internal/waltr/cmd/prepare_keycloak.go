package cmd

import (
	"context"

	"github.com/adnoctem/gopskit/internal/waltr/app"
	"github.com/adnoctem/gopskit/internal/waltr/util"
	"github.com/adnoctem/gopskit/pkg/helpers"
	"github.com/hashicorp/vault-client-go/schema"
	"github.com/spf13/cobra"
)

var _ app.CLIOpt = NewPrepareKeycloakCommand // assure type compatibility

func NewPrepareKeycloakCommand(app *app.State) *cobra.Command {
	var (
		vf        vaultFlags
		overwrite bool
	)

	cmd := &cobra.Command{
		Use:              "keycloak",
		Short:            "Prepare Vault for Keycloak",
		Long:             "Prepare Vault with policies and roles for Keycloak",
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

			// create Kubernetes role
			const r = "keycloak"
			rola, err := util.KubernetesAuthRoles(app)
			if err != nil {
				return err
			}

			if !helpers.SliceContains(rola, r) || overwrite {
				err := app.Vault.KubernetesWriteAuthRole(context.Background(), r,
					schema.KubernetesWriteAuthRoleRequest{
						Audience:                      "vault",
						BoundServiceAccountNames:      []string{r},
						BoundServiceAccountNamespaces: []string{r},
						TokenPeriod:                   "24h",
						TokenPolicies: []string{
							r,
						},
						TokenTtl: "0",
					})

				if err != nil {
					return err
				}

				app.Log.Infof("configured Vault Kubernetes Auth Role %s for Keycloak", r)
			} else {
				app.Log.Infof("skipped configuration of Vault Kubernetes Auth Role %s for Keycloak", r)
			}

			// create Admin credentials
			const adminPath = "keycloak/config"
			adminPass, err := util.GeneratePasswordFromPolicy(app, "alphanumeric-password")
			if err != nil {
				return err
			}

			adminExists := util.HasKvV2Secret(app, adminPath, "kv/")
			if !adminExists || overwrite {
				err := util.WriteKvV2Secret(app, adminPath, "kv/", map[string]interface{}{
					"username": "mg",
					"password": adminPass,
				})

				if err != nil {
					return err
				}

				app.Log.Infof("configured Keycloak Admin credentials at path: %s", adminPath)
			} else {
				app.Log.Infof("skipped configuration of Keycloak Admin credentials at path: %s", adminPath)
			}

			// create PostgreSQL credentials (skip if exists)
			const psqlPath = "keycloak/credentials/postgresql"
			psqlPass, err := util.GeneratePasswordFromPolicy(app, "alphanumeric-password")
			if err != nil {
				return err
			}

			psqlExists := util.HasKvV2Secret(app, psqlPath, "kv/")
			if !psqlExists || overwrite {
				err := util.WriteKvV2Secret(app, psqlPath, "kv/", map[string]interface{}{
					"username": "keycloak",
					"password": psqlPass,
				})

				if err != nil {
					return err
				}

				app.Log.Infof("configured Keycloak PostgreSQL credentials at path: %s", psqlPath)
			} else {
				app.Log.Infof("skipped configuration of Keycloak PostgreSQL credentials at path: %s", psqlPath)
			}

			return nil
		},
	}

	vf.add(cmd, true)
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing configuration")
	return cmd
}
