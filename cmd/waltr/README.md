# waltr

Bootstrap and configure HashiCorp Vault on Kubernetes.

`waltr` takes a freshly installed Vault - typically from the official Helm chart - to a usable
state: it initializes it and unseals every Pod, then brings Vault's configuration in line with what
you declare in `waltr.yaml`: secrets engines, auth methods, password and ACL policies, Kubernetes
auth roles and the KV secrets applications read at startup, with generated passwords. Everything can
be run again; what already matches is left alone, and nothing you haven't declared is deleted.

The same configuration can be changed one object at a time from the command line - `waltr policy
write`, `waltr role write` and so on - for an administrator at a shell, or declared in full and
applied at once with `waltr apply`, for a file kept in Git or rendered by a tool such as Ansible.
Both go through the same engine and have the same effect.

It talks to Vault's HTTP API. It does not need the `vault` CLI, but it reads the same environment
variables, so a shell set up for one works for the other.

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [Connecting to Vault](#connecting-to-vault)
- [The root token and keys](#the-root-token-and-keys)
- [Commands](#commands)
  - [`init`](#init)
  - [`validate`](#validate)
  - [`apply`](#apply)
  - [Changing one object](#changing-one-object)
  - [`proxy`](#proxy)
- [Configuration](#configuration)
- [The Vault state](#the-vault-state)
- [A full bootstrap](#a-full-bootstrap)
- [Built-in assumptions](#built-in-assumptions)

## Install

```sh
# From the repository root.
go build -o waltr ./cmd/waltr

# Or with Bazel, which is what CI uses.
bazel build //cmd/waltr
```

## Quick start

Vault from the official chart in the namespace `vault`, serving TLS with a certificate from a
private CA, and a workstation with `kubectl` access to the cluster:

```sh
# 1. Once per cluster: deploy the bridge, so waltr can reach cluster-internal names.
waltr proxy setup --network-policy --allow-namespace vault

# 2. Tell waltr where Vault is. These are the vault CLI's own variables.
export VAULT_ADDR=https://vault.vault.svc:8200
export VAULT_CACERT=/etc/ssl/private-ca.pem

# 3. Initialize and unseal every Pod. The root token and unseal keys are stored locally
#    for the environment, `dev` unless -e says otherwise.
waltr init --namespace vault

# 4. Start from the example configuration, adapt it, and apply it.
cp waltr.example.yaml waltr.yaml
waltr validate
waltr apply --dry-run
waltr apply
```

[`waltr.example.yaml`](waltr.example.yaml) is a production-ready starting point: a KV-v2 and a
transit engine, Kubernetes authentication, password policies, an administrator policy, and the
policy, role and secrets of one example application.

Nothing after step 3 needs a token on the command line: the other commands use the root token
`init` stored. See [The root token and keys](#the-root-token-and-keys) before using this on
anything you care about.

## Connecting to Vault

Every command that talks to Vault takes the same four flags. Each also reads the environment
variable the `vault` CLI uses:

| Flag                         | Variable            | Meaning                                              |
| ---------------------------- | ------------------- | ---------------------------------------------------- |
| `--vault-addr`               | `VAULT_ADDR`        | Vault's address, e.g. `https://vault.vault.svc:8200` |
| `-t`, `--token`              | `VAULT_TOKEN`       | the token (not on `init`, which needs none)          |
| `--ca-cert`                  | `VAULT_CACERT`      | a PEM CA certificate to trust; repeatable            |
| `--insecure-skip-tls-verify` | `VAULT_SKIP_VERIFY` | don't verify Vault's certificate                     |

There is no default address; a command without one fails and says how to set it.

TLS is verified by default: against the system trust store, or, when `--ca-cert` is given, against
the certificates in those files. Each file must hold at least one PEM certificate, or the command
stops before connecting. Verification is
against the host name in the address, so the certificate must carry that name - for example
`vault.vault.svc`, and for `init` also each Pod's own name (see [`init`](#init)). If your
certificate only names the public Ingress host, `--insecure-skip-tls-verify` gets you through, at
the cost that anyone on the path can read the token.

Each request times out after 60 seconds.

### Reaching cluster-internal addresses

Addresses ending in `.svc` or `.cluster.local` are reached through the gopskit bridge, a small
SOCKS5 server in the cluster, over a port-forward that each command opens and closes itself. Set it
up once with [`waltr proxy setup`](#proxy). `--via` changes the choice:

- `auto` (default): the bridge for `*.svc` and `*.cluster.local`, directly otherwise
- `direct`: always directly, e.g. to an Ingress
- `bridge`: always through the bridge

`--bridge-namespace` (default `gopskit-bridge`) says where the bridge runs. Using the bridge needs
Kubernetes access; reaching Vault directly does not.

### Kubernetes access

`--kubeconfig` and `--context` select the cluster. Without them, the rules `kubectl` uses apply:
`$KUBECONFIG`, then `~/.kube/config`, and its current context.

Only `init` (which finds and execs into the Vault Pods), the `proxy` commands and anything going
through the bridge touch the cluster. `apply` and the other configuration commands against a
directly reachable Vault run without a kubeconfig, and `validate` contacts nothing at all.

## The root token and keys

When `init` initializes Vault it writes the root token and the unseal keys - or, with
`--high-availability`, the recovery keys - to a JSON file:

```text
<user cache dir>/waltr/<environment>/vault-credentials.json
```

On Linux that is `~/.cache/waltr/dev/vault-credentials.json` (or under `$XDG_CACHE_HOME`); on macOS
`~/Library/Caches/waltr/dev/vault-credentials.json`. The file is created with mode `0600`. Its
contents:

```json
{
  "created": "2026-10-12T10:00:00Z",
  "keys": ["..."],
  "keys_base64": ["..."],
  "token": "hvs...."
}
```

With `--high-availability` the keys are under `recovery_keys` and `recovery_keys_base64` instead.

`-e`, `--environment` (`dev`, `stage` or `prod`; default `dev`) selects which file, so one
workstation can hold the credentials of several clusters. `--credential-file` names a file
explicitly instead; when it is set, `--environment` no longer affects the path.

Every other command finds its token in this order:

1. `--token`
2. `$VAULT_TOKEN`
3. `token` in the configuration file
4. the root token in the credentials file for `--environment` (or `--credential-file`)

The credentials file is the only copy of the unseal (or recovery) keys `waltr` keeps. Back it up somewhere
safer than a laptop's cache directory, and consider revoking the root token once you have other
ways in.

`init --secret-file` additionally writes the root token into a values file encrypted with the
[Helm secrets plugin](https://github.com/jkroepke/helm-secrets), under `secrets.vault.token`. The
file must already exist; `waltr` decrypts it with `helm secrets decrypt -i`, merges the value in
and encrypts it again with `helm secrets encrypt -i`. If that fails, `init` logs the error and
carries on - the token is still in the credentials file.

## Commands

All of them can be run again safely. Each one compares what Vault has with what is asked for, and
changes only what differs.

### `init`

```sh
waltr init --namespace vault
```

`initialize`, with the alias `init`, initializes Vault and unseals every Vault Pod.

1. It lists the Pods matching `-l`, `--label` (default `app.kubernetes.io/name=vault`) in
   `-n`, `--namespace`, or in the whole cluster when no namespace is given. Pods found in more
   than one namespace are an error; set `--namespace`.
2. It waits for each Pod to be `Running`, and replaces `/home/vault/.ash_history` in it with a
   link to `/dev/null`, so commands typed in `kubectl exec` sessions aren't kept. A failure here is
   logged and does not stop `init`.
3. It decides whether unsealing is needed. If the Pods mount a ConfigMap as the volume `config` -
   which the official chart does - and that ConfigMap's `extraconfig-from-values.hcl` contains a
   `seal` stanza, Vault auto-unseals and the unseal steps are skipped.
4. It picks the leader: the Pod that also carries `vault-active=true`, or, without one, the Pod
   whose name ends in `-0`.
5. If Vault isn't initialized, it initializes it through the leader with `--shares` key shares
   (default 7) and a `--threshold` (default 4), and stores the root token and keys (see
   [above](#the-root-token-and-keys)). If it already is, it reads the stored credentials instead.
6. It unseals each sealed Pod with the first `--threshold` keys, and checks that it is unsealed
   afterwards. Pods that are already unsealed are skipped.

`init` reaches each Pod at its own DNS name, built from the Pod's `spec.hostname` and
`spec.subdomain`, with the scheme and port of `--vault-addr`:

```text
--vault-addr https://vault.vault.svc:8200   →   https://vault-0.vault-internal.vault.svc:8200
```

So `--vault-addr` is required here even though no request goes to that address, and Vault must run
as a StatefulSet with a headless Service, as the official chart does. These names resolve even
while a Pod is not ready, which unsealing depends on. Vault's certificate must cover them too -
for example with `*.vault-internal.vault.svc` - or use `--insecure-skip-tls-verify`.

Running `init` again after Pods restart unseals them with the stored keys. If Vault is initialized
but there are no stored credentials for the environment, `init` stops: it cannot unseal without
keys it never saw.

| Flag                  | Default | Meaning                                                                                  |
| --------------------- | ------- | ---------------------------------------------------------------------------------------- |
| `--shares`            | `7`     | number of key shares Vault issues                                                        |
| `--threshold`         | `4`     | number of shares needed to unseal; also how many `init` submits                          |
| `--high-availability` | off     | initialize with recovery keys instead of unseal keys, as Vault requires with auto-unseal |
| `--secret-file`       |         | a Helm secrets file to add the root token to                                             |

`--threshold` must match the value Vault was initialized with: an unseal attempt that runs out of
keys while the Pod is still sealed fails with a hint to check it.

### `validate`

```sh
waltr validate
waltr validate --config cmd/waltr/waltr.example.yaml
```

Checks the `vault:` section of the configuration file without contacting Vault: that it decodes -
unknown and misspelled keys are errors, reported with their line - that every value is well-formed,
and that everything it refers to is declared in it: the policies roles attach, the auth method
roles belong to, the password policies generated values use, and the KV-v2 mounts secrets live in.
It needs no address, token or cluster, so it suits CI and pre-commit hooks.

### `apply`

```sh
waltr apply --dry-run
waltr apply
```

Brings Vault in line with the `vault:` section of the configuration file. The section is validated
first, as `validate` does; then `apply` reads what Vault has, plans the changes, and makes them in
this order, so that everything exists before something refers to it:

1. secrets engines (`mounts`), and the keys of transit engines
2. auth methods (`auth`), and the configuration of kubernetes ones
3. password policies (`passwordPolicies`)
4. ACL policies (`policies`)
5. Kubernetes auth roles (`roles`)
6. KV-v2 secrets (`secrets`)

Each change is logged as it is made. `--dry-run` prints the planned changes to standard output, one
per line, and changes nothing:

```text
create mount kv (kv-v2)
create transit key transit/vault-secrets-operator
update policy grafana
update role kubernetes/grafana (token_period)
update secret kv/grafana/admin (+password ~username)
```

In a secret's details, `+key` is a key that will be added and `~key` one whose value will change.
When Vault already matches, nothing is printed.

What `apply` does and does not do:

- **It never deletes.** A policy, role, secret or key removed from the file stays in Vault; delete
  it with the `vault` CLI.
- **It never remounts.** A secrets engine or auth method of another type at a declared path is an
  error, since replacing it would destroy its data or drop its roles. Descriptions are set when an
  engine or method is created and not changed afterwards.
- **Policies and roles are made to match.** An ACL or password policy whose text differs is
  rewritten; a role whose ServiceAccounts, namespaces, policies, audience, TTL or period differ is
  rewritten. A kubernetes auth method's host and CA are rewritten when they differ.
- **Literal secret values are made to match**, and keys a secret has but the file doesn't declare
  are kept - an application can store its own keys next to the declared ones.
- **Generated values are generated once.** A `{generate: <password policy>}` value is filled from
  the password policy when the key doesn't exist yet, and kept on every later run. `--rotate`
  replaces generated values that exist - which means the applications using the old ones have to
  be updated to match.
- **Transit keys are created, never replaced.** Rotating a key's material is Vault's own operation
  (`vault write -f transit/keys/<key>/rotate`).

| Flag        | Meaning                                            |
| ----------- | -------------------------------------------------- |
| `--dry-run` | print what would change, change nothing            |
| `--rotate`  | replace generated secret values that already exist |

### Changing one object

Each part of the `vault:` section has a command group. `apply` within a group applies only that
part of the file - the whole section is still validated:

```sh
waltr policy apply          # only the policies: section
waltr secret apply --rotate # only the secrets:, rotating generated values
```

The other commands in a group change a single object directly, without a configuration file. They
build the same object the file would declare and apply it through the same engine, so the result
is the same, and they take `--dry-run` too. Unlike in a file, the object may refer to anything Vault
already has, such as a policy written earlier.

| Command                                                                                      | Changes                                              |
| -------------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| `waltr mount enable PATH --type TYPE [--description D] [--key K]...`                         | a secrets engine, and a transit engine's keys        |
| `waltr auth enable PATH --type TYPE [--kubernetes-host URL] [--kubernetes-ca-cert FILE]`     | an auth method, and a kubernetes one's configuration |
| `waltr password-policy write NAME -f FILE`                                                   | a password policy, from an HCL file                  |
| `waltr policy write NAME -f FILE`                                                            | an ACL policy, from an HCL file                      |
| `waltr role write NAME --service-account SA --service-account-namespace NS --policy P [...]` | a Kubernetes auth role                               |
| `waltr secret write MOUNT/PATH [--set KEY=VALUE]... [--generate KEY=POLICY]... [--rotate]`   | keys of a KV-v2 secret                               |

`role write` also takes `--auth` (the auth method's path, default `kubernetes`), `--audience`,
`--token-ttl` and `--token-period` (durations such as `1h`). `--service-account`,
`--service-account-namespace` and `--policy` are repeatable. For example:

```sh
waltr mount enable kv --type kv-v2
waltr auth enable kubernetes --type kubernetes --kubernetes-host https://kubernetes.default.svc
waltr password-policy write alphanumeric -f alphanumeric.hcl
waltr policy write grafana -f grafana.hcl
waltr role write grafana --service-account grafana --service-account-namespace monitoring \
  --policy grafana --audience vault --token-period 24h
waltr secret write kv/grafana/admin --set username=admin --generate password=alphanumeric
```

has the same effect as declaring those objects in the `vault:` section - as in the example under
[The Vault state](#the-vault-state) - and running `waltr apply`.

### `proxy`

The bridge commands are shared by all gopskit CLIs; the bridge one of them deploys serves all of
them. In short:

```sh
# deploy or update it; safe to repeat
waltr proxy setup --network-policy --allow-namespace vault

# is it running, and can it reach Vault's Pods?
waltr proxy check --target vault-0.vault-internal.vault.svc:8200

# remove it
waltr proxy cleanup
```

`setup` deploys `ghcr.io/adnoctem/gopskit-bridge`, by default at the tag matching the CLI's own
release version (`latest` for development builds; `--image` overrides it), into
`--bridge-namespace`. `--network-policy` adds a NetworkPolicy allowing no ingress and egress only
to DNS and the `--allow-namespace` namespaces (all, if none are given); running `setup` without it
removes the policy again. `cleanup` deletes the namespace if `setup` created it, and otherwise only
the bridge's own resources.

The bridge's design and security model are in [`cmd/gopskit-bridge`](../gopskit-bridge/README.md).

## Configuration

Every flag can also be set in a YAML file. `--config` names it; otherwise the first of these that
exists is used:

1. `./waltr.yaml`
2. `~/.config/waltr/waltr.yaml` (the user configuration directory)
3. `/etc/waltr/waltr.yaml`

Keys are flag names without dashes. A mapping under a subcommand's name, or one of its aliases,
applies only to that subcommand; top-level keys apply to every command that has the flag. A value
on the command line wins over the environment variable, which wins over the file, which wins over
the default. Unknown keys are an error. The details are in
[configuring the CLIs](../../docs/configuration.md).

The same file holds the Vault state under `vault:`, described [below](#the-vault-state):

```yaml
# waltr.yaml
vault-addr: https://vault.vault.svc:8200
ca-cert: [/etc/ssl/private-ca.pem]
# insecure-skip-tls-verify: true   # if the certificate lacks the in-cluster names

environment: prod
namespace: vault # where `waltr init` looks for the Vault Pods
# credential-file: /secure/vault-credentials.json

initialize:
  threshold: 3
  shares: 5

vault:
  mounts:
    kv: { type: kv-v2 }
  # ...
```

The token is better kept in `$VAULT_TOKEN`, or left to the credentials file, than written into
`waltr.yaml`.

## The Vault state

The `vault:` section declares the configuration `apply` brings Vault in line with. Every part is
optional, and each is a mapping keyed by a path or name. It is decoded strictly: a key `waltr`
doesn't know is an error, not ignored. [`waltr.example.yaml`](waltr.example.yaml) shows all of it.

```yaml
vault:
  # Secrets engines, keyed by path.
  mounts:
    kv:
      type: kv-v2 # any engine type Vault knows
      description: Application secrets # optional; set when the engine is created
    transit:
      type: transit
      keys: [vault-secrets-operator] # encryption keys; transit engines only

  # Auth methods, keyed by path.
  auth:
    kubernetes:
      type: kubernetes
      description: Workloads authenticating with their ServiceAccount tokens
      kubernetes: # kubernetes methods only
        host: https://kubernetes.default.svc # the API server as Vault reaches it (required)
        caCertFile: /etc/ssl/kubernetes-ca.pem # optional; for a Vault outside the cluster
    oidc:
      type: oidc # enabled only; configure it with the vault CLI

  # Password policies, in HCL, keyed by name.
  passwordPolicies:
    alphanumeric: |
      length = 64
      rule "charset" {
        charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
      }

  # ACL policies, in HCL, keyed by name.
  policies:
    grafana: |
      path "kv/data/grafana/*" {
        capabilities = ["read"]
      }

  # Kubernetes auth roles, keyed by name.
  roles:
    grafana:
      auth: kubernetes # the auth method's path; default kubernetes
      serviceAccounts: [grafana] # required
      namespaces: [monitoring] # required
      policies: [grafana] # required; "default" needs no declaration
      audience: vault # optional
      tokenTTL: 1h # optional, a duration
      tokenPeriod: 24h # optional, a duration

  # KV-v2 secrets, keyed by <mount>/<path>.
  secrets:
    kv/grafana/admin:
      username: admin # a literal value
      password: { generate: alphanumeric } # generated from a password policy, once
```

Everything the section refers to must be declared in it: a role's auth method and policies (other
than `default`), a generated value's password policy, and a secret's mount, which must be of type
`kv-v2`. `waltr validate` checks this, and `apply` refuses a section that fails it.

## A full bootstrap

From a freshly installed chart to a configured Vault, with a `waltr.yaml` adapted from
[`waltr.example.yaml`](waltr.example.yaml) in the working directory:

```sh
# The bridge, allowed to reach Vault only.
waltr proxy setup --network-policy --allow-namespace vault
waltr proxy check --target vault.vault.svc:8200

# Initialize and unseal. Back up ~/.cache/waltr/prod/vault-credentials.json right away.
waltr init

# Engines, auth methods, policies, roles and secrets, as declared.
waltr validate
waltr apply --dry-run
waltr apply
```

To add an application later, declare its policy, role and secrets in the file and run `waltr apply`
again; only the new objects are created. After a restart of the Vault Pods, `waltr init` alone
unseals them again.

## Built-in assumptions

Everything about the Vault configuration comes from the file or the command line. What remains
built in concerns finding and unsealing the Vault Pods, and matches the official Helm chart:

| What                                    | Value                                              | Used by |
| --------------------------------------- | -------------------------------------------------- | ------- |
| Pod label                               | `app.kubernetes.io/name=vault` (`--label`)         | `init`  |
| Shell history file disabled in the Pods | `/home/vault/.ash_history`                         | `init`  |
| Seal detection                          | volume `config`, key `extraconfig-from-values.hcl` | `init`  |
