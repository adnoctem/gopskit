package keycloak

import (
	"context"

	"github.com/Nerzal/gocloak/v14"
)

// CreateRealm creates a realm.
func (kc *Client) CreateRealm(ctx context.Context, realm gocloak.RealmRepresentation) error {
	_, err := withAuth(kc, func(token string) (string, error) {
		return kc.api.CreateRealm(ctx, token, realm)
	})

	return err
}

// CreateGroup creates a top-level group in realm.
func (kc *Client) CreateGroup(ctx context.Context, realm string, group gocloak.Group) error {
	_, err := withAuth(kc, func(token string) (string, error) {
		return kc.api.CreateGroup(ctx, token, realm, group)
	})

	return err
}

// CreateClient creates a client in realm and returns its internal ID - not its client ID.
func (kc *Client) CreateClient(ctx context.Context, realm string, client gocloak.Client) (string, error) {
	return withAuth(kc, func(token string) (string, error) {
		return kc.api.CreateClient(ctx, token, realm, client)
	})
}

// UpdateClient replaces the client identified by client.ID in realm.
func (kc *Client) UpdateClient(ctx context.Context, realm string, client gocloak.Client) error {
	_, err := withAuth(kc, func(token string) (struct{}, error) {
		return struct{}{}, kc.api.UpdateClient(ctx, token, realm, client)
	})

	return err
}
