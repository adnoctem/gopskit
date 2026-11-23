package helm

import (
	"fmt"
	"time"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
)

// DefaultApplyTimeout is Helm's timeout for an install or upgrade operation.
const DefaultApplyTimeout = 10 * time.Minute

type applyOptions struct {
	waitForJobs bool
	timeout     time.Duration
}

// ApplyOption configures an install or upgrade.
type ApplyOption func(*applyOptions)

// WithWaitForJobs controls whether readiness checks include Jobs. The default is true.
func WithWaitForJobs(wait bool) ApplyOption {
	return func(o *applyOptions) { o.waitForJobs = wait }
}

// WithApplyTimeout sets Helm's timeout for resource waits and other Kubernetes operations.
// It must be positive. The default is DefaultApplyTimeout.
func WithApplyTimeout(timeout time.Duration) ApplyOption {
	return func(o *applyOptions) { o.timeout = timeout }
}

// Apply installs or upgrades name to the given chart/values, mirroring `helm upgrade --install`.
// It rolls back automatically on failure (Atomic) and waits for resources, including Jobs,
// to become ready. Options can change Job waiting and the timeout without disabling Atomic.
//
// action.Upgrade's own Install field is purely informative and does not actually make Run() fall
// back to installing a missing release (see the field's doc comment in the Helm SDK) - that
// fallback has to be implemented by the caller, the same way Helm's own `helm upgrade --install`
// CLI command does it: check the release's history first, and install instead of upgrading if
// it doesn't exist (or was previously uninstalled).
func (c *Client) Apply(name string, ch *chart.Chart, vals map[string]interface{}, opts ...ApplyOption) (*release.Release, error) {
	options := applyOptions{waitForJobs: true, timeout: DefaultApplyTimeout}
	for _, opt := range opts {
		opt(&options)
	}
	if options.timeout <= 0 {
		return nil, fmt.Errorf("timeout must be greater than zero")
	}

	installed, err := c.Installed(name)
	if err != nil {
		return nil, fmt.Errorf("could not determine whether %q is installed: %w", name, err)
	}

	if !installed {
		install := action.NewInstall(c.cfg)
		install.ReleaseName = name
		install.Namespace = c.namespace
		install.Wait = true
		install.WaitForJobs = options.waitForJobs
		install.Timeout = options.timeout
		install.Atomic = true

		return install.Run(ch, vals)
	}

	client := action.NewUpgrade(c.cfg)
	client.Namespace = c.namespace
	client.Install = true
	client.Wait = true
	client.WaitForJobs = options.waitForJobs
	client.Timeout = options.timeout
	client.Atomic = true
	client.CleanupOnFail = true

	return client.Run(name, ch, vals)
}
