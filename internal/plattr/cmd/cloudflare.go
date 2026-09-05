package cmd

import (
	"errors"
	"strings"

	"github.com/adnoctem/gopskit/internal/plattr/app"
	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ app.CLIOpt = NewCloudflareCommand

func NewCloudflareCommand(app *app.State) *cobra.Command {
	var (
		token             string
		reflectNamespaces []string
	)

	cmd := &cobra.Command{
		Use:              "cloudflare",
		Short:            "Configure Cloudflare API credentials",
		Long:             "Configure Cloudflare API credentials via Kubernetes Secrets",
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var annotations = make(map[string]string)
			var exists = true

			namespace := proc.Must(cmd.Flags().GetString("namespace"))
			reflect := proc.Must(cmd.Flags().GetBool("reflect"))

			if token == "" {
				return errors.New("cannot create Cloudflare Secret without credentials")
			}

			if reflect {
				annotations = map[string]string{
					"reflector.v1.k8s.emberstack.com/reflection-allowed":            "true",
					"reflector.v1.k8s.emberstack.com/reflection-allowed-namespaces": strings.Join(reflectNamespaces, ","),
					"reflector.v1.k8s.emberstack.com/reflection-auto-enabled":       "true",
					"reflector.v1.k8s.emberstack.com/reflection-auto-namespaces":    strings.Join(reflectNamespaces, ","),
				}
			}

			secret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:        "cloudflare-api-credentials",
					Namespace:   namespace,
					Annotations: annotations,
				},
				Type: "Opaque",
				Data: map[string][]byte{
					"cloudflare_api_token": []byte(token),
				},
			}

			_, err := app.Kube.Secret(secret.Namespace, secret.Name, metav1.GetOptions{})
			if err != nil {
				exists = false
			}

			if exists {
				app.Log.Infof("Skipping creation of Cloudflare credentials secret in namespace: %s. Secret exists: %s",
					secret.Namespace,
					secret.Name)
				return nil
			}

			if err := app.Kube.CreateSecret(secret.Namespace, secret, metav1.CreateOptions{}); err != nil {
				return err
			}
			app.Log.Infof("Successfully created secret: %s in namespace: %s", secret.Name, secret.Namespace)
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&token, "token", "", "Cloudflare API token")
	cmd.PersistentFlags().StringArrayVar(&reflectNamespaces, "reflect-namespaces", []string{
		"kube-system",
		"cert-manager",
		"external-dns"},
		"Namespaces to enable for reflection")

	return cmd
}
