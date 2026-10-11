package cmd

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/adnoctem/gopskit/internal/bridgecmd"
	"github.com/adnoctem/gopskit/internal/waltr/app"
	"github.com/adnoctem/gopskit/internal/waltr/util"
	apivault "github.com/adnoctem/gopskit/pkg/api/vault"
	"github.com/adnoctem/gopskit/pkg/config"
	"github.com/adnoctem/gopskit/pkg/core"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
)

// vaultFlags are the flags of every command that talks to Vault. Their environment variables
// are the ones the vault CLI itself reads.
type vaultFlags struct {
	addr     string
	token    string
	caCerts  []string
	insecure bool

	conn bridgecmd.Connection
}

func (f *vaultFlags) add(cmd *cobra.Command, withToken bool) {
	fs := cmd.Flags()

	fs.StringVar(&f.addr, "vault-addr", "", "Vault's address, e.g. https://vault.vault.svc:8200")
	fs.StringSliceVar(&f.caCerts, "ca-cert", nil, "A PEM CA certificate to trust for Vault's TLS certificate (repeatable)")
	fs.BoolVar(&f.insecure, "insecure-skip-tls-verify", false,
		"Don't verify Vault's TLS certificate. Insecure: anyone on the path can read the token")
	f.conn.AddFlags(fs)

	config.BindEnv(fs, "vault-addr", "VAULT_ADDR")
	config.BindEnv(fs, "ca-cert", "VAULT_CACERT")
	config.BindEnv(fs, "insecure-skip-tls-verify", "VAULT_SKIP_VERIFY")

	if withToken {
		fs.StringVarP(&f.token, "token", "t", "",
			"The Vault token (default: the root token 'waltr init' stored for --environment)")
		config.BindEnv(fs, "token", "VAULT_TOKEN")
	}
}

// connect points s.Vault at addr, reached as --via says, and authenticates it with the token
// from --token, $VAULT_TOKEN or the credentials stored for environment. The returned function
// releases the connection.
func (f *vaultFlags) connect(ctx context.Context, s *app.State, addr string, environment core.Environment) (func(), error) {
	release, err := f.dial(ctx, s, addr)
	if err != nil {
		return nil, err
	}

	token := f.token
	if token == "" {
		s.Log.Debug("no token given, using the credentials stored by 'waltr init'")

		creds, err := util.ReadCredentials(s, environment)
		if err != nil {
			release()
			return nil, fmt.Errorf("no Vault token: set --token or $VAULT_TOKEN, or run 'waltr init' first (%w)", err)
		}
		token = creds.Token
	}

	if err := s.Vault.SetToken(&apivault.Credentials{Token: token}); err != nil {
		release()
		return nil, err
	}

	return release, nil
}

// dial points s.Vault at addr without authenticating it, for operations such as initialising and
// unsealing that need no token.
func (f *vaultFlags) dial(ctx context.Context, s *app.State, addr string) (func(), error) {
	if addr == "" {
		return nil, errors.New("vault's address is required: set --vault-addr, $VAULT_ADDR or vault-addr in the configuration file")
	}

	if err := checkCACerts(f.caCerts); err != nil {
		return nil, fmt.Errorf("--ca-cert: %w", err)
	}

	tr, release, err := f.conn.Transport(ctx, bridgeOptions(s), addr, nil)
	if err != nil {
		return nil, err
	}

	opts := []apivault.ClientOpt{
		apivault.WithTransport(tr),
		apivault.WithTimeout(60 * time.Second),
		apivault.WithInsecureTLS(f.insecure),
	}
	if len(f.caCerts) > 0 {
		opts = append(opts, apivault.WithCACerts(f.caCerts...))
	}

	s.Vault = apivault.New(addr, opts...)
	return release, nil
}

// checkCACerts makes sure every file holds a PEM certificate. The Vault client exits the process
// on one that doesn't, so a bad file must be caught before it gets there.
func checkCACerts(paths []string) error {
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if !x509.NewCertPool().AppendCertsFromPEM(raw) {
			return fmt.Errorf("%s holds no PEM certificate", p)
		}
	}

	return nil
}

// podAddress is the address of one Vault Pod: the scheme and port of base, at the Pod's own DNS
// name. Pods of a StatefulSet with a headless Service - which is how the official chart runs
// Vault - have one, and it resolves even while the Pod isn't ready, which unsealing depends on.
func podAddress(pod corev1.Pod, base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid Vault address %q", base)
	}

	if pod.Spec.Hostname == "" || pod.Spec.Subdomain == "" {
		return "", fmt.Errorf("pod %s/%s has no stable DNS name (spec.hostname and spec.subdomain); "+
			"Vault must run as a StatefulSet with a headless Service", pod.Namespace, pod.Name)
	}

	host := strings.Join([]string{pod.Spec.Hostname, pod.Spec.Subdomain, pod.Namespace, "svc"}, ".")
	if port := u.Port(); port != "" {
		host = net.JoinHostPort(host, port)
	}

	return (&url.URL{Scheme: u.Scheme, Host: host}).String(), nil
}

// bridgeOptions describes waltr to the bridge commands and helpers.
func bridgeOptions(s *app.State) bridgecmd.Options {
	return bridgecmd.Options{
		CLI:     app.Name,
		Version: s.Stamp.Version,
		Kube:    s.KubeClient,
		Log:     s.Log,
	}
}

// environmentOf reads the --environment root flag, which keys the stored credentials.
func environmentOf(cmd *cobra.Command) (core.Environment, error) {
	name, err := cmd.Flags().GetString("environment")
	if err != nil {
		return 0, err
	}

	return core.EnvFromString(name)
}
