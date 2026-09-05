// package keycloak implements an HTTP API client for the Keycloak SSO provider.
package keycloak

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Nerzal/gocloak/v14"
	"github.com/adnoctem/gopskit/pkg/api"
	fs "github.com/adnoctem/gopskit/pkg/fsi"
	"github.com/adnoctem/gopskit/pkg/helpers"
	"github.com/adnoctem/gopskit/pkg/log"
)

const (
	DefaultRealm = "master"
)

// compile-time checks
var _ api.OIDCClient = &Client{}
var _ api.Credentials = &Auth{}

type login int

const (
	AdminCLILogin login = iota
	ClientLogin
)

// Auth defines a structured JSON-en/decodable object for the required API (OIDC) credentials
type Auth struct {
	// path is the filesystem path, we're persisting credentials to
	path string

	// Admin CLI
	//
	// username is the name of a (temporary) admin user to use when logging into
	// the Keycloak server to obtain the first JWT. Setting the field requires
	// password to also be set
	username string

	// password is the password of a (temporary) admin user to use when logging into
	// the Keycloak server to obtain the first JWT. Requires username to be set also
	password string

	// Client
	//
	// clientId is the OIDC Client ID of a (temporary) service account to use to
	// authenticate to the Keycloak server to obtain the first JWT. Setting
	// the field requires clientSecret to also be set
	clientId string

	// clientSecret is the OIDC Client Secret of a (temporary) service account to use to
	// authenticate to  the Keycloak server to obtain the first JWT. Requires clientId to
	// be set also
	clientSecret string

	// login is the type of user Login you want to perform, most likely it will be using the
	// AdminCLI
	login login

	// Created is a timestamp to know when the credentials were created
	Created time.Time `json:"created,omitempty"`

	// JWT is the complete Keycloak JWT object returned from the API
	JWT *gocloak.JWT `json:"jwt,omitempty"`
}

// Save implements the Credentials interface for Auth
func (a *Auth) Save() error {
	jsn, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}

	return fs.Write(a.path, jsn)
}

// Load implements the Credentials interface for Auth
func (a *Auth) Load() error {
	raw, err := fs.Read(a.path)
	if err != nil {
		return err
	}

	var creds Auth
	err = json.Unmarshal(raw, &creds)
	if err != nil {
		return err
	}

	creds.path = a.path
	*a = creds
	return nil
}

// type Config struct {
// 	// Host defines the hostname for the new Keycloak API client
// 	Host string

// 	// Dir is the directory within we which we may or may not persist the API
// 	// credentials
// 	Dir string
// }

type Client struct {
	// api is the underlying gocloak Keycloak API client which we're wrapping
	api *gocloak.GoCloak

	// realm is the current Keycloak Realm to use when making API requests it is
	// master by default
	realm string

	// auth represents the actual credentials to use when authenticating to the Keycloak
	// server
	auth *Auth

	// cfg is the Config object the API client was initialized with
	// cfg *Config

	// tls is a pointer to a tls.Config object to use for the API client
	tls *tls.Config
}

type ClientOpt func(kc *Client) error

func New(host string, options ...ClientOpt) *Client {
	kc := &Client{
		api:   gocloak.NewClient(host),
		realm: DefaultRealm,
		auth: &Auth{
			login: AdminCLILogin,
		},
		tls: &tls.Config{},
	}

	// configure
	for _, o := range options {
		err := o(kc)
		if err != nil {
			log.Global.Fatalf("couldn't configure keycloak.Client. Error: %v\n", err)
		}
	}

	kc.applyTLSConfig()
	return kc
}

func WithRealm(realm string) ClientOpt {
	return func(kc *Client) error {
		kc.realm = realm
		return nil
	}
}

func WithUsername(username string) ClientOpt {
	return func(kc *Client) error {
		kc.auth.username = username
		return nil
	}
}

func WithPassword(password string) ClientOpt {
	return func(kc *Client) error {
		kc.auth.password = password
		return nil
	}
}

func WithClientID(clientID string) ClientOpt {
	return func(kc *Client) error {
		kc.auth.clientId = clientID
		return nil
	}
}

func WithClientSecret(clientSecret string) ClientOpt {
	return func(kc *Client) error {
		kc.auth.clientSecret = clientSecret
		return nil
	}
}

func WithAuthPath(path string) ClientOpt {
	return func(kc *Client) error {
		kc.auth.path = path
		return nil
	}
}

func WithLogin(lgn login) ClientOpt {
	return func(kc *Client) error {
		kc.auth.login = lgn
		return nil
	}
}

func WithInsecureTLS(insecure bool) ClientOpt {
	return func(kc *Client) error {
		kc.tls.InsecureSkipVerify = insecure
		return nil
	}
}

func WithCACerts(certs ...string) ClientOpt {
	return func(kc *Client) error {
		pool, err := api.LoadCACertPool(certs...)
		if err != nil {
			return err
		}

		kc.tls.RootCAs = pool
		return nil
	}
}

func WithTLSCert(cert, key string) ClientOpt {
	return func(kc *Client) error {
		crt, err := api.LoadX509KeyPair(cert, key)
		if err != nil {
			return err
		}

		kc.tls.Certificates = []tls.Certificate{crt}
		return nil
	}
}

