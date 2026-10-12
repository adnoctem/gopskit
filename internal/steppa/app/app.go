package app

import (
	"github.com/adnoctem/gopskit/pkg/core"
	fs "github.com/adnoctem/gopskit/pkg/fsi"
	"github.com/adnoctem/gopskit/pkg/log"
	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/adnoctem/gopskit/pkg/stamp"
	"github.com/spf13/cobra"
)

const (
	Name string = "steppa"
)

// CLIOpt adds a subcommand to the 'steppa' CLI
type CLIOpt func(a *State) *cobra.Command

// State is the implementation for the `steppa` command-line application state
type State struct {
	*core.API
}

// New creates a newly initialized instance of the State type
func New() (*State, error) {
	platf, err := fs.Paths(fs.WithAppName(Name))
	if err != nil {
		return nil, err
	}

	exec, err := proc.NewExecutor(proc.WithInheritedEnv())
	if err != nil {
		return nil, err
	}

	return &State{
		API: &core.API{
			Name: Name,
			Exec: exec,
			// standard output can carry the chart values, so diagnostics go to standard error
			Log:   log.New(log.WithOutputPaths("stderr")),
			Paths: platf,
			Stamp: stamp.New(),
		},
	}, nil
}
