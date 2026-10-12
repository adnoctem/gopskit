// Package k3stest runs a disposable Kubernetes cluster for tests: k3s, in a container.
package k3stest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/adnoctem/gopskit/pkg/kube"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/k3s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Image is the k3s release the tests run against. k3s enforces NetworkPolicies and ships a
// local-path storage provisioner out of the box.
const Image = "rancher/k3s:v1.36.5-k3s1"

// Start runs k3s and returns the container and a client for it, once the cluster's add-ons -
// DNS in particular - are available. Tests are skipped when no container runtime is available,
// and the container is removed when the test ends.
func Start(t *testing.T) (*k3s.K3sContainer, *kube.Client) {
	t.Helper()

	if testing.Short() {
		t.Skip("container test skipped in -short mode")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctx := t.Context()

	ctr, err := k3s.Run(ctx, Image)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("could not start k3s: %v", err)
	}

	cfg, err := ctr.GetKubeConfig(ctx)
	if err != nil {
		t.Fatalf("could not read the k3s kubeconfig: %v", err)
	}

	path := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(path, cfg, 0o600); err != nil {
		t.Fatal(err)
	}

	kc, err := kube.NewClient(kube.WithConfigPath(path))
	if err != nil {
		t.Fatalf("could not create a client for k3s: %v", err)
	}

	// a fresh k3s answers API requests before its add-ons run; cluster DNS in particular must be
	// up before anything resolves service names
	for _, name := range []string{"coredns", "metrics-server", "local-path-provisioner"} {
		deadline := time.Now().Add(3 * time.Minute)
		for {
			dep, err := kc.Client.AppsV1().Deployments("kube-system").Get(ctx, name, metav1.GetOptions{})
			if err == nil && dep.Status.AvailableReplicas > 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("kube-system/%s did not become available", name)
			}
			time.Sleep(time.Second)
		}
	}

	return ctr, kc
}
