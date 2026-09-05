package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/fmjstudios/gopskit/internal/waltr/app"
	"github.com/fmjstudios/gopskit/internal/waltr/util"
	apivault "github.com/fmjstudios/gopskit/pkg/api/vault"
	"github.com/fmjstudios/gopskit/pkg/core"
	"github.com/fmjstudios/gopskit/pkg/helpers"
	"github.com/fmjstudios/gopskit/pkg/proc"
	"github.com/hashicorp/vault-client-go/schema"
	"github.com/spf13/cobra"
)

var _ app.CLIOpt = NewTransitCommand // assure type compatibility

func NewTransitCommand(app *app.State) *cobra.Command {
	var (
		token     string
		overwrite bool
	)

	cmd := &cobra.Command{
		Use:              "transit",
		Short:            "Configure Vault Transit-Encryption",
		Aliases:          []string{"encryption", "transit-encryption"},
		Long:             "Configure Vault for Transit-Encryption with the Vault-Secrets-Operator",
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

			s, err := util.SecretsEngines(app)
			if err != nil {
				return err
			}

			// enable transit-encryption
			const tep = "transit/"
			if !helpers.SliceContains(s, tep) || overwrite {
				err := app.Vault.MountsEnableSecretsEngine(context.Background(),
					strings.TrimSuffix(tep, "/"),
					schema.MountsEnableSecretsEngineRequest{
						Type:        "transit",
						Description: "encrypt secrets in transit",
					})

				if err != nil {
					return fmt.Errorf("could not enable Vault transit secrets engine. Error: %v", err)
				}

				app.Log.Info("Enabled Vault transit secrets engine")
			} else {
				app.Log.Info("Vault secrets engine transit already enabled")
			}

			// create transit secret key (if required, or if overwriting)
			const tkp = "transit/keys/vso-client-cache"
			_, err = app.Vault.Read(context.Background(), tkp)
			exists := err == nil
			if err != nil && !apivault.IsNotFound(err) {
				return err
			}

			if !exists || overwrite {
				if err := app.Vault.Write(context.Background(), tkp, map[string]interface{}{}); err != nil {
					return err
				}

				app.Log.Infof("created Vault transit encryption key: %s", tkp)
			} else {
				app.Log.Infof("skipped creation of Vault transit encryption key: %s", tkp)
			}

			// create ACL policy
			pol, err := util.Policies(app)
			if err != nil {
				return err
			}

			const p = "vso-auth"
			if !helpers.SliceContains(pol, p) || overwrite {
				err := app.Vault.PoliciesWriteAclPolicy(context.Background(), p, fmt.Sprintf(util.ConfigReleasePolicyTemplate, p))
				if err != nil {
					return err
				}

				app.Log.Infof("configured Vault ACL policy for Transit-Encryption: %s", p)
			} else {
				app.Log.Infof("skipped configuration of Vault ACL policy for Transit-Encryption: %s", p)
			}

			// create Kubernetes role
			rola, err := util.KubernetesAuthRoles(app)
			if err != nil {
				return err
			}

			if !helpers.SliceContains(rola, p) || overwrite {
				err := app.Vault.KubernetesWriteAuthRole(context.Background(), p, schema.KubernetesWriteAuthRoleRequest{
					Audience:                      "vault",
					BoundServiceAccountNames:      []string{"vault-secrets-operator"},
					BoundServiceAccountNamespaces: []string{"vault-secrets-operator"},
					TokenPeriod:                   "120",
					TokenPolicies: []string{
						"keycloak",
						"awx",
						"crowdsec",
						"gitlab",
						"gitlab-runner",
						"harbor",
						"headlamp",
						"homepage",
						"jenkins",
						"kubescape",
						"loki",
						"matomo",
						p,
					},
					TokenTtl: "0",
				})

				if err != nil {
					return err
				}

				app.Log.Infof("configured Vault Kubernetes Auth Role %s for Transit-Encryption", p)
			} else {
				app.Log.Infof("skipped configuration of Vault Kubernetes Auth Role %s for Transit-Encryption", p)
			}

			return nil
		},
	}

	cmd.PersistentFlags().BoolVar(&overwrite, "overwrite", false, "Overwrite existing configuration")
	cmd.PersistentFlags().StringVarP(&token, "token", "t", "", "The Vault root token")

	return cmd
}
