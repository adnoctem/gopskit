package cmd

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/adnoctem/gopskit/internal/waltr/app"
	"github.com/adnoctem/gopskit/internal/waltr/util"
	apivault "github.com/adnoctem/gopskit/pkg/api/vault"
	"github.com/adnoctem/gopskit/pkg/api/vault/vaulttest"
	"github.com/adnoctem/gopskit/pkg/core"
	"github.com/adnoctem/gopskit/pkg/kube"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// newState is waltr's state with its credential cache in a temporary directory and a fake
// cluster holding pods.
func newState(t *testing.T, pods ...corev1.Pod) *app.State {
	t.Helper()

	// keep the tests away from a real configuration file and credential cache
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	s, err := app.New()
	require.NoError(t, err)
	s.Paths.Cache = t.TempDir()

	cs := fake.NewClientset()
	for i := range pods {
		_, err := cs.CoreV1().Pods(pods[i].Namespace).Create(context.Background(), &pods[i], metav1.CreateOptions{})
		require.NoError(t, err)
	}
	s.Kube = &kube.Client{Client: cs}

	return s
}

func run(t *testing.T, s *app.State, args ...string) error {
	t.Helper()

	root := NewRootCommand(s)
	root.SetArgs(args)
	root.SetOut(&bytes.Buffer{})

	return root.ExecuteContext(t.Context())
}

