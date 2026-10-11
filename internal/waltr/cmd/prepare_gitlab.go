package cmd

import (
	"context"

	"github.com/adnoctem/gopskit/internal/waltr/app"
	"github.com/adnoctem/gopskit/internal/waltr/util"
	"github.com/adnoctem/gopskit/pkg/helpers"
	"github.com/hashicorp/vault-client-go/schema"
	"github.com/spf13/cobra"
)

var _ app.CLIOpt = NewPrepareGitLabCommand // assure type compatibility

func NewPrepareGitLabCommand(app *app.State) *cobra.Command {
	var (
		vf        vaultFlags
		overwrite bool
	)

	cmd := &cobra.Command{
		Use:              "gitlab",
		Short:            "Prepare Vault for GitLab",
		Long:             "Prepare Vault with policies and roles for GitLab",
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
			const r = "gitlab"
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

				app.Log.Infof("configured Vault Kubernetes Auth Role %s for GitLab", r)
			} else {
				app.Log.Infof("skipped configuration of Vault Kubernetes Auth Role %s for GitLab", r)
			}

			// create Admin credentials
			const adminPath = "gitlab/config"
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

				app.Log.Infof("configured GitLab Admin credentials at path: %s", adminPath)
			} else {
				app.Log.Infof("skipped configuration of GitLab Admin credentials at path: %s", adminPath)
			}

			// create Redis credentials (skip if exists)
			const redisPath = "gitlab/credentials/redis"
			redisPass, err := util.GeneratePasswordFromPolicy(app, "alphanumeric-password")
			if err != nil {
				return err
			}

			redisExists := util.HasKvV2Secret(app, redisPath, "kv/")
			if !redisExists || overwrite {
				err := util.WriteKvV2Secret(app, redisPath, "kv/", map[string]interface{}{
					"username": "redis",
					"password": redisPass,
				})

				if err != nil {
					return err
				}

				app.Log.Infof("configured GitLab Redis credentials at path: %s", redisPath)
			} else {
				app.Log.Infof("skipped configuration of GitLab Redis credentials at path: %s", redisPath)
			}

			// create PostgreSQL credentials (skip if exists)
			const psqlPath = "gitlab/credentials/postgresql"
			psqlPass, err := util.GeneratePasswordFromPolicy(app, "alphanumeric-password")
			if err != nil {
				return err
			}

			psqlExists := util.HasKvV2Secret(app, psqlPath, "kv/")
			if !psqlExists || overwrite {
				err := util.WriteKvV2Secret(app, psqlPath, "kv/", map[string]interface{}{
					"username": "gitlab",
					"password": psqlPass,
				})

				if err != nil {
					return err
				}

				app.Log.Infof("configured GitLab PostgreSQL credentials at path: %s", psqlPath)
			} else {
				app.Log.Infof("skipped configuration of GitLab PostgreSQL credentials at path: %s", psqlPath)
			}

			// create MinIO credentials (skip if exists)
			const minioPath = "gitlab/credentials/minio"
			minioAccessKey, err := util.GeneratePasswordFromPolicy(app, "s3-access-key")
			if err != nil {
				return err
			}

			minioSecretKey, err := util.GeneratePasswordFromPolicy(app, "s3-secret-key")
			if err != nil {
				return err
			}

			minioExists := util.HasKvV2Secret(app, minioPath, "kv/")
			if !minioExists || overwrite {
				err := util.WriteKvV2Secret(app, minioPath, "kv/", map[string]interface{}{
					"access_key": minioAccessKey,
					"secret_key": minioSecretKey,
				})

				if err != nil {
					return err
				}

				app.Log.Infof("configured GitLab MinIO credentials at path: %s", minioPath)
			} else {
				app.Log.Infof("skipped configuration of GitLab MinIO credentials at path: %s", minioPath)
			}

			return nil
		},
	}

	vf.add(cmd, true)
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing configuration")
	return cmd
}
