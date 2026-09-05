package app

import (
	"fmt"
	"time"

	apivault "github.com/adnoctem/gopskit/pkg/api/vault"
	"github.com/adnoctem/gopskit/pkg/core"
	fs "github.com/adnoctem/gopskit/pkg/fsi"
	"github.com/adnoctem/gopskit/pkg/log"
	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/spf13/cobra"

	"github.com/adnoctem/gopskit/pkg/kube"
	"github.com/adnoctem/gopskit/pkg/stamp"
)

const (
	Name         = "waltr"
	DefaultLabel = "app.kubernetes.io/name=vault"
)

// Opt is configuration option for the application State
type Opt func(a *State)

type CLIOpt func(a *State) *cobra.Command

// State is the implementation for the `waltr` command-line application state
type State struct {
	*core.API

	// Vault is the gopskit Vault API client, which waltr uses for nearly all of its functionality
	Vault *apivault.Client
}

// New creates a newly initialized instance of the State type
func New(opts ...Opt) (*State, error) {
	var err error

	platf, err := fs.Paths(fs.WithAppName(Name))
	if err != nil {
		return nil, err
	}

	lgr := log.New()
	defer func() {
		err = lgr.Sync()
	}()
	if err != nil {
		return nil, err
	}

	exec, err := proc.NewExecutor(proc.WithInheritedEnv())
	if err != nil {
		return nil, err
	}

	kc, err := kube.NewClient()
	if err != nil {
		return nil, fmt.Errorf("could not create kubernetes client: %v", err)
	}

	// enforce HTTPS
	host := fmt.Sprintf("https://127.0.0.1:%s", kube.DefaultLocalPort)
	vc := apivault.New(host, apivault.WithTimeout(60*time.Second), apivault.WithInsecureTLS(true))
	stamps := stamp.New()

	a := &State{
		API: &core.API{
			Name:  Name,
			Exec:  exec,
			Kube:  kc,
			Log:   lgr,
			Paths: platf,
			Stamp: stamps,
		},
		Vault: vc,
	}

	// (re-)configure if the user wants to do so
	for _, o := range opts {
		o(a)
	}

	return a, nil
}

// WithVaultOpts (re-)configures waltr's Vault API client for the given host, applying any of the
// provided apivault.ClientOpts
func WithVaultOpts(host string, opts ...apivault.ClientOpt) Opt {
	return func(a *State) {
		a.Vault = apivault.New(host, opts...)
	}
}
