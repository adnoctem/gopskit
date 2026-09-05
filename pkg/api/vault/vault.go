// package vault implements an HTTP API client for the secret-management solution Vault from
// HashCorp Inc.
package vault

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/adnoctem/gopskit/pkg/api"
	fs "github.com/adnoctem/gopskit/pkg/fsi"
	"github.com/adnoctem/gopskit/pkg/log"
	vault "github.com/hashicorp/vault-client-go"
)

// compile-time checks
var _ api.VaultClient = &Client{}
var _ api.Credentials = &Credentials{}

// Credentials is a custom type which is used to write and load Vault credentials to and from a
// file. It holds the cluster's root token and its unseal (or recovery) keys - not an OIDC login,
// hence the distinct name from keycloak.Auth.
type Credentials struct {
	// path is the filesystem path we're persisting the credentials to and loading them from
	path string

	// Created is a timestamp to know when the credentials were created
	Created time.Time `json:"created,omitempty"`

	// Keys are the Vault unseal (or recovery) keys
	Keys []string `json:"keys,omitempty"`

	// KeysB64 are the Vault unseal (or recovery) keys, base64-encoded
	KeysB64 []string `json:"keys_base64,omitempty"`

	// Token is the Vault root/auth token, used to authenticate to the API
	Token string `json:"token,omitempty"`
}

// Save implements the api.Credentials interface for Credentials
func (c *Credentials) Save() error {
	if c.path == "" {
		return fmt.Errorf("cannot save Vault credentials: no filesystem path configured")
	}

	jsn, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return fs.Write(c.path, jsn)
}

// Load implements the api.Credentials interface for Credentials
func (c *Credentials) Load() error {
	if c.path == "" {
		return fmt.Errorf("cannot load Vault credentials: no filesystem path configured")
	}

	raw, err := fs.Read(c.path)
	if err != nil {
		return err
	}

	var loaded Credentials
	if err := json.Unmarshal(raw, &loaded); err != nil {
		return err
	}

	loaded.path = c.path
	*c = loaded
	return nil
}

// Client wraps the official HashCorp Vault HTTP API client, adding credential persistence and
// a smaller, purpose-built surface for the operations gopskit's CLIs need.
type Client struct {
	// api is the underlying HashCorp Vault HTTP API client we're wrapping
	api *vault.Client

	// auth are the credentials (root token + unseal/recovery keys) currently loaded for this client
	auth *Credentials

	// host is the Vault server's base address, e.g. https://127.0.0.1:8200
	host string

	// timeout is the amount of time to elapse before prematurely cancelling a HTTP request. Zero
	// means "use the SDK default".
	timeout time.Duration

	// tls holds the TLS settings applied to the underlying HTTP client at construction time
	tls vault.TLSConfiguration
}

// ClientOpt configures a Client during New
type ClientOpt func(c *Client) error

// New creates a new Vault API Client for the given host, applying any of the provided ClientOpts
func New(host string, opts ...ClientOpt) *Client {
	c := &Client{
		host: host,
		auth: &Credentials{},
	}

	for _, o := range opts {
		if err := o(c); err != nil {
			log.Global.Fatalf("couldn't configure vault.Client. Error: %v\n", err)
		}
	}

	vaultOpts := []vault.ClientOption{
		vault.WithAddress(c.host),
		vault.WithTLS(c.tls),
	}
	if c.timeout != 0 {
		vaultOpts = append(vaultOpts, vault.WithRequestTimeout(c.timeout))
	}

	client, err := vault.New(vaultOpts...)
	if err != nil {
		log.Global.Fatalf("could not create Vault HTTP API client. Error: %v\n", err)
	}

	c.api = client
	return c
}

// WithTimeout sets the request timeout to use for the Vault API client
func WithTimeout(d time.Duration) ClientOpt {
	return func(c *Client) error {
		c.timeout = d
		return nil
	}
}

// WithInsecureTLS configures whether the client verifies the Vault server's TLS certificate
func WithInsecureTLS(insecure bool) ClientOpt {
	return func(c *Client) error {
		c.tls.InsecureSkipVerify = insecure
		return nil
	}
}

// WithCACerts configures a bundle of PEM-encoded CA certificates the client should trust in
// addition to the system pool
func WithCACerts(certs ...string) ClientOpt {
	return func(c *Client) error {
		var bundle []byte
		for _, p := range certs {
			if !fs.CheckIfExists(p) {
				return fmt.Errorf("cannot add CA certificate: %s to bundle. file not found", p)
			}

			b, err := fs.Read(p)
			if err != nil {
				return err
			}

			bundle = append(bundle, b...)
			bundle = append(bundle, '\n')
		}

		c.tls.ServerCertificate = vault.ServerCertificateEntry{FromBytes: bundle}
		return nil
	}
}

// WithTLSCert configures a client certificate/key pair for mutual TLS authentication
func WithTLSCert(cert, key string) ClientOpt {
	return func(c *Client) error {
		if !fs.CheckIfExists(cert) {
			return fmt.Errorf("cannot load certificate: %s. file not found", cert)
		}
		if !fs.CheckIfExists(key) {
			return fmt.Errorf("cannot load private key: %s. file not found", key)
		}

		c.tls.ClientCertificate = vault.ClientCertificateEntry{FromFile: cert}
		c.tls.ClientCertificateKey = vault.ClientCertificateKeyEntry{FromFile: key}
		return nil
	}
}

// WithAuthPath configures the filesystem path at which credentials are persisted/loaded
func WithAuthPath(path string) ClientOpt {
	return func(c *Client) error {
		c.auth.path = path
		return nil
	}
}

// SetAuthPath updates the filesystem path at which credentials are persisted/loaded. Unlike most
// of the client's configuration this often can't be known until after construction (e.g. it's
// derived from a CLI flag parsed by the calling command), hence the separate setter alongside
// WithAuthPath.
func (c *Client) SetAuthPath(path string) {
	c.auth.path = path
}

// -----------
// api.VaultClient
// -----------

// Token implements the api.VaultClient interface for Client
func (c *Client) Token() api.Credentials {
	return c.auth
}

// SetToken implements the api.VaultClient interface for Client. Besides updating the in-memory
// credentials it also propagates the token to the underlying Vault SDK client so subsequent API
// calls are authenticated.
func (c *Client) SetToken(creds api.Credentials) error {
	asserted, ok := creds.(*Credentials)
	if !ok {
		return fmt.Errorf("cannot set Vault API credentials to invalid type. must be *vault.Credentials")
	}

	c.auth = asserted
	return c.api.SetToken(asserted.Token)
}

// Valid implements the api.VaultClient interface for Client
func (c *Client) Valid() bool {
	return c.auth != nil && c.auth.Token != ""
}

// -----------
// internal helpers shared by system.go/auth.go/secrets.go
// -----------

// decodeInto re-decodes a generic API response payload into a concrete type. The Vault HTTP API
// client returns several responses (e.g. Initialize) as untyped maps; this centralizes the
// map->struct conversion instead of every caller hand-rolling its own JSON round-trip.
func decodeInto(data map[string]interface{}, out interface{}) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return json.Unmarshal(raw, out)
}
