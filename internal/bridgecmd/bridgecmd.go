// Package bridgecmd is the CLI side of the gopskit bridge, shared by every gopskit CLI that talks
// to cluster-internal services: the `proxy setup|check|cleanup` command group, and the flags and
// helper commands use to reach a service either directly or through the bridge.
package bridgecmd

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/adnoctem/gopskit/pkg/bridge"
	"github.com/adnoctem/gopskit/pkg/kube"
	"github.com/adnoctem/gopskit/pkg/log"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Options carries what the proxy commands need from the hosting CLI.
type Options struct {
	// CLI is the name of the hosting CLI, used in hints such as "run `ssolo proxy setup`".
	CLI string

	// Version is the hosting CLI's version, which selects the matching bridge image.
	Version string

	// Kube returns the Kubernetes client. It is called only when a command needs it.
	Kube func() (*kube.Client, error)

	// Log receives progress output.
	Log *log.Logger
}

// NewProxyCommand creates the `proxy` command group.
func NewProxyCommand(o Options) *cobra.Command {
	var namespace string

	cmd := &cobra.Command{
		Use:   "proxy",
		Short: "Manage the gopskit bridge into the cluster",
		Long: fmt.Sprintf(`Manage the gopskit bridge: a small SOCKS5 server deployed into the cluster, through which
%s reaches cluster-internal services such as Keycloak and Vault by their service names.

Set it up once with 'proxy setup'; commands then reach services such as
https://keycloak.keycloak.svc:8443 through it, and fail if it isn't running.`, o.CLI),
		TraverseChildren: true,
		// overrides the hosting CLI's hook, so the commands create the Kubernetes client
		// themselves, only when they need it
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
	}

	cmd.PersistentFlags().StringVar(&namespace, "bridge-namespace", bridge.DefaultNamespace,
		"The namespace the bridge runs in")

	cmd.AddCommand(newSetupCommand(o, &namespace), newCheckCommand(o, &namespace), newCleanupCommand(o, &namespace))
	return cmd
}

func newSetupCommand(o Options, namespace *string) *cobra.Command {
	var (
		image           string
		networkPolicy   bool
		allowNamespaces []string
		wait            bool
		timeout         time.Duration
	)

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Deploy or update the bridge",
		Long: `Deploy the bridge, or update an existing one. Safe to run repeatedly.

The bridge Pod satisfies the "restricted" Pod Security Standard, listens on its loopback interface
only and is reached exclusively through port-forwards, so access to it is governed by the RBAC
permission to port-forward to Pods in its namespace.

With --network-policy, the bridge also gets its own NetworkPolicy: no ingress at all, and egress
to DNS and to the namespaces given with --allow-namespace (every namespace if none are given).
Running setup again without the flag removes that policy.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			kc, err := o.Kube()
			if err != nil {
				return err
			}

			if image == "" {
				image = bridge.DefaultImage(o.Version)
			}

			cfg := bridge.Config{
				Namespace:       *namespace,
				Image:           image,
				NetworkPolicy:   networkPolicy,
				AllowNamespaces: allowNamespaces,
			}
			if err := bridge.Setup(cmd.Context(), kc.Client, cfg, o.Log.Infof); err != nil {
				return err
			}

			if !wait {
				return nil
			}

			o.Log.Infof("waiting up to %s for the bridge to become ready", timeout)
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			if err := bridge.WaitReady(ctx, kc.Client, *namespace); err != nil {
				return err
			}

			o.Log.Info("the bridge is ready")
			return nil
		},
	}

	cmd.Flags().StringVar(&image, "image", "",
		fmt.Sprintf("The gopskit-bridge image to run (default: %s)", bridge.DefaultImage(o.Version)))
	cmd.Flags().BoolVar(&networkPolicy, "network-policy", false, "Also deploy the bridge's own NetworkPolicy")
	cmd.Flags().StringSliceVar(&allowNamespaces, "allow-namespace", nil,
		"Namespaces the NetworkPolicy lets the bridge reach (repeatable; default: all)")
	cmd.Flags().BoolVar(&wait, "wait", true, "Wait for the bridge to become ready")
	cmd.Flags().DurationVar(&timeout, "timeout", 2*time.Minute, "How long --wait waits")

	return cmd
}

func newCheckCommand(o Options, namespace *string) *cobra.Command {
	var (
		targets []string
		timeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Verify the bridge is running and reachable",
		Long: `Verify the bridge is deployed and ready, open a tunnel to it and complete a SOCKS5 handshake.

With --target, additionally connect to each given host:port through the bridge, e.g.
--target keycloak.keycloak.svc:8443 - which also exercises cluster DNS and any NetworkPolicy.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			kc, err := o.Kube()
			if err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			st, err := bridge.GetStatus(ctx, kc.Client, *namespace)
			if err != nil {
				return hint(o, err)
			}
			o.Log.Infof("bridge Deployment: %d ready, image %s, NetworkPolicy: %t", st.ReadyReplicas, st.Image, st.NetworkPolicy)

			tun, err := bridge.Connect(ctx, kc, *namespace)
			if err != nil {
				return hint(o, err)
			}
			defer func() { _ = tun.Close() }()
			o.Log.Infof("tunnel open on %s, bridge answers", tun.Addr())

			var failed []string
			for _, target := range targets {
				conn, err := tun.Dial(ctx, target)
				if err != nil {
					o.Log.Errorf("cannot reach %s through the bridge: %v", target, err)
					failed = append(failed, target)
					continue
				}
				_ = conn.Close()
				o.Log.Infof("reached %s through the bridge", target)
			}

			if len(failed) > 0 {
				return fmt.Errorf("could not reach: %s", strings.Join(failed, ", "))
			}

			return nil
		},
	}

	cmd.Flags().StringSliceVar(&targets, "target", nil, "A host:port to connect to through the bridge (repeatable)")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "How long the whole check may take")

	return cmd
}

