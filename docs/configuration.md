# Configuring the gopskit CLIs

Every flag of `ssolo` and `waltr` can also be set through the environment (where noted) or a
configuration file, so repetitive administration - or a tool like Ansible - doesn't have to repeat
the same flags on every call.

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

mounts:
  kubernetes-host: https://kubernetes.default.svc
```

```shell
waltr init          # initialises Vault and unseals every Pod at its own DNS name
waltr mounts
waltr configure
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

| CLI     | Flag                         | Variable                  |
| ------- | ---------------------------- | ------------------------- |
| `ssolo` | `--url`                      | `KEYCLOAK_URL`            |
| `ssolo` | `--username`                 | `KEYCLOAK_ADMIN`          |
| `ssolo` | `--password`                 | `KEYCLOAK_ADMIN_PASSWORD` |
| `ssolo` | `client apply --vault-addr`  | `VAULT_ADDR`              |
| `ssolo` | `client apply --vault-token` | `VAULT_TOKEN`             |
| `waltr` | `--vault-addr`               | `VAULT_ADDR`              |
| `waltr` | `--token`                    | `VAULT_TOKEN`             |
| `waltr` | `--ca-cert`                  | `VAULT_CACERT`            |
| `waltr` | `--insecure-skip-tls-verify` | `VAULT_SKIP_VERIFY`       |

`waltr` reads the same variables as the `vault` CLI, so an environment set up for one works for
the other.
