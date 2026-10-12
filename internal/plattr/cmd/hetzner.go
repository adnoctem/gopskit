package cmd

import (
	"errors"

	"github.com/adnoctem/gopskit/internal/plattr/app"
	"github.com/adnoctem/gopskit/pkg/config"
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

			secret := &corev1.Secret{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Secret",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:        "hetzner-volume-encryption",
					Namespace:   namespace,
					Annotations: reflectorAnnotations(reflect, reflectNamespaces),
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

			// the Secret is never replaced: volumes encrypted with the old passphrase could no longer
			// be unlocked with a new one
			if err := applySecret(cmd.Context(), app, secret, false, "Hetzner Volume Encryption"); err != nil {
				return err
			}

			return ensureStorageClass(cmd.Context(), app, storageClass, "Hetzner Volume Encryption")
		},
	}

	cmd.PersistentFlags().StringVar(&passphrase, "passphrase", "", "The encryption passphrase")
	config.BindEnv(cmd.PersistentFlags(), "passphrase", "PLATTR_ENCRYPTION_PASSPHRASE")
	cmd.PersistentFlags().BoolVar(&generatePassphrase, "generate-passphrase", false, "Generate an encryption passphrase")
	cmd.PersistentFlags().StringVar(&passphraseCharset, "passphrase-charset", helpers.PassphraseDefaultCharset, "The charset for the (generated) passphrase")
	cmd.PersistentFlags().IntVar(&passphraseLength, "passphrase-length", helpers.PassphraseDefaultLength, "The length for the (generated) passphrase")
	cmd.PersistentFlags().StringArrayVar(&reflectNamespaces, "reflect-namespaces", []string{"kube-system"}, "Namespaces to enable for reflection")

	return cmd
}