func newCleanupCommand(o Options, namespace *string) *cobra.Command {
	return &cobra.Command{
		Use:   "cleanup",
		Short: "Remove the bridge",
		Long: `Remove the bridge. A namespace 'proxy setup' created is deleted along with everything in it;
in a namespace that existed before, only the bridge's own resources are removed.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			kc, err := o.Kube()
			if err != nil {
				return err
			}

			return bridge.Cleanup(cmd.Context(), kc.Client, *namespace, o.Log.Infof)
		},
	}
}

func hint(o Options, err error) error {
	if errors.Is(err, bridge.ErrNotRunning) {
		return fmt.Errorf("%w - run '%s proxy setup' first", err, o.CLI)
	}

	return err
}

// ---- connecting to services ----

// Via selects how a CLI reaches a service.
type Via string

const (
	// ViaAuto uses the bridge for cluster-internal host names and connects directly otherwise.
	ViaAuto Via = "auto"

	// ViaDirect always connects directly, e.g. to an Ingress or a local container.
	ViaDirect Via = "direct"

	// ViaBridge always connects through the bridge.
	ViaBridge Via = "bridge"
)

// Connection holds the flags that decide how a command reaches a service.
type Connection struct {
	Via       string
	Namespace string
}

// AddFlags registers --via and --bridge-namespace.
func (c *Connection) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.Via, "via", string(ViaAuto),
		"How to reach services: auto (bridge for *.svc and *.cluster.local hosts), direct, or bridge")
	fs.StringVar(&c.Namespace, "bridge-namespace", bridge.DefaultNamespace, "The namespace the bridge runs in")
}

// UsesBridge reports whether a request to rawURL goes through the bridge.
func (c *Connection) UsesBridge(rawURL string) (bool, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false, fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}
	if u.Hostname() == "" {
		return false, fmt.Errorf("URL %q has no host", rawURL)
	}

	switch Via(c.Via) {
	case ViaAuto, "":
		return bridge.InCluster(u.Hostname()), nil
	case ViaDirect:
		return false, nil
	case ViaBridge:
		return true, nil
	default:
		return false, fmt.Errorf("invalid --via %q: must be auto, direct or bridge", c.Via)
	}
}

// Transport returns an HTTP transport reaching rawURL as the flags ask, and a function that
// releases it. Through the bridge, that opens a tunnel, which the release function closes.
func (c *Connection) Transport(ctx context.Context, o Options, rawURL string, tlsConfig *tls.Config) (*http.Transport, func(), error) {
	viaBridge, err := c.UsesBridge(rawURL)
	if err != nil {
		return nil, nil, err
	}

	if !viaBridge {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.TLSClientConfig = tlsConfig
		return tr, func() {}, nil
	}

	kc, err := o.Kube()
	if err != nil {
		return nil, nil, fmt.Errorf("reaching %s through the bridge needs Kubernetes access: %w", rawURL, err)
	}

	namespace := c.Namespace
	if namespace == "" {
		namespace = bridge.DefaultNamespace
	}

	tun, err := bridge.Connect(ctx, kc, namespace)
	if err != nil {
		return nil, nil, hint(o, err)
	}
	o.Log.Debugf("reaching %s through the bridge in namespace %s", rawURL, namespace)

	return tun.Transport(tlsConfig), func() { _ = tun.Close() }, nil
}
