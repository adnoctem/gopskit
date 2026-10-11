package cmd

import (
	"fmt"

	"github.com/Nerzal/gocloak/v14"
	"github.com/adnoctem/gopskit/internal/ssolo/app"
	"github.com/adnoctem/gopskit/pkg/api/keycloak"
	"github.com/spf13/cobra"
)

var _ app.CLIOpt = NewInitCommand // assure type compatibility

// NewInitCommand creates the Option which injects the 'init' subcommand into
// the 'ssolo' CLI application
func NewInitCommand(ssolo *app.State) *cobra.Command {
	var (
		kf             keycloakFlags
		groups, realms []string
	)

	cmd := &cobra.Command{
		Use:     "initialize",
		Short:   "Initialize Keycloak",
		Aliases: []string{"init"},
		Long: `Create the realms, and the groups within each of them, that the cluster's applications sign
their users in with. Safe to run repeatedly: what already exists is left as it is.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			release, err := kf.connect(ctx, ssolo)
			if err != nil {
				return err
			}
			defer release()

			kc := ssolo.Keycloak
			for _, realm := range realms {
				exists, err := kc.RealmExists(ctx, realm)
				if err != nil {
					return err
				}

				if exists {
					ssolo.Log.Infof("realm %s already exists", realm)
				} else {
					err := kc.CreateRealm(ctx, gocloak.RealmRepresentation{
						Enabled:     gocloak.BoolP(true),
						Realm:       gocloak.StringP(realm),
						DisplayName: gocloak.StringP(keycloak.CreateDisplayName(realm)),
					})
					if err != nil {
						return fmt.Errorf("could not create realm %s: %w", realm, err)
					}
					ssolo.Log.Infof("created realm %s", realm)
				}

				for _, group := range groups {
					exists, err := kc.GroupExists(ctx, realm, group)
					if err != nil {
						return err
					}

					if exists {
						ssolo.Log.Infof("group %s already exists in realm %s", group, realm)
						continue
					}

					if err := kc.CreateGroup(ctx, realm, gocloak.Group{Name: gocloak.StringP(group)}); err != nil {
						return fmt.Errorf("could not create group %s in realm %s: %w", group, realm, err)
					}
					ssolo.Log.Infof("created group %s in realm %s", group, realm)
				}
			}

			ssolo.Log.Info("Keycloak is initialized")
			return nil
		},
	}

	kf.add(cmd)
	cmd.Flags().StringSliceVar(&realms, "realms", []string{"operations", "applications"}, "The realms to create")
	cmd.Flags().StringSliceVar(&groups, "groups", []string{"admins", "users"}, "The groups to create in each realm")

	return cmd
}
