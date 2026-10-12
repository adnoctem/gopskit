package bridge_test

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/adnoctem/gopskit/pkg/bridge"
	"github.com/adnoctem/gopskit/pkg/bridge/socks5"
	"github.com/adnoctem/gopskit/pkg/kube"
	"github.com/adnoctem/gopskit/pkg/kube/k3stest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	// bridgeImage is the tag the image tarball carries; see //cmd/gopskit-bridge:load.
	bridgeImage = "gopskit-bridge:dev"

	// imageTarEnv names the docker-format image tarball. Bazel sets it from
	// //cmd/gopskit-bridge:tarball; under plain `go test`, build that target and point this at it.
	imageTarEnv = "GOPSKIT_BRIDGE_IMAGE_TAR"

	// a target in kube-system that k3s ships, used to observe the NetworkPolicy
	metricsServer = "metrics-server.kube-system.svc:443"
)

// TestBridgeEndToEnd deploys the real bridge image into a k3s cluster and verifies the properties
// the CLIs depend on, including the security ones the design rests on.
func TestBridgeEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end test skipped in -short mode")
	}

	tarPath := os.Getenv(imageTarEnv)
	if tarPath == "" {
		t.Skipf("%s is unset; run under Bazel, or `bazel build //cmd/gopskit-bridge:tarball` and set it", imageTarEnv)
	}
	// Bazel passes a path relative to the runfiles root, while tests run in their package's
	// directory
	if srcDir, ws := os.Getenv("TEST_SRCDIR"), os.Getenv("TEST_WORKSPACE"); srcDir != "" && !filepath.IsAbs(tarPath) {
		tarPath = filepath.Join(srcDir, ws, tarPath)
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx := t.Context()
	kc := startCluster(t, ctx, tarPath)

	cfg := bridge.Config{Namespace: bridge.DefaultNamespace, Image: bridgeImage}
	require.NoError(t, bridge.Setup(ctx, kc.Client, cfg, t.Logf))
	waitReady(t, ctx, kc)

	t.Run("reaches cluster services by name", func(t *testing.T) {
		tun := connect(t, ctx, kc)

		client := &http.Client{
			Timeout: 30 * time.Second,
			// the test only proves the request arrives; the API server's certificate is k3s's own
			Transport: tun.Transport(&tls.Config{InsecureSkipVerify: true}), //nolint:gosec
		}

		// kubernetes.default.svc resolves only inside the cluster, so a response proves the
		// name travelled unresolved and the cluster's DNS answered it. k3s disables anonymous
		// access, so the API server answers 401 - which is still its answer.
		resp, err := client.Get("https://kubernetes.default.svc/version")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, []int{http.StatusOK, http.StatusUnauthorized}, resp.StatusCode, "body: %s", body)
		assert.Contains(t, string(body), `"apiVersion"`, "the response must come from the Kubernetes API server")
	})

	t.Run("is unreachable on its pod IP", func(t *testing.T) {
		tun := connect(t, ctx, kc)

		pods, err := kc.Client.CoreV1().Pods(bridge.DefaultNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{"app.kubernetes.io/name": bridge.Name}).String(),
		})
		require.NoError(t, err)
		require.NotEmpty(t, pods.Items)
		podIP := pods.Items[0].Status.PodIP
		require.NotEmpty(t, podIP)

		// ask the bridge to connect to its own pod IP: since it listens on loopback only, that
		// is exactly what any other Pod in the cluster would get
		_, err = tun.Dial(ctx, net.JoinHostPort(podIP, strconv.Itoa(bridge.Port)))

		var replyErr *socks5.ReplyError
		require.ErrorAs(t, err, &replyErr)
		assert.Equal(t, socks5.ReplyConnectionRefused, replyErr.Code)
	})

	t.Run("network policy denies ingress and limits egress", func(t *testing.T) {
		require.Eventually(t, func() bool { return dialOK(ctx, t, kc, metricsServer) }, 3*time.Minute, 2*time.Second,
			"%s must be reachable before the policy is applied", metricsServer)

		cfg := cfg
		cfg.NetworkPolicy = true
		cfg.AllowNamespaces = []string{"default"}
		require.NoError(t, bridge.Setup(ctx, kc.Client, cfg, t.Logf))

		// a port-forward enters the Pod's loopback interface, which the ingress denial must not
		// affect - this is the assumption the bridge's access model rests on
		require.Eventually(t, func() bool { return !dialOK(ctx, t, kc, metricsServer) }, time.Minute, 2*time.Second,
			"egress to kube-system must be blocked while only default is allowed")
		connect(t, ctx, kc)

		cfg.AllowNamespaces = []string{"kube-system"}
		require.NoError(t, bridge.Setup(ctx, kc.Client, cfg, t.Logf))
		require.Eventually(t, func() bool { return dialOK(ctx, t, kc, metricsServer) }, time.Minute, 2*time.Second,
			"egress to kube-system must be allowed once it is listed")
	})

	t.Run("cleanup removes the bridge", func(t *testing.T) {
		require.NoError(t, bridge.Cleanup(ctx, kc.Client, bridge.DefaultNamespace, t.Logf))

		ns, err := kc.Client.CoreV1().Namespaces().Get(ctx, bridge.DefaultNamespace, metav1.GetOptions{})
		if !apierrors.IsNotFound(err) {
			require.NoError(t, err)
			assert.NotNil(t, ns.DeletionTimestamp, "the namespace setup created must be deleted")
		}

		// namespace deletion is asynchronous; the bridge Pod outlives the call for a moment
		assert.Eventually(t, func() bool {
			tun, err := bridge.Connect(ctx, kc, bridge.DefaultNamespace)
			if err == nil {
				_ = tun.Close()
			}
			return errors.Is(err, bridge.ErrNotRunning)
		}, 2*time.Minute, 2*time.Second, "the bridge must be gone after cleanup")
	})
}

