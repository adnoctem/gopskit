package helm

import (
	"errors"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
)

// Apply installs or upgrades name to the given chart/values, mirroring `helm upgrade --install`.
// It rolls back automatically on failure (Atomic) and waits for resources to become ready.
//
// action.Upgrade's own Install field is purely informative and does not actually make Run() fall
// back to installing a missing release (see the field's doc comment in the Helm SDK) - that
// fallback has to be implemented by the caller, the same way Helm's own `helm upgrade --install`
// CLI command does it: check the release's history first, and install instead of upgrading if
// it doesn't exist (or was previously uninstalled).
func (c *Client) Apply(name string, ch *chart.Chart, vals map[string]interface{}) (*release.Release, error) {
	hist := action.NewHistory(c.cfg)
	hist.Max = 1
	versions, err := hist.Run(name)
	notInstalledYet := errors.Is(err, driver.ErrReleaseNotFound) ||
		(err == nil && len(versions) > 0 && versions[len(versions)-1].Info.Status == release.StatusUninstalled)

	if notInstalledYet {
		install := action.NewInstall(c.cfg)
		install.ReleaseName = name
		install.Namespace = c.namespace
		install.Wait = true
		install.Timeout = 10 * time.Minute
		install.Atomic = true

		return install.Run(ch, vals)
	}

	client := action.NewUpgrade(c.cfg)
	client.Namespace = c.namespace
	client.Install = true
	client.Wait = true
	client.Timeout = 10 * time.Minute
	client.Atomic = true
	client.CleanupOnFail = true

	return client.Run(name, ch, vals)
}
