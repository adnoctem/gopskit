package app

import (
	"fmt"
	"path/filepath"

	"github.com/fmjstudios/gopskit/pkg/api/keycloak"
	"github.com/fmjstudios/gopskit/pkg/core"
	fs "github.com/fmjstudios/gopskit/pkg/fsi"
	"github.com/fmjstudios/gopskit/pkg/kube"
	"github.com/fmjstudios/gopskit/pkg/log"
	"github.com/fmjstudios/gopskit/pkg/proc"
	"github.com/fmjstudios/gopskit/pkg/stamp"
	"github.com/spf13/cobra"
)

const (
	Name             string = "ssolo"
	DefaultLabel     string = "app=keycloak,app.kubernetes.io/instance=keycloak"
	DefaultNamespace string = "keycloak"
	DefaultRealmName string = "Operations"
)

var (
	DefaultHostname string = fmt.Sprintf("https://127.0.0.1:%s", kube.DefaultLocalPort)
)

// Opt is configuration option for the application State
type Opt func(a *State)

type CLIOpt func(a *State) *cobra.Command

// State is the implementation for the `ssolo` command-line application state
type State struct {
	*core.API

	// KeycloakClient is the Keycloak API client used to manage its' resources
	// Keycloak *gocloak.GoCloak
	Keycloak *keycloak.Client
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

	kec := keycloak.New(
		DefaultHostname,
		keycloak.WithAuthPath(getAuthPath(platf.Cache)),
		keycloak.WithInsecureTLS(true), // TODO(FMJdev): remove this to improve security
		keycloak.WithRealm("master"),
	)

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
		},
		Keycloak: kec,
	}

	// (re-)configure if the user wants to do so
	for _, o := range opts {
		o(a)
	}

	return a, nil
}

func getAuthPath(baseDir string) string {
	return filepath.Join(baseDir, "creds", "keycloak-credentials.json")
}
