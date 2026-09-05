package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/adnoctem/gopskit/internal/plattr/app"
	"github.com/adnoctem/gopskit/pkg/helpers"
	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ app.CLIOpt = NewLonghornEncryptionCommand

func NewLonghornEncryptionCommand(app *app.State) *cobra.Command {
	var (
		passphrase          string
		passphraseCharset   string
		passphraseLength    int
		generatePassphrase  bool
		reflectNamespaces   []string
		encryptionReplicas  int
		staleReplicaTimeout int
		fromBackup          string
	)

	cmd := &cobra.Command{
		Use:              "longhorn-encryption",
		Short:            "Configure Longhorn Volume Encryption",
		Long:             "Configure Longhorn Volume Encryption via Kubernetes Secrets",
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
				return errors.New("cannot create Longhorn Volume Encryption secret without passphrase")
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
					Name:        "longhorn-crypto",
					Namespace:   namespace,
					Annotations: annotations,
					Labels: map[string]string{
						"component": "longhorn",
						"use":       "encryption",
					},
				},
				Type: "Opaque",
				StringData: map[string]string{
					"CRYPTO_KEY_VALUE":    passphrase,
					"CRYPTO_KEY_PROVIDER": "secret",
					"CRYPTO_KEY_CIPHER":   "aes-xts-plain64", //gitleaks:allow -- cipher algorithm name, not a secret
					"CRYPTO_KEY_HASH":     "sha256",
					"CRYPTO_KEY_SIZE":     "256",
					"CRYPTO_PBKDF":        "argon2i",
				},
			}

			storageClass := &storagev1.StorageClass{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "StorageClass",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "longhorn-encrypted",
				},
				Provisioner:          "driver.longhorn.io",
				AllowVolumeExpansion: helpers.Ptr(true),
				Parameters: map[string]string{
					"numberOfReplicas":    fmt.Sprintf("%d", encryptionReplicas),
					"staleReplicaTimeout": fmt.Sprintf("%d", staleReplicaTimeout),
					"fromBackup":          fromBackup,
					"encrypted":           "true",
					"csi.storage.k8s.io/provisioner-secret-name":       secret.Name,
					"csi.storage.k8s.io/provisioner-secret-namespace":  secret.Namespace,
					"csi.storage.k8s.io/node-publish-secret-name":      secret.Name,
					"csi.storage.k8s.io/node-publish-secret-namespace": secret.Namespace,
					"csi.storage.k8s.io/node-stage-secret-name":        secret.Name,
					"csi.storage.k8s.io/node-stage-secret-namespace":   secret.Namespace,
				},
			}

			// create secret (or skip) - independent of the storage class below, so a missing
			// secret is always created even if the storage class already exists (and vice versa)
			_, err := app.Kube.Secret(secret.Namespace, secret.Name, metav1.GetOptions{})
			if err != nil {
				secretExists = false
			}

			if secretExists {
				app.Log.Infof("Skipping creation of Longhorn Volume Encryption Secret in namespace: %s. Secret exists: %s",
					secret.Namespace,
					secret.Name)
			} else {
				if err := app.Kube.CreateSecret(secret.Namespace, secret, metav1.CreateOptions{}); err != nil {
					return err
				}
				app.Log.Infof("Successfully created Longhorn Volume Encryption Secret: %s in namespace: %s", secret.Name, secret.Namespace)
			}

			// create storage-class (or skip)
			_, err = app.Kube.StorageClass(storageClass.Name, metav1.GetOptions{})
			if err != nil {
				storageClassExists = false
			}

			if storageClassExists {
				app.Log.Infof("Skipping creation of Longhorn Volume Encryption StorageClass: %s. StorageClass exists.",
					storageClass.Name)
				return nil
			}

			if err := app.Kube.CreateStorageClass(storageClass, metav1.CreateOptions{}); err != nil {
				return err
			}
			app.Log.Infof("Successfully created Longhorn Volume Encryption StorageClass: %s", storageClass.Name)
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&passphrase, "passphrase", "", "The encryption passphrase")
	cmd.PersistentFlags().BoolVar(&generatePassphrase, "generate-passphrase", false, "Generate an encryption passphrase")
	cmd.PersistentFlags().StringVar(&passphraseCharset, "passphrase-charset", helpers.PassphraseDefaultCharset, "The charset for the (generated) passphrase")
	cmd.PersistentFlags().IntVar(&passphraseLength, "passphrase-length", helpers.PassphraseDefaultLength, "The length for the (generated) passphrase")
	cmd.PersistentFlags().StringArrayVar(&reflectNamespaces, "reflect-namespaces", []string{"kube-system", "longhorn-system"}, "Namespaces to enable for reflection")
	cmd.PersistentFlags().IntVar(&encryptionReplicas, "encryption-replicas", 3, "The amount of replicas to create of encrypted drives")
	cmd.PersistentFlags().IntVar(&staleReplicaTimeout, "stale-replica-timeout", 2880, "The timeout before stale replicas are removed")
	cmd.PersistentFlags().StringVar(&fromBackup, "from-backup", "", "An optional backup to create replicas from")

	return cmd
}
