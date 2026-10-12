# waltr

Bootstrap and configure HashiCorp Vault on Kubernetes.

`waltr` takes a freshly installed Vault - typically from the official Helm chart - to a usable
state: it initializes it, unseals every Pod, mounts the authentication methods and secrets engines
the rest of the platform expects, and writes the policies, Kubernetes auth roles and generated
credentials that applications such as Keycloak and GitLab read at startup. Every step can be run
again; what already exists is left alone unless you ask otherwise.

It talks to Vault's HTTP API. It does not need the `vault` CLI, but it reads the same environment
variables, so a shell set up for one works for the other.

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [Connecting to Vault](#connecting-to-vault)
- [The root token and keys](#the-root-token-and-keys)
- [Commands](#commands)
  - [`init`](#init)
  - [`mounts`](#mounts)
  - [`configure`](#configure)
  - [`transit`](#transit)
  - [`prepare keycloak`](#prepare-keycloak)
  - [`prepare gitlab`](#prepare-gitlab)
  - [`proxy`](#proxy)
- [Configuration](#configuration)
- [A full bootstrap](#a-full-bootstrap)
- [Fixed names](#fixed-names)

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

# 4. Mount auth methods and secrets engines, then write policies.
waltr mounts
waltr configure
```

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
through the bridge touch the cluster. `mounts`, `configure`, `transit` and `prepare` against a
directly reachable Vault run without a kubeconfig.

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

All of them can be run again safely. Each one checks what exists before writing, logs what it
created and what it skipped, and creates only what is missing unless `--overwrite` says otherwise.

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

### `mounts`

```sh
waltr mounts
```

Alias `mount`. Enables, if not yet enabled:

| Path          | Type                     | Notes                                          |
| ------------- | ------------------------ | ---------------------------------------------- |
| `kubernetes/` | auth method `kubernetes` | configured right after enabling, see below     |
| `oidc/`       | auth method `oidc`       | enabled only; `waltr` doesn't configure it     |
| `kv/`         | secrets engine `kv-v2`   | where `configure` and `prepare` expect secrets |

The Kubernetes auth method is configured with `--kubernetes-host`, the API server address as
_Vault_ reaches it (default `https://kubernetes.default.svc`). For a Vault inside the cluster that
is all it needs: it uses the CA from its own Pod. For a Vault outside the cluster, give the API
server's CA with `--kubernetes-ca-cert <pem file>` and an address it can reach.

The configuration is written when `waltr` enables the method. On later runs it is left as it is,
unless `--overwrite` is given, which rewrites it from the flags - for example after the API
server's address or CA changed.

### `configure`

```sh
waltr configure
```

Aliases `conf` and `config`. Writes ACL policies and password policies.

One read-only ACL policy per application, named after it, each granting `read` on
`kv/data/<name>/*`:

```text
keycloak  awx  crowdsec  gitlab  gitlab-runner  harbor  headlamp  homepage  jenkins
kubescape  loki  matomo
```

The list is fixed; see [Fixed names](#fixed-names).

An ACL policy `admin`, which grants management of ACL policies (`sys/policies/acl/*`), auth methods
(`auth/*`, `sys/auth/*`), secrets engines (`sys/mounts/*`), identities (`identity/*`), the
`kv/*`, `transit/*` and `cubbyhole/*` paths, and `read` on `sys/health`.

Four password policies, used by `prepare` to generate secrets:

| Name                            | Length | Composition (at least)                                  |
| ------------------------------- | ------ | ------------------------------------------------------- |
| `alphanumeric-password`         | 64     | 16 lowercase, 16 uppercase, 16 digits                   |
| `alphanumeric-special-password` | 64     | 12 lowercase, 12 uppercase, 12 digits, 12 of `!@#$%^&*` |
| `s3-access-key`                 | 32     | 16 uppercase, 12 digits; uppercase and digits only      |
| `s3-secret-key`                 | 64     | 32 uppercase, 24 digits; uppercase and digits only      |

Existing policies are skipped. `--overwrite` rewrites the per-application policies; it does not
touch `admin` or the password policies.

### `transit`

```sh
waltr transit
```

Aliases `encryption` and `transit-encryption`. Prepares Vault for the Vault Secrets Operator's
encrypted client cache:

- the `transit` secrets engine at `transit/`
- the encryption key `transit/keys/vso-client-cache`
- an ACL policy `vso-auth`, granting `create` and `update` on `transit/encrypt/vso-client-cache` and
  `transit/decrypt/vso-client-cache` - what the operator does with the key
- a Kubernetes auth role `vso-auth`, bound to the ServiceAccount `vault-secrets-operator` in the
  namespace `vault-secrets-operator`, with audience `vault`, token period 120 seconds, no TTL, and
  the policies `vso-auth` plus every per-application policy `configure` writes

Run `mounts` and `configure` first: the role belongs to the `kubernetes/` auth method and refers to
their policies. `--overwrite` rewrites the policy and the role; the `transit/` engine itself is
never mounted again, which would fail while it is mounted and destroy its keys if it didn't.

### `prepare keycloak`

```sh
waltr prepare keycloak
```

Needs `mounts` and `configure` to have run: it writes to `kv/`, uses the `kubernetes/` auth method,
refers to the `keycloak` policy and generates passwords with `alphanumeric-password`. It creates:

- a Kubernetes auth role `keycloak`, bound to the ServiceAccount `keycloak` in the namespace
  `keycloak`, audience `vault`, token period 24h, no TTL, policy `keycloak`
- `kv/keycloak/config` with `username: mg` and a generated `password` - the admin account
- `kv/keycloak/credentials/postgresql` with `username: keycloak` and a generated `password`

The role's policy lets Keycloak read everything under `kv/keycloak/`. With the `vault` CLI:

```sh
vault kv get kv/keycloak/config
```

Existing roles and secrets are skipped. `--overwrite` rewrites them, which **replaces the stored
passwords with new ones** - the applications using the old ones have to be updated to match.

### `prepare gitlab`

```sh
waltr prepare gitlab
```

Same prerequisites as `prepare keycloak`, plus the `s3-access-key` and `s3-secret-key` password
policies. It creates:

- a Kubernetes auth role `gitlab`, bound to the ServiceAccount `gitlab` in the namespace `gitlab`,
  audience `vault`, token period 24h, no TTL, policy `gitlab`
- `kv/gitlab/config` with `username: mg` and a generated `password`
- `kv/gitlab/credentials/redis` with `username: redis` and a generated `password`
- `kv/gitlab/credentials/postgresql` with `username: gitlab` and a generated `password`
- `kv/gitlab/credentials/minio` with a generated `access_key` and `secret_key`

`--overwrite` behaves as for Keycloak, including the password rotation.

The roles in both `prepare` commands, and the one `transit` creates, set the audience `vault`: the
ServiceAccount token an application presents must be issued for that audience.

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

mounts:
  kubernetes-host: https://kubernetes.default.svc
```

The token is better kept in `$VAULT_TOKEN`, or left to the credentials file, than written into
`waltr.yaml`.

## A full bootstrap

From a freshly installed chart to Vault ready for the Secrets Operator, Keycloak and GitLab, with
the `waltr.yaml` above in the working directory:

```sh
# The bridge, allowed to reach Vault only.
waltr proxy setup --network-policy --allow-namespace vault
waltr proxy check --target vault.vault.svc:8200

# Initialize and unseal. Back up ~/.cache/waltr/prod/vault-credentials.json right away.
waltr init

# Auth methods, secrets engines and policies.
waltr mounts
waltr configure

# The Vault Secrets Operator's client cache.
waltr transit

# Application secrets and roles.
waltr prepare keycloak
waltr prepare gitlab
```

The `oidc/` auth method is enabled but not configured; point it at your identity provider with
the `vault` CLI or the API.

After a restart of the Vault Pods, `waltr init` alone unseals them again.

## Fixed names

Several values are built into `waltr` and are not configurable. On a cluster laid out differently,
check them before running the commands that use them.

| What                                    | Value                                                                                       | Used by                              |
| --------------------------------------- | ------------------------------------------------------------------------------------------- | ------------------------------------ |
| KV-v2 mount                             | `kv/`                                                                                       | `mounts`, `configure`, `prepare`     |
| Per-application policies                | the twelve names under [`configure`](#configure)                                            | `configure`, `transit`               |
| Admin username in `kv/<app>/config`     | `mg`                                                                                        | `prepare keycloak`, `prepare gitlab` |
| Database and Redis usernames            | `keycloak`, `gitlab`, `redis`                                                               | `prepare`                            |
| Role ServiceAccount and namespace       | `keycloak`/`keycloak`, `gitlab`/`gitlab`, `vault-secrets-operator`/`vault-secrets-operator` | `prepare`, `transit`                 |
| Role audience                           | `vault`                                                                                     | `prepare`, `transit`                 |
| Transit key                             | `transit/keys/vso-client-cache`                                                             | `transit`                            |
| Shell history file disabled in the Pods | `/home/vault/.ash_history`                                                                  | `init`                               |
| Seal detection                          | volume `config`, key `extraconfig-from-values.hcl`                                          | `init`                               |
