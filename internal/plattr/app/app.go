package app

import (
	"fmt"

	"github.com/adnoctem/gopskit/pkg/core"
	fs "github.com/adnoctem/gopskit/pkg/fsi"
	"github.com/adnoctem/gopskit/pkg/log"
	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/spf13/cobra"

	"github.com/adnoctem/gopskit/pkg/kube"
	"github.com/adnoctem/gopskit/pkg/stamp"
)

const (
	Name             = "plattr"
	DefaultNamespace = "kube-system"
)

// Opt is configuration option for the application State
type Opt func(a *State)

type CLIOpt func(a *State) *cobra.Command

// State is the implementation for the `plattr` command-line application state
type State struct {
	*core.API
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

	stamps := stamp.New()

	a := &State{
		API: &core.API{
			Name:  Name,
			Exec:  exec,
			Kube:  kc,
			Log:   lgr,
			Paths: platf,
			Stamp: stamps,
		}}

	// (re-)configure if the user wants to do so
	for _, o := range opts {
		o(a)
	}

	return a, nil
}
