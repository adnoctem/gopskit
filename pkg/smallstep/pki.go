// Package smallstep generates a complete private certificate authority for Smallstep's step-ca,
// in the layout the smallstep/step-certificates Helm chart expects.
//
// It mirrors what `step ca init --helm` produces - an EC P-256 root and intermediate CA, a JWK
// provisioner whose private key is stored encrypted in ca.json, and optionally the SSH host and
// user CA keys - using step's own crypto library, so no step binary is needed. Unlike step, which
// encrypts everything with one password, every key gets its own: the chart accepts separate
// passwords for the CA, the provisioner and both SSH keys, and its template advises a root key
// password used for nothing else.
package smallstep

import (
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"

	"go.step.sm/crypto/jose"
	"go.step.sm/crypto/keyutil"
	"go.step.sm/crypto/pemutil"
	"golang.org/x/crypto/ssh"

	"github.com/adnoctem/gopskit/pkg/helpers"
)

// Paths inside the step-ca container, as the chart mounts them.
const (
	certsDir   = "/home/step/certs"
	secretsDir = "/home/step/secrets"
	configDir  = "/home/step/config"

	RootCertPath         = certsDir + "/root_ca.crt"
	IntermediateCertPath = certsDir + "/intermediate_ca.crt"
	IntermediateKeyPath  = secretsDir + "/intermediate_ca_key"
	SSHHostKeyPath       = secretsDir + "/ssh_host_ca_key"
	SSHUserKeyPath       = secretsDir + "/ssh_user_ca_key"
	CAConfigPath         = configDir + "/ca.json"
)

const (
	// DefaultAddress is where step-ca listens, matching the chart's service target port.
	DefaultAddress = ":9000"

	// DefaultProvisioner names the JWK provisioner, as the chart does.
	DefaultProvisioner = "admin"

	// validity of the root and intermediate certificates, as step uses
	validity = 10 * 365 * 24 * time.Hour

	// passwordLength is the length of generated passwords
	passwordLength = 48
)

// Options describe the CA to generate.
type Options struct {
	// Name is the CA's name: the certificates are called "<Name> Root CA" and
	// "<Name> Intermediate CA".
	Name string

	// DNSNames are the names step-ca serves under. The first is the URL clients use.
	DNSNames []string

	// Address is where step-ca listens; DefaultAddress when empty.
	Address string

	// Provisioner names the JWK provisioner; DefaultProvisioner when empty.
	Provisioner string

	// SSH additionally creates the SSH host and user CA keys and an SSHPOP provisioner.
	SSH bool

	// ACME additionally adds an ACME provisioner named "acme".
	ACME bool

	// Passwords are generated where empty.
	Passwords Passwords
}

// Passwords are the passwords the generated keys are encrypted with.
type Passwords struct {
	// Root encrypts the root CA key, which step-ca itself never needs.
	Root string `json:"root"`

	// CA encrypts the intermediate CA key, which step-ca signs with.
	CA string `json:"ca"`

	// Provisioner encrypts the JWK provisioner's private key.
	Provisioner string `json:"provisioner"`

	// SSHHost and SSHUser encrypt the SSH CA keys.
	SSHHost string `json:"sshHost,omitempty"`
	SSHUser string `json:"sshUser,omitempty"`
}

// PKI is a generated certificate authority. PEM fields hold the encoded certificate or the
// encrypted private key.
type PKI struct {
	Options Options

	RootCert         []byte
	RootKey          []byte
	IntermediateCert []byte
	IntermediateKey  []byte

	// SSH keys, set only with Options.SSH. The public keys are in authorized_keys format.
	SSHHostKey       []byte
	SSHHostPublicKey []byte
	SSHUserKey       []byte
	SSHUserPublicKey []byte

	// Fingerprint is the SHA-256 fingerprint of the root certificate, which clients pin.
	Fingerprint string

	// CAConfig and Defaults are the contents of ca.json and defaults.json.
	CAConfig map[string]any
	Defaults map[string]any
}

// Generate creates a CA as opts describe.
func Generate(opts Options) (*PKI, error) {
	if opts.Name == "" {
		return nil, errors.New("the CA needs a name")
	}
	if len(opts.DNSNames) == 0 {
		return nil, errors.New("the CA needs at least one DNS name")
	}
	if opts.Address == "" {
		opts.Address = DefaultAddress
	}
	if opts.Provisioner == "" {
		opts.Provisioner = DefaultProvisioner
	}
	opts.Passwords = opts.Passwords.withDefaults(opts.SSH)

	p := &PKI{Options: opts}

	root, rootKey, err := p.createCA(opts.Name+" Root CA", nil, nil, 1, opts.Passwords.Root)
	if err != nil {
		return nil, fmt.Errorf("could not create the root CA: %w", err)
	}
	p.RootCert, p.RootKey = pemCert(root), rootKey.pem
	sum := sha256.Sum256(root.Raw)
	p.Fingerprint = hex.EncodeToString(sum[:])

	intermediate, intermediateKey, err := p.createCA(opts.Name+" Intermediate CA", root, rootKey.signer, 0, opts.Passwords.CA)
	if err != nil {
		return nil, fmt.Errorf("could not create the intermediate CA: %w", err)
	}
	p.IntermediateCert, p.IntermediateKey = pemCert(intermediate), intermediateKey.pem

	if opts.SSH {
		if p.SSHHostKey, p.SSHHostPublicKey, err = sshCAKey(opts.Passwords.SSHHost); err != nil {
			return nil, fmt.Errorf("could not create the SSH host CA key: %w", err)
		}
		if p.SSHUserKey, p.SSHUserPublicKey, err = sshCAKey(opts.Passwords.SSHUser); err != nil {
			return nil, fmt.Errorf("could not create the SSH user CA key: %w", err)
		}
	}

	provisioners, err := p.provisioners()
	if err != nil {
		return nil, err
	}
	p.CAConfig = p.caConfig(provisioners)
	p.Defaults = map[string]any{
		"ca-url":      "https://" + opts.DNSNames[0],
		"ca-config":   CAConfigPath,
		"fingerprint": p.Fingerprint,
		"root":        RootCertPath,
	}

	return p, nil
}

