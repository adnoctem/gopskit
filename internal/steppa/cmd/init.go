package cmd

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/adnoctem/gopskit/internal/bridgecmd"
	"github.com/adnoctem/gopskit/internal/steppa/app"
	apivault "github.com/adnoctem/gopskit/pkg/api/vault"
	"github.com/adnoctem/gopskit/pkg/config"
	"github.com/adnoctem/gopskit/pkg/smallstep"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ app.CLIOpt = NewInitCommand

// Outputs `init` can write the CA to.
const (
	outputValues     = "values"
	outputKubernetes = "kubernetes"
	outputVault      = "vault"
)

var outputs = []string{outputValues, outputKubernetes, outputVault}

type initFlags struct {
	opts       smallstep.Options
	outputs    []string
	release    string
	namespace  string
	valuesFile string
	rootBackup string

	includeRootKey bool
	force          bool

	vaultAddr     string
	vaultToken    string
	vaultMount    string
	vaultPath     string
	vaultCACerts  []string
	vaultInsecure bool
	conn          bridgecmd.Connection
}

// NewInitCommand creates the 'init' subcommand.
func NewInitCommand(steppa *app.State) *cobra.Command {
	var f initFlags

	cmd := &cobra.Command{
		Use:     "initialize",
		Aliases: []string{"init", "generate"},
		Short:   "Create a certificate authority for the step-certificates chart",
		Long: `Create a private certificate authority for step-ca - an EC P-256 root and intermediate CA, a JWK
provisioner and optionally the SSH host and user CA keys - as the smallstep/step-certificates
chart expects it. Every key is encrypted with its own password; passwords not given are generated.

The CA goes to every --output given:

  values       chart values (--values-file): with everything in them for the chart's "inject"
               mode - or, together with kubernetes, only the values switching the chart to its
               "existingSecrets" mode
  kubernetes   the Secrets the chart's "existingSecrets" mode reads, named for --release in
               --namespace
  vault        everything, including the root key and every password, as a KV-v2 secret

A CA is created once: running init again would create a different one and invalidate every
certificate the first one issued. So no output replaces an existing CA without --force.

step-ca never needs the root key, so it is kept out of the cluster unless --include-root-key is
given; it must then be kept elsewhere: in Vault, or in the --root-backup file.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInit(cmd.Context(), steppa, &f, cmd.OutOrStdout())
		},
	}

	fs := cmd.Flags()
	fs.StringVar(&f.opts.Name, "name", "", "The CA's name; its certificates are called '<name> Root CA' and '<name> Intermediate CA' (required)")
	fs.StringSliceVar(&f.opts.DNSNames, "dns", nil, "A DNS name step-ca is reached at; the first is the URL clients use (repeatable, required)")
	fs.StringVar(&f.opts.Address, "address", smallstep.DefaultAddress, "The address step-ca listens on")
	fs.StringVar(&f.opts.Provisioner, "provisioner", smallstep.DefaultProvisioner, "The name of the JWK provisioner")
	fs.BoolVar(&f.opts.SSH, "ssh", false, "Also create the SSH host and user CA keys and an SSHPOP provisioner")
	fs.BoolVar(&f.opts.ACME, "acme", false, "Also add an ACME provisioner")

	fs.StringVar(&f.opts.Passwords.Root, "root-password", "", "The root CA key's password (default: generated)")
	fs.StringVar(&f.opts.Passwords.CA, "password", "", "The intermediate CA key's password (default: generated)")
	fs.StringVar(&f.opts.Passwords.Provisioner, "provisioner-password", "", "The JWK provisioner key's password (default: generated)")
	fs.StringVar(&f.opts.Passwords.SSHHost, "ssh-host-password", "", "The SSH host CA key's password (default: generated)")
	fs.StringVar(&f.opts.Passwords.SSHUser, "ssh-user-password", "", "The SSH user CA key's password (default: generated)")

	fs.StringSliceVarP(&f.outputs, "output", "o", []string{outputValues}, "Where to write the CA: values, kubernetes, vault (repeatable)")
	fs.StringVar(&f.release, "release", smallstep.ChartName, "The chart's release name, which its Secrets are named for")
	fs.StringVar(&f.namespace, "namespace", "",
		"The release's namespace; also adds the chart Service's in-cluster names to --dns")
	fs.StringVar(&f.valuesFile, "values-file", "", "Where --output values writes the chart values; - for stdout (default: RELEASE-values.yaml)")
	fs.StringVar(&f.rootBackup, "root-backup", "",
		"A file to store the root certificate, its encrypted key and its password in - keep it offline")
	fs.BoolVar(&f.includeRootKey, "include-root-key", false, "Also put the encrypted root key into the cluster outputs")
	fs.BoolVar(&f.force, "force", false, "Replace an existing CA in the outputs, invalidating every certificate it issued")

	fs.StringVar(&f.vaultAddr, "vault-addr", "", "Vault's address for --output vault")
	fs.StringVar(&f.vaultToken, "vault-token", "", "A Vault token allowed to write --vault-path")
	fs.StringVar(&f.vaultMount, "vault-mount", "kv", "The KV-v2 secrets engine's mount path")
	fs.StringVar(&f.vaultPath, "vault-path", "", "The secret's path within the mount (default: RELEASE/ca)")
	fs.StringSliceVar(&f.vaultCACerts, "vault-ca-cert", nil, "A PEM CA certificate to trust for Vault's TLS certificate (repeatable)")
	fs.BoolVar(&f.vaultInsecure, "vault-insecure-skip-tls-verify", false, "Don't verify Vault's TLS certificate. Insecure")
	f.conn.AddFlags(fs)

	for flag, env := range map[string]string{
		"root-password":        "STEPPA_ROOT_PASSWORD",
		"password":             "STEPPA_PASSWORD",
		"provisioner-password": "STEPPA_PROVISIONER_PASSWORD",
		"ssh-host-password":    "STEPPA_SSH_HOST_PASSWORD",
		"ssh-user-password":    "STEPPA_SSH_USER_PASSWORD",
		"vault-addr":           "VAULT_ADDR",
		"vault-token":          "VAULT_TOKEN",
	} {
		config.BindEnv(fs, flag, env)
	}

	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("dns")

	return cmd
}

func runInit(ctx context.Context, s *app.State, f *initFlags, stdout io.Writer) error {
	if err := f.validate(); err != nil {
		return err
	}

	// refuse before generating anything, so an existing CA is never touched by accident
	if err := f.checkNothingExists(ctx, s); err != nil {
		return err
	}

	if f.namespace != "" {
		fullname := smallstep.Fullname(f.release)
		for _, name := range []string{
			fmt.Sprintf("%s.%s.svc.cluster.local", fullname, f.namespace),
			fmt.Sprintf("%s.%s.svc", fullname, f.namespace),
		} {
			if !slices.Contains(f.opts.DNSNames, name) {
				f.opts.DNSNames = append(f.opts.DNSNames, name)
			}
		}
	}

	pki, err := smallstep.Generate(f.opts)
	if err != nil {
		return err
	}
	s.Log.Infof("created the CA %q, root fingerprint %s", f.opts.Name, pki.Fingerprint)

	// the root backup first: if anything later fails, the root key is already safe
	if f.rootBackup != "" {
		if err := writeRootBackup(f.rootBackup, pki); err != nil {
			return err
		}
		s.Log.Infof("wrote the root backup to %s - keep it offline", f.rootBackup)
	}

	if f.wants(outputVault) {
		if err := f.writeVault(ctx, s, pki); err != nil {
			return fmt.Errorf("vault output: %w", err)
		}
	}

	if f.wants(outputKubernetes) {
		if err := f.writeSecrets(ctx, s, pki); err != nil {
			return fmt.Errorf("kubernetes output: %w", err)
		}
	}

	if f.wants(outputValues) {
		values := pki.HelmValues(f.includeRootKey)
		if f.wants(outputKubernetes) {
			values = pki.ExistingSecretsValues()
		}

		if err := writeValues(f.valuesFile, values, stdout); err != nil {
			return fmt.Errorf("values output: %w", err)
		}
		if f.valuesFile != "-" {
			s.Log.Infof("wrote the chart values to %s", f.valuesFile)
		}
	}

	s.Log.Infof("clients reach the CA at %s and pin the root fingerprint %s", pki.Defaults["ca-url"], pki.Fingerprint)
	return nil
}

func (f *initFlags) validate() error {
	if len(f.outputs) == 0 {
		return errors.New("at least one --output is required")
	}
	for _, out := range f.outputs {
		if !slices.Contains(outputs, out) {
			return fmt.Errorf("invalid --output %q: must be one of %s", out, strings.Join(outputs, ", "))
		}
	}

	if f.valuesFile == "" {
		f.valuesFile = f.release + "-values.yaml"
	}

	if f.wants(outputKubernetes) && f.namespace == "" {
		return errors.New("--output kubernetes requires --namespace")
	}

	if f.wants(outputVault) {
		if f.vaultAddr == "" || f.vaultToken == "" {
			return errors.New("--output vault requires --vault-addr and --vault-token")
		}
		if f.vaultPath == "" {
			f.vaultPath = f.release + "/ca"
		}
		// the Vault client exits the process on a bad CA file, so check them up front
		for _, ca := range f.vaultCACerts {
			raw, err := os.ReadFile(ca)
			if err != nil {
				return fmt.Errorf("--vault-ca-cert: %w", err)
			}
			if !x509.NewCertPool().AppendCertsFromPEM(raw) {
				return fmt.Errorf("--vault-ca-cert: %s holds no PEM certificate", ca)
			}
		}
	}

	if !f.includeRootKey && !f.wants(outputVault) && f.rootBackup == "" {
		return errors.New("the root CA key would not be kept anywhere, and without it no new intermediate CA " +
			"can ever be issued: use --output vault, --root-backup FILE or --include-root-key")
	}

	return nil
}

func (f *initFlags) wants(output string) bool {
	return slices.Contains(f.outputs, output)
}

// checkNothingExists fails if an output already holds a CA and --force wasn't given.
func (f *initFlags) checkNothingExists(ctx context.Context, s *app.State) error {
	if f.force {
		return nil
	}

	refuse := func(where string) error {
		return fmt.Errorf("%s already exists; replacing a CA invalidates every certificate it issued - "+
			"pass --force if that is what you want", where)
	}

	for _, path := range []string{f.rootBackup, f.valuesIfWritten()} {
		if path == "" || path == "-" {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			return refuse(path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	if f.wants(outputKubernetes) {
		kc, err := s.KubeClient()
		if err != nil {
			return err
		}

		name := smallstep.Fullname(f.release) + "-ca-password"
		_, err = kc.Client.CoreV1().Secrets(f.namespace).Get(ctx, name, metav1.GetOptions{})
		switch {
		case err == nil:
			return refuse(fmt.Sprintf("the Secret %s/%s", f.namespace, name))
		case !apierrors.IsNotFound(err):
			return fmt.Errorf("could not look up Secret %s/%s: %w", f.namespace, name, err)
		}
	}

	if f.wants(outputVault) {
		vc, release, err := f.vaultClient(ctx, s)
		if err != nil {
			return err
		}
		defer release()

		_, err = vc.KvV2Read(ctx, f.vaultMount, f.vaultPath)
		switch {
		case err == nil:
			return refuse(fmt.Sprintf("the Vault secret %s/%s", f.vaultMount, f.vaultPath))
		case !apivault.IsNotFound(err):
			return fmt.Errorf("could not look up the Vault secret %s/%s: %w", f.vaultMount, f.vaultPath, err)
		}
	}

	return nil
}

func (f *initFlags) valuesIfWritten() string {
	if f.wants(outputValues) {
		return f.valuesFile
	}

	return ""
}

func writeValues(path string, values map[string]any, stdout io.Writer) error {
	out, err := yaml.Marshal(values)
	if err != nil {
		return err
	}

	header := "# chart values for smallstep/step-certificates, generated by steppa\n"
	if path == "-" {
		_, err := fmt.Fprint(stdout, header+string(out))
		return err
	}

	// the inject values hold keys and passwords
	return os.WriteFile(path, []byte(header+string(out)), 0o600)
}

func writeRootBackup(path string, pki *smallstep.PKI) error {
	out, err := yaml.Marshal(map[string]string{
		"root_ca.crt":   string(pki.RootCert),
		"root_ca_key":   string(pki.RootKey),
		"root_password": pki.Options.Passwords.Root,
		"fingerprint":   pki.Fingerprint,
	})
	if err != nil {
		return err
	}

	return os.WriteFile(path, out, 0o600)
}

func (f *initFlags) writeSecrets(ctx context.Context, s *app.State, pki *smallstep.PKI) error {
	kc, err := s.KubeClient()
	if err != nil {
		return err
	}

	secrets, err := pki.Secrets(f.release, f.namespace, f.includeRootKey)
	if err != nil {
		return err
	}

	client := kc.Client.CoreV1().Secrets(f.namespace)
	for _, secret := range secrets {
		have, err := client.Get(ctx, secret.Name, metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(err):
			if _, err := client.Create(ctx, secret, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("could not create Secret %s/%s: %w", f.namespace, secret.Name, err)
			}
			s.Log.Infof("created Secret %s/%s", f.namespace, secret.Name)
		case err != nil:
			return fmt.Errorf("could not look up Secret %s/%s: %w", f.namespace, secret.Name, err)
		default:
			// only reached with --force
			have.Data, have.StringData, have.Labels = nil, secret.StringData, secret.Labels
			if _, err := client.Update(ctx, have, metav1.UpdateOptions{}); err != nil {
				return fmt.Errorf("could not replace Secret %s/%s: %w", f.namespace, secret.Name, err)
			}
			s.Log.Infof("replaced Secret %s/%s", f.namespace, secret.Name)
		}
	}

	return nil
}

func (f *initFlags) writeVault(ctx context.Context, s *app.State, pki *smallstep.PKI) error {
	vc, release, err := f.vaultClient(ctx, s)
	if err != nil {
		return err
	}
	defer release()

	data := map[string]any{}
	for k, v := range pki.Bundle() {
		data[k] = v
	}

	if err := vc.KvV2Write(ctx, f.vaultMount, f.vaultPath, data); err != nil {
		return err
	}

	s.Log.Infof("wrote the CA to %s/%s in Vault", f.vaultMount, f.vaultPath)
	return nil
}

func (f *initFlags) vaultClient(ctx context.Context, s *app.State) (*apivault.Client, func(), error) {
	tr, release, err := f.conn.Transport(ctx, bridgeOptions(s), f.vaultAddr, nil)
	if err != nil {
		return nil, nil, err
	}

	opts := []apivault.ClientOpt{
		apivault.WithTransport(tr),
		apivault.WithTimeout(30 * time.Second),
		apivault.WithInsecureTLS(f.vaultInsecure),
	}
	if len(f.vaultCACerts) > 0 {
		opts = append(opts, apivault.WithCACerts(f.vaultCACerts...))
	}

	vc := apivault.New(f.vaultAddr, opts...)
	if err := vc.SetToken(&apivault.Credentials{Token: f.vaultToken}); err != nil {
		release()
		return nil, nil, err
	}

	return vc, release, nil
}
