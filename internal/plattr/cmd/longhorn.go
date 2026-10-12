package cmd

import (
	"errors"
	"fmt"

	"github.com/adnoctem/gopskit/internal/plattr/app"
	"github.com/adnoctem/gopskit/pkg/config"
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

			namespace := proc.Must(cmd.Flags().GetString("namespace"))
			reflect := proc.Must(cmd.Flags().GetBool("reflect"))
			annotations, err := reflectorAnnotations(reflect, reflectNamespaces)
			if err != nil {
				return err
			}

			if generatePassphrase && passphrase != "" {
				return errors.New("--generate-passphrase and a given passphrase exclude each other")
			}

			if generatePassphrase {
				passphrase = helpers.GeneratePassphrase(
					helpers.WithLength(passphraseLength),
					helpers.WithCharSet(passphraseCharset))
			}

			// an existing Secret is never replaced, so a passphrase is only needed to create one
			if passphrase == "" {
				exists, err := secretExists(cmd.Context(), app, namespace, "longhorn-crypto")
				if err != nil {
					return err
				}
				if !exists {
					return errors.New("cannot create the Longhorn Volume Encryption Secret without a passphrase: " +
						"give --passphrase, $PLATTR_ENCRYPTION_PASSPHRASE or --generate-passphrase")
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
					APIVersion: "storage.k8s.io/v1",
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

			// the Secret is never replaced: volumes encrypted with the old passphrase could no longer
			// be unlocked with a new one
			if err := applySecret(cmd.Context(), app, secret, false, "Longhorn Volume Encryption"); err != nil {
				return err
			}

			return ensureStorageClass(cmd.Context(), app, storageClass, "Longhorn Volume Encryption")
		},
	}

	cmd.PersistentFlags().StringVar(&passphrase, "passphrase", "", "The encryption passphrase")
	config.BindEnv(cmd.PersistentFlags(), "passphrase", "PLATTR_ENCRYPTION_PASSPHRASE")
	cmd.PersistentFlags().BoolVar(&generatePassphrase, "generate-passphrase", false, "Generate an encryption passphrase")
	cmd.PersistentFlags().StringVar(&passphraseCharset, "passphrase-charset", helpers.PassphraseDefaultCharset, "The charset for the (generated) passphrase")
	cmd.PersistentFlags().IntVar(&passphraseLength, "passphrase-length", helpers.PassphraseDefaultLength, "The length for the (generated) passphrase")
	cmd.PersistentFlags().StringArrayVar(&reflectNamespaces, "reflect-namespaces", nil, "Namespaces to mirror the Secret into with --reflect")
	cmd.PersistentFlags().IntVar(&encryptionReplicas, "encryption-replicas", 3, "The amount of replicas to create of encrypted drives")
	cmd.PersistentFlags().IntVar(&staleReplicaTimeout, "stale-replica-timeout", 2880, "The timeout before stale replicas are removed")
	cmd.PersistentFlags().StringVar(&fromBackup, "from-backup", "", "An optional backup to create replicas from")

	return cmd
}
