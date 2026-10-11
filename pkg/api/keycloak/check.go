package keycloak

import (
	"context"
	"errors"

	"github.com/Nerzal/gocloak/v14"
)

// RealmExists reports whether a realm named name exists.
func (kc *Client) RealmExists(ctx context.Context, name string) (bool, error) {
	realms, err := kc.Realms(ctx)
	if err != nil {
		return false, err
	}

	for _, v := range realms {
		if v.Realm != nil && *v.Realm == name {
			return true, nil
		}
	}

	return false, nil
}

// GroupExists reports whether a top-level group named name exists in realm.
func (kc *Client) GroupExists(ctx context.Context, realm, name string) (bool, error) {
	groups, err := kc.Groups(ctx, realm, gocloak.GetGroupsParams{
		Search: &name,
	})
	if err != nil {
		return false, err
	}

	for _, v := range groups {
		if v.Name != nil && *v.Name == name {
			return true, nil
		}
	}

	return false, nil
}

// ClientExists reports whether a client with the given client ID exists in realm.
func (kc *Client) ClientExists(ctx context.Context, realm, clientID string) (bool, error) {
	_, err := kc.GetClient(ctx, realm, clientID)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, ErrClientNotFound):
		return false, nil
	default:
		return false, err
	}
}
