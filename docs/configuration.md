# Configuring the gopskit CLIs

Every flag of the gopskit CLIs - `carto`, `plattr`, `ssolo`, `steppa` and `waltr` - can also be set
through the environment (where noted) or a configuration file, so repetitive administration - or a
tool like Ansible - doesn't have to repeat the same flags on every call. Each CLI's own README in
`cmd/<cli>/` documents its flags; this page describes what they share.

## Where settings come from

For each flag, the first of these that sets it wins:

1. the command line
2. the flag's environment variable, where it has one (shown as `(env ...)` in `--help`)
3. the configuration file, the most specific setting first (see below)
4. the flag's default

## The configuration file

`--config` names it explicitly. Otherwise the first of these that exists is used:

1. `./<cli>.yaml` in the working directory
2. `<user config dir>/<cli>/<cli>.yaml`, e.g. `~/.config/ssolo/ssolo.yaml`
3. `/etc/<cli>/<cli>.yaml`

The keys are the flag names, without the dashes in front. A key whose value is a mapping and that
names a subcommand (or one of its aliases) scopes the settings within it to that subcommand.
Settings at the top level apply to every command that has the flag.

Unknown keys are an error rather than being ignored, so a typo surfaces immediately.

## Bootstrapping a cluster

The bridge lets the CLIs reach cluster-internal services such as `keycloak.keycloak.svc` through a
single port-forward. Set it up once, then point the CLIs at the services' real addresses:

```shell
ssolo proxy setup --network-policy --allow-namespace keycloak,vault
```

### `waltr.yaml`

```yaml
vault-addr: https://vault.vault.svc:8200
ca-cert: [/etc/ssl/private-ca.pem]
# insecure-skip-tls-verify: true   # if the certificate lacks the in-cluster names

namespace: vault # where `waltr init` finds the Vault Pods

initialize:
  threshold: 3
  shares: 5

# the Vault configuration `waltr apply` brings Vault in line with
vault:
  mounts:
    kv: { type: kv-v2 }
  auth:
    kubernetes:
      type: kubernetes
      kubernetes: { host: https://kubernetes.default.svc }
  # passwordPolicies, policies, roles, secrets: see cmd/waltr/waltr.example.yaml
```

```shell
waltr init          # initialises Vault and unseals every Pod at its own DNS name
waltr validate      # checks the vault: section without contacting Vault
waltr apply         # brings Vault in line with it
```

`waltr init` stores the root token for `--environment` (or at `--credential-file`); the other
commands use it unless `--token` or `$VAULT_TOKEN` gives another.

### `ssolo.yaml`

```yaml
url: https://keycloak.keycloak.svc:8443
credentials-secret: keycloak/keycloak-admin
ca-cert: [/etc/ssl/private-ca.pem]

initialize:
  realms: [operations, applications]
  groups: [admins, users]

client:
  apply:
    realm: applications
    public-url: https://sso.example.org
    output: [kubernetes, vault]
    vault-addr: https://vault.vault.svc:8200
    # vault-token: better from $VAULT_TOKEN than from a file
```

```shell
ssolo init
ssolo client apply vaultwarden \
  --redirect-uri https://vault.example.org/identity/connect/oidc-signin \
  --secret-namespace vaultwarden
```

## Environment variables

| CLI      | Flag                                                       | Variable                       |
| -------- | ---------------------------------------------------------- | ------------------------------ |
| `plattr` | `aws --access-key-id`                                      | `AWS_ACCESS_KEY_ID`            |
| `plattr` | `aws --secret-access-key`                                  | `AWS_SECRET_ACCESS_KEY`        |
| `plattr` | `cloudflare --token`                                       | `CLOUDFLARE_API_TOKEN`         |
| `plattr` | `hetzner-encryption`, `longhorn-encryption` `--passphrase` | `PLATTR_ENCRYPTION_PASSPHRASE` |
| `ssolo`  | `--url`                                                    | `KEYCLOAK_URL`                 |
| `ssolo`  | `--username`                                               | `KEYCLOAK_ADMIN`               |
| `ssolo`  | `--password`                                               | `KEYCLOAK_ADMIN_PASSWORD`      |
| `ssolo`  | `client apply --vault-addr`                                | `VAULT_ADDR`                   |
| `ssolo`  | `client apply --vault-token`                               | `VAULT_TOKEN`                  |
| `steppa` | `init --root-password`                                     | `STEPPA_ROOT_PASSWORD`         |
| `steppa` | `init --password`                                          | `STEPPA_PASSWORD`              |
| `steppa` | `init --provisioner-password`                              | `STEPPA_PROVISIONER_PASSWORD`  |
| `steppa` | `init --ssh-host-password`                                 | `STEPPA_SSH_HOST_PASSWORD`     |
| `steppa` | `init --ssh-user-password`                                 | `STEPPA_SSH_USER_PASSWORD`     |
| `steppa` | `init --vault-addr`                                        | `VAULT_ADDR`                   |
| `steppa` | `init --vault-token`                                       | `VAULT_TOKEN`                  |
| `waltr`  | `--vault-addr`                                             | `VAULT_ADDR`                   |
| `waltr`  | `--token`                                                  | `VAULT_TOKEN`                  |
| `waltr`  | `--ca-cert`                                                | `VAULT_CACERT`                 |
| `waltr`  | `--insecure-skip-tls-verify`                               | `VAULT_SKIP_VERIFY`            |

`waltr` reads the same variables as the `vault` CLI, so an environment set up for one works for
the other. Secrets are better passed this way, or in a file readable only by you, than as
command-line arguments, which end up in shell history and the process list.

## Choosing the cluster

The CLIs that talk to Kubernetes find the kubeconfig the way `kubectl` does: `--kubeconfig`, else
the files in `$KUBECONFIG`, else `~/.kube/config` - falling back to
`~/.config/gopskit/kubeconfig` and, inside a Pod, the in-cluster configuration. `--context`
selects a context other than the current one. Both can be set in the configuration file like any
other flag.
