// Package keycloaktest runs a disposable Keycloak server for tests, in a container.
package keycloaktest

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// Image is the Keycloak release the tests run against.
	Image = "quay.io/keycloak/keycloak:26.8.0"

	// AdminUser and AdminPassword are the bootstrap administrator's credentials.
	AdminUser     = "admin"
	AdminPassword = "admin"
)

// Start runs Keycloak in development mode and returns its base URL. Tests are skipped when no
// container runtime is available, and the container is removed when the test ends.
func Start(t *testing.T) string {
	t.Helper()

	if testing.Short() {
		t.Skip("container test skipped in -short mode")
	}
	testcontainers.SkipIfProviderIsNotHealthy(t)

	ctr, err := testcontainers.Run(t.Context(), Image,
		testcontainers.WithCmd("start-dev"),
		testcontainers.WithEnv(map[string]string{
			"KC_BOOTSTRAP_ADMIN_USERNAME": AdminUser,
			"KC_BOOTSTRAP_ADMIN_PASSWORD": AdminPassword,
		}),
		testcontainers.WithExposedPorts("8080/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/realms/master").
				WithPort("8080/tcp").
				WithStatusCodeMatcher(func(code int) bool { return code == http.StatusOK }).
				WithStartupTimeout(3*time.Minute),
		),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("could not start Keycloak: %v", err)
	}

	endpoint, err := ctr.PortEndpoint(t.Context(), "8080/tcp", "http")
	if err != nil {
		t.Fatalf("could not determine the Keycloak endpoint: %v", err)
	}

	return fmt.Sprint(endpoint)
}
