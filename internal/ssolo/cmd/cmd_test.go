package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adnoctem/gopskit/internal/ssolo/app"
	"github.com/adnoctem/gopskit/pkg/api/keycloak"
	"github.com/adnoctem/gopskit/pkg/api/keycloak/keycloaktest"
	apivault "github.com/adnoctem/gopskit/pkg/api/vault"
	"github.com/adnoctem/gopskit/pkg/api/vault/vaulttest"
	"github.com/adnoctem/gopskit/pkg/kube"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// newState is ssolo's state with its token cache in a temporary directory and a fake cluster.
func newState(t *testing.T, objects ...corev1.Secret) (*app.State, *fake.Clientset) {
	t.Helper()

	// keep the tests away from a real configuration file and token cache
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())

	s, err := app.New()
	require.NoError(t, err)
	s.Paths.Cache = t.TempDir()

	cs := fake.NewClientset()
	for i := range objects {
		_, err := cs.CoreV1().Secrets(objects[i].Namespace).Create(context.Background(), &objects[i], metav1.CreateOptions{})
		require.NoError(t, err)
	}
	s.Kube = &kube.Client{Client: cs}

	return s, cs
}

// run executes ssolo with args and returns what it printed to standard output.
func run(t *testing.T, s *app.State, args ...string) (string, error) {
	t.Helper()

	var stdout bytes.Buffer
	root := NewRootCommand(s)
	root.SetArgs(args)
	root.SetOut(&stdout)

	err := root.ExecuteContext(t.Context())
	return stdout.String(), err
}

func adminFlags(url string) []string {
	return []string{"--url", url, "--username", keycloaktest.AdminUser, "--password", keycloaktest.AdminPassword}
}

func adminClient(t *testing.T, url string) *keycloak.Client {
	t.Helper()

	kc := keycloak.New(url,
		keycloak.WithAuthPath(filepath.Join(t.TempDir(), "creds.json")),
		keycloak.WithUsername(keycloaktest.AdminUser),
		keycloak.WithPassword(keycloaktest.AdminPassword),
	)
	require.NoError(t, kc.Login())

	return kc
}

