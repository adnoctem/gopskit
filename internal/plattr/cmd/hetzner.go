package cmd

import (
	"errors"
	"strings"

	"github.com/adnoctem/gopskit/internal/plattr/app"
	"github.com/adnoctem/gopskit/pkg/helpers"
	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ app.CLIOpt = NewHetznerEncryptionCommand

func NewHetznerEncryptionCommand(app *app.State) *cobra.Command {
	var (
		passphrase         string
		passphraseCharset  string
		passphraseLength   int
		generatePassphrase bool
		reflectNamespaces  []string
	)

	cmd := &cobra.Command{
		Use:              "hetzner-encryption",
		Short:            "Configure Hetzner Volume Encryption",
		Long:             "Configure Hetzner Volume Encryption via Kubernetes Secrets",
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var annotations = make(map[string]string)
			var secretExists, storageClassExists = true, true // assume true to avoid creations

			namespace := proc.Must(cmd.Flags().GetString("namespace"))
			reflect := proc.Must(cmd.Flags().GetBool("reflect"))

			if generatePassphrase {
				passphrase = helpers.GeneratePassphrase(
					helpers.WithLength(passphraseLength),
					helpers.WithCharSet(passphraseCharset))
			}

			// if still unset, fail
			if passphrase == "" {
				return errors.New("cannot create Hetzner Volume Encryption Secret without passphrase")
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
					Name:        "hetzner-volume-encryption",
					Namespace:   namespace,
					Annotations: annotations,
				},
				Type: "Opaque",
				StringData: map[string]string{
					"encryption-passphrase": passphrase,
				},
			}

			storageClass := &storagev1.StorageClass{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "StorageClass",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "hcloud-encrypted-volumes",
				},
				Provisioner:          "csi.hetzner.cloud",
				ReclaimPolicy:        helpers.Ptr(corev1.PersistentVolumeReclaimDelete),
				VolumeBindingMode:    helpers.Ptr(storagev1.VolumeBindingWaitForFirstConsumer),
				AllowVolumeExpansion: helpers.Ptr(true),
				Parameters: map[string]string{
					"csi.storage.k8s.io/node-publish-secret-name":      secret.Name,
					"csi.storage.k8s.io/node-publish-secret-namespace": secret.Namespace,
				},
			}

			// create secret (or skip)
			_, err := app.Kube.Secret(secret.Namespace, secret.Name, metav1.GetOptions{})
			if err != nil {
				secretExists = false
			}

			if secretExists {
				app.Log.Infof("Skipping creation of Hetzner Volume Encryption Secret in namespace: %s. Secret exists: %s",
					secret.Namespace,
					secret.Name)
				return nil
			}

			// create storage-class (or skip)
			_, err = app.Kube.StorageClass(storageClass.Name, metav1.GetOptions{})
			if err != nil {
				storageClassExists = false
			}

			if storageClassExists {
				app.Log.Infof("Skipping creation of Hetzner Volume Encryption StorageClass: %s. StorageClass exists.",
					storageClass.Name)
				return nil
			}

			if err := app.Kube.CreateStorageClass(storageClass, metav1.CreateOptions{}); err != nil {
				return err
			}
			app.Log.Infof("Successfully created Hetzner Volume Encryption StorageClass: %s", storageClass.Name)
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&passphrase, "passphrase", "", "The encryption passphrase")
	cmd.PersistentFlags().BoolVar(&generatePassphrase, "generate-passphrase", false, "Generate an encryption passphrase")
	cmd.PersistentFlags().StringVar(&passphraseCharset, "passphrase-charset", helpers.PassphraseDefaultCharset, "The charset for the (generated) passphrase")
	cmd.PersistentFlags().IntVar(&passphraseLength, "passphrase-length", helpers.PassphraseDefaultLength, "The length for the (generated) passphrase")
	cmd.PersistentFlags().StringArrayVar(&reflectNamespaces, "reflect-namespaces", []string{"kube-system"}, "Namespaces to enable for reflection")

	return cmd
}