func (pw Passwords) withDefaults(sshEnabled bool) Passwords {
	gen := func(v string) string {
		if v != "" {
			return v
		}
		return helpers.GeneratePassphrase(helpers.WithLength(passwordLength))
	}

	pw.Root, pw.CA, pw.Provisioner = gen(pw.Root), gen(pw.CA), gen(pw.Provisioner)
	if sshEnabled {
		pw.SSHHost, pw.SSHUser = gen(pw.SSHHost), gen(pw.SSHUser)
	} else {
		pw.SSHHost, pw.SSHUser = "", ""
	}

	return pw
}

type encryptedKey struct {
	signer crypto.Signer
	pem    []byte
}

// createCA creates a CA certificate - self-signed when parent is nil - and its key, encrypted
// with password. maxPathLen follows step: 1 for the root, 0 for the intermediate.
func (p *PKI) createCA(cn string, parent *x509.Certificate, parentKey crypto.Signer, maxPathLen int, password string) (*x509.Certificate, *encryptedKey, error) {
	signer, err := keyutil.GenerateDefaultSigner()
	if err != nil {
		return nil, nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn, Organization: []string{p.Options.Name}},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(validity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            maxPathLen,
		MaxPathLenZero:        maxPathLen == 0,
	}

	if parent == nil {
		parent, parentKey = tmpl, signer
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, signer.Public(), parentKey)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, err
	}

	encoded, err := encryptKey(signer, password)
	if err != nil {
		return nil, nil, err
	}

	return cert, &encryptedKey{signer: signer, pem: encoded}, nil
}

// sshCAKey creates an SSH CA key: the private key encrypted with password, and the public key in
// authorized_keys format.
func sshCAKey(password string) ([]byte, []byte, error) {
	signer, err := keyutil.GenerateDefaultSigner()
	if err != nil {
		return nil, nil, err
	}

	pub, err := ssh.NewPublicKey(signer.Public())
	if err != nil {
		return nil, nil, err
	}

	encoded, err := encryptKey(signer, password)
	if err != nil {
		return nil, nil, err
	}

	return encoded, ssh.MarshalAuthorizedKey(pub), nil
}

// encryptKey encodes key as encrypted PEM, the way step does.
func encryptKey(key crypto.PrivateKey, password string) ([]byte, error) {
	block, err := pemutil.Serialize(key, pemutil.WithPassword([]byte(password)))
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(block), nil
}

func pemCert(c *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
}

// provisioners builds ca.json's provisioners: the JWK provisioner, whose private key is stored
// encrypted with the provisioner password, plus ACME and SSHPOP when enabled.
func (p *PKI) provisioners() ([]any, error) {
	pub, priv, err := jose.GenerateDefaultKeyPair([]byte(p.Options.Passwords.Provisioner))
	if err != nil {
		return nil, fmt.Errorf("could not create the provisioner key: %w", err)
	}

	encrypted, err := priv.CompactSerialize()
	if err != nil {
		return nil, fmt.Errorf("could not serialize the provisioner key: %w", err)
	}

	// round-trip the public key through JSON so ca.json holds plain JSON values
	var key map[string]any
	raw, err := json.Marshal(pub)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &key); err != nil {
		return nil, err
	}

	jwk := map[string]any{
		"type":         "JWK",
		"name":         p.Options.Provisioner,
		"key":          key,
		"encryptedKey": encrypted,
	}
	if p.Options.SSH {
		jwk["claims"] = map[string]any{"enableSSHCA": true}
	}

	provisioners := []any{jwk}
	if p.Options.ACME {
		provisioners = append(provisioners, map[string]any{"type": "ACME", "name": "acme"})
	}
	if p.Options.SSH {
		provisioners = append(provisioners, map[string]any{
			"type":   "SSHPOP",
			"name":   "sshpop",
			"claims": map[string]any{"enableSSHCA": true},
		})
	}

	return provisioners, nil
}

func (p *PKI) caConfig(provisioners []any) map[string]any {
	dnsNames := make([]any, len(p.Options.DNSNames))
	for i, n := range p.Options.DNSNames {
		dnsNames[i] = n
	}

	cfg := map[string]any{
		"root":          RootCertPath,
		"federateRoots": []any{},
		"crt":           IntermediateCertPath,
		"key":           IntermediateKeyPath,
		"address":       p.Options.Address,
		"dnsNames":      dnsNames,
		"logger":        map[string]any{"format": "json"},
		"db": map[string]any{
			"type":       "badgerv2",
			"dataSource": "/home/step/db",
		},
		"authority": map[string]any{
			"enableAdmin":  false,
			"provisioners": provisioners,
		},
		// step-ca's defaults
		"tls": map[string]any{
			"cipherSuites": []any{
				"TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256",
				"TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
			},
			"minVersion":    1.2,
			"maxVersion":    1.3,
			"renegotiation": false,
		},
	}

	if p.Options.SSH {
		cfg["ssh"] = map[string]any{
			"hostKey": SSHHostKeyPath,
			"userKey": SSHUserKeyPath,
		}
	}

	return cfg
}
