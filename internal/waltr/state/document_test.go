package state

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func decode(t *testing.T, src string) Document {
	t.Helper()

	var doc Document
	require.NoError(t, yaml.Unmarshal([]byte(src), &doc))

	return doc
}

const valid = `
mounts:
  kv: {type: kv-v2}
  transit: {type: transit, keys: [cache]}
auth:
  kubernetes:
    type: kubernetes
    kubernetes: {host: https://kubernetes.default.svc}
passwordPolicies:
  strong: length = 32
policies:
  app: path "kv/data/app/*" { capabilities = ["read"] }
roles:
  app:
    serviceAccounts: [app]
    namespaces: [apps]
    policies: [app, default]
    tokenPeriod: 24h
secrets:
  kv/app/config:
    username: admin
    password: {generate: strong}
`

func TestValidDocument(t *testing.T) {
	doc := decode(t, valid)
	require.NoError(t, doc.Validate(true))

	assert.Equal(t, Value{Literal: "admin"}, doc.Secrets["kv/app/config"]["username"])
	assert.Equal(t, Value{Generate: "strong"}, doc.Secrets["kv/app/config"]["password"])
	assert.False(t, doc.Empty())
}

func TestReferencesMustBeDeclared(t *testing.T) {
	cases := map[string]struct {
		from, to string
		want     string
	}{
		"undeclared policy":          {"policies: [app, default]", "policies: [app, missing]", `the policy "missing"`},
		"undeclared auth method":     {"tokenPeriod: 24h", "tokenPeriod: 24h\n    auth: elsewhere", `the auth method "elsewhere"`},
		"undeclared password policy": {"{generate: strong}", "{generate: weak}", `the password policy "weak"`},
		"undeclared mount":           {"kv/app/config:", "other/app/config:", `the mount "other"`},
		"secret in a non-KV mount":   {"kv/app/config:", "transit/app/config:", "not kv-v2"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			doc := decode(t, replace(t, valid, c.from, c.to))

			err := doc.Validate(true)
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)

			assert.NoError(t, doc.Validate(false), "a single object built by a command may refer to anything Vault has")
		})
	}
}

func TestMalformedValues(t *testing.T) {
	cases := map[string]struct {
		from, to string
		want     string
	}{
		"transit keys on kv":   {"kv: {type: kv-v2}", "kv: {type: kv-v2, keys: [x]}", "keys only apply to a transit engine"},
		"mount without type":   {"kv: {type: kv-v2}", "kv: {description: x}", "type is required"},
		"bad duration":         {"tokenPeriod: 24h", "tokenPeriod: a day", "is not a duration"},
		"role without policy":  {"policies: [app, default]", "policies: []", "policies are required"},
		"kubernetes on oidc":   {"type: kubernetes\n", "type: oidc\n", "only applies to type kubernetes"},
		"secret without mount": {"kv/app/config:", "config:", "the key is <mount>/<path>"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := decode(t, replace(t, valid, c.from, c.to)).Validate(false)
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
		})
	}
}

func TestValueSyntax(t *testing.T) {
	var bad Document
	err := yaml.Unmarshal([]byte("secrets:\n  kv/a:\n    password: {generator: strong}\n"), &bad)
	assert.ErrorContains(t, err, "{generate: <password policy>}", "a misspelled generate must not pass as a literal")

	err = yaml.Unmarshal([]byte("secrets:\n  kv/a:\n    password: [a, b]\n"), &bad)
	assert.ErrorContains(t, err, "a string or {generate")
}

func TestSplitSecretPath(t *testing.T) {
	mount, rest := SplitSecretPath("kv/app/config")
	assert.Equal(t, "kv", mount)
	assert.Equal(t, "app/config", rest)

	mount, rest = SplitSecretPath("/kv/x/")
	assert.Equal(t, "kv", mount)
	assert.Equal(t, "x", rest)
}

func replace(t *testing.T, s, old, new string) string {
	t.Helper()

	require.Contains(t, s, old)
	return strings.Replace(s, old, new, 1)
}
