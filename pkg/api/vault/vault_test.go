package vault

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/adnoctem/gopskit/pkg/api"
	vaultsdk "github.com/hashicorp/vault-client-go"
	"github.com/stretchr/testify/assert"
)

func TestCredentialsSaveAndLoad(t *testing.T) {
	asrt := assert.New(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "creds.json")

	c := &Credentials{path: path, Token: "s.abc", Keys: []string{"k1", "k2"}, Created: time.Now()}
	asrt.NoError(c.Save())

	loaded := &Credentials{path: path}
	asrt.NoError(loaded.Load())
	asrt.Equal("s.abc", loaded.Token)
	asrt.Equal([]string{"k1", "k2"}, loaded.Keys)
	asrt.Equal(path, loaded.path)
}

func TestCredentialsSaveWithoutPath(t *testing.T) {
	asrt := assert.New(t)

	c := &Credentials{Token: "s.abc"}
	asrt.Error(c.Save())
}

func TestCredentialsLoadWithoutPath(t *testing.T) {
	asrt := assert.New(t)

	c := &Credentials{}
	asrt.Error(c.Load())
}

func TestCredentialsLoadMissingFile(t *testing.T) {
	asrt := assert.New(t)

	c := &Credentials{path: filepath.Join(t.TempDir(), "missing.json")}
	asrt.Error(c.Load())
}

func TestClientValid(t *testing.T) {
	asrt := assert.New(t)

	asrt.False((&Client{}).Valid(), "nil auth must not be valid")
	asrt.False((&Client{auth: &Credentials{}}).Valid(), "an empty token must not be valid")
	asrt.True((&Client{auth: &Credentials{Token: "s.abc"}}).Valid())
}

func TestClientToken(t *testing.T) {
	asrt := assert.New(t)

	creds := &Credentials{Token: "s.abc"}
	c := &Client{auth: creds}
	asrt.Equal(api.Credentials(creds), c.Token())
}

func TestClientSetToken(t *testing.T) {
	t.Run("rejects a credentials value of the wrong type", func(t *testing.T) {
		asrt := assert.New(t)

		c := New("https://vault.example.com:8200")
		err := c.SetToken(wrongCredentialsType{})
		asrt.Error(err)
	})

	t.Run("accepts and applies real vault.Credentials", func(t *testing.T) {
		asrt := assert.New(t)

		c := New("https://vault.example.com:8200")
		err := c.SetToken(&Credentials{Token: "s.abc"})
		asrt.NoError(err)
		asrt.True(c.Valid())
	})
}

func TestWithCACerts(t *testing.T) {
	t.Run("errors on a certificate file that doesn't exist", func(t *testing.T) {
		asrt := assert.New(t)

		c := &Client{}
		err := WithCACerts(filepath.Join(t.TempDir(), "missing.pem"))(c)
		asrt.Error(err)
	})
}

func TestWithTLSCert(t *testing.T) {
	t.Run("errors when the certificate file doesn't exist", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		key := filepath.Join(dir, "key.pem")
		asrt.NoError(writeFile(key, "irrelevant"))

		c := &Client{}
		err := WithTLSCert(filepath.Join(dir, "missing-cert.pem"), key)(c)
		asrt.Error(err)
	})

	t.Run("errors when the key file doesn't exist", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		cert := filepath.Join(dir, "cert.pem")
		asrt.NoError(writeFile(cert, "irrelevant"))

		c := &Client{}
		err := WithTLSCert(cert, filepath.Join(dir, "missing-key.pem"))(c)
		asrt.Error(err)
	})
}

func TestIsNotFound(t *testing.T) {
	asrt := assert.New(t)

	asrt.True(IsNotFound(&vaultsdk.ResponseError{StatusCode: 404}))
	asrt.False(IsNotFound(&vaultsdk.ResponseError{StatusCode: 500}))
	asrt.False(IsNotFound(nil))
}

func TestDecodeInto(t *testing.T) {
	asrt := assert.New(t)

	type target struct {
		Name string `json:"name"`
	}

	var out target
	asrt.NoError(decodeInto(map[string]interface{}{"name": "value"}, &out))
	asrt.Equal("value", out.Name)
}

type wrongCredentialsType struct{}

func (wrongCredentialsType) Save() error { return nil }
func (wrongCredentialsType) Load() error { return nil }

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0600)
}
