package core

import (
	"fmt"
	"sync"

	"github.com/spf13/cobra"

	fs "github.com/adnoctem/gopskit/pkg/fsi"
	"github.com/adnoctem/gopskit/pkg/kube"
	"github.com/adnoctem/gopskit/pkg/log"
	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/adnoctem/gopskit/pkg/stamp"
)

// API is the common central application type embedded by most, if not
// all gopskit applications. It merely saves us some writing for commonly
// required API's
type API struct {
	// Name is the name of the application this object is instanced for
	Name string

	// Exec represents a command-line executor to quickly new start processes
	// and closely evaluate their output as well as potential errors
	Exec *proc.Executor

	// Kube is a Kubernetes Client capable of performing all Kubernetes
	// operations, initialized from the same sources as `kubectl`. Applications that create it
	// lazily populate it through KubeClient before a command runs.
	Kube *kube.Client

	// Platform is a utility object performing OS-specific lookups and other
	// data gathering tasks
	Paths *fs.PlatformPaths

	// Log is a wrapper object around Uber's `zap` logger
	Log *log.Logger

	// KV is the embedded BadgerDB database wrapper which we us to persist
	// Vault tokens, unseal keys and recovery keys, e.g. after Vault initialization.
	// These values may or may not be made accessible via the CLIs via a subcommand.
	// KV *kv.Database

	// Stamp is build-time information that is linked into the final executable
	// by LD. Our Bazel builds stamps builds via LD using the 'x_defs' attribute
	// on the 'go_library' rule
	Stamp *stamp.Stamps

	// KubeConfig and KubeContext select the kubeconfig file and context KubeClient uses; empty
	// means kubectl's defaults ($KUBECONFIG, ~/.kube/config, the current context)
	KubeConfig  string
	KubeContext string

	kubeOnce sync.Once
	kubeErr  error
}

// AddKubeFlags registers --kubeconfig and --context on cmd, normally the root command, for
// KubeClient to use.
func (a *API) AddKubeFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&a.KubeConfig, "kubeconfig", "",
		"The kubeconfig file to use (default: $KUBECONFIG, then ~/.kube/config)")
	cmd.PersistentFlags().StringVar(&a.KubeContext, "context", "",
		"The kubeconfig context to use (default: the current context)")
}

// KubeClient returns Kube, creating it on first use from the same sources as `kubectl`. Creating
// the client lazily lets commands that never touch the cluster - help output, or reaching a
// service directly - run without a kubeconfig.
func (a *API) KubeClient() (*kube.Client, error) {
	a.kubeOnce.Do(func() {
		if a.Kube != nil {
			return
		}

		var opts []kube.Opt
		if a.KubeConfig != "" {
			opts = append(opts, kube.WithConfigPath(a.KubeConfig))
		}
		if a.KubeContext != "" {
			opts = append(opts, kube.WithContext(a.KubeContext))
		}

		a.Kube, a.kubeErr = kube.NewClient(opts...)
		if a.kubeErr != nil {
			a.kubeErr = fmt.Errorf("could not create kubernetes client: %w", a.kubeErr)
		}
	})

	return a.Kube, a.kubeErr
}
