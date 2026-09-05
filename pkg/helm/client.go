// Package helm implements a client for the Helm SDK, used to manage Helm chart releases -
// carto's "backbone components" - on a Kubernetes cluster.
package helm

import (
	"fmt"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// DefaultDriver is the Helm storage driver used to persist release records, matching the
// Kubernetes-standard "secret" driver.
const DefaultDriver = "secret"

// Client wraps the Helm SDK's action.Configuration, adding a smaller, purpose-built surface for
// the operations carto needs.
type Client struct {
	// cfg is the underlying Helm action.Configuration we're wrapping
	cfg *action.Configuration

	// settings holds the Helm CLI environment settings (repo/plugin/cache paths, namespace)
	settings *cli.EnvSettings

	// namespace is the Kubernetes namespace this client operates releases in
	namespace string

	// getter resolves the Kubernetes REST client used to talk to the cluster
	getter genericclioptions.RESTClientGetter

	// driver is the Helm storage driver used to persist release records
	driver string

	// debugLog receives Helm's internal debug output
	debugLog action.DebugLog
}

// ClientOpt configures a Client during New
type ClientOpt func(c *Client) error

// New creates a new Helm API Client scoped to namespace, applying any of the provided ClientOpts
func New(namespace string, opts ...ClientOpt) (*Client, error) {
	c := &Client{
		namespace: namespace,
		driver:    DefaultDriver,
		debugLog:  func(string, ...interface{}) {},
	}

	for _, o := range opts {
		if err := o(c); err != nil {
			return nil, err
		}
	}

	if c.getter == nil {
		return nil, fmt.Errorf("cannot create Helm client without a RESTClientGetter (use WithRESTClientGetter)")
	}

	c.settings = cli.New()
	c.settings.SetNamespace(namespace)

	regClient, err := registry.NewClient()
	if err != nil {
		return nil, fmt.Errorf("could not create Helm OCI registry client: %w", err)
	}

	c.cfg = new(action.Configuration)
	if err := c.cfg.Init(c.getter, namespace, c.driver, c.debugLog); err != nil {
		return nil, fmt.Errorf("could not initialize Helm action configuration: %w", err)
	}
	c.cfg.RegistryClient = regClient

	return c, nil
}

// WithRESTClientGetter configures the Kubernetes REST client getter (e.g. kube.Client.Flags) the
// Helm client uses to talk to the cluster. Required.
func WithRESTClientGetter(getter genericclioptions.RESTClientGetter) ClientOpt {
	return func(c *Client) error {
		c.getter = getter
		return nil
	}
}

// WithDriver configures the Helm storage driver used to persist release records (default
// DefaultDriver, "secret")
func WithDriver(driver string) ClientOpt {
	return func(c *Client) error {
		c.driver = driver
		return nil
	}
}

// WithDebugLog configures the debug logger passed to Helm's action.Configuration
func WithDebugLog(fn action.DebugLog) ClientOpt {
	return func(c *Client) error {
		c.debugLog = fn
		return nil
	}
}