func vaultPod(name string) corev1.Pod {
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "vault", Labels: map[string]string{"app.kubernetes.io/name": "vault"}},
		Spec:       corev1.PodSpec{Hostname: name, Subdomain: "vault-internal"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func rootClient(t *testing.T, addr, token string) *apivault.Client {
	t.Helper()

	vc := apivault.New(addr)
	require.NoError(t, vc.SetToken(&apivault.Credentials{Token: token}))

	return vc
}

func TestPodAddress(t *testing.T) {
	asrt := assert.New(t)

	got, err := podAddress(vaultPod("vault-1"), "https://vault.vault.svc:8200")
	require.NoError(t, err)
	asrt.Equal("https://vault-1.vault-internal.vault.svc:8200", got)

	got, err = podAddress(vaultPod("vault-0"), "http://vault.vault.svc")
	require.NoError(t, err)
	asrt.Equal("http://vault-0.vault-internal.vault.svc", got, "a default port stays implicit")

	bare := vaultPod("vault-0")
	bare.Spec.Subdomain = ""
	_, err = podAddress(bare, "https://vault.vault.svc:8200")
	asrt.ErrorContains(err, "headless Service")

	_, err = podAddress(vaultPod("vault-0"), "vault.vault.svc:8200")
	asrt.Error(err)
}

// TestInitializeAndUnseal runs waltr's initialisation against a real, freshly started Vault,
// with each "Pod" reached at that server.
func TestInitializeAndUnseal(t *testing.T) {
	asrt := assert.New(t)
	addr := vaulttest.StartServer(t)
	ctx := t.Context()

	pods := []corev1.Pod{vaultPod("vault-0"), vaultPod("vault-1")}
	s := newState(t, pods...)

	vf := vaultFlags{}
	vf.conn.Via = "direct"
	reach := func(ctx context.Context, _ corev1.Pod) (func(), error) { return vf.dial(ctx, s, addr) }
	opts := initOptions{threshold: 3, shares: 5}

	require.NoError(t, initializeAndUnseal(ctx, s, core.Development, opts, pods[0], pods, true, reach))

	status, err := rootClient(t, addr, "").SealStatus(ctx)
	require.NoError(t, err)
	asrt.True(status.Initialized)
	asrt.False(status.Sealed)

	creds, err := util.ReadCredentials(s, core.Development)
	require.NoError(t, err)
	asrt.Len(creds.Keys, 5)
	_, err = rootClient(t, addr, creds.Token).PoliciesListAclPolicies(ctx)
	asrt.NoError(err, "the stored root token must work")

	// a second run must find Vault initialized and unsealed and change nothing
	require.NoError(t, initializeAndUnseal(ctx, s, core.Development, opts, pods[0], pods, true, reach))
	again, err := util.ReadCredentials(s, core.Development)
	require.NoError(t, err)
	asrt.Equal(creds.Token, again.Token)
}

// With auto-unseal, a fresh Vault is initialized with recovery keys and never unsealed by waltr;
// the first run used to fail afterwards, looking for unseal keys there are none of.
func TestInitializeWithAutoUnseal(t *testing.T) {
	addr := vaulttest.StartServer(t)
	ctx := t.Context()

	pods := []corev1.Pod{vaultPod("vault-0")}
	s := newState(t, pods...)

	vf := vaultFlags{}
	vf.conn.Via = "direct"
	reach := func(ctx context.Context, _ corev1.Pod) (func(), error) { return vf.dial(ctx, s, addr) }

	// needsUnseal=false is what a chart with a seal stanza yields; the test server has no real
	// auto-unseal, so it is initialized with unseal keys, which must still be stored
	require.NoError(t, initializeAndUnseal(ctx, s, core.Development, initOptions{threshold: 3, shares: 5}, pods[0], pods, false, reach))

	creds, err := util.ReadCredentials(s, core.Development)
	require.NoError(t, err)
	assert.NotEmpty(t, creds.Token)
}

// TestCommandsAgainstVault runs the configuration commands in the order a bootstrap does, with
// every connection setting coming from waltr.yaml.
func TestCommandsAgainstVault(t *testing.T) {
	asrt := assert.New(t)
	addr := vaulttest.StartDev(t)
	ctx := t.Context()
	s := newState(t)

	path := filepath.Join(t.TempDir(), "waltr.yaml")
	require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf(`
vault-addr: %s
token: %s
via: direct
mounts:
  kubernetes-host: https://kubernetes.default.svc:443
  kubernetes-ca-cert: %s
`, addr, vaulttest.RootToken, writeCA(t))), 0o600))

	for _, args := range [][]string{
		{"mounts"},
		{"configure"},
		{"transit"},
		{"prepare", "keycloak"},
		{"prepare", "gitlab"},
	} {
		require.NoError(t, run(t, s, append(args, "--config", path)...), "waltr %v", args)
	}

	vc := rootClient(t, addr, vaulttest.RootToken)

	methods, err := vc.AuthListEnabledMethods(ctx)
	require.NoError(t, err)
	asrt.Contains(methods, "kubernetes/")
	asrt.Contains(methods, "oidc/")

	engines, err := vc.MountsListSecretsEngines(ctx)
	require.NoError(t, err)
	asrt.Contains(engines, "kv/")

	policies, err := vc.PoliciesListAclPolicies(ctx)
	require.NoError(t, err)
	asrt.Contains(policies, "keycloak")

	k8s, err := vc.Read(ctx, "auth/kubernetes/config")
	require.NoError(t, err)
	asrt.Equal("https://kubernetes.default.svc:443", k8s["kubernetes_host"], "the scoped setting must reach mounts")

	// a policy changed by hand must survive a re-run without --overwrite, which needs the
	// existing policies to be seen
	require.NoError(t, vc.PoliciesWriteAclPolicy(ctx, "keycloak", `path "kv/data/manual" { capabilities = ["read"] }`))
	require.NoError(t, run(t, s, "configure", "--config", path))
	acl, err := vc.Read(ctx, "sys/policies/acl/keycloak")
	require.NoError(t, err)
	asrt.Contains(acl["policy"], "kv/data/manual")

	// the transit policy must grant what the operator's client cache does with the key
	vso, err := vc.Read(ctx, "sys/policies/acl/vso-auth")
	require.NoError(t, err)
	asrt.Contains(vso["policy"], "transit/encrypt/vso-client-cache")

	// re-running with --overwrite must work, not trip over the mounted engine
	require.NoError(t, run(t, s, "transit", "--overwrite", "--config", path))

	// and mounts --overwrite rewrites the Kubernetes auth configuration
	require.NoError(t, run(t, s, "mounts", "--overwrite", "--kubernetes-host", "https://api.example.org:6443", "--config", path))
	k8s, err = vc.Read(ctx, "auth/kubernetes/config")
	require.NoError(t, err)
	asrt.Equal("https://api.example.org:6443", k8s["kubernetes_host"])

	for _, secret := range []string{"keycloak/config", "keycloak/credentials/postgresql"} {
		data, err := vc.KvV2Read(ctx, "kv", secret)
		require.NoError(t, err, secret)
		asrt.NotEmpty(data["password"], secret)
	}
}

// writeCA writes a self-signed CA certificate, standing in for the Kubernetes API server's, which
// a Vault running outside the cluster has to be given.
func writeCA(t *testing.T) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "kubernetes"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "ca.crt")
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600))

	return path
}

func TestCommandsNeedAnAddress(t *testing.T) {
	s := newState(t)

	err := run(t, s, "configure", "--token", "x")
	assert.ErrorContains(t, err, "vault-addr")
}
