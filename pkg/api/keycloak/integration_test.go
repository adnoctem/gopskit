package keycloak_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Nerzal/gocloak/v14"
	"github.com/adnoctem/gopskit/pkg/api/keycloak"
	"github.com/adnoctem/gopskit/pkg/api/keycloak/keycloaktest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAgainstKeycloak runs the client against a real Keycloak server. The subtests share one
// server, since starting Keycloak dominates the runtime; each uses its own realm.
func TestAgainstKeycloak(t *testing.T) {
	url := keycloaktest.Start(t)
	ctx := t.Context()

	newClient := func(t *testing.T) *keycloak.Client {
		t.Helper()

		kc := keycloak.New(url,
			keycloak.WithAuthPath(filepath.Join(t.TempDir(), "creds.json")),
			keycloak.WithUsername(keycloaktest.AdminUser),
			keycloak.WithPassword(keycloaktest.AdminPassword),
		)
		require.NoError(t, kc.Login())

		return kc
	}

	t.Run("refuses to work before logging in", func(t *testing.T) {
		kc := keycloak.New(url)
		_, err := kc.Realms(ctx)
		assert.ErrorIs(t, err, keycloak.ErrNotLoggedIn)
	})

	t.Run("refreshes its token", func(t *testing.T) {
		kc := newClient(t)

		// Refresh loads the persisted token before using it; it used to wipe the login
		// settings while doing so and fail with ErrAdminAuthUnset every time
		require.NoError(t, kc.Refresh())
		assert.True(t, kc.Valid())

		_, err := kc.Realms(ctx)
		assert.NoError(t, err)
	})

	t.Run("creates realms and groups in them", func(t *testing.T) {
		asrt := assert.New(t)
		kc := newClient(t)

		exists, err := kc.RealmExists(ctx, "groups-test")
		require.NoError(t, err)
		asrt.False(exists)

		require.NoError(t, kc.CreateRealm(ctx, gocloak.RealmRepresentation{
			Realm:   gocloak.StringP("groups-test"),
			Enabled: gocloak.BoolP(true),
		}))
		exists, err = kc.RealmExists(ctx, "groups-test")
		require.NoError(t, err)
		asrt.True(exists)

		require.NoError(t, kc.CreateGroup(ctx, "groups-test", gocloak.Group{Name: gocloak.StringP("admins")}))

		inRealm, err := kc.GroupExists(ctx, "groups-test", "admins")
		require.NoError(t, err)
		asrt.True(inRealm)

		// the client logs in to master; a group meant for another realm must not land there
		inMaster, err := kc.GroupExists(ctx, keycloak.DefaultRealm, "admins")
		require.NoError(t, err)
		asrt.False(inMaster)
	})

	t.Run("applies a confidential OIDC client idempotently", func(t *testing.T) {
		asrt := assert.New(t)
		kc := newClient(t)
		realm := createRealm(t, kc, "oidc-confidential")

		spec := keycloak.OIDCClientSpec{
			ClientID:     "vaultwarden",
			Name:         "Vaultwarden",
			RootURL:      "https://vault.example.org",
			RedirectURIs: []string{"https://vault.example.org/identity/connect/oidc-signin"},
		}

		first, err := kc.ApplyOIDCClient(ctx, realm, spec)
		require.NoError(t, err)
		asrt.True(first.Created)
		asrt.NotEmpty(first.Secret)

		c, err := kc.GetClient(ctx, realm, "vaultwarden")
		require.NoError(t, err)
		asrt.Equal("openid-connect", *c.Protocol)
		asrt.False(*c.PublicClient)
		asrt.True(*c.StandardFlowEnabled)
		asrt.False(*c.ImplicitFlowEnabled)
		asrt.False(*c.DirectAccessGrantsEnabled)
		asrt.Equal(spec.RedirectURIs, c.RedirectURIs)
		asrt.Equal([]string{"+"}, c.WebOrigins)
		asrt.Equal("+", c.Attributes["post.logout.redirect.uris"])

		// a setting changed by hand in Keycloak, which a later apply must leave alone
		c.Attributes["access.token.lifespan"] = "600"
		require.NoError(t, kc.UpdateClient(ctx, realm, *c))

		spec.RedirectURIs = []string{"https://vw.example.org/identity/connect/oidc-signin"}
		second, err := kc.ApplyOIDCClient(ctx, realm, spec)
		require.NoError(t, err)
		asrt.False(second.Created)
		asrt.Equal(first.ID, second.ID)
		asrt.Equal(first.Secret, second.Secret, "applying again must never rotate the secret")

		c, err = kc.GetClient(ctx, realm, "vaultwarden")
		require.NoError(t, err)
		asrt.Equal(spec.RedirectURIs, c.RedirectURIs)
		asrt.Equal("600", c.Attributes["access.token.lifespan"])

		clients, err := kc.Clients(ctx, realm, gocloak.GetClientsParams{ClientID: gocloak.StringP("vaultwarden")})
		require.NoError(t, err)
		asrt.Len(clients, 1)
	})

	t.Run("applies a public OIDC client with PKCE", func(t *testing.T) {
		asrt := assert.New(t)
		kc := newClient(t)
		realm := createRealm(t, kc, "oidc-public")

		res, err := kc.ApplyOIDCClient(ctx, realm, keycloak.OIDCClientSpec{
			ClientID:               "spa",
			RedirectURIs:           []string{"https://app.example.org/*"},
			PostLogoutRedirectURIs: []string{"https://app.example.org/", "https://app.example.org/bye"},
			Public:                 true,
		})
		require.NoError(t, err)
		asrt.Empty(res.Secret)

		c, err := kc.GetClient(ctx, realm, "spa")
		require.NoError(t, err)
		asrt.True(*c.PublicClient)
		asrt.Equal("S256", c.Attributes["pkce.code.challenge.method"])
		asrt.Equal("https://app.example.org/##https://app.example.org/bye", c.Attributes["post.logout.redirect.uris"])
	})

	t.Run("refuses to convert a non-OIDC client", func(t *testing.T) {
		kc := newClient(t)
		realm := createRealm(t, kc, "oidc-saml")

		_, err := kc.CreateClient(ctx, realm, gocloak.Client{
			ClientID: gocloak.StringP("gitlab"),
			Protocol: gocloak.StringP("saml"),
		})
		require.NoError(t, err)

		_, err = kc.ApplyOIDCClient(ctx, realm, keycloak.OIDCClientSpec{
			ClientID:     "gitlab",
			RedirectURIs: []string{"https://gitlab.example.org/*"},
		})
		assert.ErrorContains(t, err, "refusing to convert")
	})

	t.Run("reports a missing client", func(t *testing.T) {
		kc := newClient(t)

		_, err := kc.GetClient(ctx, keycloak.DefaultRealm, "does-not-exist")
		assert.ErrorIs(t, err, keycloak.ErrClientNotFound)

		exists, err := kc.ClientExists(ctx, keycloak.DefaultRealm, "admin-cli")
		require.NoError(t, err)
		assert.True(t, exists)
	})
}

func createRealm(t *testing.T, kc *keycloak.Client, name string) string {
	t.Helper()

	require.NoError(t, kc.CreateRealm(context.Background(), gocloak.RealmRepresentation{
		Realm:   gocloak.StringP(name),
		Enabled: gocloak.BoolP(true),
	}))

	return name
}
