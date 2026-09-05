package helm

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chartutil"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
)

// newTestClient builds a Client backed entirely by in-memory/no-op Helm test doubles - an
// in-memory release-storage driver and a fake Kube client that always reports success without
// touching a real cluster - so pkg/helm's own logic can be exercised without a live cluster.
func newTestClient(t *testing.T, namespace string) *Client {
	t.Helper()

	cfg := &action.Configuration{
		Releases:     storage.Init(driver.NewMemory()),
		KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard, LogOutput: io.Discard},
		Capabilities: chartutil.DefaultCapabilities,
		Log:          func(string, ...interface{}) {},
	}

	c, err := New(namespace, WithConfiguration(cfg))
	if err != nil {
		t.Fatal(err)
	}

	return c
}

func TestNewRequiresARESTClientGetterWithoutAConfiguration(t *testing.T) {
	asrt := assert.New(t)

	_, err := New("ns")
	asrt.Error(err)
}

func TestNewAcceptsAPrebuiltConfiguration(t *testing.T) {
	asrt := assert.New(t)

	c := newTestClient(t, "ns")
	asrt.NotNil(c)
	asrt.Equal("ns", c.namespace)
}
