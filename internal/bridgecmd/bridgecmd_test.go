package bridgecmd

import (
	"context"
	"crypto/tls"
	"testing"

	"github.com/adnoctem/gopskit/pkg/bridge"
	"github.com/adnoctem/gopskit/pkg/kube"
	"github.com/adnoctem/gopskit/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestUsesBridge(t *testing.T) {
	cases := []struct {
		via  string
		url  string
		want bool
	}{
		{"auto", "https://keycloak.keycloak.svc:8443", true},
		{"auto", "https://vault-0.vault-internal.vault.svc.cluster.local:8200", true},
		{"auto", "https://sso.example.org", false},
		{"", "https://keycloak.keycloak.svc", true},
		{"direct", "https://keycloak.keycloak.svc:8443", false},
		{"bridge", "https://sso.example.org", true},
	}

	for _, c := range cases {
		got, err := (&Connection{Via: c.via}).UsesBridge(c.url)
		require.NoError(t, err, "%s %s", c.via, c.url)
		assert.Equal(t, c.want, got, "--via %q for %s", c.via, c.url)
	}
}

func TestUsesBridgeRejectsBadInput(t *testing.T) {
	asrt := assert.New(t)

	_, err := (&Connection{Via: "teleport"}).UsesBridge("https://x.svc")
	asrt.ErrorContains(err, "invalid --via")

	_, err = (&Connection{}).UsesBridge("not a url")
	asrt.Error(err)
}

func TestDirectTransportNeedsNoKubernetes(t *testing.T) {
	o := Options{CLI: "ssolo", Kube: func() (*kube.Client, error) {
		t.Fatal("a direct connection must not touch Kubernetes")
		return nil, nil
	}, Log: log.New()}

	tlsConfig := &tls.Config{ServerName: "sso.example.org"}
	tr, release, err := (&Connection{Via: "auto"}).Transport(context.Background(), o, "https://sso.example.org", tlsConfig)
	require.NoError(t, err)
	defer release()
	assert.Same(t, tlsConfig, tr.TLSClientConfig)
}

func TestBridgeTransportFailsWithHintWhenNotRunning(t *testing.T) {
	kc := &kube.Client{Client: fake.NewClientset()}
	o := Options{CLI: "ssolo", Kube: func() (*kube.Client, error) { return kc, nil }, Log: log.New()}

	_, _, err := (&Connection{Via: "auto"}).Transport(context.Background(), o, "https://keycloak.keycloak.svc:8443", nil)
	assert.ErrorIs(t, err, bridge.ErrNotRunning)
	assert.ErrorContains(t, err, "run 'ssolo proxy setup' first")
}

func TestSetupAndCleanupCommands(t *testing.T) {
	asrt := assert.New(t)
	ctx := context.Background()

	cs := fake.NewClientset()
	o := Options{
		CLI:     "ssolo",
		Version: "1.4.0",
		Kube:    func() (*kube.Client, error) { return &kube.Client{Client: cs}, nil },
		Log:     log.New(),
	}

	setup := NewProxyCommand(o)
	setup.SetArgs([]string{"setup", "--wait=false", "--network-policy", "--allow-namespace", "keycloak,vault"})
	require.NoError(t, setup.ExecuteContext(ctx))

	dep, err := cs.AppsV1().Deployments(bridge.DefaultNamespace).Get(ctx, bridge.Name, metav1.GetOptions{})
	require.NoError(t, err)
	asrt.Equal(bridge.ImageRepository+":1.4.0", dep.Spec.Template.Spec.Containers[0].Image,
		"the image must default to the CLI's own version")

	np, err := cs.NetworkingV1().NetworkPolicies(bridge.DefaultNamespace).Get(ctx, bridge.Name, metav1.GetOptions{})
	require.NoError(t, err)
	asrt.Equal([]string{"keycloak", "vault"}, np.Spec.Egress[1].To[0].NamespaceSelector.MatchExpressions[0].Values)

	cleanup := NewProxyCommand(o)
	cleanup.SetArgs([]string{"cleanup"})
	require.NoError(t, cleanup.ExecuteContext(ctx))

	_, err = cs.CoreV1().Namespaces().Get(ctx, bridge.DefaultNamespace, metav1.GetOptions{})
	asrt.True(apierrors.IsNotFound(err))
}

func TestCheckCommandHintsAtSetup(t *testing.T) {
	o := Options{
		CLI:  "waltr",
		Kube: func() (*kube.Client, error) { return &kube.Client{Client: fake.NewClientset()}, nil },
		Log:  log.New(),
	}

	check := NewProxyCommand(o)
	check.SetArgs([]string{"check"})
	check.SilenceUsage = true

	err := check.ExecuteContext(context.Background())
	assert.ErrorContains(t, err, "run 'waltr proxy setup' first")
}
