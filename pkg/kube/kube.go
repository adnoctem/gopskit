package kube

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	fs "github.com/adnoctem/gopskit/pkg/fsi"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	DefaultNamespace = "default"
	DefaultLocalPort = "7150"
)

type Client struct {
	// ConfigPath is the configuration file path for which the current client(-set) was created
	ConfigPath string

	// Executor is the connection between local and remote IO streams
	Executor RemoteExecutor

	// PortForwarder is the implementation for the port-forward functionality
	PortForwarder PortForwarder

	// Namespace is the Kubernetes namespace the client is configured to access
	namespace string

	// Config is the rest.Config for which the client was built
	Config *rest.Config

	// Client is the embedded Kubernetes ClientSet
	Client *kubernetes.Clientset

	// flags are the Kubernetes-specific flags which will be injected into the CLI
	Flags *genericclioptions.ConfigFlags
}

// Opt represents a configuration option for the KubeClient
type Opt func(c *Client)

// NewClient instantiates a new KubeClient with a configured and embedded Clientset
func NewClient(opts ...Opt) (*Client, error) {
	var err error

	kc := &Client{
		Executor:      &DefaultRemoteExecutor{},
		PortForwarder: &DefaultPortForwarder{},
	}

	for _, opt := range opts {
		opt(kc)
	}

	// if WithConfigPath wasn't in the opts
	if kc.ConfigPath == "" {
		kc.ConfigPath, err = findKubeConfig()
		if err != nil {
			return nil, err
		}
	}

	if kc.namespace == "" {
		kc.namespace = DefaultNamespace
	}

	// keep Flags (used by consumers needing a genericclioptions.RESTClientGetter, e.g. the Helm
	// SDK) resolving against the same kubeconfig/namespace as Config/Client above, instead of
	// independently re-discovering its own defaults
	kc.Flags = genericclioptions.NewConfigFlags(true)
	kc.Flags.KubeConfig = &kc.ConfigPath
	kc.Flags.Namespace = &kc.namespace

	kc.Config, err = clientcmd.BuildConfigFromFlags("", kc.ConfigPath)
	if err != nil {
		return nil, err
	}

	kc.Client, err = kubernetes.NewForConfig(kc.Config)
	if err != nil {
		return nil, err
	}

	return kc, nil
}

// WithConfigPath configures the KubeClient with a predetermined path for the 'kubeconfig' file
// This avoids the usual searches done within findKubeConfig
func WithConfigPath(path string) func(c *Client) {
	return func(c *Client) {
		c.ConfigPath = path
	}
}

// WithNamespace configures the KubeClient with a custom default namespace
func WithNamespace(namespace string) func(c *Client) {
	return func(c *Client) {
		c.namespace = namespace
	}
}

// TODO(MVProwess): evaluate validation of the found file path
//
// findKubeConfig searches the filesystem for possible locations of a KubeConfig file, which is most commonly
// located at "$HOME/.kube/config". In addition to the aforementioned path we add "$HOME/.config/gopskit/kubeconfig" as
// another possible location for the configuration file
//
// The function returns the first path which exists and (for now) does zero checking if we're actually looking at a
// possibly valid file
func findKubeConfig() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	config, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	paths := make([]string, 0)
	paths = append(paths, filepath.Join(home, ".kube", "config"), filepath.Join(config, "gopskit", "kubeconfig"))

	for _, path := range paths {
		exists := fs.CheckIfExists(path)
		if exists {
			return path, nil
		} else {
			continue
		}
	}

	return "", errors.New("couldn't find Kubeconfig file at known paths: [" + strings.Join(paths, ", ") + "]")
}
