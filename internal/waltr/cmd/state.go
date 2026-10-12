package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/adnoctem/gopskit/internal/waltr/app"
	"github.com/adnoctem/gopskit/internal/waltr/state"
	"github.com/adnoctem/gopskit/pkg/config"
	"github.com/spf13/cobra"
)

// The configuration sections each command group manages: `waltr <group> apply` applies its own,
// `waltr apply` all of them.
const (
	sectionMounts           = "mounts"
	sectionAuth             = "auth"
	sectionPasswordPolicies = "passwordPolicies"
	sectionPolicies         = "policies"
	sectionRoles            = "roles"
	sectionSecrets          = "secrets"
)

// only returns the part of doc in section.
func only(doc state.Document, section string) state.Document {
	switch section {
	case sectionMounts:
		return state.Document{Mounts: doc.Mounts}
	case sectionAuth:
		return state.Document{Auth: doc.Auth}
	case sectionPasswordPolicies:
		return state.Document{PasswordPolicies: doc.PasswordPolicies}
	case sectionPolicies:
		return state.Document{Policies: doc.Policies}
	case sectionRoles:
		return state.Document{Roles: doc.Roles}
	case sectionSecrets:
		return state.Document{Secrets: doc.Secrets}
	default:
		return doc
	}
}

// planFlags are the flags of every command that changes Vault's configuration.
type planFlags struct {
	vf     vaultFlags
	dryRun bool
	rotate bool
}

func (f *planFlags) add(cmd *cobra.Command, withRotate bool) {
	f.vf.add(cmd, true)
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "Show what would change without changing anything")
	if withRotate {
		cmd.Flags().BoolVar(&f.rotate, "rotate", false, "Replace generated secret values that already exist")
	}
}

// run plans doc against Vault and applies it, or with --dry-run only shows the plan.
func (f *planFlags) run(cmd *cobra.Command, a *app.State, doc state.Document) error {
	environment, err := environmentOf(cmd)
	if err != nil {
		return err
	}

	release, err := f.vf.connect(cmd.Context(), a, f.vf.addr, environment)
	if err != nil {
		return err
	}
	defer release()

	changes, err := state.Plan(cmd.Context(), a.Vault, doc, state.Options{Rotate: f.rotate})
	if err != nil {
		return err
	}

	unchanged := 0
	for _, c := range changes {
		if c.Action == state.Unchanged {
			unchanged++
		}
	}

	if f.dryRun || !state.Pending(changes) {
		out := cmd.OutOrStdout()
		for _, c := range changes {
			if c.Action != state.Unchanged {
				_, _ = fmt.Fprintln(out, c)
			}
		}
		if f.dryRun {
			a.Log.Infof("dry run: %d changes, %d objects already as declared; nothing was changed", len(changes)-unchanged, unchanged)
		} else {
			a.Log.Infof("Vault is as declared: %d objects unchanged", unchanged)
		}
		return nil
	}

	if err := state.Apply(cmd.Context(), changes, func(c state.Change) { a.Log.Info(c.String()) }); err != nil {
		return err
	}

	a.Log.Infof("applied %d changes; %d objects were already as declared", len(changes)-unchanged, unchanged)
	return nil
}

// document reads and validates the `vault:` section of the configuration file. A complete
// document must be self-contained, so its references are checked too.
func document(cmd *cobra.Command) (state.Document, error) {
	var doc state.Document

	present, err := config.Section(cmd, state.Section, &doc)
	if err != nil {
		return doc, err
	}
	if !present {
		where := config.Path(cmd)
		if where == "" {
			return doc, errors.New("there is no configuration file; declare the Vault state in waltr.yaml's vault: section, or give --config")
		}
		return doc, fmt.Errorf("the configuration file %s has no vault: section", where)
	}

	if err := doc.Validate(true); err != nil {
		return doc, fmt.Errorf("the vault: section of %s is invalid:\n%w", config.Path(cmd), err)
	}

	return doc, nil
}

