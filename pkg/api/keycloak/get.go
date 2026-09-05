package keycloak

import (
	"context"

	"github.com/Nerzal/gocloak/v13"
)

func (kc *Client) Realms() ([]*gocloak.RealmRepresentation, error) {
	r, err := kc.api.GetRealms(context.Background(), kc.auth.JWT.AccessToken)
	if err != nil {
		if handled := kc.handleAuthorizationError(err); handled != nil {
			return nil, handled
		}

		return kc.api.GetRealms(context.Background(), kc.auth.JWT.AccessToken)
	}

	return r, nil
}

func (kc *Client) Groups(params gocloak.GetGroupsParams) ([]*gocloak.Group, error) {
	g, err := kc.api.GetGroups(context.Background(), kc.auth.JWT.AccessToken, kc.realm, params)
	if err != nil {
		if handled := kc.handleAuthorizationError(err); handled != nil {
			return nil, handled
		}

		return kc.api.GetGroups(context.Background(), kc.auth.JWT.AccessToken, kc.realm, params)
	}

	return g, nil
}

func (kc *Client) Clients(params gocloak.GetClientsParams) ([]*gocloak.Client, error) {
	c, err := kc.api.GetClients(context.Background(), kc.auth.JWT.AccessToken, kc.realm, params)
	if err != nil {
		if handled := kc.handleAuthorizationError(err); handled != nil {
			return nil, handled
		}

		return kc.api.GetClients(context.Background(), kc.auth.JWT.AccessToken, kc.realm, params)
	}

	return c, nil
}
