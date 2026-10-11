package app

import (
	"path/filepath"

	"github.com/adnoctem/gopskit/pkg/api/keycloak"
	"github.com/adnoctem/gopskit/pkg/core"
	fs "github.com/adnoctem/gopskit/pkg/fsi"
	"github.com/adnoctem/gopskit/pkg/log"
	"github.com/adnoctem/gopskit/pkg/proc"
	"github.com/adnoctem/gopskit/pkg/stamp"
	"github.com/spf13/cobra"
)

const (
	Name string = "ssolo"
)

// Opt is configuration option for the application State
type Opt func(a *State)

type CLIOpt func(a *State) *cobra.Command

// State is the implementation for the `ssolo` command-line application state
type State struct {
	*core.API

	// Keycloak is the Keycloak API client used to manage its resources. Commands that talk to
	// Keycloak set it up when they connect, since where Keycloak is and how it's reached are
	// command-line flags.
	Keycloak *keycloak.Client
}

// New creates a newly initialized instance of the State type
func New(opts ...Opt) (*State, error) {
	platf, err := fs.Paths(fs.WithAppName(Name))
	if err != nil {
		return nil, err
	}

	// standard output carries data - e.g. the credentials `client apply --output stdout` prints -
	// so diagnostics go to standard error
	lgr := log.New(log.WithOutputPaths("stderr"))

	exec, err := proc.NewExecutor(proc.WithInheritedEnv())
	if err != nil {
		return nil, err
	}

	a := &State{
		API: &core.API{
			Name:  Name,
			Exec:  exec,
			Log:   lgr,
			Paths: platf,
			Stamp: stamp.New(),
		},
	}

	// (re-)configure if the user wants to do so
	for _, o := range opts {
		o(a)
	}

	return a, nil
}

// AuthPath is where the Keycloak access and refresh tokens are persisted between invocations.
func (a *State) AuthPath() string {
	return getAuthPath(a.Paths.Cache)
}

func getAuthPath(baseDir string) string {
	return filepath.Join(baseDir, "creds", "keycloak-credentials.json")
}