// NewApplyCommand creates `waltr apply`.
func NewApplyCommand(a *app.State) *cobra.Command {
	var f planFlags

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Bring Vault in line with the vault: section of the configuration file",
		Long: `Apply the Vault configuration declared in the vault: section of waltr.yaml: secrets engines,
auth methods, password and ACL policies, Kubernetes auth roles and KV secrets.

Applying is idempotent. What already matches is left alone, nothing that isn't declared is ever
deleted, and generated secret values are kept once they exist unless --rotate is given. A secrets
engine or auth method of a different type at a declared path is an error rather than remounted.

Each group - mount, auth, password-policy, policy, role and secret - also has its own apply, for
applying one section, and commands for changing a single object directly.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			doc, err := document(cmd)
			if err != nil {
				return err
			}

			return f.run(cmd, a, doc)
		},
	}

	f.add(cmd, true)
	return cmd
}

// NewValidateCommand creates `waltr validate`.
func NewValidateCommand(a *app.State) *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Check the vault: section of the configuration file without contacting Vault",
		Long: `Check the vault: section of waltr.yaml: that it decodes, that every value is well-formed, and that
everything it refers to - the policies roles attach, the password policies generated values use,
the mounts and auth methods - is declared in it. Nothing is contacted, so it suits CI.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			doc, err := document(cmd)
			if err != nil {
				return err
			}

			a.Log.Infof("%s is valid: %d mounts, %d auth methods, %d password policies, %d policies, %d roles, %d secrets",
				config.Path(cmd), len(doc.Mounts), len(doc.Auth), len(doc.PasswordPolicies), len(doc.Policies), len(doc.Roles), len(doc.Secrets))
			return nil
		},
	}
}

// groupCommand creates a resource group: its apply, applying the group's section of the
// configuration file, and the commands that change one object directly.
func groupCommand(a *app.State, use, short, section string, rotate bool, direct ...*cobra.Command) *cobra.Command {
	group := &cobra.Command{Use: use, Short: short}

	var f planFlags
	apply := &cobra.Command{
		Use:   "apply",
		Short: fmt.Sprintf("Apply the %s declared in the configuration file", section),
		Long: fmt.Sprintf(`Apply only the %s from the vault: section of waltr.yaml. The whole section is still
validated, including what the %s refer to. See 'waltr apply --help'.`, section, section),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			doc, err := document(cmd)
			if err != nil {
				return err
			}

			return f.run(cmd, a, only(doc, section))
		},
	}
	f.add(apply, rotate)

	group.AddCommand(append([]*cobra.Command{apply}, direct...)...)
	return group
}

// runSingle validates a document with one object, built by a direct command, and applies it.
// References aren't checked: a single object may refer to anything Vault already has.
func runSingle(cmd *cobra.Command, a *app.State, f *planFlags, doc state.Document) error {
	if err := doc.Validate(false); err != nil {
		return err
	}

	return f.run(cmd, a, doc)
}

// NewMountCommand creates `waltr mount`.
func NewMountCommand(a *app.State) *cobra.Command {
	var (
		f     planFlags
		mount state.Mount
	)

	enable := &cobra.Command{
		Use:   "enable PATH",
		Short: "Enable a secrets engine",
		Example: `  waltr mount enable kv --type kv-v2
  waltr mount enable transit --type transit --key vault-secrets-operator`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSingle(cmd, a, &f, state.Document{Mounts: map[string]state.Mount{args[0]: mount}})
		},
	}
	f.add(enable, false)
	enable.Flags().StringVar(&mount.Type, "type", "", "The engine type, e.g. kv-v2 or transit (required)")
	enable.Flags().StringVar(&mount.Description, "description", "", "A description")
	enable.Flags().StringSliceVar(&mount.Keys, "key", nil, "An encryption key to create in a transit engine (repeatable)")
	_ = enable.MarkFlagRequired("type")

	return groupCommand(a, "mount", "Manage secrets engines", sectionMounts, false, enable)
}

// NewAuthCommand creates `waltr auth`.
func NewAuthCommand(a *app.State) *cobra.Command {
	var (
		f          planFlags
		method     state.AuthMethod
		kubernetes state.KubernetesConfig
	)

	enable := &cobra.Command{
		Use:     "enable PATH",
		Short:   "Enable an auth method, and configure a kubernetes one",
		Example: `  waltr auth enable kubernetes --type kubernetes --kubernetes-host https://kubernetes.default.svc`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if kubernetes.Host != "" || kubernetes.CACertFile != "" {
				k := kubernetes
				method.Kubernetes = &k
			}
			return runSingle(cmd, a, &f, state.Document{Auth: map[string]state.AuthMethod{args[0]: method}})
		},
	}
	f.add(enable, false)
	enable.Flags().StringVar(&method.Type, "type", "", "The method type, e.g. kubernetes or oidc (required)")
	enable.Flags().StringVar(&method.Description, "description", "", "A description")
	enable.Flags().StringVar(&kubernetes.Host, "kubernetes-host", "",
		"For type kubernetes: the API server's address as Vault reaches it, e.g. https://kubernetes.default.svc")
	enable.Flags().StringVar(&kubernetes.CACertFile, "kubernetes-ca-cert", "",
		"For type kubernetes: a PEM file with the API server's CA, for a Vault outside the cluster")
	_ = enable.MarkFlagRequired("type")

	return groupCommand(a, "auth", "Manage auth methods", sectionAuth, false, enable)
}

