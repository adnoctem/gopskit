package cmd

import (
	"fmt"

	"github.com/Nerzal/gocloak/v14"
	"github.com/fmjstudios/gopskit/internal/ssolo/app"
	"github.com/spf13/cobra"
)

var _ app.CLIOpt = NewGitLabCommand

func NewGitLabCommand(ssolo *app.State) *cobra.Command {
	var (
		username          string
		password          string
		realm             string
		reflectNamespaces []string
	)

	cmd := &cobra.Command{
		Use:              "gitlab",
		Short:            "Configure GitLab for SAML authentication with Keycloak",
		Long:             "Configure GitLab for SAML authentication with Keycloak as the IdP",
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if password == "" {
				return fmt.Errorf("can't login to Keycloak without password")
			}

			ssolo.Keycloak.SetUser(username)
			ssolo.Keycloak.SetPassword(password)
			ssolo.Keycloak.SetRealm(realm)

			if err := ssolo.Keycloak.Login(); err != nil {
				return err
			}

			const operationsRealm = "operations"
			exists, err := ssolo.Keycloak.RealmExists(operationsRealm)
			if err != nil {
				return err
			}

			if !exists {
				if err := ssolo.Keycloak.CreateRealm(&gocloak.RealmRepresentation{
					Realm:   gocloak.StringP(operationsRealm),
					Enabled: gocloak.BoolP(true),
				}); err != nil {
					return err
				}

				ssolo.Log.Infof("successfully created Keycloak realm: %s", operationsRealm)
			} else {
				ssolo.Log.Infof("skipping creation of Keycloak realm: %s. Realm already exists.", operationsRealm)
			}

			// TODO(FMJdev): register GitLab as a SAML client (protocol mappers, IdP-initiated SSO,
			// GitLab-specific SAML attributes) and provision the Ingress-Nginx Diffie-Hellman
			// parameter Secret this command used to stub out. Neither was ever actually
			// implemented, even before this client rewrite - both are net-new feature work.
			return nil
		},
	}

	cmd.PersistentFlags().StringVarP(&username, "username", "u", "admin", "The username for the management account within Keycloak")
	cmd.PersistentFlags().StringVarP(&password, "password", "p", "", "The password for the management account within Keycloak")
	cmd.PersistentFlags().StringVarP(&realm, "realm", "r", "master", "The realm for to log into within Keycloak")
	cmd.PersistentFlags().StringArrayVar(&reflectNamespaces, "reflect-namespaces", []string{
		"kube-system",
		"ingress-nginx"},
		"Namespaces to enable for reflection")

	return cmd
}