// startCluster runs k3s, imports the bridge image into it and returns a client for it.
func startCluster(t *testing.T, ctx context.Context, tarPath string) *kube.Client {
	t.Helper()

	ctr, kc := k3stest.Start(t)

	const inContainer = "/tmp/gopskit-bridge.tar"
	require.NoError(t, ctr.CopyFileToContainer(ctx, tarPath, inContainer, 0o644))

	code, out, err := ctr.Exec(ctx, []string{"ctr", "-n=k8s.io", "images", "import", inContainer})
	require.NoError(t, err)
	if code != 0 {
		msg, _ := io.ReadAll(out)
		t.Fatalf("importing the bridge image failed (exit %d): %s", code, msg)
	}

	return kc
}

func waitReady(t *testing.T, ctx context.Context, kc *kube.Client) {
	t.Helper()

	waitCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	require.NoError(t, bridge.WaitReady(waitCtx, kc.Client, bridge.DefaultNamespace))
}

func connect(t *testing.T, ctx context.Context, kc *kube.Client) *bridge.Tunnel {
	t.Helper()

	tun, err := bridge.Connect(ctx, kc, bridge.DefaultNamespace)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tun.Close() })

	return tun
}

// dialOK reports whether target is reachable through a fresh tunnel. A policy that drops packets
// shows up as a dial timeout, so each attempt is bounded.
func dialOK(ctx context.Context, t *testing.T, kc *kube.Client, target string) bool {
	attempt, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	tun, err := bridge.Connect(attempt, kc, bridge.DefaultNamespace)
	if err != nil {
		t.Logf("tunnel: %v", err)
		return false
	}
	defer func() { _ = tun.Close() }()

	conn, err := tun.Dial(attempt, target)
	if err != nil {
		var replyErr *socks5.ReplyError
		if !errors.As(err, &replyErr) {
			t.Logf("dial %s: %v", target, err)
		}
		return false
	}
	_ = conn.Close()

	return true
}
