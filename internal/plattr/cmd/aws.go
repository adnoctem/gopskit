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

var _ app.CLIOpt = NewAWSCommand

func NewAWSCommand(app *app.State) *cobra.Command {
	var (
		accessKeyId       string
		secretAccessKey   string
		reflectNamespaces []string
		overwrite         bool
	)

	cmd := &cobra.Command{
		Use:              "aws",
		Short:            "Configure AWS credentials",
		Long:             "Configure AWS credentials via Kubernetes Secrets",
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			namespace := proc.Must(cmd.Flags().GetString("namespace"))
			reflect := proc.Must(cmd.Flags().GetBool("reflect"))
			annotations, err := reflectorAnnotations(reflect, reflectNamespaces)
			if err != nil {
				return err
			}

			if accessKeyId == "" || secretAccessKey == "" {
				return errors.New("cannot create AWS Secret without credentials")
			}

			secret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:        "aws-credentials",
					Namespace:   namespace,
					Annotations: annotations,
				},
				Type: "Opaque",
				Data: map[string][]byte{
					"access_key_id":     []byte(accessKeyId),
					"secret_access_key": []byte(secretAccessKey),
				},
			}

			return applySecret(cmd.Context(), app, secret, overwrite, "AWS credentials")
		},
	}

	cmd.PersistentFlags().StringVar(&accessKeyId, "access-key-id", "", "AWS Access Key ID")
	cmd.PersistentFlags().StringVar(&secretAccessKey, "secret-access-key", "", "AWS Secret Access Key")
	cmd.PersistentFlags().BoolVar(&overwrite, "overwrite", false, "Replace an existing Secret, e.g. to rotate the credentials")
	config.BindEnv(cmd.PersistentFlags(), "access-key-id", "AWS_ACCESS_KEY_ID")
	config.BindEnv(cmd.PersistentFlags(), "secret-access-key", "AWS_SECRET_ACCESS_KEY")
	cmd.PersistentFlags().StringArrayVar(&reflectNamespaces, "reflect-namespaces", nil, "Namespaces to mirror the Secret into with --reflect")

	return cmd
}
