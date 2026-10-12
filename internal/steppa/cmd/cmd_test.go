package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/adnoctem/gopskit/internal/steppa/app"
	apivault "github.com/adnoctem/gopskit/pkg/api/vault"
	"github.com/adnoctem/gopskit/pkg/api/vault/vaulttest"
	"github.com/adnoctem/gopskit/pkg/kube"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// newState is steppa's state in a temporary working directory, isolated from any real
// configuration file, with a fake cluster.
func newState(t *testing.T) (*app.State, *fake.Clientset, string) {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(dir)

	s, err := app.New()
	require.NoError(t, err)

	cs := fake.NewClientset()
	s.Kube = &kube.Client{Client: cs}

	return s, cs, dir
}

func run(t *testing.T, s *app.State, args ...string) (string, error) {
	t.Helper()

	var stdout bytes.Buffer
	root := NewRootCommand(s)
	root.SetArgs(args)
	root.SetOut(&stdout)

	err := root.ExecuteContext(t.Context())
	return stdout.String(), err
}

// caArgs is `steppa init` for a minimal CA, followed by extra.
func caArgs(extra ...string) []string {
	return append([]string{"init", "--name", "Test", "--dns", "ca.example.org"}, extra...)
}

func TestRootKeyMustBeKeptSomewhere(t *testing.T) {
	s, _, _ := newState(t)

	_, err := run(t, s, caArgs()...)
	assert.ErrorContains(t, err, "root CA key would not be kept anywhere")
}

func TestValuesForInjectMode(t *testing.T) {
	asrt := assert.New(t)
	s, _, dir := newState(t)

	_, err := run(t, s, caArgs("--root-backup", "root.yaml", "--namespace", "pki")...)
	require.NoError(t, err)

	raw, err := os.ReadFile(filepath.Join(dir, "step-certificates-values.yaml"))
	require.NoError(t, err)

	var values map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &values))

	inject := values["inject"].(map[string]any)
	asrt.Equal(true, inject["enabled"])

	caJSON := inject["config"].(map[string]any)["files"].(map[string]any)["ca.json"].(map[string]any)
	asrt.Equal([]any{
		"ca.example.org",
		"step-certificates.pki.svc.cluster.local",
		"step-certificates.pki.svc",
	}, caJSON["dnsNames"], "--namespace adds the chart Service's in-cluster names")

	asrt.Empty(inject["secrets"].(map[string]any)["x509"].(map[string]any)["root_ca_key"],
		"the root key stays out of the cluster by default")

	backup, err := os.Stat(filepath.Join(dir, "root.yaml"))
	require.NoError(t, err)
	asrt.Equal(os.FileMode(0o600), backup.Mode().Perm())
}

func TestExistingCAIsNotReplaced(t *testing.T) {
	s, _, _ := newState(t)

	args := caArgs("--root-backup", "root.yaml")
	_, err := run(t, s, args...)
	require.NoError(t, err)

	_, err = run(t, s, args...)
	assert.ErrorContains(t, err, "already exists")

	_, err = run(t, s, append(args, "--force")...)
	assert.NoError(t, err)
}

func TestKubernetesSecrets(t *testing.T) {
	asrt := assert.New(t)
	s, cs, _ := newState(t)

	args := caArgs("--ssh", "--include-root-key", "-o", "kubernetes,values", "--values-file", "-", "--namespace", "pki")
	stdout, err := run(t, s, args...)
	require.NoError(t, err)

	secrets, err := cs.CoreV1().Secrets("pki").List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	asrt.Len(secrets.Items, 7, "keys, certs, config and four passwords")

	keys, err := cs.CoreV1().Secrets("pki").Get(context.Background(), "step-certificates-secrets", metav1.GetOptions{})
	require.NoError(t, err)
	asrt.Contains(keys.StringData, "root_ca_key", "--include-root-key puts the root key into the cluster")

	// alongside the Secrets, the values only switch the chart to them
	var values map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(stdout), &values))
	asrt.Equal(true, values["existingSecrets"].(map[string]any)["enabled"])
	asrt.Equal(false, values["inject"].(map[string]any)["enabled"])

	_, err = run(t, s, args...)
	asrt.ErrorContains(err, "already exists", "a second run must not replace the CA")
}

func TestVaultBundle(t *testing.T) {
	asrt := assert.New(t)
	addr := vaulttest.StartDev(t)
	s, _, dir := newState(t)

	// every setting from steppa.yaml, as an automated bootstrap would provide them
	cfg := filepath.Join(dir, "steppa.yaml")
	require.NoError(t, os.WriteFile(cfg, []byte(`
initialize:
  name: Test
  dns: [ca.example.org]
  output: [vault]
  vault-addr: `+addr+`
  vault-token: `+vaulttest.RootToken+`
  vault-mount: `+vaulttest.KVMount+`
  via: direct
`), 0o600))

	_, err := run(t, s, "init", "--config", cfg)
	require.NoError(t, err)

	vc := apivault.New(addr)
	require.NoError(t, vc.SetToken(&apivault.Credentials{Token: vaulttest.RootToken}))
	bundle, err := vc.KvV2Read(t.Context(), vaulttest.KVMount, "step-certificates/ca")
	require.NoError(t, err)

	for _, key := range []string{"root_ca.crt", "root_ca_key", "root_password", "intermediate_ca_key", "ca_password", "provisioner_password", "ca.json"} {
		asrt.NotEmpty(bundle[key], key)
	}

	_, err = run(t, s, "init", "--config", cfg)
	asrt.ErrorContains(err, "already exists")
}
