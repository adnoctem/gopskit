package state

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	apivault "github.com/adnoctem/gopskit/pkg/api/vault"
	"github.com/hashicorp/vault-client-go/schema"
)

// Action is what applying a Change does.
type Action string

const (
	Create    Action = "create"
	Update    Action = "update"
	Unchanged Action = "unchanged"
)

// Change is one item of a plan: a declared object and what applying the plan does to it.
type Change struct {
	// Kind is the kind of object, e.g. "policy" or "secret".
	Kind string

	// Name identifies the object within its kind.
	Name string

	// Action is what applying does.
	Action Action

	// Detail says what an update changes, where that is more than the object as a whole.
	Detail string

	apply func(ctx context.Context) error
}

// String renders the change for output, e.g. "update policy keycloak (rewrite)".
func (c Change) String() string {
	s := fmt.Sprintf("%s %s %s", c.Action, c.Kind, c.Name)
	if c.Detail != "" {
		s += " (" + c.Detail + ")"
	}

	return s
}

// Options adjust a plan.
type Options struct {
	// Rotate replaces generated secret values that already exist.
	Rotate bool
}

// Plan compares doc with the Vault behind vc and returns a change per declared object, in the
// order they must be applied: engines and auth methods before what lives in them, password and ACL
// policies before the roles and secrets that use them. It changes nothing.
func Plan(ctx context.Context, vc *apivault.Client, doc Document, opts Options) ([]Change, error) {
	p := planner{ctx: ctx, vc: vc, doc: doc, opts: opts}

	for _, step := range []func() error{
		p.mounts, p.auth, p.passwordPolicies, p.policies, p.transitKeys, p.roles, p.secrets,
	} {
		if err := step(); err != nil {
			return nil, err
		}
	}

	return p.changes, nil
}

// Apply applies the changes of a plan in order, stopping at the first failure. report, which may
// be nil, is told about each change as it is applied.
func Apply(ctx context.Context, changes []Change, report func(Change)) error {
	for _, c := range changes {
		if c.Action == Unchanged {
			continue
		}

		if err := c.apply(ctx); err != nil {
			return fmt.Errorf("%s %s %s: %w", c.Action, c.Kind, c.Name, err)
		}
		if report != nil {
			report(c)
		}
	}

	return nil
}

// Pending reports whether a plan changes anything.
func Pending(changes []Change) bool {
	return slices.ContainsFunc(changes, func(c Change) bool { return c.Action != Unchanged })
}

type planner struct {
	ctx  context.Context
	vc   *apivault.Client
	doc  Document
	opts Options

	changes []Change

	// created records the mounts and auth methods this plan creates, so what lives in them is
	// known not to exist yet without asking Vault about paths that aren't there
	created map[string]bool
}

func (p *planner) add(c Change) {
	p.changes = append(p.changes, c)
}

func (p *planner) markCreated(path string) {
	if p.created == nil {
		p.created = map[string]bool{}
	}
	p.created[path] = true
}

func (p *planner) mounts() error {
	if len(p.doc.Mounts) == 0 {
		return nil
	}

	existing, err := p.vc.MountsListSecretsEngines(p.ctx)
	if err != nil {
		return fmt.Errorf("could not list the secrets engines: %w", err)
	}

	for _, path := range sortedKeys(p.doc.Mounts) {
		m, clean := p.doc.Mounts[path], cleanPath(path)

		if current, ok := existing[clean+"/"]; ok {
			if have := engineType(current); have != m.Type {
				return fmt.Errorf("the secrets engine at %s/ is of type %s, not %s - waltr never remounts an engine, which would destroy its data",
					clean, have, m.Type)
			}
			p.add(Change{Kind: "mount", Name: clean, Action: Unchanged})
			continue
		}

		p.markCreated("mount:" + clean)
		p.add(Change{Kind: "mount", Name: clean, Action: Create, Detail: m.Type, apply: func(ctx context.Context) error {
			req := schema.MountsEnableSecretsEngineRequest{Type: m.Type, Description: m.Description}
			if m.Type == "kv-v2" {
				req.Options = map[string]any{"version": "2"}
			}
			return p.vc.MountsEnableSecretsEngine(ctx, clean, req)
		}})
	}

	return nil
}

