package keycloak

import (
	"github.com/Nerzal/gocloak/v14"
)

func (kc *Client) RealmExists(name string) (bool, error) {
	realms, err := kc.Realms()
	if err != nil {
		return false, err
	}

	for _, v := range realms {
		if *v.Realm == name {
			return true, nil
		}
	}

	return false, nil
}

func (kc *Client) GroupExists(name string) (bool, error) {
	groups, err := kc.Groups(gocloak.GetGroupsParams{
		Search: &name,
	})
	if err != nil {
		return false, err
	}

	for _, v := range groups {
		if *v.Name == name {
			return true, nil
		}
	}

	return false, nil
}

func (kc *Client) ClientExists(clientId string) (bool, error) {
	clients, err := kc.Clients(gocloak.GetClientsParams{
		ClientID: &clientId,
		Search:   gocloak.BoolP(true),
	})
	if err != nil {
		return false, err
	}

	for _, v := range clients {
		if *v.ClientID == clientId {
			return true, nil
		}
	}

	return false, nil
}
