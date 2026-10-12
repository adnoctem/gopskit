package cmd

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/adnoctem/gopskit/internal/bridgecmd"
	"github.com/adnoctem/gopskit/internal/ssolo/app"
	"github.com/adnoctem/gopskit/pkg/api/keycloak"
	apivault "github.com/adnoctem/gopskit/pkg/api/vault"
	"github.com/adnoctem/gopskit/pkg/config"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ app.CLIOpt = NewClientCommand

// The keys an application's OIDC settings are stored under, in every output. They follow
// oauth2-proxy's naming, which many charts reuse.
const (
	keyClientID     = "client-id"
	keyClientSecret = "client-secret"
	keyIssuer       = "issuer"
)

// Outputs `client apply` can write the application's OIDC settings to.
const (
	outputStdout     = "stdout"
	outputKubernetes = "kubernetes"
	outputVault      = "vault"
)

var outputs = []string{outputStdout, outputKubernetes, outputVault}

// NewClientCommand creates the 'client' command group.
func NewClientCommand(ssolo *app.State) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "client",
		Short: "Manage the OIDC clients applications sign their users in with",
	}

	cmd.AddCommand(newClientApplyCommand(ssolo))
	return cmd
}

// clientOutput holds the flags deciding where an application's OIDC settings go.
type clientOutput struct {
	outputs []string
	format  string

	secretNamespace string
	secretName      string

	vaultAddr     string
	vaultToken    string
	vaultMount    string
	vaultPath     string
	vaultCACerts  []string
	vaultInsecure bool

	// conn reaches Vault; it is Keycloak's, so --via applies to both, host by host
	conn *bridgecmd.Connection
}

func newClientApplyCommand(ssolo *app.State) *cobra.Command {
	var (
		kf        keycloakFlags
		out       clientOutput
		spec      keycloak.OIDCClientSpec
		realm     string
		publicURL string
	)

	cmd := &cobra.Command{
		Use:     "apply CLIENT_ID",
		Aliases: []string{"create"},
		Short:   "Create or update an application's OIDC client",
		Long: `Create the OpenID Connect client an application signs its users in with, or bring an existing
one in line with the flags. Safe to run repeatedly: only the settings the flags cover change, and
an existing client keeps its secret.

The application's settings - client-id, client-secret and issuer - go to every --output given:

  stdout       printed, as KEY=value lines or JSON (--format)
  kubernetes   a Secret, --secret-name in --secret-namespace
  vault        a KV-v2 secret at --vault-path on --vault-mount

For example, for Vaultwarden:

  ssolo client apply vaultwarden --realm applications \
    --redirect-uri https://vault.example.org/identity/connect/oidc-signin \
    --output kubernetes --secret-namespace vaultwarden`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			spec.ClientID = args[0]

			if err := out.validate(spec.ClientID); err != nil {
				return err
			}

			if kf.url == "" {
				return errors.New("the URL of Keycloak is required: set --url, $KEYCLOAK_URL or url in the configuration file")
			}
			if publicURL == "" {
				publicURL = kf.url
			}
			issuer, err := issuerURL(publicURL, realm)
			if err != nil {
				return err
			}

			release, err := kf.connect(ctx, ssolo)
			if err != nil {
				return err
			}
			defer release()

			res, err := ssolo.Keycloak.ApplyOIDCClient(ctx, realm, spec)
			if err != nil {
				return err
			}
			if res.Created {
				ssolo.Log.Infof("created client %s in realm %s", res.ClientID, realm)
			} else {
				ssolo.Log.Infof("updated client %s in realm %s", res.ClientID, realm)
			}

			settings := map[string]string{keyClientID: res.ClientID, keyIssuer: issuer}
			if res.Secret != "" {
				settings[keyClientSecret] = res.Secret
			}

			return out.write(ctx, ssolo, cmd.OutOrStdout(), spec.ClientID, settings)
		},
	}

	kf.add(cmd)
	out.conn = &kf.conn

	fs := cmd.Flags()
	fs.StringVar(&realm, "realm", "applications", "The realm the client belongs to")
	fs.StringVar(&publicURL, "public-url", "",
		"Keycloak's URL as the application reaches it, which the issuer is derived from (default: --url)")
	fs.StringVar(&spec.Name, "name", "", "The client's display name")
	fs.StringVar(&spec.RootURL, "root-url", "", "The application's base URL")
	fs.StringSliceVar(&spec.RedirectURIs, "redirect-uri", nil, "Where Keycloak may send users after signing in (repeatable, required)")
	fs.StringSliceVar(&spec.PostLogoutRedirectURIs, "post-logout-redirect-uri", nil,
		"Where Keycloak may send users after signing out (repeatable; default: the redirect URIs)")
	fs.StringSliceVar(&spec.WebOrigins, "web-origin", nil, "Allowed CORS origins (repeatable; default: those of the redirect URIs)")
	fs.BoolVar(&spec.Public, "public", false, "Create a public client without a secret, which must use PKCE")

	fs.StringSliceVarP(&out.outputs, "output", "o", []string{outputStdout},
		"Where to write the settings: stdout, kubernetes, vault (repeatable)")
	fs.StringVar(&out.format, "format", "env", "The stdout format: env or json")
	fs.StringVar(&out.secretNamespace, "secret-namespace", "", "The namespace of the Secret --output kubernetes writes (required for it)")
	fs.StringVar(&out.secretName, "secret-name", "", "The name of that Secret (default: CLIENT_ID-oidc)")
	fs.StringVar(&out.vaultAddr, "vault-addr", "", "Vault's address (required for --output vault)")
	fs.StringVar(&out.vaultToken, "vault-token", "", "A Vault token allowed to read and write --vault-path (required for --output vault)")
	fs.StringVar(&out.vaultMount, "vault-mount", "kv", "The KV-v2 secrets engine's mount path")
	fs.StringVar(&out.vaultPath, "vault-path", "", "The secret's path within the mount (default: CLIENT_ID/credentials/oidc)")
	fs.StringSliceVar(&out.vaultCACerts, "vault-ca-cert", nil, "A PEM CA certificate to trust for Vault's TLS certificate (repeatable)")
	fs.BoolVar(&out.vaultInsecure, "vault-insecure-skip-tls-verify", false, "Don't verify Vault's TLS certificate. Insecure")

	config.BindEnv(fs, "vault-addr", "VAULT_ADDR")
	config.BindEnv(fs, "vault-token", "VAULT_TOKEN")

	_ = cmd.MarkFlagRequired("redirect-uri")

	return cmd
}

