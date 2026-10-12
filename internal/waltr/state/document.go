// Package state is the declarative half of waltr: a document describing the Vault configuration a
// cluster should have - secrets engines, auth methods, policies, Kubernetes auth roles and KV
// secrets - and the engine that brings a Vault in line with it.
//
// The document is the `vault:` section of waltr.yaml. It is decoded strictly, since a misspelled
// key in a description of desired state would otherwise be silently ignored, and validated before
// anything is changed. Applying it is idempotent: what already matches is left alone, nothing
// that isn't declared is ever deleted, and generated secret values are kept once they exist.
//
// The imperative commands - `waltr policy write`, `waltr role write` and the rest - build a
// document with a single item and apply it through the same engine, so running a command by hand
// and declaring the same thing in the file have the same effect.
package state

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Section is the key of the document in waltr.yaml.
const Section = "vault"

// Document is the declared Vault configuration.
type Document struct {
	// Mounts are the secrets engines, keyed by their path.
	Mounts map[string]Mount `yaml:"mounts,omitempty"`

	// Auth are the auth methods, keyed by their path.
	Auth map[string]AuthMethod `yaml:"auth,omitempty"`

	// PasswordPolicies are Vault password policies (HCL), keyed by name. Generated secret values
	// name one of them.
	PasswordPolicies map[string]string `yaml:"passwordPolicies,omitempty"`

	// Policies are ACL policies (HCL), keyed by name.
	Policies map[string]string `yaml:"policies,omitempty"`

	// Roles are Kubernetes auth roles, keyed by name.
	Roles map[string]Role `yaml:"roles,omitempty"`

	// Secrets are KV-v2 secrets, keyed by "<mount>/<path>", each a map of keys to values.
	Secrets map[string]map[string]Value `yaml:"secrets,omitempty"`
}

// Mount is a secrets engine.
type Mount struct {
	// Type is the engine type, e.g. kv-v2 or transit.
	Type string `yaml:"type"`

	// Description is shown in Vault's UI and API.
	Description string `yaml:"description,omitempty"`

	// Keys are the encryption keys to create in a transit engine.
	Keys []string `yaml:"keys,omitempty"`
}

// AuthMethod is an auth method.
type AuthMethod struct {
	// Type is the method type, e.g. kubernetes or oidc.
	Type string `yaml:"type"`

	// Description is shown in Vault's UI and API.
	Description string `yaml:"description,omitempty"`

	// Kubernetes configures a method of type kubernetes.
	Kubernetes *KubernetesConfig `yaml:"kubernetes,omitempty"`
}

// KubernetesConfig is how a kubernetes auth method reaches the Kubernetes API server.
type KubernetesConfig struct {
	// Host is the API server's address as Vault reaches it, e.g. https://kubernetes.default.svc.
	Host string `yaml:"host"`

	// CACertFile is a PEM file with the API server's CA, for a Vault running outside the
	// cluster. Inside it, Vault reads the CA from its own Pod.
	CACertFile string `yaml:"caCertFile,omitempty"`
}

// Role is a Kubernetes auth role.
type Role struct {
	// Auth is the path of the kubernetes auth method the role belongs to; "kubernetes" when empty.
	Auth string `yaml:"auth,omitempty"`

	// ServiceAccounts and Namespaces bind the role to these ServiceAccounts in these namespaces.
	ServiceAccounts []string `yaml:"serviceAccounts"`
	Namespaces      []string `yaml:"namespaces"`

	// Policies are attached to the tokens the role issues.
	Policies []string `yaml:"policies"`

	// Audience is the audience the ServiceAccount token must be issued for. Optional.
	Audience string `yaml:"audience,omitempty"`

	// TokenTTL and TokenPeriod are Go durations, e.g. 1h or 24h. Optional.
	TokenTTL    string `yaml:"tokenTTL,omitempty"`
	TokenPeriod string `yaml:"tokenPeriod,omitempty"`
}

// AuthPath is the role's auth method path.
func (r Role) AuthPath() string {
	if r.Auth == "" {
		return DefaultKubernetesAuth
	}

	return strings.Trim(r.Auth, "/")
}

// DefaultKubernetesAuth is the path roles belong to unless they name another.
const DefaultKubernetesAuth = "kubernetes"

// Value is the value of a key in a KV secret: a literal, or one generated from a password policy.
//
// In YAML a literal is a plain string, and a generated value a mapping:
//
//	username: admin
//	password: {generate: alphanumeric-password}
type Value struct {
	Literal  string
	Generate string
}

// Generated reports whether the value is generated rather than literal.
func (v Value) Generated() bool { return v.Generate != "" }

// UnmarshalYAML accepts a scalar literal or a {generate: <policy>} mapping.
func (v *Value) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*v = Value{Literal: node.Value}
		return nil
	case yaml.MappingNode:
		var m struct {
			Generate string `yaml:"generate"`
		}
		// the strict decoder doesn't reach into a custom unmarshaller, so check the keys here
		if len(node.Content) != 2 || node.Content[0].Value != "generate" {
			return fmt.Errorf("line %d: a generated value is {generate: <password policy>}", node.Line)
		}
		if err := node.Decode(&m); err != nil {
			return err
		}
		if m.Generate == "" {
			return fmt.Errorf("line %d: generate names no password policy", node.Line)
		}
		*v = Value{Generate: m.Generate}
		return nil
	default:
		return fmt.Errorf("line %d: a secret value is a string or {generate: <password policy>}", node.Line)
	}
}