func (kc *Client) Login() error {
	var err error
	ctx := context.Background()

	if helpers.EmptyString(kc.auth.path) {
		return fmt.Errorf("logging in without 'authPath' set is non-sensical. access and refresh tokens won't be saved")
	}

	if kc.auth.login == AdminCLILogin && helpers.EmptyStrings(kc.auth.username, kc.auth.password) {
		return fmt.Errorf("cannot authenticate to the Keycloak API as 'admin-cli' without username and password being set")
	}

	if kc.auth.login == ClientLogin && helpers.EmptyStrings(kc.auth.clientId, kc.auth.clientSecret) {
		return fmt.Errorf("cannot authenticate to the Keycloak API as 'client' without clientId and clientSecret being set")
	}

	switch kc.auth.login {
	case AdminCLILogin:
		kc.auth.JWT, err = kc.api.LoginAdmin(ctx, kc.auth.username, kc.auth.password, kc.realm)
		if err != nil {
			return err
		}

		kc.auth.Created = time.Now()
	case ClientLogin:
		kc.auth.JWT, err = kc.api.LoginClient(ctx, kc.auth.clientId, kc.auth.clientSecret, kc.realm)
		if err != nil {
			return err
		}

		kc.auth.Created = time.Now()
	}

	return kc.auth.Save()
}

func (kc *Client) Refresh() error {
	var err error
	ctx := context.Background()

	// try to load old credentials; a missing file is fine (first ever refresh), anything else
	// is a real problem worth surfacing rather than proceeding with stale/empty auth
	if err := kc.auth.Load(); err != nil && !errors.Is(err, ErrAuthPathNotFound) {
		return err
	}

	if kc.auth.login == AdminCLILogin && helpers.EmptyStrings(kc.auth.username, kc.auth.password) {
		return ErrAdminAuthUnset
	}

	if kc.auth.login == ClientLogin && helpers.EmptyStrings(kc.auth.clientId, kc.auth.clientSecret) {
		return ErrClientAuthUnset
	}

	switch kc.auth.login {
	case AdminCLILogin:
		{
			kc.auth.JWT, err = kc.api.RefreshToken(ctx, kc.auth.JWT.RefreshToken, "admin-cli", "", kc.realm)
			if err != nil {
				return err
			}

			kc.auth.Created = time.Now()
		}

	case ClientLogin:
		{
			kc.auth.JWT, err = kc.api.RefreshToken(ctx, kc.auth.JWT.RefreshToken, kc.auth.clientId, kc.auth.clientSecret, kc.realm)
			if err != nil {
				return err
			}

			kc.auth.Created = time.Now()
		}
	}

	return kc.auth.Save()
}

// -----------
// Setters
// -----------

func (kc *Client) SetAuth(creds api.Credentials) error {
	asserted, ok := creds.(*Auth)
	if !ok {
		return fmt.Errorf("cannot set Keycloak API credentials to invalid type. must be keycloak.Auth")
	}

	kc.auth = asserted
	return nil
}

func (kc *Client) SetUser(username string) {
	kc.auth.username = username
}

func (kc *Client) SetPassword(pwd string) {
	kc.auth.password = pwd
}

func (kc *Client) SetClientID(clientID string) {
	kc.auth.clientId = clientID
}

func (kc *Client) SetClientSecret(clientSecret string) {
	kc.auth.clientSecret = clientSecret
}

func (kc *Client) SetAuthPath(path string) {
	kc.auth.path = path
}

func (kc *Client) SetLogin(login login) {
	kc.auth.login = login
}

func (kc *Client) SetRealm(r string) {
	kc.realm = r
}

func (kc *Client) SetInsecureTLS(insecure bool) {
	kc.tls.InsecureSkipVerify = insecure
}

func (kc *Client) SetCACertPool(certs ...string) error {
	pool, err := api.LoadCACertPool(certs...)
	if err != nil {
		return err
	}

	kc.tls.RootCAs = pool
	kc.applyTLSConfig()
	return nil
}

func (kc *Client) SetKeyPair(cert, key string) error {
	crt, err := api.LoadX509KeyPair(cert, key)
	if err != nil {
		return err
	}

	kc.tls.Certificates = []tls.Certificate{crt}
	kc.applyTLSConfig()
	return nil
}

// -----------
// Getters
// -----------

func (kc *Client) Auth() api.Credentials {
	return kc.auth
}

func (kc *Client) Valid() bool {
	return !kc.hasTokenExpired()
}

// -----------
// public Utils
// -----------

func LoginFromArg(arg string) (login, error) {
	switch arg {
	case "admin-cli":
		return AdminCLILogin, nil
	case "client":
		return ClientLogin, nil
	default:
		return AdminCLILogin, fmt.Errorf("invalid login: %s", arg)
	}
}

// -----------
// private
// -----------

func (kc *Client) applyTLSConfig() {
	kc.api.RestyClient().SetTLSClientConfig(kc.tls)
}

// handleAuthorizationError attempts to recover from a 401 response by reloading persisted
// credentials and refreshing the access token if it has expired. It returns nil if recovery
// succeeded and the caller should retry its original request exactly once, or a non-nil error
// if recovery wasn't possible. It deliberately never dispatches another authenticated API
// request itself (e.g. via kc.Realms()) - doing so would re-enter this handler on a persistently
// invalid token and recurse indefinitely.
func (kc *Client) handleAuthorizationError(err error) error {
	if !kc.isUnauthorizedErr(err) {
		return err
	}

	if err := kc.auth.Load(); err != nil {
		return err
	}

	if kc.hasTokenExpired() {
		if err := kc.Refresh(); err != nil {
			return ErrAuthExpired
		}
	}

	return nil
}

func (kc *Client) isUnauthorizedErr(err error) bool {
	return strings.Contains(err.Error(), "401 Unauthorized")
}

func (kc *Client) hasTokenExpired() bool {
	validUntil := kc.auth.Created.Add(time.Duration(kc.auth.JWT.ExpiresIn) * time.Second)
	return validUntil.Before(time.Now())
}

// func dieWithError(err error) {
// 	log.Global.Fatalf("could not create Keycloak API client. Error: %v\n", err)
// }