// issuerURL is the OIDC issuer of realm on the Keycloak at base.
func issuerURL(base, realm string) (string, error) {
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid Keycloak URL %q", base)
	}

	return strings.TrimSuffix(u.String(), "/") + "/realms/" + url.PathEscape(realm), nil
}

// validate checks the output flags before anything is changed in Keycloak, so a typo can't
// leave a client created whose secret went nowhere.
func (o *clientOutput) validate(clientID string) error {
	if len(o.outputs) == 0 {
		return errors.New("at least one --output is required")
	}

	for _, out := range o.outputs {
		if !slices.Contains(outputs, out) {
			return fmt.Errorf("invalid --output %q: must be one of %s", out, strings.Join(outputs, ", "))
		}
	}

	if o.wants(outputStdout) && o.format != "env" && o.format != "json" {
		return fmt.Errorf("invalid --format %q: must be env or json", o.format)
	}

	if o.wants(outputKubernetes) {
		if o.secretNamespace == "" {
			return errors.New("--output kubernetes requires --secret-namespace")
		}
		if o.secretName == "" {
			o.secretName = clientID + "-oidc"
		}
	}

	if o.wants(outputVault) {
		if o.vaultAddr == "" || o.vaultToken == "" {
			return errors.New("--output vault requires --vault-addr and --vault-token")
		}
		if o.vaultPath == "" {
			o.vaultPath = clientID + "/credentials/oidc"
		}
		// the Vault client exits the process on a bad CA file, so check them up front
		for _, ca := range o.vaultCACerts {
			raw, err := os.ReadFile(ca)
			if err != nil {
				return fmt.Errorf("--vault-ca-cert: %w", err)
			}
			if !x509.NewCertPool().AppendCertsFromPEM(raw) {
				return fmt.Errorf("--vault-ca-cert: %s holds no PEM certificate", ca)
			}
		}
	}

	return nil
}

func (o *clientOutput) wants(output string) bool {
	return slices.Contains(o.outputs, output)
}

