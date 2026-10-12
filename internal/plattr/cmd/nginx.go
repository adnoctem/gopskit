package cmd

import (
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
		overwrite         bool
	)

	cmd := &cobra.Command{
		Use:              "ingress-nginx",
		Short:            "Configure Diffie-Hellman parameters for Ingress-Nginx",
		Long:             "Configure Diffie-Hellman parameters for Ingress-Nginx via Kubernetes Secrets",
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := make([]helpers.DiffieHellmanOpt, 0)
			namespace := proc.Must(cmd.Flags().GetString("namespace"))
			reflect := proc.Must(cmd.Flags().GetBool("reflect"))

			secret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:        "dhparam",
					Namespace:   namespace,
					Annotations: reflectorAnnotations(reflect, reflectNamespaces),
				},
				Type: "Opaque",
			}

			// check for existence before generating, since that takes ages
			exists, err := secretExists(cmd.Context(), app, secret.Namespace, secret.Name)
			if err != nil {
				return err
			}
			if exists && !overwrite {
				app.Log.Infof("the Ingress-Nginx Diffie-Hellman parameter Secret %s/%s already exists; --overwrite replaces it",
					secret.Namespace, secret.Name)
				return nil
			}

			// PEM, as ingress-nginx expects - Secret data is base64-encoded by the API itself
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

			return applySecret(cmd.Context(), app, secret, overwrite, "Ingress-Nginx Diffie-Hellman parameter")
		},
	}

	cmd.PersistentFlags().IntVarP(&bits, "bits", "b", helpers.DHParamDefaultBits, "The amount for bits to generate")
	cmd.PersistentFlags().BoolVar(&overwrite, "overwrite", false, "Replace an existing Secret with newly generated parameters")
	cmd.PersistentFlags().StringArrayVar(&reflectNamespaces, "reflect-namespaces", []string{
		"kube-system",
		"ingress-nginx"},
		"Namespaces to enable for reflection")

	return cmd
}
