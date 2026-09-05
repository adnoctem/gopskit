package keycloak

import (
	"context"

	"github.com/Nerzal/gocloak/v14"
)

func (kc *Client) CreateRealm(realm *gocloak.RealmRepresentation) error {
	_, err := kc.api.CreateRealm(context.Background(), kc.auth.JWT.AccessToken, *realm)
	if err != nil {
		if handled := kc.handleAuthorizationError(err); handled != nil {
			return handled
		}

		_, err = kc.api.CreateRealm(context.Background(), kc.auth.JWT.AccessToken, *realm)
		return err
	}

	return nil
}

func (kc *Client) CreateGroup(group gocloak.Group) error {
	_, err := kc.api.CreateGroup(context.Background(), kc.auth.JWT.AccessToken, kc.realm, group)
	if err != nil {
		if handled := kc.handleAuthorizationError(err); handled != nil {
			return handled
		}

		_, err = kc.api.CreateGroup(context.Background(), kc.auth.JWT.AccessToken, kc.realm, group)
		return err
	}

	return nil
}

func (kc *Client) CreateClient(client gocloak.Client) error {
	_, err := kc.api.CreateClient(context.Background(), kc.auth.JWT.AccessToken, kc.realm, client)
	if err != nil {
		if handled := kc.handleAuthorizationError(err); handled != nil {
			return handled
		}

		_, err = kc.api.CreateClient(context.Background(), kc.auth.JWT.AccessToken, kc.realm, client)
		return err
	}

	return nil
}
