package cmd

import (
	"strings"

	"github.com/adnoctem/gopskit/internal/plattr/app"
	"github.com/adnoctem/gopskit/pkg/helpers"
	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ app.CLIOpt = NewNginxCommand

func NewNginxCommand(app *app.State) *cobra.Command {
	var (
		bits              int
		reflectNamespaces []string
	)

	cmd := &cobra.Command{
		Use:              "ingress-nginx",
		Short:            "Configure Diffie-Hellman parameters for Ingress-Nginx",
		Long:             "Configure Diffie-Hellman parameters for Ingress-Nginx via Kubernetes Secrets",
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var exists = true

			opts := make([]helpers.DiffieHellmanOpt, 0)
			namespace := proc.Must(cmd.Flags().GetString("namespace"))
			// reflect := proc.Must(cmd.Flags().GetBool("reflect"))

			secret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "dhparam",
					Namespace: namespace,
					Annotations: map[string]string{
						"reflector.v1.k8s.emberstack.com/reflection-allowed":            "true",
						"reflector.v1.k8s.emberstack.com/reflection-allowed-namespaces": strings.Join(reflectNamespaces, ","),
						"reflector.v1.k8s.emberstack.com/reflection-auto-enabled":       "true",
						"reflector.v1.k8s.emberstack.com/reflection-auto-namespaces":    strings.Join(reflectNamespaces, ","),
					},
				},
				Type: "Opaque",
			}

			// check for existence before generating since it takes ages
			_, err := app.Kube.Secret(secret.Namespace, secret.Name, metav1.GetOptions{})
			if err != nil {
				exists = false
			}

			if exists {
				app.Log.Infof("Skipping creation of Ingress-Nginx Diffie-Hellman parameter secret in namespace: %s. Secret exists: %s", secret.Namespace, secret.Name)
				return nil
			}

			// force Base64 for Kubernetes secrets
			opts = append(opts, helpers.WithEncoding(helpers.Base64))
			if bits != helpers.DHParamDefaultBits {
				opts = append(opts, helpers.WithBits(bits))
			}

			app.Log.Info("Generating Diffie-Hellman parameters. This may take a long time...")
			params, err := helpers.GenerateDiffieHellmanParams(opts...)
			if err != nil {
				return err
			}

			secret.Data = map[string][]byte{
				"dhparam.pem": []byte(params),
			}

			if err := app.Kube.CreateSecret(secret.Namespace, secret, metav1.CreateOptions{}); err != nil {
				return err
			}
			app.Log.Infof("Successfully created Ingress-Nginx Diffie-Hellman parameter secret: %s in namespace: %s", secret.Name, secret.Namespace)
			return nil
		},
	}

	cmd.PersistentFlags().IntVarP(&bits, "bits", "b", helpers.DHParamDefaultBits, "The amount for bits to generate")
	cmd.PersistentFlags().StringArrayVar(&reflectNamespaces, "reflect-namespaces", []string{
		"kube-system",
		"ingress-nginx"},
		"Namespaces to enable for reflection")

	return cmd
}
