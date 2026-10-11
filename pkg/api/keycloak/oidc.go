package keycloak

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Nerzal/gocloak/v14"
)

const (
	protocolOIDC = "openid-connect"

	// attrPostLogoutRedirectURIs holds the allowed post-logout redirect URIs, separated by "##".
	// "+" allows the client's redirect URIs.
	attrPostLogoutRedirectURIs = "post.logout.redirect.uris"

	// attrPKCEMethod enforces PKCE with the given challenge method.
	attrPKCEMethod = "pkce.code.challenge.method"
)

// OIDCClientSpec describes an OpenID Connect client for an application that signs its users in
// through Keycloak with the authorization code flow, such as Vaultwarden.
type OIDCClientSpec struct {
	// ClientID is the client ID the application presents.
	ClientID string

	// Name is the display name shown on Keycloak's consent and account pages. Optional.
	Name string

	// RootURL is the application's base URL; relative redirect URIs resolve against it. Optional.
	RootURL string

	// RedirectURIs are where Keycloak may send users back to after signing in. Required.
	RedirectURIs []string

	// PostLogoutRedirectURIs are where Keycloak may send users after signing out. Empty allows
	// the RedirectURIs.
	PostLogoutRedirectURIs []string

	// WebOrigins are the CORS origins the application calls Keycloak from. Empty allows the
	// origins of the RedirectURIs.
	WebOrigins []string

	// Public makes a client without a secret, for applications that cannot keep one, such as
	// single-page apps. Public clients must use PKCE.
	Public bool
}

// OIDCClient is the outcome of ApplyOIDCClient: what an application needs to sign users in.
type OIDCClient struct {
	// ID is Keycloak's internal ID of the client, not its client ID.
	ID string

	// ClientID is the client ID the application presents.
	ClientID string

	// Secret is the client secret, empty for a public client.
	Secret string

	// Created reports whether the client was created rather than updated.
	Created bool
}

// ApplyOIDCClient creates the client spec describes in realm, or brings an existing client with
// the same client ID in line with it. Only the settings spec covers are changed, so adjustments
// made in Keycloak by hand survive; an existing client's secret is kept, never rotated.
func (kc *Client) ApplyOIDCClient(ctx context.Context, realm string, spec OIDCClientSpec) (*OIDCClient, error) {
	if spec.ClientID == "" {
		return nil, errors.New("an OIDC client needs a client ID")
	}
	if len(spec.RedirectURIs) == 0 {
		return nil, fmt.Errorf("OIDC client %s needs at least one redirect URI", spec.ClientID)
	}

	existing, err := kc.GetClient(ctx, realm, spec.ClientID)
	switch {
	case errors.Is(err, ErrClientNotFound):
		existing = nil
	case err != nil:
		return nil, err
	}

	result := &OIDCClient{ClientID: spec.ClientID}

	if existing == nil {
		id, err := kc.CreateClient(ctx, realm, *spec.apply(&gocloak.Client{}))
		if err != nil {
			return nil, fmt.Errorf("could not create client %s in realm %s: %w", spec.ClientID, realm, err)
		}
		result.ID, result.Created = id, true
	} else {
		if existing.Protocol != nil && *existing.Protocol != protocolOIDC {
			return nil, fmt.Errorf("client %s in realm %s uses protocol %s, not %s; refusing to convert it",
				spec.ClientID, realm, *existing.Protocol, protocolOIDC)
		}
		if existing.ID == nil {
			return nil, fmt.Errorf("client %s in realm %s has no ID", spec.ClientID, realm)
		}

		if err := kc.UpdateClient(ctx, realm, *spec.apply(existing)); err != nil {
			return nil, fmt.Errorf("could not update client %s in realm %s: %w", spec.ClientID, realm, err)
		}
		result.ID = *existing.ID
	}

	if spec.Public {
		return result, nil
	}

	result.Secret, err = kc.ClientSecret(ctx, realm, result.ID)
	if err != nil {
		return nil, fmt.Errorf("could not read the secret of client %s in realm %s: %w", spec.ClientID, realm, err)
	}

	return result, nil
}

// apply sets the settings the spec covers on c and returns it.
func (spec OIDCClientSpec) apply(c *gocloak.Client) *gocloak.Client {
	c.ClientID = gocloak.StringP(spec.ClientID)
	c.Protocol = gocloak.StringP(protocolOIDC)
	c.Enabled = gocloak.BoolP(true)
	c.PublicClient = gocloak.BoolP(spec.Public)

	// the authorization code flow only: no implicit flow, no passwords sent to the client, and
	// no service account the application didn't ask for
	c.StandardFlowEnabled = gocloak.BoolP(true)
	c.ImplicitFlowEnabled = gocloak.BoolP(false)
	c.DirectAccessGrantsEnabled = gocloak.BoolP(false)
	c.ServiceAccountsEnabled = gocloak.BoolP(false)

	if !spec.Public {
		c.ClientAuthenticatorType = gocloak.StringP("client-secret")
	}

	if spec.Name != "" {
		c.Name = gocloak.StringP(spec.Name)
	}
	if spec.RootURL != "" {
		c.RootURL = gocloak.StringP(spec.RootURL)
	}

	c.RedirectURIs = spec.RedirectURIs
	c.WebOrigins = spec.WebOrigins
	if len(c.WebOrigins) == 0 {
		c.WebOrigins = []string{"+"}
	}

	attrs := c.Attributes
	if attrs == nil {
		attrs = map[string]string{}
	}

	attrs[attrPostLogoutRedirectURIs] = "+"
	if len(spec.PostLogoutRedirectURIs) > 0 {
		attrs[attrPostLogoutRedirectURIs] = strings.Join(spec.PostLogoutRedirectURIs, "##")
	}

	// a public client has no secret to prove it is the one that started the sign-in, so PKCE
	// is what keeps an intercepted authorization code useless
	if spec.Public {
		attrs[attrPKCEMethod] = "S256"
	}
	c.Attributes = attrs

	return c
}
