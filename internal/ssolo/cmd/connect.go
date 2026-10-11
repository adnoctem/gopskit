package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/adnoctem/gopskit/internal/bridgecmd"
	"github.com/adnoctem/gopskit/internal/ssolo/app"
	"github.com/adnoctem/gopskit/pkg/api/keycloak"
	"github.com/adnoctem/gopskit/pkg/kube/kubeutil"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// keycloakFlags are the flags of every command that talks to Keycloak.
type keycloakFlags struct {
	url               string
	username          string
	password          string
	credentialsSecret string
	loginRealm        string
	caCerts           []string
	insecure          bool

	conn bridgecmd.Connection
}

func (f *keycloakFlags) add(cmd *cobra.Command) {
	fs := cmd.Flags()

	fs.StringVar(&f.url, "url", os.Getenv("KEYCLOAK_URL"),
		"Keycloak's base URL, e.g. https://keycloak.keycloak.svc:8443 (default $KEYCLOAK_URL)")
	fs.StringVar(&f.username, "username", os.Getenv("KEYCLOAK_ADMIN"),
		"The administrator to log in as (default $KEYCLOAK_ADMIN)")
	fs.StringVar(&f.password, "password", "",
		"The administrator's password (default $KEYCLOAK_ADMIN_PASSWORD)")
	fs.StringVar(&f.credentialsSecret, "credentials-secret", "",
		"Read the administrator's credentials from the 'username' and 'password' keys of this Kubernetes Secret, as namespace/name")
	fs.StringVar(&f.loginRealm, "login-realm", keycloak.DefaultRealm, "The realm the administrator logs in to")
	fs.StringSliceVar(&f.caCerts, "ca-cert", nil, "A PEM CA certificate to trust for Keycloak's TLS certificate (repeatable)")
	fs.BoolVar(&f.insecure, "insecure-skip-tls-verify", false,
		"Don't verify Keycloak's TLS certificate. Insecure: anyone on the path can read the credentials")

	f.conn.AddFlags(fs)
}

// connect logs in to Keycloak as the flags describe and stores the client on s. The returned
// function releases the connection.
func (f *keycloakFlags) connect(ctx context.Context, s *app.State) (func(), error) {
	if f.url == "" {
		return nil, errors.New("the URL of Keycloak is required: set --url or $KEYCLOAK_URL")
	}

	username, password, err := f.credentials(ctx, s)
	if err != nil {
		return nil, err
	}

	kc := keycloak.New(f.url,
		keycloak.WithAuthPath(s.AuthPath()),
		keycloak.WithInsecureTLS(f.insecure),
		keycloak.WithRealm(f.loginRealm),
		keycloak.WithUsername(username),
		keycloak.WithPassword(password),
	)
	if len(f.caCerts) > 0 {
		if err := kc.SetCACertPool(f.caCerts...); err != nil {
			return nil, fmt.Errorf("could not load --ca-cert: %w", err)
		}
	}

	tr, release, err := f.conn.Transport(ctx, bridgeOptions(s), f.url, nil)
	if err != nil {
		return nil, err
	}
	kc.SetTransport(tr)

	if err := kc.Login(); err != nil {
		release()
		return nil, fmt.Errorf("could not log in to Keycloak at %s: %w", f.url, err)
	}

	s.Keycloak = kc
	return release, nil
}

func (f *keycloakFlags) credentials(ctx context.Context, s *app.State) (string, string, error) {
	if f.credentialsSecret == "" {
		password := f.password
		if password == "" {
			password = os.Getenv("KEYCLOAK_ADMIN_PASSWORD")
		}

		if f.username == "" || password == "" {
			return "", "", errors.New(
				"keycloak administrator credentials are required: set --username and --password, or --credentials-secret")
		}

		return f.username, password, nil
	}

	namespace, name, ok := strings.Cut(f.credentialsSecret, "/")
	if !ok || namespace == "" || name == "" {
		return "", "", fmt.Errorf("--credentials-secret must be namespace/name, got %q", f.credentialsSecret)
	}

	kc, err := s.KubeClient()
	if err != nil {
		return "", "", err
	}

	secret, err := kc.Client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf("could not read Secret %s: %w", f.credentialsSecret, err)
	}

	creds, err := kubeutil.ParseSecretKeys(secret, "username", "password")
	if err != nil {
		return "", "", err
	}

	return creds["username"], creds["password"], nil
}

// bridgeOptions describes ssolo to the bridge commands and helpers.
func bridgeOptions(s *app.State) bridgecmd.Options {
	return bridgecmd.Options{
		CLI:     app.Name,
		Version: s.Stamp.Version,
		Kube:    s.KubeClient,
		Log:     s.Log,
	}
}