// policyWriteCommand creates the `write` of the two policy groups.
func policyWriteCommand(a *app.State, kind string, doc func(name, policy string) state.Document) *cobra.Command {
	var (
		f    planFlags
		file string
	)

	write := &cobra.Command{
		Use:   "write NAME",
		Short: fmt.Sprintf("Write a %s from an HCL file", kind),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("--file: %w", err)
			}
			return runSingle(cmd, a, &f, doc(args[0], string(raw)))
		},
	}
	f.add(write, false)
	write.Flags().StringVarP(&file, "file", "f", "", fmt.Sprintf("The %s, in HCL (required)", kind))
	_ = write.MarkFlagRequired("file")

	return write
}

// NewPasswordPolicyCommand creates `waltr password-policy`.
func NewPasswordPolicyCommand(a *app.State) *cobra.Command {
	write := policyWriteCommand(a, "password policy", func(name, policy string) state.Document {
		return state.Document{PasswordPolicies: map[string]string{name: policy}}
	})

	return groupCommand(a, "password-policy", "Manage password policies, which generated secret values use",
		sectionPasswordPolicies, false, write)
}

// NewPolicyCommand creates `waltr policy`.
func NewPolicyCommand(a *app.State) *cobra.Command {
	write := policyWriteCommand(a, "ACL policy", func(name, policy string) state.Document {
		return state.Document{Policies: map[string]string{name: policy}}
	})

	return groupCommand(a, "policy", "Manage ACL policies", sectionPolicies, false, write)
}

// NewRoleCommand creates `waltr role`.
func NewRoleCommand(a *app.State) *cobra.Command {
	var (
		f    planFlags
		role state.Role
	)

	write := &cobra.Command{
		Use:   "write NAME",
		Short: "Write a Kubernetes auth role",
		Example: `  waltr role write grafana --service-account grafana --service-account-namespace monitoring \
    --policy grafana --audience vault --token-period 24h`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSingle(cmd, a, &f, state.Document{Roles: map[string]state.Role{args[0]: role}})
		},
	}
	f.add(write, false)
	fs := write.Flags()
	fs.StringVar(&role.Auth, "auth", state.DefaultKubernetesAuth, "The path of the kubernetes auth method")
	fs.StringSliceVar(&role.ServiceAccounts, "service-account", nil, "A ServiceAccount the role is bound to (repeatable, required)")
	fs.StringSliceVar(&role.Namespaces, "service-account-namespace", nil,
		"A namespace the role's ServiceAccounts are in (repeatable, required)")
	fs.StringSliceVar(&role.Policies, "policy", nil, "A policy attached to the role's tokens (repeatable, required)")
	fs.StringVar(&role.Audience, "audience", "", "The audience the ServiceAccount token must be issued for")
	fs.StringVar(&role.TokenTTL, "token-ttl", "", "The tokens' TTL, e.g. 1h")
	fs.StringVar(&role.TokenPeriod, "token-period", "", "The tokens' period, e.g. 24h")
	for _, required := range []string{"service-account", "service-account-namespace", "policy"} {
		_ = write.MarkFlagRequired(required)
	}

	return groupCommand(a, "role", "Manage Kubernetes auth roles", sectionRoles, false, write)
}

// NewSecretCommand creates `waltr secret`.
func NewSecretCommand(a *app.State) *cobra.Command {
	var (
		f         planFlags
		literals  []string
		generated []string
	)

	write := &cobra.Command{
		Use:   "write MOUNT/PATH",
		Short: "Write keys of a KV-v2 secret",
		Long: `Write keys of a KV-v2 secret, keeping the keys it doesn't name. --set gives a literal value;
--generate has Vault generate one from a password policy, once - an existing generated value is kept
unless --rotate is given.`,
		Example: `  waltr secret write kv/grafana/admin --set username=admin --generate password=alphanumeric`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			values := map[string]state.Value{}
			for _, kv := range literals {
				k, v, ok := strings.Cut(kv, "=")
				if !ok || k == "" {
					return fmt.Errorf("--set %q: expected KEY=VALUE", kv)
				}
				values[k] = state.Value{Literal: v}
			}
			for _, kv := range generated {
				k, policy, ok := strings.Cut(kv, "=")
				if !ok || k == "" || policy == "" {
					return fmt.Errorf("--generate %q: expected KEY=PASSWORD_POLICY", kv)
				}
				values[k] = state.Value{Generate: policy}
			}

			return runSingle(cmd, a, &f, state.Document{Secrets: map[string]map[string]state.Value{args[0]: values}})
		},
	}
	f.add(write, true)
	write.Flags().StringArrayVar(&literals, "set", nil, "A key with a literal value, KEY=VALUE (repeatable)")
	write.Flags().StringArrayVar(&generated, "generate", nil, "A key with a generated value, KEY=PASSWORD_POLICY (repeatable)")

	return groupCommand(a, "secret", "Manage KV-v2 secrets", sectionSecrets, true, write)
}