// write sends settings to every requested output. All are attempted even if one fails, so a
// single unreachable destination doesn't hide whether the others got the secret.
func (o *clientOutput) write(ctx context.Context, s *app.State, stdout io.Writer, clientID string, settings map[string]string) error {
	var errs []error

	if o.wants(outputKubernetes) {
		if err := o.writeKubernetes(ctx, s, settings); err != nil {
			errs = append(errs, fmt.Errorf("kubernetes output: %w", err))
		}
	}

	if o.wants(outputVault) {
		if err := o.writeVault(ctx, s, settings); err != nil {
			errs = append(errs, fmt.Errorf("vault output: %w", err))
		}
	}

	if o.wants(outputStdout) {
		if err := o.writeStdout(stdout, settings); err != nil {
			errs = append(errs, fmt.Errorf("stdout output: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("client %s was applied, but not every output was written: %w", clientID, errors.Join(errs...))
	}

	return nil
}

func (o *clientOutput) writeStdout(w io.Writer, settings map[string]string) error {
	if o.format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(settings)
	}

	// KEY=value with shell-safe names, so the output can be eval'd or used as an env file
	keys := make([]string, 0, len(settings))
	for k := range settings {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	for _, k := range keys {
		name := "OIDC_" + strings.ToUpper(strings.ReplaceAll(k, "-", "_"))
		if _, err := fmt.Fprintf(w, "%s=%s\n", name, shellQuote(settings[k])); err != nil {
			return err
		}
	}

	return nil
}

func shellQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
}

func (o *clientOutput) writeKubernetes(ctx context.Context, s *app.State, settings map[string]string) error {
	kc, err := s.KubeClient()
	if err != nil {
		return err
	}

	secrets := kc.Client.CoreV1().Secrets(o.secretNamespace)
	labels := map[string]string{"app.kubernetes.io/managed-by": app.Name}

	have, err := secrets.Get(ctx, o.secretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err := secrets.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: o.secretName, Namespace: o.secretNamespace, Labels: labels},
			Type:       corev1.SecretTypeOpaque,
			StringData: settings,
		}, metav1.CreateOptions{})
		if err != nil {
			return err
		}

		s.Log.Infof("created Secret %s/%s", o.secretNamespace, o.secretName)
		return nil
	}
	if err != nil {
		return err
	}

	// other keys in an existing Secret, e.g. a cookie secret added by hand, are kept
	if have.Data == nil {
		have.Data = map[string][]byte{}
	}
	for k, v := range settings {
		have.Data[k] = []byte(v)
	}
	// a client turned public no longer has a secret; don't leave the old one behind
	if _, ok := settings[keyClientSecret]; !ok {
		delete(have.Data, keyClientSecret)
	}
	if have.Labels == nil {
		have.Labels = map[string]string{}
	}
	maps.Copy(have.Labels, labels)

	if _, err := secrets.Update(ctx, have, metav1.UpdateOptions{}); err != nil {
		return err
	}

	s.Log.Infof("updated Secret %s/%s", o.secretNamespace, o.secretName)
	return nil
}

func (o *clientOutput) writeVault(ctx context.Context, s *app.State, settings map[string]string) error {
	tr, release, err := o.conn.Transport(ctx, bridgeOptions(s), o.vaultAddr, nil)
	if err != nil {
		return err
	}
	defer release()

	opts := []apivault.ClientOpt{
		apivault.WithTransport(tr),
		apivault.WithTimeout(30 * time.Second),
		apivault.WithInsecureTLS(o.vaultInsecure),
	}
	if len(o.vaultCACerts) > 0 {
		opts = append(opts, apivault.WithCACerts(o.vaultCACerts...))
	}

	vc := apivault.New(o.vaultAddr, opts...)
	if err := vc.SetToken(&apivault.Credentials{Token: o.vaultToken}); err != nil {
		return err
	}

	// like the Kubernetes output, keep the other keys an existing secret holds
	data := map[string]any{}
	existing, err := vc.KvV2Read(ctx, o.vaultMount, o.vaultPath)
	switch {
	case err == nil:
		maps.Copy(data, existing)
	case !apivault.IsNotFound(err):
		return fmt.Errorf("could not read %s/%s: %w", o.vaultMount, o.vaultPath, err)
	}

	// a client turned public no longer has a secret; don't leave the old one behind
	if _, ok := settings[keyClientSecret]; !ok {
		delete(data, keyClientSecret)
	}
	for k, v := range settings {
		data[k] = v
	}

	if err := vc.KvV2Write(ctx, o.vaultMount, o.vaultPath, data); err != nil {
		return err
	}

	s.Log.Infof("wrote %s/%s in Vault", o.vaultMount, o.vaultPath)
	return nil
}