// MarshalYAML renders the value the way it is written.
func (v Value) MarshalYAML() (any, error) {
	if v.Generated() {
		return map[string]string{"generate": v.Generate}, nil
	}

	return v.Literal, nil
}

// Validate reports every problem in the document, without contacting Vault.
//
// With references set, it also checks that what the document refers to is declared in it: the
// policies roles attach, the auth method roles belong to, the password policies generated values
// use, and the mounts secrets live in. A complete document - the one in waltr.yaml - must be
// self-contained; a single item built by an imperative command can refer to anything Vault has.
func (d Document) Validate(references bool) error {
	var errs []error
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	for _, path := range sortedKeys(d.Mounts) {
		m := d.Mounts[path]
		if cleanPath(path) == "" {
			fail("mounts: a mount needs a path")
		}
		if m.Type == "" {
			fail("mounts.%s: type is required", path)
		}
		if len(m.Keys) > 0 && m.Type != "transit" {
			fail("mounts.%s: keys only apply to a transit engine, not %s", path, m.Type)
		}
	}

	for _, path := range sortedKeys(d.Auth) {
		a := d.Auth[path]
		if cleanPath(path) == "" {
			fail("auth: an auth method needs a path")
		}
		if a.Type == "" {
			fail("auth.%s: type is required", path)
		}
		if a.Kubernetes != nil {
			if a.Type != "kubernetes" {
				fail("auth.%s: kubernetes configuration only applies to type kubernetes, not %s", path, a.Type)
			}
			if a.Kubernetes.Host == "" {
				fail("auth.%s.kubernetes: host is required", path)
			}
		}
	}

	for _, name := range sortedKeys(d.PasswordPolicies) {
		if strings.TrimSpace(d.PasswordPolicies[name]) == "" {
			fail("passwordPolicies.%s: the policy is empty", name)
		}
	}

	for _, name := range sortedKeys(d.Policies) {
		if strings.TrimSpace(d.Policies[name]) == "" {
			fail("policies.%s: the policy is empty", name)
		}
	}

	for _, name := range sortedKeys(d.Roles) {
		r := d.Roles[name]
		if len(r.ServiceAccounts) == 0 || len(r.Namespaces) == 0 {
			fail("roles.%s: serviceAccounts and namespaces are required", name)
		}
		if len(r.Policies) == 0 {
			fail("roles.%s: policies are required", name)
		}
		for field, value := range map[string]string{"tokenTTL": r.TokenTTL, "tokenPeriod": r.TokenPeriod} {
			if value == "" {
				continue
			}
			if _, err := time.ParseDuration(value); err != nil {
				fail("roles.%s.%s: %q is not a duration such as 1h or 24h", name, field, value)
			}
		}

		if !references {
			continue
		}
		if auth, ok := d.Auth[r.AuthPath()]; !ok {
			fail("roles.%s: the auth method %q is not declared under auth", name, r.AuthPath())
		} else if auth.Type != "kubernetes" {
			fail("roles.%s: the auth method %q is of type %s, not kubernetes", name, r.AuthPath(), auth.Type)
		}
		for _, p := range r.Policies {
			if _, ok := d.Policies[p]; !ok && p != "default" {
				fail("roles.%s: the policy %q is not declared under policies", name, p)
			}
		}
	}

	for _, path := range sortedKeys(d.Secrets) {
		mount, rest := SplitSecretPath(path)
		if mount == "" || rest == "" {
			fail("secrets.%s: the key is <mount>/<path>, e.g. kv/app/config", path)
		}
		if len(d.Secrets[path]) == 0 {
			fail("secrets.%s: the secret has no keys", path)
		}

		if !references {
			continue
		}
		if m, ok := d.Mounts[mount]; !ok {
			fail("secrets.%s: the mount %q is not declared under mounts", path, mount)
		} else if m.Type != "kv-v2" {
			fail("secrets.%s: the mount %q is of type %s, not kv-v2", path, mount, m.Type)
		}
		for _, key := range sortedKeys(d.Secrets[path]) {
			v := d.Secrets[path][key]
			if _, ok := d.PasswordPolicies[v.Generate]; v.Generated() && !ok {
				fail("secrets.%s.%s: the password policy %q is not declared under passwordPolicies", path, key, v.Generate)
			}
		}
	}

	return errors.Join(errs...)
}

// Empty reports whether the document declares nothing.
func (d Document) Empty() bool {
	return len(d.Mounts) == 0 && len(d.Auth) == 0 && len(d.PasswordPolicies) == 0 &&
		len(d.Policies) == 0 && len(d.Roles) == 0 && len(d.Secrets) == 0
}

// SplitSecretPath splits "<mount>/<path>" at its first slash.
func SplitSecretPath(path string) (mount, rest string) {
	mount, rest, _ = strings.Cut(strings.Trim(path, "/"), "/")
	return mount, rest
}

func cleanPath(path string) string {
	return strings.Trim(path, "/")
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return keys
}
