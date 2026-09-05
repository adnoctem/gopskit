package cmd

import (
	"testing"

	"github.com/adnoctem/gopskit/internal/plattr/app"
	"github.com/adnoctem/gopskit/pkg/core"
	"github.com/adnoctem/gopskit/pkg/kube"
	"github.com/adnoctem/gopskit/pkg/log"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newTestPlattrState() *app.State {
	return &app.State{
		API: &core.API{
			Kube: &kube.Client{Client: fake.NewSimpleClientset()},
			Log:  log.New(),
		},
	}
}

func TestLonghornEncryptionCommand(t *testing.T) {
	t.Run("errors without a passphrase", func(t *testing.T) {
		asrt := assert.New(t)

		root := NewRootCommand(newTestPlattrState())
		root.SetArgs([]string{"longhorn-encryption"})
		asrt.Error(root.Execute())
	})

	t.Run("creates both the secret and the storage class on first run", func(t *testing.T) {
		asrt := assert.New(t)

		state := newTestPlattrState()
		root := NewRootCommand(state)
		root.SetArgs([]string{"longhorn-encryption", "--passphrase", "s3cr3t"})
		asrt.NoError(root.Execute())

		secret, err := state.Kube.Secret(app.DefaultNamespace, "longhorn-crypto", metav1.GetOptions{})
		asrt.NoError(err)
		asrt.Equal("s3cr3t", secret.StringData["CRYPTO_KEY_VALUE"])

		sc, err := state.Kube.StorageClass("longhorn-encrypted", metav1.GetOptions{})
		asrt.NoError(err)
		asrt.Equal("longhorn-crypto", sc.Parameters["csi.storage.k8s.io/provisioner-secret-name"])
	})

	t.Run("skips the secret but still creates the storage class if only the secret pre-exists", func(t *testing.T) {
		asrt := assert.New(t)

		state := newTestPlattrState()
		_, err := state.Kube.Client.CoreV1().Secrets(app.DefaultNamespace).Create(t.Context(), &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "longhorn-crypto", Namespace: app.DefaultNamespace},
		}, metav1.CreateOptions{})
		asrt.NoError(err)

		root := NewRootCommand(state)
		root.SetArgs([]string{"longhorn-encryption", "--passphrase", "s3cr3t"})
		asrt.NoError(root.Execute())

		_, err = state.Kube.StorageClass("longhorn-encrypted", metav1.GetOptions{})
		asrt.NoError(err, "the storage class must still be created even though the secret already existed")
	})
}
