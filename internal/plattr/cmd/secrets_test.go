package cmd

import (
	"context"
	"encoding/pem"
	"errors"
	"testing"

	"github.com/adnoctem/gopskit/internal/plattr/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// run executes plattr with args, isolated from any real configuration file.
func run(t *testing.T, state *app.State, args ...string) error {
	t.Helper()

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	root := NewRootCommand(state)
	root.SetArgs(args)
	return root.Execute()
}

func getSecret(t *testing.T, state *app.State, name string) *corev1.Secret {
	t.Helper()

	s, err := state.Kube.Client.CoreV1().Secrets(app.DefaultNamespace).Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err)

	return s
}

func TestAWSCredentials(t *testing.T) {
	asrt := assert.New(t)
	state := newTestPlattrState()

	require.NoError(t, run(t, state, "aws", "--access-key-id", "AKIA1", "--secret-access-key", "first"))
	asrt.Equal("first", string(getSecret(t, state, "aws-credentials").Data["secret_access_key"]))

	// without --overwrite an existing Secret stays as it is
	require.NoError(t, run(t, state, "aws", "--access-key-id", "AKIA1", "--secret-access-key", "second"))
	asrt.Equal("first", string(getSecret(t, state, "aws-credentials").Data["secret_access_key"]))

	// rotating credentials replaces it
	require.NoError(t, run(t, state, "aws", "--access-key-id", "AKIA2", "--secret-access-key", "rotated", "--overwrite"))
	secret := getSecret(t, state, "aws-credentials")
	asrt.Equal("AKIA2", string(secret.Data["access_key_id"]))
	asrt.Equal("rotated", string(secret.Data["secret_access_key"]))
	asrt.Empty(secret.Annotations, "without --reflect there are no Reflector annotations")
}

func TestCredentialsFromEnvironment(t *testing.T) {
	state := newTestPlattrState()
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIA-ENV")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "from-env")
	t.Setenv("CLOUDFLARE_API_TOKEN", "cf-env")

	require.NoError(t, run(t, state, "aws"))
	assert.Equal(t, "from-env", string(getSecret(t, state, "aws-credentials").Data["secret_access_key"]))

	require.NoError(t, run(t, state, "cloudflare"))
	assert.Equal(t, "cf-env", string(getSecret(t, state, "cloudflare-api-credentials").Data["cloudflare_api_token"]))
}

func TestReflection(t *testing.T) {
	state := newTestPlattrState()

	require.NoError(t, run(t, state, "cloudflare", "--token", "t", "--reflect", "--reflect-namespaces", "cert-manager"))

	annotations := getSecret(t, state, "cloudflare-api-credentials").Annotations
	assert.Equal(t, "true", annotations["reflector.v1.k8s.emberstack.com/reflection-auto-enabled"])
	assert.Equal(t, "cert-manager", annotations["reflector.v1.k8s.emberstack.com/reflection-auto-namespaces"])
}

// A lookup that fails for any reason other than the Secret not existing must be reported, not
// taken to mean the Secret is missing.
func TestLookupErrorsAreReported(t *testing.T) {
	cs := fake.NewSimpleClientset()
	cs.PrependReactor("get", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("forbidden: no permission to read secrets")
	})

	state := newTestPlattrState()
	state.Kube.Client = cs

	err := run(t, state, "aws", "--access-key-id", "a", "--secret-access-key", "b")
	assert.ErrorContains(t, err, "forbidden")
}

func TestEncryptionSecretsAreNeverReplaced(t *testing.T) {
	state := newTestPlattrState()

	require.NoError(t, run(t, state, "hetzner-encryption", "--passphrase", "original"))
	require.NoError(t, run(t, state, "hetzner-encryption", "--passphrase", "different"))

	assert.Equal(t, "original", getSecret(t, state, "hetzner-volume-encryption").StringData["encryption-passphrase"],
		"replacing the passphrase would lock every volume encrypted with the old one")
}

// ingress-nginx reads dhparam.pem as PEM; the API base64-encodes Secret data itself, so storing
// base64 text left the controller with parameters it couldn't parse.
func TestDHParamsAreStoredAsPEM(t *testing.T) {
	if testing.Short() {
		t.Skip("generating Diffie-Hellman parameters is slow")
	}

	state := newTestPlattrState()
	require.NoError(t, run(t, state, "ingress-nginx", "--bits", "512"))

	raw := getSecret(t, state, "dhparam").Data["dhparam.pem"]
	block, _ := pem.Decode(raw)
	require.NotNil(t, block, "dhparam.pem must hold PEM, got %q", raw)
	assert.Equal(t, "DH PARAMETERS", block.Type)
}
