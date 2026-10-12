package smallstep

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.step.sm/crypto/jose"
)

// stepCAImage is the step-ca release the generated CA is checked against.
const stepCAImage = "smallstep/step-ca:0.30.2"

// TestStepCAAcceptsGeneratedCA runs the real step-ca with a generated CA laid out the way the Helm
// chart mounts it, and has it issue a certificate through the JWK provisioner. That exercises
// everything the chart relies on: ca.json, the encrypted keys and every password.
func TestStepCAAcceptsGeneratedCA(t *testing.T) {
	if testing.Short() {
		t.Skip("container test skipped in -short mode")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)

	const dns = "ca.example.org"
	p := generate(t, Options{Name: "Gopskit Test", DNSNames: []string{dns}, SSH: true})
	pw := p.Options.Passwords

	caJSON, err := json.Marshal(p.CAConfig)
	require.NoError(t, err)

	file := func(content []byte, path string) testcontainers.ContainerFile {
		return testcontainers.ContainerFile{Reader: bytes.NewReader(content), ContainerFilePath: path, FileMode: 0o644}
	}

	ctr, err := testcontainers.Run(t.Context(), stepCAImage,
		testcontainers.WithFiles(
			file(caJSON, CAConfigPath),
			file(p.RootCert, RootCertPath),
			file(p.IntermediateCert, IntermediateCertPath),
			file(p.IntermediateKey, IntermediateKeyPath),
			file(p.SSHHostKey, SSHHostKeyPath),
			file(p.SSHUserKey, SSHUserKeyPath),
			// the chart mounts each password Secret as a directory holding "password"
			file([]byte(pw.CA), "/home/step/secrets/passwords/password"),
			file([]byte(pw.SSHHost), "/home/step/secrets/ssh-host-ca/password"),
			file([]byte(pw.SSHUser), "/home/step/secrets/ssh-user-ca/password"),
		),
		// the arguments the chart's Deployment passes
		testcontainers.WithEntrypoint("/usr/local/bin/step-ca"),
		testcontainers.WithCmd(CAConfigPath,
			"--password-file", "/home/step/secrets/passwords/password",
			"--ssh-host-password-file", "/home/step/secrets/ssh-host-ca/password",
			"--ssh-user-password-file", "/home/step/secrets/ssh-user-ca/password",
		),
		testcontainers.WithExposedPorts("9000/tcp"),
		testcontainers.WithWaitStrategy(wait.ForLog("Serving HTTPS on").WithStartupTimeout(2*time.Minute)),
	)
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)

	endpoint, err := ctr.PortEndpoint(t.Context(), "9000/tcp", "")
	require.NoError(t, err)
	base := "https://" + endpoint

	// trust only the generated root, presenting the CA's own name, like a real client would
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(p.RootCert))
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: dns}},
	}

	t.Run("serves the generated root", func(t *testing.T) {
		resp, err := client.Get(base + "/roots")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		// step-ca answers this GET with 201 Created
		require.Less(t, resp.StatusCode, 300, "status %d", resp.StatusCode)

		var body struct {
			Crts []string `json:"crts"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.Len(t, body.Crts, 1)
		assert.Equal(t, strings.TrimSpace(string(p.RootCert)), strings.TrimSpace(body.Crts[0]))
	})

	t.Run("issues a certificate through the JWK provisioner", func(t *testing.T) {
		const cn = "service.example.org"

		ott := provisionerToken(t, p, "https://"+dns+"/1.0/sign", cn)

		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
			Subject:  pkix.Name{CommonName: cn},
			DNSNames: []string{cn},
		}, key)
		require.NoError(t, err)

		req, err := json.Marshal(map[string]string{
			"csr": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})),
			"ott": ott,
		})
		require.NoError(t, err)

		resp, err := client.Post(base+"/1.0/sign", "application/json", bytes.NewReader(req))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		var body struct {
			Crt     string `json:"crt"`
			Message string `json:"message"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		require.Equal(t, http.StatusCreated, resp.StatusCode, "step-ca said: %s", body.Message)

		leaf := parseCert(t, []byte(body.Crt))
		assert.Equal(t, []string{cn}, leaf.DNSNames)

		intermediates := x509.NewCertPool()
		intermediates.AddCert(parseCert(t, p.IntermediateCert))
		_, err = leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, DNSName: cn})
		assert.NoError(t, err, "the issued certificate must chain to the generated root")
	})
}

// provisionerToken signs a one-time token with the JWK provisioner's key, which only opens with
// the provisioner password - the way `step ca token` does.
func provisionerToken(t *testing.T, p *PKI, audience, subject string) string {
	t.Helper()

	prov := p.CAConfig["authority"].(map[string]any)["provisioners"].([]any)[0].(map[string]any)
	raw, err := jose.Decrypt([]byte(prov["encryptedKey"].(string)), jose.WithPassword([]byte(p.Options.Passwords.Provisioner)))
	require.NoError(t, err)

	var jwk jose.JSONWebKey
	require.NoError(t, jwk.UnmarshalJSON(raw))

	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.SignatureAlgorithm(jwk.Algorithm), Key: jwk.Key},
		new(jose.SignerOptions).WithType("JWT").WithHeader("kid", jwk.KeyID),
	)
	require.NoError(t, err)

	now := time.Now()
	claims := map[string]any{
		"iss":  p.Options.Provisioner,
		"aud":  audience,
		"sub":  subject,
		"sans": []string{subject},
		"sha":  p.Fingerprint,
		"iat":  now.Unix(),
		"nbf":  now.Add(-time.Minute).Unix(),
		"exp":  now.Add(5 * time.Minute).Unix(),
		"jti":  fmt.Sprintf("%d", now.UnixNano()),
	}

	token, err := jose.Signed(signer).Claims(claims).CompactSerialize()
	require.NoError(t, err)

	return token
}
