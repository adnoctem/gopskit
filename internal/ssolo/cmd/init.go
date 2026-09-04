package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Nerzal/gocloak/v13"
	"github.com/fmjstudios/gopskit/internal/ssolo/app"
	"github.com/fmjstudios/gopskit/pkg/api/keycloak"
	"github.com/fmjstudios/gopskit/pkg/kube/kubeutil"
	"github.com/fmjstudios/gopskit/pkg/proc"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ app.CLIOpt = NewInitCommand // assure type compatibility

// NewInitCommand creates the Option which injects the 'init' subcommand into
// the 'ssolo' CLI application
func NewInitCommand(app *app.State) *cobra.Command {
	var (
		username, password, secretName string
		groups, realms                 []string
	)

	cmd := &cobra.Command{
		Use:              "initialize",
		Short:            "Initialize Keycloak",
		Aliases:          []string{"init"},
		Long:             "Initialize Keycloak using a Bootstrap admin user",
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			label := proc.Must(cmd.Flags().GetString("label"))
			namespace := proc.Must(cmd.Flags().GetString("namespace"))

			var basicAuth, secretAuth bool
			var podNamespace string
			var leaderPod *corev1.Pod

			// handle flags
			switch {
			case secretName != "":
				secretAuth = true
			case username != "" && password != "":
				basicAuth = true
			default:
				return fmt.Errorf("impossible to authenticate to Keycloak without providing username/password or a Secret")
			}

			// look across the entire cluster
			pods, err := app.Kube.Pods("", metav1.ListOptions{LabelSelector: label})
			if err != nil {
				return fmt.Errorf("could not retrieve Keycloak Pods for labelSelector: %s. Error: %v", label, err)
			}

			// ensure we're using a single namespace, or it's passed in
			podNamespace, err = kubeutil.EnsureNamespace(pods)
			if err != nil && namespace == "" {
				return fmt.Errorf("found multiple possible Keycloak pods. the namespace option is unset and %v", err)
			}

			if secretAuth {
				secret, err := app.Kube.Secret(namespace, secretName, metav1.GetOptions{})
				if err != nil {
					return err
				}

				result, err := kubeutil.ParseSecretKeys(secret, "username", "password")
				if err != nil {
					return err
				}

				username = result["username"]
				password = result["password"]
			}

			if !basicAuth && !secretAuth {
				return errors.New("exhausted Keycloak authentication options, cannot configure")
			}

			// wait until the pod is running
			// TODO(FMJdev): make the latter label configurable
			leaderPod, err = kubeutil.LeaderPod(app.Kube, podNamespace, label, "apps.kubernetes.io/pod-index=0")
			if err != nil {
				return err
			}
			elapsed := kubeutil.WaitUntilRunning(leaderPod)
			app.Log.Infof("Keycloak Pod took %d ms to become running", elapsed.Milliseconds()/1000)

			// port-forward the (leader)
			app.Log.Infof("Port-forwarding Keycloak leader pod: %s", leaderPod.Name)
			ctx, cancel := context.WithCancel(context.Background())
			readyChan := make(chan struct{})
			go func(rc chan struct{}) {
				err := app.Kube.PortForward(ctx, *leaderPod, rc)
				if err != nil {
					panic(err)
				}
			}(readyChan)

			app.Log.Info("Waiting for port-forwarded Keycloak API to become available...")
			<-readyChan

			// authenticate to the API as soon as it's locally available
			app.Keycloak.SetUser(username)
			app.Keycloak.SetPassword(password)
			app.Keycloak.SetRealm("master")

			// obtain & save the access token
			err = app.Keycloak.Login()
			if err != nil {
				cancel()
				return err
			}

			// create Keycloak groups
			var createGroups []gocloak.Group
			for _, v := range groups {
				createGroups = append(createGroups, gocloak.Group{
					Name: gocloak.StringP(v),
				})
			}

			for _, v := range createGroups {
				exists, err := app.Keycloak.GroupExists(*v.Name)
				if err != nil {
					cancel()
					return err
				}

				if exists {
					app.Log.Infof("skipping creation of Keycloak group: %s. Group already exists.", *v.Name)
					continue
				}

				err = app.Keycloak.CreateGroup(v)
				if err != nil {
					cancel()
					return fmt.Errorf("could not create Keycloak group: %s. Error: %v", *v.Name, err)
				}
			}
			app.Log.Infof("successfully created Keycloak groups: [%s]", strings.Join(groups, ", "))

			// create Keycloak realms
			var createRealms []*gocloak.RealmRepresentation
			for _, v := range realms {
				createRealms = append(createRealms, &gocloak.RealmRepresentation{
					Enabled:     gocloak.BoolP(true),
					Realm:       &v,
					DisplayName: gocloak.StringP(keycloak.CreateDisplayName(v)),
				})
			}

			for _, v := range createRealms {
				exists, err := app.Keycloak.RealmExists(*v.Realm)
				if err != nil {
					cancel()
					return err
				}

				if exists {
					app.Log.Infof("skipping creation of Keycloak realm: %s. Realm already exists.", *v.Realm)
					continue
				}

				err = app.Keycloak.CreateRealm(v)
				if err != nil {
					cancel()
					return fmt.Errorf("could not create Keycloak realm: %s. Error: %v", *v.Realm, err)
				}
			}
			app.Log.Infof("successfully created Keycloak realms: [%s]", strings.Join(realms, ", "))

			// Finished
			app.Log.Info("successfully initialized Keycloak!")
			cancel()
			return nil
		},
	}

	cmd.PersistentFlags().StringVar(&username, "username", "", "The name of the bootstrap admin user")
	cmd.PersistentFlags().StringVar(&password, "password", "", "The password for the bootstrap admin user")
	cmd.PersistentFlags().StringVar(&secretName, "secret-name", "", "The name of a Kubernetes Secret containing a username and password key")
	cmd.PersistentFlags().StringArrayVar(&realms, "realms", []string{"operations", "applications"}, "The Keycloak realms to create")
	cmd.PersistentFlags().StringArrayVar(&groups, "groups", []string{"admins", "users"}, "The Keycloak groups to create")

	return cmd
}
