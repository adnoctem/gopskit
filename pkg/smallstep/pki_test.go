package smallstep

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.step.sm/crypto/jose"
	"go.step.sm/crypto/pemutil"
	"golang.org/x/crypto/ssh"
)

func generate(t *testing.T, opts Options) *PKI {
	t.Helper()

	p, err := Generate(opts)
	require.NoError(t, err)

	return p
}

func parseCert(t *testing.T, raw []byte) *x509.Certificate {
	t.Helper()

	block, _ := pem.Decode(raw)
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	return cert
}

func TestCertificateChain(t *testing.T) {
	asrt := assert.New(t)
	p := generate(t, Options{Name: "Example", DNSNames: []string{"ca.example.org"}})

	root, intermediate := parseCert(t, p.RootCert), parseCert(t, p.IntermediateCert)
	asrt.Equal("Example Root CA", root.Subject.CommonName)
	asrt.Equal("Example Intermediate CA", intermediate.Subject.CommonName)
	asrt.Equal(1, root.MaxPathLen)
	asrt.True(intermediate.MaxPathLenZero, "the intermediate may only sign leaves")

	roots := x509.NewCertPool()
	roots.AddCert(root)
	_, err := intermediate.Verify(x509.VerifyOptions{Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
	asrt.NoError(err, "the intermediate must chain to the root")

	sum := sha256.Sum256(root.Raw)
	asrt.Equal(hex.EncodeToString(sum[:]), p.Fingerprint)
	asrt.Equal(p.Fingerprint, p.Defaults["fingerprint"])
}

// Every key must open with its own password - and only with that one, which is the point of
// not sharing a password the way step does.
func TestKeysUseTheirOwnPasswords(t *testing.T) {
	p := generate(t, Options{Name: "Example", DNSNames: []string{"ca.example.org"}, SSH: true})
	pw := p.Options.Passwords

	keys := map[string]struct {
		pem      []byte
		password string
	}{
		"root":         {p.RootKey, pw.Root},
		"intermediate": {p.IntermediateKey, pw.CA},
		"ssh host":     {p.SSHHostKey, pw.SSHHost},
		"ssh user":     {p.SSHUserKey, pw.SSHUser},
	}

	all := []string{pw.Root, pw.CA, pw.Provisioner, pw.SSHHost, pw.SSHUser}
	for name, k := range keys {
		_, err := pemutil.ParseKey(k.pem, pemutil.WithPassword([]byte(k.password)))
		assert.NoError(t, err, "%s key with its password", name)

		for _, other := range all {
			if other == k.password {
				continue
			}
			_, err := pemutil.ParseKey(k.pem, pemutil.WithPassword([]byte(other)))
			assert.Error(t, err, "%s key must not open with another password", name)
		}
	}

	seen := map[string]bool{}
	for _, v := range all {
		assert.False(t, seen[v], "generated passwords must differ")
		seen[v] = true
	}
}

func TestProvisionerKeyOpensWithProvisionerPassword(t *testing.T) {
	p := generate(t, Options{Name: "Example", DNSNames: []string{"ca.example.org"}, Passwords: Passwords{Provisioner: "prov-pass"}})

	prov := p.CAConfig["authority"].(map[string]any)["provisioners"].([]any)[0].(map[string]any)
	assert.Equal(t, "JWK", prov["type"])
	assert.Equal(t, DefaultProvisioner, prov["name"])

	plain, err := jose.Decrypt([]byte(prov["encryptedKey"].(string)), jose.WithPassword([]byte("prov-pass")))
	require.NoError(t, err)

	var key jose.JSONWebKey
	require.NoError(t, key.UnmarshalJSON(plain))
	assert.False(t, key.IsPublic(), "the encrypted key must be the private key")
}

func TestSSHAndACME(t *testing.T) {
	asrt := assert.New(t)
	p := generate(t, Options{Name: "Example", DNSNames: []string{"ca.example.org"}, SSH: true, ACME: true})

	_, _, _, _, err := ssh.ParseAuthorizedKey(p.SSHHostPublicKey)
	asrt.NoError(err)
	_, _, _, _, err = ssh.ParseAuthorizedKey(p.SSHUserPublicKey)
	asrt.NoError(err)

	asrt.Equal(map[string]any{"hostKey": SSHHostKeyPath, "userKey": SSHUserKeyPath}, p.CAConfig["ssh"])

	var types []string
	for _, prov := range p.CAConfig["authority"].(map[string]any)["provisioners"].([]any) {
		types = append(types, prov.(map[string]any)["type"].(string))
	}
	asrt.Equal([]string{"JWK", "ACME", "SSHPOP"}, types)

	plain := generate(t, Options{Name: "Example", DNSNames: []string{"ca.example.org"}})
	asrt.NotContains(plain.CAConfig, "ssh")
	asrt.Empty(plain.Options.Passwords.SSHHost)
}

func TestGenerateNeedsNameAndDNS(t *testing.T) {
	_, err := Generate(Options{DNSNames: []string{"ca.example.org"}})
	assert.Error(t, err)
	_, err = Generate(Options{Name: "Example"})
	assert.Error(t, err)
}

func TestFullname(t *testing.T) {
	assert.Equal(t, "step-certificates", Fullname("step-certificates"))
	assert.Equal(t, "my-step-certificates", Fullname("my-step-certificates"))
	assert.Equal(t, "pki-step-certificates", Fullname("pki"))
}

func TestHelmValuesCarryBase64Passwords(t *testing.T) {
	asrt := assert.New(t)
	p := generate(t, Options{Name: "Example", DNSNames: []string{"ca.example.org"}, SSH: true})

	inject := p.HelmValues(false)["inject"].(map[string]any)
	secrets := inject["secrets"].(map[string]any)

	decoded, err := base64.StdEncoding.DecodeString(secrets["ca_password"].(string))
	require.NoError(t, err)
	asrt.Equal(p.Options.Passwords.CA, string(decoded))

	asrt.Empty(secrets["x509"].(map[string]any)["root_ca_key"], "the root key stays out of the cluster by default")
	asrt.Equal(string(p.RootKey), p.HelmValues(true)["inject"].(map[string]any)["secrets"].(map[string]any)["x509"].(map[string]any)["root_ca_key"])

	sshSecrets := secrets["ssh"].(map[string]any)
	asrt.Equal(true, sshSecrets["enabled"])
	asrt.NotEmpty(sshSecrets["host_ca_password"])
}

func TestExistingSecrets(t *testing.T) {
	asrt := assert.New(t)
	p := generate(t, Options{Name: "Example", DNSNames: []string{"ca.example.org"}, SSH: true})

	secrets, err := p.Secrets("pki", "step", false)
	require.NoError(t, err)

	byName := map[string]string{}
	for _, s := range secrets {
		asrt.Equal("step", s.Namespace)
		byName[s.Name] = string(s.Type)
	}
	asrt.Equal(map[string]string{
		"pki-step-certificates-secrets":              "smallstep.com/private-keys",
		"pki-step-certificates-certs":                "Opaque",
		"pki-step-certificates-config":               "Opaque",
		"pki-step-certificates-ca-password":          "smallstep.com/ca-password",
		"pki-step-certificates-provisioner-password": "smallstep.com/provisioner-password",
		"pki-step-certificates-ssh-host-ca-password": "smallstep.com/ssh-host-ca-password",
		"pki-step-certificates-ssh-user-ca-password": "smallstep.com/ssh-user-ca-password",
	}, byName)

	asrt.NotContains(secrets[0].StringData, "root_ca_key")
	asrt.Equal(p.Options.Passwords.CA, secrets[3].StringData["password"], "Secret data holds the password itself")
}