func TestAgainstKeycloak(t *testing.T) {
	url := keycloaktest.Start(t)
	ctx := t.Context()

	t.Run("init creates realms with their groups", func(t *testing.T) {
		asrt := assert.New(t)
		s, _ := newState(t)

		args := append([]string{"init", "--realms", "ops,apps", "--groups", "admins,users"}, adminFlags(url)...)
		_, err := run(t, s, args...)
		require.NoError(t, err)

		// a second run must find everything in place rather than fail
		_, err = run(t, s, args...)
		require.NoError(t, err)

		kc := adminClient(t, url)
		for _, realm := range []string{"ops", "apps"} {
			for _, group := range []string{"admins", "users"} {
				exists, err := kc.GroupExists(ctx, realm, group)
				require.NoError(t, err)
				asrt.True(exists, "group %s in realm %s", group, realm)
			}
		}

		inMaster, err := kc.GroupExists(ctx, keycloak.DefaultRealm, "admins")
		require.NoError(t, err)
		asrt.False(inMaster, "groups must be created in the new realms, not in master")
	})

	t.Run("init reads credentials from a Secret", func(t *testing.T) {
		s, _ := newState(t, corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "keycloak-admin", Namespace: "keycloak"},
			Data: map[string][]byte{
				"username": []byte(keycloaktest.AdminUser),
				"password": []byte(keycloaktest.AdminPassword),
			},
		})

		_, err := run(t, s, "init", "--url", url, "--credentials-secret", "keycloak/keycloak-admin", "--realms", "from-secret")
		require.NoError(t, err)

		exists, err := adminClient(t, url).RealmExists(ctx, "from-secret")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("settings come from the configuration file", func(t *testing.T) {
		s, _ := newState(t)

		path := filepath.Join(t.TempDir(), "ssolo.yaml")
		require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf(`
url: %s
username: %s
password: %s
initialize:
  realms: [from-config]
  groups: [readers]
`, url, keycloaktest.AdminUser, keycloaktest.AdminPassword)), 0o600))

		// the section is written under the command's name; "init" is only an alias of it
		_, err := run(t, s, "init", "--config", path)
		require.NoError(t, err)

		exists, err := adminClient(t, url).GroupExists(ctx, "from-config", "readers")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("client apply writes the settings to every output", func(t *testing.T) {
		asrt := assert.New(t)
		vaultAddr := vaulttest.StartDev(t)

		s, cs := newState(t, corev1.Secret{
			// an existing Secret with a key of the application's own, which must survive
			ObjectMeta: metav1.ObjectMeta{Name: "vaultwarden-oidc", Namespace: "vaultwarden"},
			Data:       map[string][]byte{"cookie-secret": []byte("keep-me")},
		})

		_, err := run(t, s, append([]string{"init", "--realms", "applications", "--groups", "users"}, adminFlags(url)...)...)
		require.NoError(t, err)

		args := append([]string{
			"client", "apply", "vaultwarden", "--realm", "applications",
			"--redirect-uri", "https://vault.example.org/identity/connect/oidc-signin",
			"--public-url", "https://sso.example.org",
			"-o", "stdout,kubernetes,vault", "--format", "json",
			"--secret-namespace", "vaultwarden",
			"--vault-addr", vaultAddr, "--vault-token", vaulttest.RootToken, "--vault-mount", vaulttest.KVMount,
		}, adminFlags(url)...)

		// a key the application keeps at the same Vault path, which apply must leave alone
		preexisting := rootClientFor(t, vaultAddr)
		require.NoError(t, preexisting.KvV2Write(ctx, vaulttest.KVMount, "vaultwarden/credentials/oidc",
			map[string]any{"admin-token": "keep-me"}))

		stdout, err := run(t, s, args...)
		require.NoError(t, err)

		var printed map[string]string
		require.NoError(t, json.Unmarshal([]byte(stdout), &printed), "stdout must hold only the JSON document: %q", stdout)
		asrt.Equal("vaultwarden", printed["client-id"])
		asrt.Equal("https://sso.example.org/realms/applications", printed["issuer"])
		asrt.NotEmpty(printed["client-secret"])

		secret, err := cs.CoreV1().Secrets("vaultwarden").Get(ctx, "vaultwarden-oidc", metav1.GetOptions{})
		require.NoError(t, err)
		asrt.Equal(printed["client-secret"], string(secret.Data["client-secret"]))
		asrt.Equal(printed["issuer"], string(secret.Data["issuer"]))
		asrt.Equal("keep-me", string(secret.Data["cookie-secret"]))

		vc := apivault.New(vaultAddr)
		require.NoError(t, vc.SetToken(&apivault.Credentials{Token: vaulttest.RootToken}))
		stored, err := vc.KvV2Read(ctx, vaulttest.KVMount, "vaultwarden/credentials/oidc")
		require.NoError(t, err)
		asrt.Equal(printed["client-secret"], stored["client-secret"])
		asrt.Equal("vaultwarden", stored["client-id"])
		asrt.Equal("keep-me", stored["admin-token"], "other keys at the path must survive")

		again, err := run(t, s, args...)
		require.NoError(t, err)
		var reprinted map[string]string
		require.NoError(t, json.Unmarshal([]byte(again), &reprinted))
		asrt.Equal(printed["client-secret"], reprinted["client-secret"], "applying again must keep the secret")
	})

	t.Run("client apply prints shell-safe env lines", func(t *testing.T) {
		s, _ := newState(t)

		stdout, err := run(t, s, append([]string{
			"client", "apply", "grafana", "--realm", "master",
			"--redirect-uri", "https://grafana.example.org/login/generic_oauth",
		}, adminFlags(url)...)...)
		require.NoError(t, err)

		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		require.Len(t, lines, 3)
		assert.Equal(t, "OIDC_CLIENT_ID='grafana'", lines[0])
		assert.True(t, strings.HasPrefix(lines[1], "OIDC_CLIENT_SECRET='"))
		assert.Equal(t, "OIDC_ISSUER='"+url+"/realms/master'", lines[2])
	})

	t.Run("client apply checks its outputs before touching Keycloak", func(t *testing.T) {
		s, _ := newState(t)

		_, err := run(t, s, append([]string{
			"client", "apply", "never-created", "--realm", "master",
			"--redirect-uri", "https://x.example.org/cb", "-o", "kubernetes",
		}, adminFlags(url)...)...)
		require.ErrorContains(t, err, "--secret-namespace")

		exists, err := adminClient(t, url).ClientExists(ctx, keycloak.DefaultRealm, "never-created")
		require.NoError(t, err)
		assert.False(t, exists, "a client must not be created when its secret has nowhere to go")
	})
}

func rootClientFor(t *testing.T, addr string) *apivault.Client {
	t.Helper()

	vc := apivault.New(addr)
	require.NoError(t, vc.SetToken(&apivault.Credentials{Token: vaulttest.RootToken}))

	return vc
}

func TestClientApplyNeedsURL(t *testing.T) {
	t.Setenv("KEYCLOAK_URL", "")
	s, _ := newState(t)

	_, err := run(t, s, "client", "apply", "x", "--realm", "apps", "--redirect-uri", "https://x/cb")
	assert.ErrorContains(t, err, "URL of Keycloak is required")
}

// Realm names are the cluster's own, so there are none to assume.
func TestRealmsMustBeNamed(t *testing.T) {
	s, _ := newState(t)

	_, err := run(t, s, "init", "--url", "https://sso.example.org")
	assert.ErrorContains(t, err, "--realms")

	_, err = run(t, s, "client", "apply", "x", "--url", "https://sso.example.org", "--redirect-uri", "https://x/cb")
	assert.ErrorContains(t, err, "--realm")
}

func TestShellQuote(t *testing.T) {
	assert.Equal(t, `'plain'`, shellQuote("plain"))
	assert.Equal(t, `'it'\''s'`, shellQuote("it's"))
}

func TestIssuerURL(t *testing.T) {
	got, err := issuerURL("https://sso.example.org/", "applications")
	require.NoError(t, err)
	assert.Equal(t, "https://sso.example.org/realms/applications", got)

	_, err = issuerURL("sso.example.org", "applications")
	assert.Error(t, err)
}
