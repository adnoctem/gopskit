// Package vaulttest runs a disposable Vault server for tests, in a container.
package vaulttest

import (
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/vault"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// Image is the Vault release the tests run against.
	Image = "hashicorp/vault:2.1.2"

	// RootToken authenticates as root.
	RootToken = "root-token"

	// KVMount is where development mode mounts its KV-v2 secrets engine.
	KVMount = "secret"
)

// StartDev runs Vault in development mode - initialised, unsealed, with a KV-v2 engine at KVMount -
// and returns its address. Tests are skipped when no container runtime is available, and the
// container is removed when the test ends.
func StartDev(t *testing.T) string {
	t.Helper()

	if testing.Short() {
		t.Skip("container test skipped in -short mode")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctr, err := vault.Run(t.Context(), Image, vault.WithToken(RootToken))
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("could not start Vault: %v", err)
	}

	addr, err := ctr.HttpHostAddress(t.Context())
	if err != nil {
		t.Fatalf("could not determine the Vault address: %v", err)
	}

	return addr
}

// serverConfig runs Vault the way a fresh deployment starts: uninitialised and sealed, with
// in-memory storage and a plain-HTTP listener.
const serverConfig = `{
  "storage": {"inmem": {}},
  "listener": [{"tcp": {"address": "0.0.0.0:8200", "tls_disable": true}}],
  "disable_mlock": true
}`

// StartServer runs an uninitialised, sealed Vault server and returns its address - for testing
// initialisation and unsealing, which development mode skips. Tests are skipped when no container
// runtime is available, and the container is removed when the test ends.
func StartServer(t *testing.T) string {
	t.Helper()

	if testing.Short() {
		t.Skip("container test skipped in -short mode")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctr, err := testcontainers.Run(t.Context(), Image,
		testcontainers.WithCmd("server"),
		testcontainers.WithEnv(map[string]string{"VAULT_LOCAL_CONFIG": serverConfig}),
		testcontainers.WithExposedPorts("8200/tcp"),
		testcontainers.WithWaitStrategy(
			// an uninitialised Vault answers its health endpoint with 501
			wait.ForHTTP("/v1/sys/health").
				WithPort("8200/tcp").
				WithStatusCodeMatcher(func(code int) bool { return code == 501 }).
				WithStartupTimeout(2*time.Minute),
		),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("could not start Vault: %v", err)
	}

	endpoint, err := ctr.PortEndpoint(t.Context(), "8200/tcp", "http")
	if err != nil {
		t.Fatalf("could not determine the Vault address: %v", err)
	}

	return fmt.Sprint(endpoint)
}
