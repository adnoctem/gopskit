package cmd

import (
	"errors"

	"github.com/adnoctem/gopskit/internal/plattr/app"
	"github.com/adnoctem/gopskit/pkg/config"
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
		overwrite         bool
	)

	cmd := &cobra.Command{
		Use:              "cloudflare",
		Short:            "Configure Cloudflare API credentials",
		Long:             "Configure Cloudflare API credentials via Kubernetes Secrets",
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace := proc.Must(cmd.Flags().GetString("namespace"))
			reflect := proc.Must(cmd.Flags().GetBool("reflect"))

			if token == "" {
				return errors.New("cannot create Cloudflare Secret without credentials")
			}

			secret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:        "cloudflare-api-credentials",
					Namespace:   namespace,
					Annotations: reflectorAnnotations(reflect, reflectNamespaces),
				},
				Type: "Opaque",
				Data: map[string][]byte{
					"cloudflare_api_token": []byte(token),
				},
			}

			return applySecret(cmd.Context(), app, secret, overwrite, "Cloudflare API credentials")
		},
	}

	cmd.PersistentFlags().StringVar(&token, "token", "", "Cloudflare API token")
	cmd.PersistentFlags().BoolVar(&overwrite, "overwrite", false, "Replace an existing Secret, e.g. to rotate the token")
	config.BindEnv(cmd.PersistentFlags(), "token", "CLOUDFLARE_API_TOKEN")
	cmd.PersistentFlags().StringArrayVar(&reflectNamespaces, "reflect-namespaces", []string{
		"kube-system",
		"cert-manager",
		"external-dns"},
		"Namespaces to enable for reflection")

	return cmd
}
