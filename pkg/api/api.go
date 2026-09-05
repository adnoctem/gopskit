// package api defines types and interfaces for generic API clients. They are used
// by the nested packages, which implement API clients for their respective applications.

package api

// interface Credentials defines the capabilities that API client credential objects
// must support
type Credentials interface {
	// Save persists the currently loaded credentials to a given filesystem path
	Save() error

	// Load loads credentials from a given filesystem path
	Load() error
}

// interface OIDCClient specifies required capabilities for an OIDC-authenticated API client
type OIDCClient interface {
	// Login is the initial entrypoint to the client
	Login() error

	// Auth provides access to the currently loaded credentials
	Auth() Credentials

	// SetCredentials updates the in-memory credentials which have been loaded for an
	// API client. It expects
	SetAuth(c Credentials) error

	// Refresh tries to refresh to current OIDC access token using the refresh token
	// if both have expired, an error will be returned and user must Login again
	Refresh() error

	// Valid denotes whether or not the current credentials are valid
	Valid() bool
}

type VaultClient interface {
	// Auth provides access to the currently loaded credentials
	Token() Credentials

	// SetCredentials updates the in-memory credentials which have been loaded for an
	// API client. It expects
	SetToken(c Credentials) error

	// Valid denotes whether or not the current credentials are valid
	Valid() bool
}
