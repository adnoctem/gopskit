package keycloak

import "errors"

var (
	ErrAdminAuthUnset   = errors.New("cannot refresh Keycloak API 'admin-cli' credentials without username and password being set")
	ErrClientAuthUnset  = errors.New("cannot refresh Keycloak API 'client' credentials without clientId and clientSecret being set")
	ErrAuthPathNotFound = errors.New("the requested credentials file does not exist. cannot load credentials")
	ErrAuthExpired      = errors.New("cannot refresh Keycloak Access Token. Known credentials have expired")
)
