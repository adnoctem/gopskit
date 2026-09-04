package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/fmjstudios/gopskit/internal/waltr/app"
	cmdutil "github.com/fmjstudios/gopskit/internal/waltr/util"
	apivault "github.com/fmjstudios/gopskit/pkg/api/vault"
	"github.com/fmjstudios/gopskit/pkg/core"
	"github.com/fmjstudios/gopskit/pkg/helpers"
	"github.com/fmjstudios/gopskit/pkg/proc"
	"github.com/hashicorp/vault-client-go/schema"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ app.CLIOpt = NewMountsCommand // assure type compatibility

func NewMountsCommand(app *app.State) *cobra.Command {
	var (
		token string
	)

	cmd := &cobra.Command{
		Use:              "mounts",
		Short:            "Mount Vault's authentication methods and secrets engines",
		Aliases:          []string{"mount"},
		Long:             "Mount Vault's authentication methods and secrets engines",
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			envF := proc.Must(cmd.Flags().GetString("environment"))
			label := proc.Must(cmd.Flags().GetString("label"))
			namespace := proc.Must(cmd.Flags().GetString("namespace"))
			environment := proc.Must(core.EnvFromString(envF))

			var vaultNamespace string
			var vaultLeaderPod *corev1.Pod

			// look across the entire cluster
			pods, err := cmdutil.Pods(app, "", label)
			if err != nil {
				return fmt.Errorf("could not retrieve Vault pods for label: %s. Error: %v", label, err)
			}

			// ensure we're using a single namespace, or it's passed in
			vaultNamespace, err = cmdutil.EnsureNamespace(pods)
			if err != nil && namespace == "" {
				return fmt.Errorf("found multiple possible Vault pods. the namespace option is unset and %v", err)
			}

			// wait until the pod is running
			vaultLeaderPod, err = cmdutil.LeaderPod(app, pods, vaultNamespace, label)
			if err != nil {
				return err
			}
			if err := cmdutil.WaitUntilRunning(app, *vaultLeaderPod); err != nil {
				return err
			}

			if token == "" {
				app.Log.Debug("'token' option is unset, falling back to credentials in cache path!")
				creds, err := cmdutil.ReadCredentials(app, environment)
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

			// get current methods
			m, err := cmdutil.AuthMethods(app)
			if err != nil {
				return err
			}

			// enable Kubernetes
			const kp = "kubernetes/"
			if !helpers.SliceContains(m, kp) {
				// get API server info
				svc, err := app.Kube.Service("default", "kubernetes", v1.GetOptions{
					TypeMeta: v1.TypeMeta{
						APIVersion: "v1",
						Kind:       "Service",
					},
				})

				if err != nil {
					return fmt.Errorf("could not find Kubernetes API server service")
				}

				// enable
				err = app.Vault.AuthEnableMethod(context.Background(), strings.TrimSuffix(kp, "/"),
					schema.AuthEnableMethodRequest{
						Description: "authenticate with Kubernetes Service Account Tokens",
						Type:        strings.TrimSuffix(kp, "/"),
					})

				if err != nil {
					return fmt.Errorf("could not enable Vault authentication method Kubernetes. Error: %v", err)
				}

				// configure
				err = app.Vault.KubernetesConfigureAuth(context.Background(), strings.TrimSuffix(kp, "/"),
					schema.KubernetesConfigureAuthRequest{
						KubernetesHost: fmt.Sprintf("https://%s:%d", svc.Spec.ClusterIP, svc.Spec.Ports[0].Port),
					})

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

	cmd.PersistentFlags().StringVarP(&token, "token", "t", "", "The Vault root token")

	return cmd
}
