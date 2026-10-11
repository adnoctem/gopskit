// Package vaulttest runs a disposable Vault server for tests, in a container.
package vaulttest

import (
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/vault"
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