// engineType normalises a mount listing's type: Vault reports kv-v2 as type kv with version 2.
func engineType(listing any) string {
	m, _ := listing.(map[string]any)
	typ := fmt.Sprint(m["type"])

	if typ == "kv" {
		if opts, ok := m["options"].(map[string]any); ok && fmt.Sprint(opts["version"]) == "2" {
			return "kv-v2"
		}
	}

	return typ
}

func (p *planner) auth() error {
	if len(p.doc.Auth) == 0 {
		return nil
	}

	existing, err := p.vc.AuthListEnabledMethods(p.ctx)
	if err != nil {
		return fmt.Errorf("could not list the auth methods: %w", err)
	}

	for _, path := range sortedKeys(p.doc.Auth) {
		a, clean := p.doc.Auth[path], cleanPath(path)

		if current, ok := existing[clean+"/"]; ok {
			m, _ := current.(map[string]any)
			if have := fmt.Sprint(m["type"]); have != a.Type {
				return fmt.Errorf("the auth method at %s/ is of type %s, not %s - waltr never re-enables an auth method, which would drop its roles",
					clean, have, a.Type)
			}
			p.add(Change{Kind: "auth", Name: clean, Action: Unchanged})
		} else {
			p.markCreated("auth:" + clean)
			p.add(Change{Kind: "auth", Name: clean, Action: Create, Detail: a.Type, apply: func(ctx context.Context) error {
				return p.vc.AuthEnableMethod(ctx, clean, schema.AuthEnableMethodRequest{Type: a.Type, Description: a.Description})
			}})
		}

		if a.Kubernetes != nil {
			if err := p.kubernetesConfig(clean, *a.Kubernetes); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *planner) kubernetesConfig(path string, k KubernetesConfig) error {
	req := schema.KubernetesConfigureAuthRequest{KubernetesHost: k.Host}
	if k.CACertFile != "" {
		pem, err := os.ReadFile(k.CACertFile)
		if err != nil {
			return fmt.Errorf("auth.%s.kubernetes.caCertFile: %w", path, err)
		}
		req.KubernetesCaCert = string(pem)
	}

	write := func(ctx context.Context) error { return p.vc.KubernetesConfigureAuth(ctx, path, req) }

	if p.created["auth:"+path] {
		p.add(Change{Kind: "auth config", Name: path, Action: Create, apply: write})
		return nil
	}

	current, err := p.read("auth/" + path + "/config")
	if err != nil {
		return err
	}
	if current == nil || fmt.Sprint(current["kubernetes_host"]) == "" {
		p.add(Change{Kind: "auth config", Name: path, Action: Create, apply: write})
		return nil
	}

	var changed []string
	if fmt.Sprint(current["kubernetes_host"]) != k.Host {
		changed = append(changed, "host")
	}
	if k.CACertFile != "" && strings.TrimSpace(fmt.Sprint(current["kubernetes_ca_cert"])) != strings.TrimSpace(req.KubernetesCaCert) {
		changed = append(changed, "CA")
	}

	if len(changed) == 0 {
		p.add(Change{Kind: "auth config", Name: path, Action: Unchanged})
		return nil
	}
	p.add(Change{Kind: "auth config", Name: path, Action: Update, Detail: strings.Join(changed, ", "), apply: write})
	return nil
}

func (p *planner) passwordPolicies() error {
	for _, name := range sortedKeys(p.doc.PasswordPolicies) {
		policy := p.doc.PasswordPolicies[name]
		if err := p.policyText("password policy", "sys/policies/password/"+name, name, policy, func(ctx context.Context) error {
			return p.vc.PoliciesWritePasswordPolicy(ctx, name, policy)
		}); err != nil {
			return err
		}
	}

	return nil
}

func (p *planner) policies() error {
	for _, name := range sortedKeys(p.doc.Policies) {
		policy := p.doc.Policies[name]
		if err := p.policyText("policy", "sys/policies/acl/"+name, name, policy, func(ctx context.Context) error {
			return p.vc.PoliciesWriteAclPolicy(ctx, name, policy)
		}); err != nil {
			return err
		}
	}

	return nil
}

// policyText plans a password or ACL policy, which Vault returns as written.
func (p *planner) policyText(kind, path, name, want string, write func(context.Context) error) error {
	current, err := p.read(path)
	if err != nil {
		return err
	}

	switch {
	case current == nil:
		p.add(Change{Kind: kind, Name: name, Action: Create, apply: write})
	case strings.TrimSpace(fmt.Sprint(current["policy"])) == strings.TrimSpace(want):
		p.add(Change{Kind: kind, Name: name, Action: Unchanged})
	default:
		p.add(Change{Kind: kind, Name: name, Action: Update, apply: write})
	}

	return nil
}

func (p *planner) transitKeys() error {
	for _, path := range sortedKeys(p.doc.Mounts) {
		m, mount := p.doc.Mounts[path], cleanPath(path)

		for _, key := range m.Keys {
			name := mount + "/" + key
			create := func(ctx context.Context) error {
				return p.vc.Write(ctx, mount+"/keys/"+key, map[string]any{})
			}

			if p.created["mount:"+mount] {
				p.add(Change{Kind: "transit key", Name: name, Action: Create, apply: create})
				continue
			}

			current, err := p.read(mount + "/keys/" + key)
			if err != nil {
				return err
			}
			if current == nil {
				p.add(Change{Kind: "transit key", Name: name, Action: Create, apply: create})
				continue
			}
			// a key's material is never replaced: rotating it is Vault's own operation
			p.add(Change{Kind: "transit key", Name: name, Action: Unchanged})
		}
	}

	return nil
}

func (p *planner) roles() error {
	for _, name := range sortedKeys(p.doc.Roles) {
		r := p.doc.Roles[name]
		auth := r.AuthPath()
		path := "auth/" + auth + "/role/" + name
		id := auth + "/" + name

		want, err := roleBody(r)
		if err != nil {
			return fmt.Errorf("roles.%s: %w", name, err)
		}
		write := func(ctx context.Context) error { return p.vc.Write(ctx, path, want) }

		if p.created["auth:"+auth] {
			p.add(Change{Kind: "role", Name: id, Action: Create, apply: write})
			continue
		}

		current, err := p.read(path)
		if err != nil {
			return err
		}
		if current == nil {
			p.add(Change{Kind: "role", Name: id, Action: Create, apply: write})
			continue
		}

		if changed := roleDiff(current, want); len(changed) > 0 {
			p.add(Change{Kind: "role", Name: id, Action: Update, Detail: strings.Join(changed, ", "), apply: write})
			continue
		}
		p.add(Change{Kind: "role", Name: id, Action: Unchanged})
	}

	return nil
}

// roleBody is the API request for a role. Durations are sent in seconds.
func roleBody(r Role) (map[string]any, error) {
	body := map[string]any{
		"bound_service_account_names":      sortedCopy(r.ServiceAccounts),
		"bound_service_account_namespaces": sortedCopy(r.Namespaces),
		"token_policies":                   sortedCopy(r.Policies),
		"audience":                         r.Audience,
		"token_ttl":                        0,
		"token_period":                     0,
	}

	for field, value := range map[string]string{"token_ttl": r.TokenTTL, "token_period": r.TokenPeriod} {
		if value == "" {
			continue
		}
		d, err := time.ParseDuration(value)
		if err != nil {
			return nil, err
		}
		body[field] = int(d.Seconds())
	}

	return body, nil
}

// roleDiff names the fields of a role whose current value differs from the wanted one.
func roleDiff(current, want map[string]any) []string {
	var changed []string

	for _, field := range []string{"bound_service_account_names", "bound_service_account_namespaces", "token_policies"} {
		if !slices.Equal(sortedCopy(stringList(current[field])), want[field].([]string)) {
			changed = append(changed, field)
		}
	}
	if fmt.Sprint(nonNil(current["audience"])) != want["audience"] {
		changed = append(changed, "audience")
	}
	for _, field := range []string{"token_ttl", "token_period"} {
		have, _ := strconv.ParseFloat(fmt.Sprint(nonNil(current[field])), 64)
		if int(have) != want[field].(int) {
			changed = append(changed, field)
		}
	}

	return changed
}

func (p *planner) secrets() error {
	for _, path := range sortedKeys(p.doc.Secrets) {
		mount, rest := SplitSecretPath(path)
		declared := p.doc.Secrets[path]

		var current map[string]any
		if !p.created["mount:"+mount] {
			data, err := p.vc.KvV2Read(p.ctx, mount, rest)
			switch {
			case err == nil:
				current = data
			case !apivault.IsNotFound(err):
				return fmt.Errorf("could not read the secret %s: %w", path, err)
			}
		}

		var changed []string
		for _, key := range sortedKeys(declared) {
			v := declared[key]
			have, exists := current[key]

			switch {
			case !exists:
				changed = append(changed, "+"+key)
			case v.Generated() && p.opts.Rotate:
				changed = append(changed, "~"+key)
			case !v.Generated() && fmt.Sprint(have) != v.Literal:
				changed = append(changed, "~"+key)
			}
		}

		switch {
		case current == nil:
			p.add(Change{Kind: "secret", Name: path, Action: Create, apply: p.writeSecret(mount, rest, declared)})
		case len(changed) > 0:
			p.add(Change{Kind: "secret", Name: path, Action: Update, Detail: strings.Join(changed, " "),
				apply: p.writeSecret(mount, rest, declared)})
		default:
			p.add(Change{Kind: "secret", Name: path, Action: Unchanged})
		}
	}

	return nil
}

// writeSecret writes a secret's declared keys, keeping keys it doesn't declare and - unless
// rotating - generated values that already exist. It reads the secret again when applied, so the
// values it keeps are the current ones.
func (p *planner) writeSecret(mount, rest string, declared map[string]Value) func(context.Context) error {
	return func(ctx context.Context) error {
		data := map[string]any{}
		current, err := p.vc.KvV2Read(ctx, mount, rest)
		switch {
		case err == nil:
			maps.Copy(data, current)
		case !apivault.IsNotFound(err):
			return err
		}

		for _, key := range sortedKeys(declared) {
			v := declared[key]
			if !v.Generated() {
				data[key] = v.Literal
				continue
			}
			if _, exists := current[key]; exists && !p.opts.Rotate {
				continue
			}

			generated, err := p.vc.PoliciesGeneratePasswordFromPasswordPolicy(ctx, v.Generate)
			if err != nil {
				return fmt.Errorf("could not generate %s with the password policy %s: %w", key, v.Generate, err)
			}
			data[key] = generated
		}

		return p.vc.KvV2Write(ctx, mount, rest, data)
	}
}

// read reads a Vault path, returning nil for one that doesn't exist.
func (p *planner) read(path string) (map[string]any, error) {
	data, err := p.vc.Read(p.ctx, path)
	switch {
	case apivault.IsNotFound(err):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("could not read %s: %w", path, err)
	}

	return data, nil
}

func sortedCopy(s []string) []string {
	out := slices.Clone(s)
	if out == nil {
		out = []string{}
	}
	sort.Strings(out)

	return out
}

func stringList(v any) []string {
	items, _ := v.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, fmt.Sprint(item))
	}

	return out
}

func nonNil(v any) any {
	if v == nil {
		return ""
	}

	return v
}
