package helm

import (
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
)

// Apply installs or upgrades name to the given chart/values, mirroring `helm upgrade --install`.
// It rolls back automatically on failure (Atomic) and waits for resources to become ready.
func (c *Client) Apply(name string, ch *chart.Chart, vals map[string]interface{}) (*release.Release, error) {
	client := action.NewUpgrade(c.cfg)
	client.Namespace = c.namespace
	client.Install = true
	client.Wait = true
	client.Timeout = 10 * time.Minute
	client.Atomic = true
	client.CleanupOnFail = true

	return client.Run(name, ch, vals)
}
