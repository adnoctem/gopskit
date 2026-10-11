package keycloak

import (
	"context"

	"github.com/Nerzal/gocloak/v14"
)

// Realms lists every realm.
func (kc *Client) Realms(ctx context.Context) ([]*gocloak.RealmRepresentation, error) {
	return withAuth(kc, func(token string) ([]*gocloak.RealmRepresentation, error) {
		return kc.api.GetRealms(ctx, token)
	})
}

// Groups lists the groups in realm matching params.
func (kc *Client) Groups(ctx context.Context, realm string, params gocloak.GetGroupsParams) ([]*gocloak.Group, error) {
	return withAuth(kc, func(token string) ([]*gocloak.Group, error) {
		return kc.api.GetGroups(ctx, token, realm, params)
	})
}

// Clients lists the clients in realm matching params.
func (kc *Client) Clients(ctx context.Context, realm string, params gocloak.GetClientsParams) ([]*gocloak.Client, error) {
	return withAuth(kc, func(token string) ([]*gocloak.Client, error) {
		return kc.api.GetClients(ctx, token, realm, params)
	})
}

// GetClient returns the client with the given client ID in realm, or ErrClientNotFound.
func (kc *Client) GetClient(ctx context.Context, realm, clientID string) (*gocloak.Client, error) {
	// Search=false makes clientId an exact match rather than a substring search
	clients, err := kc.Clients(ctx, realm, gocloak.GetClientsParams{
		ClientID: gocloak.StringP(clientID),
		Search:   gocloak.BoolP(false),
	})
	if err != nil {
		return nil, err
	}

	for _, c := range clients {
		if c.ClientID != nil && *c.ClientID == clientID {
			return c, nil
		}
	}

	return nil, ErrClientNotFound
}

// ClientSecret returns the current secret of the client with the given internal ID in realm.
func (kc *Client) ClientSecret(ctx context.Context, realm, id string) (string, error) {
	cred, err := withAuth(kc, func(token string) (*gocloak.CredentialRepresentation, error) {
		return kc.api.GetClientSecret(ctx, token, realm, id)
	})
	if err != nil {
		return "", err
	}

	if cred == nil || cred.Value == nil {
		return "", ErrNoClientSecret
	}

	return *cred.Value, nil
}
