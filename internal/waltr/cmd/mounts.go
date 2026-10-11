package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/adnoctem/gopskit/internal/waltr/app"
	cmdutil "github.com/adnoctem/gopskit/internal/waltr/util"
	"github.com/adnoctem/gopskit/pkg/helpers"
	"github.com/hashicorp/vault-client-go/schema"
	"github.com/spf13/cobra"
)

var _ app.CLIOpt = NewMountsCommand // assure type compatibility

func NewMountsCommand(app *app.State) *cobra.Command {
	var (
		vf               vaultFlags
		kubernetesHost   string
		kubernetesCACert string
	)

	cmd := &cobra.Command{
		Use:              "mounts",
		Short:            "Mount Vault's authentication methods and secrets engines",
		Aliases:          []string{"mount"},
		Long:             "Mount Vault's authentication methods and secrets engines",
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

			// get current methods
			m, err := cmdutil.AuthMethods(app)
			if err != nil {
				return err
			}

			// enable Kubernetes
			const kp = "kubernetes/"
			if !helpers.SliceContains(m, kp) {
				// enable
				err = app.Vault.AuthEnableMethod(context.Background(), strings.TrimSuffix(kp, "/"),
					schema.AuthEnableMethodRequest{
						Description: "authenticate with Kubernetes Service Account Tokens",
						Type:        strings.TrimSuffix(kp, "/"),
					})

				if err != nil {
					return fmt.Errorf("could not enable Vault authentication method Kubernetes. Error: %v", err)
				}

				// configure; inside the cluster Vault finds the API server's CA in its own Pod
				req := schema.KubernetesConfigureAuthRequest{KubernetesHost: kubernetesHost}
				if kubernetesCACert != "" {
					pem, err := os.ReadFile(kubernetesCACert)
					if err != nil {
						return fmt.Errorf("--kubernetes-ca-cert: %w", err)
					}
					req.KubernetesCaCert = string(pem)
				}

				err = app.Vault.KubernetesConfigureAuth(context.Background(), strings.TrimSuffix(kp, "/"), req)

				if err != nil {
					return fmt.Errorf("could not configure Vault authentication method Kubernetes: %v", err)
				}

				app.Log.Info("Enabled Vault authentication method Kubernetes")
			} else {
				app.Log.Info("Vault authentication method Kubernetes already enabled")
			}

			// enable OIDC
			const op = "oidc/"
			if !helpers.SliceContains(m, op) {
				err := app.Vault.AuthEnableMethod(context.Background(), strings.TrimSuffix(op, "/"),
					schema.AuthEnableMethodRequest{
						Type:        "oidc",
						Description: "authenticate with OpenID Connect",
					})

				if err != nil {
					return fmt.Errorf("could not enable Vault authentication method OIDC. Error: %v", err)
				}

				app.Log.Info("Enabled Vault authentication method OIDC")
			} else {
				app.Log.Info("Vault authentication method OIDC already enabled")
			}

			// get current secrets engines
			s, err := cmdutil.SecretsEngines(app)
			if err != nil {
				return fmt.Errorf("could not list secrets engines: %v", err)
			}

			// enable KV-V2
			const kvp = "kv/"
			if !helpers.SliceContains(s, kvp) {
				err := app.Vault.MountsEnableSecretsEngine(context.Background(),
					strings.TrimSuffix(kvp, "/"),
					schema.MountsEnableSecretsEngineRequest{
						Type:        "kv-v2",
						Description: "store secret values in key/value storage",
						Options: map[string]interface{}{
							"version": "2",
						},
					})

				if err != nil {
					return fmt.Errorf("could not enable Vault secrets engine KV-V2. Error: %v", err)
				}

				app.Log.Info("Enabled Vault secrets engine KV-V2")
			} else {
				app.Log.Info("Vault secrets engine KV-V2 already enabled")
			}

			app.Log.Info("successfully enabled Vault authentication methods: Kubernetes, OIDC")
			app.Log.Info("successfully enabled Vault secrets engines: KV-V2")
			return nil
		},
	}

	vf.add(cmd, true)
	cmd.Flags().StringVar(&kubernetesHost, "kubernetes-host", "https://kubernetes.default.svc",
		"The Kubernetes API server address Vault's Kubernetes auth method uses, as Vault reaches it")
	cmd.Flags().StringVar(&kubernetesCACert, "kubernetes-ca-cert", "",
		"A PEM file with the Kubernetes API server's CA, for a Vault running outside the cluster (default: Vault's own Pod's)")

	return cmd
}
