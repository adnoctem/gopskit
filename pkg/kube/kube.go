package kube

import (
	"fmt"
	"os"
	"path/filepath"

	fs "github.com/adnoctem/gopskit/pkg/fsi"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	DefaultNamespace = "default"
)

type Client struct {
	// ConfigPath is an explicitly chosen kubeconfig file; empty means it's found as kubectl finds it
	ConfigPath string

	// Executor is the connection between local and remote IO streams
	Executor RemoteExecutor

	// Namespace is the Kubernetes namespace the client is configured to access
	namespace string

	// context is the kubeconfig context to use instead of the current one
	context string

	// Config is the rest.Config for which the client was built
	Config *rest.Config

	// Client is the embedded Kubernetes ClientSet, typed as the kubernetes.Interface it
	// satisfies (rather than the concrete *kubernetes.Clientset) so tests can substitute
	// k8s.io/client-go/kubernetes/fake.NewSimpleClientset without a live cluster
	Client kubernetes.Interface

	// Dynamic is the dynamic (unstructured) client Apply uses, typed as dynamic.Interface for
	// the same reason as Client - so tests can substitute k8s.io/client-go/dynamic/fake
	Dynamic dynamic.Interface

	// flags are the Kubernetes-specific flags which will be injected into the CLI
	Flags *genericclioptions.ConfigFlags
}

// Opt represents a configuration option for the KubeClient
type Opt func(c *Client)

// NewClient instantiates a new KubeClient with a configured and embedded Clientset.
//
// The kubeconfig is found the way kubectl finds it: the file WithConfigPath names, or else the
// files in $KUBECONFIG, or else ~/.kube/config - falling back to $UserConfigDir/gopskit/kubeconfig
// and finally to the in-cluster configuration when running in a Pod. WithContext selects a context
// other than the current one.
func NewClient(opts ...Opt) (*Client, error) {
	var err error

	kc := &Client{
		Executor: &DefaultRemoteExecutor{},
	}

	for _, opt := range opts {
		opt(kc)
	}

	if kc.namespace == "" {
		kc.namespace = DefaultNamespace
	}

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	switch {
	case kc.ConfigPath != "":
		rules.ExplicitPath = kc.ConfigPath
	case os.Getenv(clientcmd.RecommendedConfigPathEnvVar) == "":
		// client-go fixes ~/.kube/config when the process starts; resolve it now instead
		if home, err := os.UserHomeDir(); err == nil {
			homeFile := filepath.Join(home, clientcmd.RecommendedHomeDir, clientcmd.RecommendedFileName)
			rules.Precedence = []string{homeFile}

			if fallback := gopskitKubeConfig(); fallback != "" && !fs.CheckIfExists(homeFile) {
				rules.ExplicitPath = fallback
			}
		}
	}

	overrides := &clientcmd.ConfigOverrides{CurrentContext: kc.context}
	kc.Config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("could not load a kubeconfig (set $KUBECONFIG or --kubeconfig): %w", err)
	}

	// keep Flags (used by consumers needing a genericclioptions.RESTClientGetter, e.g. the Helm
	// SDK) resolving against the same kubeconfig, context and namespace as Config/Client above,
	// instead of independently re-discovering its own defaults
	kc.Flags = genericclioptions.NewConfigFlags(true)
	if rules.ExplicitPath != "" {
		kc.Flags.KubeConfig = &rules.ExplicitPath
	}
	if kc.context != "" {
		kc.Flags.Context = &kc.context
	}
	kc.Flags.Namespace = &kc.namespace

	kc.Client, err = kubernetes.NewForConfig(kc.Config)
	if err != nil {
		return nil, err
	}

	kc.Dynamic, err = dynamic.NewForConfig(kc.Config)
	if err != nil {
		return nil, err
	}

	return kc, nil
}

// WithConfigPath configures the KubeClient with a predetermined path for the 'kubeconfig' file,
// which takes precedence over $KUBECONFIG and the default locations
func WithConfigPath(path string) func(c *Client) {
	return func(c *Client) {
		c.ConfigPath = path
	}
}

// WithContext selects a kubeconfig context other than the current one
func WithContext(context string) func(c *Client) {
	return func(c *Client) {
		c.context = context
	}
}

// WithNamespace configures the KubeClient with a custom default namespace
func WithNamespace(namespace string) func(c *Client) {
	return func(c *Client) {
		c.namespace = namespace
	}
}

// gopskitKubeConfig is the gopskit-specific kubeconfig location, $UserConfigDir/gopskit/kubeconfig,
// or "" if it doesn't exist.
func gopskitKubeConfig() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}

	path := filepath.Join(dir, "gopskit", "kubeconfig")
	if !fs.CheckIfExists(path) {
		return ""
	}

	return path
}
