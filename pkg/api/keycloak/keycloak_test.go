package keycloak

import (
	"crypto/tls"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nerzal/gocloak/v14"
	"github.com/stretchr/testify/assert"
)

func TestAuthSaveAndLoad(t *testing.T) {
	asrt := assert.New(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "creds.json")

	a := &Auth{path: path, username: "admin", JWT: &gocloak.JWT{AccessToken: "abc"}}
	asrt.NoError(a.Save())

	loaded := &Auth{path: path}
	asrt.NoError(loaded.Load())
	asrt.Equal("abc", loaded.JWT.AccessToken)
	// path must survive the load - it's unexported/not part of the JSON round-trip
	asrt.Equal(path, loaded.path)
}

func TestAuthLoadMissingFile(t *testing.T) {
	asrt := assert.New(t)

	a := &Auth{path: filepath.Join(t.TempDir(), "missing.json")}
	err := a.Load()
	asrt.ErrorIs(err, ErrAuthPathNotFound)
}

func TestAuthLoadRealError(t *testing.T) {
	asrt := assert.New(t)

	// a directory, not a file - fs.Read fails with something other than "not exist"
	dir := t.TempDir()
	a := &Auth{path: dir}
	err := a.Load()
	asrt.Error(err)
	asrt.NotErrorIs(err, ErrAuthPathNotFound)
}

func TestLoginValidation(t *testing.T) {
	t.Run("rejects a missing auth path", func(t *testing.T) {
		asrt := assert.New(t)

		kc := &Client{auth: &Auth{login: AdminCLILogin, username: "u", password: "p"}}
		asrt.Error(kc.Login())
	})

	t.Run("rejects AdminCLILogin without username/password", func(t *testing.T) {
		asrt := assert.New(t)

		kc := &Client{auth: &Auth{path: "/tmp/x", login: AdminCLILogin}}
		asrt.Error(kc.Login())
	})

	t.Run("rejects ClientLogin without clientId/clientSecret", func(t *testing.T) {
		asrt := assert.New(t)

		kc := &Client{auth: &Auth{path: "/tmp/x", login: ClientLogin}}
		asrt.Error(kc.Login())
	})
}

func TestRefreshValidation(t *testing.T) {
	t.Run("proceeds past a missing credentials file to the auth-mode check", func(t *testing.T) {
		asrt := assert.New(t)

		kc := &Client{auth: &Auth{path: filepath.Join(t.TempDir(), "missing.json"), login: AdminCLILogin}}
		err := kc.Refresh()
		// must fail on the *next* check (missing username/password), not on the missing file
		asrt.ErrorIs(err, ErrAdminAuthUnset)
	})

	t.Run("rejects AdminCLILogin without username/password", func(t *testing.T) {
		asrt := assert.New(t)

		kc := &Client{auth: &Auth{path: filepath.Join(t.TempDir(), "missing.json"), login: AdminCLILogin}}
		asrt.ErrorIs(kc.Refresh(), ErrAdminAuthUnset)
	})

	t.Run("rejects ClientLogin without clientId/clientSecret", func(t *testing.T) {
		asrt := assert.New(t)

		kc := &Client{auth: &Auth{path: filepath.Join(t.TempDir(), "missing.json"), login: ClientLogin}}
		asrt.ErrorIs(kc.Refresh(), ErrClientAuthUnset)
	})

	t.Run("propagates a real load error other than a missing file", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		kc := &Client{auth: &Auth{path: dir, login: AdminCLILogin, username: "u", password: "p"}}
		err := kc.Refresh()
		asrt.Error(err)
		asrt.NotErrorIs(err, ErrAdminAuthUnset)
	})
}

func TestValid(t *testing.T) {
	t.Run("a freshly constructed client (no JWT yet) is not valid", func(t *testing.T) {
		asrt := assert.New(t)

		kc := &Client{auth: &Auth{}}
		asrt.False(kc.Valid())
	})

	t.Run("a client with a live token is valid", func(t *testing.T) {
		asrt := assert.New(t)

		kc := &Client{auth: &Auth{
			Created: time.Now(),
			JWT:     &gocloak.JWT{ExpiresIn: 3600},
		}}
		asrt.True(kc.Valid())
	})

	t.Run("a client with an expired token is not valid", func(t *testing.T) {
		asrt := assert.New(t)

		kc := &Client{auth: &Auth{
			Created: time.Now().Add(-2 * time.Hour),
			JWT:     &gocloak.JWT{ExpiresIn: 3600},
		}}
		asrt.False(kc.Valid())
	})
}

func TestIsUnauthorizedErr(t *testing.T) {
	asrt := assert.New(t)

	kc := &Client{}
	asrt.True(kc.isUnauthorizedErr(assertError("401 Unauthorized: token expired")))
	asrt.False(kc.isUnauthorizedErr(assertError("500 Internal Server Error")))
}

func TestHandleAuthorizationError(t *testing.T) {
	t.Run("passes through a non-401 error untouched", func(t *testing.T) {
		asrt := assert.New(t)

		kc := &Client{}
		original := assertError("500 Internal Server Error")
		asrt.Equal(original, kc.handleAuthorizationError(original))
	})

	t.Run("surfaces a load failure when recovering from a 401", func(t *testing.T) {
		asrt := assert.New(t)

		kc := &Client{auth: &Auth{path: filepath.Join(t.TempDir(), "missing.json")}}
		err := kc.handleAuthorizationError(assertError("401 Unauthorized"))
		asrt.ErrorIs(err, ErrAuthPathNotFound)
	})
}

func TestLoginFromArg(t *testing.T) {
	asrt := assert.New(t)

	got, err := LoginFromArg("admin-cli")
	asrt.NoError(err)
	asrt.Equal(AdminCLILogin, got)

	got, err = LoginFromArg("client")
	asrt.NoError(err)
	asrt.Equal(ClientLogin, got)

	_, err = LoginFromArg("bogus")
	asrt.Error(err)
}

func TestSetters(t *testing.T) {
	asrt := assert.New(t)

	kc := &Client{auth: &Auth{}, tls: &tls.Config{}}
	kc.SetUser("u")
	kc.SetPassword("p")
	kc.SetClientID("cid")
	kc.SetClientSecret("secret")
	kc.SetAuthPath("/tmp/path")
	kc.SetLogin(ClientLogin)
	kc.SetRealm("custom")

	asrt.Equal("u", kc.auth.username)
	asrt.Equal("p", kc.auth.password)
	asrt.Equal("cid", kc.auth.clientId)
	asrt.Equal("secret", kc.auth.clientSecret)
	asrt.Equal("/tmp/path", kc.auth.path)
	asrt.Equal(ClientLogin, kc.auth.login)
	asrt.Equal("custom", kc.realm)
}

func assertError(msg string) error {
	return &simpleError{msg}
}

type simpleError struct{ msg string }

func (e *simpleError) Error() string { return e.msg }
