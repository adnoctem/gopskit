package cmd

import (
	"os"
	"path/filepath"
	"testing"

	apihelm "github.com/adnoctem/gopskit/pkg/helm"
	"github.com/adnoctem/gopskit/pkg/kube/k3stest"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The step-certificates chart release the generated CA is installed with.
const (
	chartRepository = "https://smallstep.github.io/helm-charts"
	chartVersion    = "1.30.0"
)

// TestChartRunsWithGeneratedCA installs the real step-certificates chart in a k3s cluster with a
// CA from steppa, in both ways the chart's installation docs describe: injected through values,
// and from Secrets created ahead of the release. Helm waits for step-ca to pass the chart's own
// readiness probe, which it only does once it has decrypted every key with its password.
func TestChartRunsWithGeneratedCA(t *testing.T) {
	_, kc := k3stest.Start(t)

	// Helm keeps its repository cache under these
	helmHome := t.TempDir()
	t.Setenv("HELM_CACHE_HOME", filepath.Join(helmHome, "cache"))
	t.Setenv("HELM_CONFIG_HOME", filepath.Join(helmHome, "config"))
	t.Setenv("HELM_DATA_HOME", filepath.Join(helmHome, "data"))

	for _, mode := range []struct {
		name    string
		outputs string
	}{
		{"inject", "values"},
		{"existing-secrets", "kubernetes,values"},
	} {
		t.Run(mode.name, func(t *testing.T) {
			ctx := t.Context()
			namespace := "pki-" + mode.name

			_, err := kc.Client.CoreV1().Namespaces().Create(ctx,
				&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
			require.NoError(t, err)

			s, _, dir := newState(t)
			s.Kube = kc

			_, err = run(t, s, "init", "--name", "Chart Test", "--dns", "ca.example.org", "--ssh", "--acme",
				"--namespace", namespace, "-o", mode.outputs, "--root-backup", "root.yaml")
			require.NoError(t, err)

			raw, err := os.ReadFile(filepath.Join(dir, "step-certificates-values.yaml"))
			require.NoError(t, err)
			var values map[string]any
			require.NoError(t, yaml.Unmarshal(raw, &values))

			hc, err := apihelm.New(namespace, apihelm.WithRESTClientGetter(kc.Flags))
			require.NoError(t, err)

			ch, err := hc.LoadChart("step-certificates", chartRepository, chartVersion)
			require.NoError(t, err)

			// Apply waits for the release's workloads to become ready
			rel, err := hc.Apply("step-certificates", ch, values)
			require.NoError(t, err, "step-ca did not come up with the generated CA")
			require.Equal(t, "deployed", rel.Info.Status.String())
		})
	}
}
