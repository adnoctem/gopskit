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
	"strings"
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

// exampleFile is the example configuration shipped with waltr. The tests apply it to a real Vault,
// so it can't drift from what waltr accepts. It is resolved when the package loads, since the tests
// change into temporary directories.
var exampleFile = func() string {
	path, err := filepath.Abs("../../../cmd/waltr/waltr.example.yaml")
	if err != nil {
		panic(err)
	}
	return path
}()

// exampleFor is the example configuration pointed at the Vault at addr. Since that Vault runs
// outside any cluster, its Kubernetes auth method is given a CA, as the example's comments say.
func exampleFor(t *testing.T, addr string) string {
	t.Helper()

	raw, err := os.ReadFile(exampleFile)
	require.NoError(t, err)

	content := strings.Replace(string(raw), "vault-addr: https://vault.vault.svc:8200",
		"vault-addr: "+addr+"\ntoken: "+vaulttest.RootToken+"\nvia: direct", 1)
	content = strings.Replace(content, "host: https://kubernetes.default.svc\n",
		"host: https://kubernetes.default.svc\n        caCertFile: "+writeCA(t)+"\n", 1)

	path := filepath.Join(t.TempDir(), "waltr.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// runOut is run, returning what the command printed to stdout.
func runOut(t *testing.T, s *app.State, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	root := NewRootCommand(s)
	root.SetArgs(args)
	root.SetOut(&out)

	err := root.ExecuteContext(t.Context())
	return out.String(), err
}

func TestExampleValidates(t *testing.T) {
	s := newState(t)
	assert.NoError(t, run(t, s, "validate", "--config", exampleFile))
}

// TestApplyExample applies the shipped example to a real Vault and checks that applying is
// idempotent, converges drift, and keeps generated values unless asked to rotate them.
func TestApplyExample(t *testing.T) {
	asrt := assert.New(t)
	addr := vaulttest.StartDev(t)
	ctx := t.Context()
	s := newState(t)
	cfg := exampleFor(t, addr)
	vc := rootClient(t, addr, vaulttest.RootToken)

	// a dry run changes nothing
	planned, err := runOut(t, s, "apply", "--dry-run", "--config", cfg)
	require.NoError(t, err)
	asrt.Contains(planned, "create mount kv (kv-v2)")
	asrt.Contains(planned, "create secret kv/grafana/admin")
	_, err = vc.KvV2Read(ctx, "kv", "grafana/admin")
	asrt.Error(err, "a dry run must not write anything")

	require.NoError(t, run(t, s, "apply", "--config", cfg))

	engines, err := vc.MountsListSecretsEngines(ctx)
	require.NoError(t, err)
	asrt.Contains(engines, "kv/")
	asrt.Contains(engines, "transit/")

	key, err := vc.Read(ctx, "transit/keys/vault-secrets-operator")
	require.NoError(t, err)
	asrt.NotEmpty(key)

	role, err := vc.Read(ctx, "auth/kubernetes/role/grafana")
	require.NoError(t, err)
	asrt.Equal([]any{"grafana"}, role["bound_service_account_names"])
	asrt.Equal([]any{"monitoring"}, role["bound_service_account_namespaces"])
	asrt.Equal("86400", fmt.Sprint(role["token_period"]))

	admin, err := vc.KvV2Read(ctx, "kv", "grafana/admin")
	require.NoError(t, err)
	asrt.Equal("admin", admin["username"])
	password, _ := admin["password"].(string)
	asrt.Len(password, 64, "generated from the alphanumeric password policy")

	t.Run("applying again changes nothing", func(t *testing.T) {
		planned, err := runOut(t, s, "apply", "--dry-run", "--config", cfg)
		require.NoError(t, err)
		assert.Empty(t, planned, "every object must already be as declared")

		require.NoError(t, run(t, s, "apply", "--config", cfg))
		again, err := vc.KvV2Read(ctx, "kv", "grafana/admin")
		require.NoError(t, err)
		assert.Equal(t, password, again["password"], "a generated value is kept")
	})

	t.Run("drift is brought back", func(t *testing.T) {
		require.NoError(t, vc.PoliciesWriteAclPolicy(ctx, "grafana", `path "kv/*" { capabilities = ["read"] }`))
		require.NoError(t, vc.KvV2Write(ctx, "kv", "grafana/admin", map[string]any{
			"username": "root", "password": password, "added-by-hand": "kept",
		}))

		planned, err := runOut(t, s, "apply", "--dry-run", "--config", cfg)
		require.NoError(t, err)
		assert.Contains(t, planned, "update policy grafana")
		assert.Contains(t, planned, "update secret kv/grafana/admin (~username)")

		require.NoError(t, run(t, s, "apply", "--config", cfg))
		policy, err := vc.Read(ctx, "sys/policies/acl/grafana")
		require.NoError(t, err)
		assert.Contains(t, policy["policy"], "kv/data/grafana/*")

		fixed, err := vc.KvV2Read(ctx, "kv", "grafana/admin")
		require.NoError(t, err)
		assert.Equal(t, "admin", fixed["username"])
		assert.Equal(t, password, fixed["password"])
		assert.Equal(t, "kept", fixed["added-by-hand"], "undeclared keys survive")
	})

	t.Run("rotate replaces generated values only", func(t *testing.T) {
		require.NoError(t, run(t, s, "secret", "apply", "--rotate", "--config", cfg))

		rotated, err := vc.KvV2Read(ctx, "kv", "grafana/admin")
		require.NoError(t, err)
		assert.NotEqual(t, password, rotated["password"])
		assert.Equal(t, "admin", rotated["username"])
	})
}

// The direct commands change one object at a time, through the same engine as apply.
func TestDirectCommands(t *testing.T) {
	asrt := assert.New(t)
	addr := vaulttest.StartDev(t)
	ctx := t.Context()
	s := newState(t)
	vc := rootClient(t, addr, vaulttest.RootToken)
	conn := []string{"--vault-addr", addr, "--token", vaulttest.RootToken, "--via", "direct"}

	policyFile := filepath.Join(t.TempDir(), "app.hcl")
	require.NoError(t, os.WriteFile(policyFile, []byte(`path "kv/data/app/*" { capabilities = ["read"] }`), 0o600))
	passwordFile := filepath.Join(t.TempDir(), "strong.hcl")
	require.NoError(t, os.WriteFile(passwordFile, []byte("length = 24\nrule \"charset\" {\n  charset = \"abc\"\n}\n"), 0o600))

	for _, args := range [][]string{
		{"mount", "enable", "kv", "--type", "kv-v2"},
		{"auth", "enable", "kubernetes", "--type", "kubernetes",
			"--kubernetes-host", "https://kubernetes.default.svc", "--kubernetes-ca-cert", writeCA(t)},
		{"password-policy", "write", "strong", "--file", passwordFile},
		{"policy", "write", "app", "--file", policyFile},
		{"role", "write", "app", "--service-account", "app", "--service-account-namespace", "apps",
			"--policy", "app", "--token-ttl", "1h"},
		{"secret", "write", "kv/app/config", "--set", "user=app", "--generate", "password=strong"},
	} {
		require.NoError(t, run(t, s, append(args, conn...)...), "waltr %v", args)
	}

	role, err := vc.Read(ctx, "auth/kubernetes/role/app")
	require.NoError(t, err)
	asrt.Equal("3600", fmt.Sprint(role["token_ttl"]))

	secret, err := vc.KvV2Read(ctx, "kv", "app/config")
	require.NoError(t, err)
	asrt.Equal("app", secret["user"])
	asrt.Len(secret["password"], 24)

	// re-enabling at a path with a different engine is refused, not a remount
	err = run(t, s, append([]string{"mount", "enable", "kv", "--type", "transit"}, conn...)...)
	asrt.ErrorContains(err, "never remounts")
}

// A group's apply only touches its own section.
func TestGroupApply(t *testing.T) {
	addr := vaulttest.StartDev(t)
	s := newState(t)
	vc := rootClient(t, addr, vaulttest.RootToken)

	require.NoError(t, run(t, s, "policy", "apply", "--config", exampleFor(t, addr)))

	policies, err := vc.PoliciesListAclPolicies(t.Context())
	require.NoError(t, err)
	assert.Contains(t, policies, "grafana")

	engines, err := vc.MountsListSecretsEngines(t.Context())
	require.NoError(t, err)
	assert.NotContains(t, engines, "kv/", "policy apply must not mount engines")
}

func TestApplyNeedsAVaultSection(t *testing.T) {
	s := newState(t)
	path := filepath.Join(t.TempDir(), "waltr.yaml")
	require.NoError(t, os.WriteFile(path, []byte("vault-addr: https://vault.example.org\n"), 0o600))

	err := run(t, s, "apply", "--config", path, "--token", "x")
	assert.ErrorContains(t, err, "has no vault: section")
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

	err := run(t, s, "mount", "enable", "kv", "--type", "kv-v2", "--token", "x")
	assert.ErrorContains(t, err, "vault-addr")
}
