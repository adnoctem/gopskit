# ssolo

Set up Keycloak for single sign-on. `ssolo` creates the realms and groups applications sign their
users in with, and the OpenID Connect client for each application, and writes the settings the
application needs - client ID, client secret and issuer - to where the application can use them: a
Kubernetes Secret, a Vault KV-v2 secret, or standard output.

Every command is safe to run repeatedly. What already exists is brought in line with the flags or
left alone; nothing is deleted, and an existing client secret is never rotated.

Settings shared by all gopskit CLIs - the configuration file, environment variables and their
precedence - are described in [`docs/configuration.md`](../../docs/configuration.md). This file
covers what is specific to `ssolo`.

## Contents

- [Quick start](#quick-start)
- [Connecting to Keycloak](#connecting-to-keycloak)
- [Realms and groups: `ssolo initialize`](#realms-and-groups-ssolo-initialize)
- [OIDC clients: `ssolo client apply`](#oidc-clients-ssolo-client-apply)
  - [What apply changes](#what-apply-changes)
  - [Where the settings go](#where-the-settings-go)
  - [The issuer](#the-issuer)
- [Example: Vaultwarden](#example-vaultwarden)
- [The gopskit bridge](#the-gopskit-bridge)
- [Configuration file](#configuration-file)
- [Environment variables](#environment-variables)

## Quick start

Keycloak runs in the cluster as the Service `keycloak` in the namespace `keycloak`, and its
administrator credentials are in the Secret `keycloak/keycloak-admin` under the keys `username`
and `password`.

```sh
# 1. Once per cluster: deploy the bridge, so ssolo can reach keycloak.keycloak.svc.
ssolo proxy setup --network-policy --allow-namespace keycloak
ssolo proxy check --target keycloak.keycloak.svc:8443

# 2. Create the realms "operations" and "applications", each with the groups
#    "admins" and "users".
export KEYCLOAK_URL=https://keycloak.keycloak.svc:8443
ssolo init --credentials-secret keycloak/keycloak-admin --ca-cert /etc/ssl/private-ca.pem

# 3. Create the client for an application and store its settings in a Secret
#    next to it.
ssolo client apply vaultwarden \
  --credentials-secret keycloak/keycloak-admin --ca-cert /etc/ssl/private-ca.pem \
  --public-url https://sso.example.org \
  --redirect-uri https://vault.example.org/identity/connect/oidc-signin \
  --output kubernetes --secret-namespace vaultwarden
```

The last command leaves the Secret `vaultwarden/vaultwarden-oidc` with the keys `client-id`,
`client-secret` and `issuer`. Without `--output`, the same settings are printed instead:

```console
$ ssolo client apply vaultwarden --credentials-secret keycloak/keycloak-admin \
    --public-url https://sso.example.org \
    --redirect-uri https://vault.example.org/identity/connect/oidc-signin 2>/dev/null
OIDC_CLIENT_ID='vaultwarden'
OIDC_CLIENT_SECRET='<the client secret>'
OIDC_ISSUER='https://sso.example.org/realms/applications'
```

Running the command again leaves the client as it is and prints the same secret.

To avoid repeating `--credentials-secret`, `--ca-cert` and the rest on every call, put them in a
[configuration file](#configuration-file).

## Connecting to Keycloak

`ssolo initialize` and `ssolo client apply` both talk to Keycloak's admin API and share these
flags:

| Flag                         | Default          | Meaning                                                              |
| ---------------------------- | ---------------- | -------------------------------------------------------------------- |
| `--url`                      |                  | Keycloak's base URL. Required. Env `KEYCLOAK_URL`.                   |
| `--username`                 |                  | The administrator to log in as. Env `KEYCLOAK_ADMIN`.                |
| `--password`                 |                  | The administrator's password. Env `KEYCLOAK_ADMIN_PASSWORD`.         |
| `--credentials-secret`       |                  | Read username and password from a Secret, as `namespace/name`.       |
| `--login-realm`              | `master`         | The realm the administrator logs in to.                              |
| `--ca-cert`                  |                  | A PEM CA certificate to trust for Keycloak (repeatable).             |
| `--insecure-skip-tls-verify` | `false`          | Don't verify Keycloak's TLS certificate.                             |
| `--via`                      | `auto`           | `auto`, `direct` or `bridge`; see [the bridge](#the-gopskit-bridge). |
| `--bridge-namespace`         | `gopskit-bridge` | The namespace the bridge runs in.                                    |

**Address.** `--url` is Keycloak as `ssolo` reaches it: through an Ingress
(`https://sso.example.org`), a local container (`http://localhost:8080`), or a cluster-internal
Service name (`https://keycloak.keycloak.svc:8443`), which goes through the bridge.

**Credentials.** Either `--username` and `--password` (or their environment variables), or
`--credentials-secret namespace/name`, which reads the keys `username` and `password` from that
Kubernetes Secret. When `--credentials-secret` is set, it is used and `--username` and
`--password` are ignored. Reading the Secret needs cluster access: `--kubeconfig` and `--context`,
otherwise the same rules as kubectl (`$KUBECONFIG`, then `~/.kube/config`, current context).

Prefer `--credentials-secret` or the environment variables over `--password` on the command line,
where the password ends up in shell history and in the process list.

**TLS** is verified. If Keycloak's certificate is issued by a private CA, pass that CA with
`--ca-cert`; the flag can be given more than once. `--insecure-skip-tls-verify` turns verification
off, which lets anyone on the network path read the administrator's credentials - use it only
against a throwaway instance.

**Output streams.** Progress and errors go to standard error. Standard output carries only data
(the settings `client apply --output stdout` prints), so it can be redirected or piped safely.

After logging in, `ssolo` stores the access and refresh tokens in its cache directory (on Linux
`~/.cache/ssolo/creds/keycloak-credentials.json`, mode `0600`). Every invocation still logs in
again with the credentials it was given.

## Realms and groups: `ssolo initialize`

```sh
ssolo initialize            # or: ssolo init
```

Creates each realm in `--realms` (default `operations,applications`), and in each of those realms
each group in `--groups` (default `admins,users`). A realm or group that already exists is left as
it is, so the command can run on every deployment.

```sh
ssolo init --realms applications --groups admins,users,auditors
```

New realms are enabled, and their display name is derived from the realm name.

## OIDC clients: `ssolo client apply`

```sh
ssolo client apply CLIENT_ID --redirect-uri URI [flags]    # alias: ssolo client create
```

Creates the OpenID Connect client an application signs its users in with, in `--realm` (default
`applications`), or brings an existing client with that client ID in line with the flags.

| Flag                         | Default                      | Meaning                                                                |
| ---------------------------- | ---------------------------- | ---------------------------------------------------------------------- |
| `--redirect-uri`             |                              | Where Keycloak may send users after signing in. Required, repeatable.  |
| `--post-logout-redirect-uri` | the redirect URIs            | Where Keycloak may send users after signing out. Repeatable.           |
| `--web-origin`               | origins of the redirect URIs | Allowed CORS origins. Repeatable.                                      |
| `--root-url`                 |                              | The application's base URL.                                            |
| `--name`                     |                              | The client's display name.                                             |
| `--public`                   | `false`                      | A public client: no secret, PKCE required.                             |
| `--realm`                    | `applications`               | The realm the client belongs to.                                       |
| `--public-url`               | `--url`                      | Keycloak as the application reaches it; see [the issuer](#the-issuer). |

### What apply changes

The client is an OIDC client for the authorization code flow, and nothing else: the standard flow
is enabled, and the implicit flow, direct access grants (passwords sent to the client) and service
accounts are disabled.

On an existing client, only the settings the flags cover are changed. Everything else - mappers,
scopes, settings adjusted by hand in the admin console - is kept. In detail:

- The redirect URIs, web origins and post-logout redirect URIs are always set from the flags. Left
  out, the web origins and post-logout redirect URIs fall back to Keycloak's `+`, meaning "those
  of the redirect URIs"; a value set by hand earlier is replaced.
- `--name` and `--root-url` are set only when given, and are otherwise left as they are.
- The client is enabled, and made public or confidential as `--public` says.
- The client secret of an existing confidential client is kept, never rotated.

If a client with that client ID exists but uses a protocol other than OpenID Connect, for example
SAML, `apply` refuses to convert it and exits with an error.

**Public clients.** `--public` creates a client without a secret, for applications that cannot
keep one, such as single-page apps. Such a client has nothing that proves it started the sign-in,
so `ssolo` enforces PKCE with the `S256` challenge method on it. A public client's settings have
no `client-secret`. Turning a public client confidential later leaves PKCE enforced - like any
setting `apply` doesn't cover, it stays as it is; remove it in Keycloak if the application doesn't
send a PKCE challenge.

### Where the settings go

The application's settings go to every `--output` (`-o`) given; the flag is repeatable and
defaults to `stdout`. The keys are the same in every output, and follow oauth2-proxy's naming,
which many Helm charts reuse:

| Key             | Value                                               |
| --------------- | --------------------------------------------------- |
| `client-id`     | The client ID.                                      |
| `client-secret` | The client secret. Absent for a `--public` client.  |
| `issuer`        | The OIDC issuer URL; see [the issuer](#the-issuer). |

All output flags are checked **before** Keycloak is touched, so a typo in them cannot leave a new
client behind whose secret went nowhere. If one output then fails - Vault unreachable, say - the
others are still written, and the error names the one that failed.

**`stdout`** prints the settings in the format `--format` selects. `env` (the default) prints
`KEY=value` lines, with the key prefixed `OIDC_`, upper-cased and dashes replaced by underscores,
and the value single-quoted for the shell. The result can be `eval`'d or used as an env file.
With the connection settings in the [configuration file](#configuration-file):

```sh
eval "$(ssolo client apply grafana --redirect-uri https://grafana.example.org/login/generic_oauth)"
echo "$OIDC_ISSUER"
```

`json` prints one object with the keys from the table above:

```json
{
  "client-id": "grafana",
  "client-secret": "<the client secret>",
  "issuer": "https://sso.example.org/realms/applications"
}
```

**`kubernetes`** writes a Secret named `--secret-name` (default `CLIENT_ID-oidc`) in
`--secret-namespace`, which is required for this output. A new Secret is of type `Opaque`. An
existing Secret is updated in place: the three keys are set, any other keys in it - a cookie secret
added by hand, for example - are kept, and `client-secret` is removed when the client is public.
Either way the Secret gets the label `app.kubernetes.io/managed-by: ssolo`. This output uses the
same cluster access as `--credentials-secret`.

**`vault`** writes the settings to a KV-v2 secrets engine:

| Flag                               | Default                      | Meaning                                                                  |
| ---------------------------------- | ---------------------------- | ------------------------------------------------------------------------ |
| `--vault-addr`                     |                              | Vault's address. Required. Env `VAULT_ADDR`.                             |
| `--vault-token`                    |                              | A token allowed to read and write the path. Required. Env `VAULT_TOKEN`. |
| `--vault-mount`                    | `kv`                         | The KV-v2 engine's mount path.                                           |
| `--vault-path`                     | `CLIENT_ID/credentials/oidc` | The secret's path within the mount.                                      |
| `--vault-ca-cert`                  |                              | A PEM CA certificate to trust for Vault (repeatable).                    |
| `--vault-insecure-skip-tls-verify` | `false`                      | Don't verify Vault's TLS certificate.                                    |

Each run writes a new version of the secret. Like the Kubernetes output, it keeps any other keys
already at that path and only sets the three (and removes `client-secret` from a client turned
public); the token must therefore be allowed to read the path as well as write it. `--via`
applies to Vault as well, host by host, so `https://vault.vault.svc:8200` is reached through the
bridge just like Keycloak.

### The issuer

The issuer is `<public URL>/realms/<realm>`, where the public URL is `--public-url`, or `--url` if
that is not set. It must be the address the **application** reaches Keycloak at, which is often
not the one `ssolo` uses: `ssolo` may talk to `https://keycloak.keycloak.svc:8443` through the
bridge while browsers and the application use `https://sso.example.org`. The issuer in the
tokens Keycloak issues follows the address users reach it at (or its configured hostname), and
applications reject tokens whose issuer differs from the one they were configured with, so set
`--public-url` whenever the two addresses differ.

## Example: Vaultwarden

Vaultwarden signs users in with the authorization code flow and keeps its client secret
server-side, so it gets a confidential client. Its redirect URI is the path
`/identity/connect/oidc-signin` on its own domain:

```sh
ssolo client apply vaultwarden --realm applications \
  --name Vaultwarden \
  --root-url https://vault.example.org \
  --redirect-uri https://vault.example.org/identity/connect/oidc-signin \
  --public-url https://sso.example.org \
  --output kubernetes --secret-namespace vaultwarden
```

This creates or updates the client `vaultwarden` in the realm `applications` and writes the Secret
`vaultwarden/vaultwarden-oidc`. Point Vaultwarden's SSO settings at its keys, for example in the
Deployment:

```yaml
env:
  - name: SSO_ENABLED
    value: "true"
  - name: SSO_AUTHORITY
    valueFrom: { secretKeyRef: { name: vaultwarden-oidc, key: issuer } }
  - name: SSO_CLIENT_ID
    valueFrom: { secretKeyRef: { name: vaultwarden-oidc, key: client-id } }
  - name: SSO_CLIENT_SECRET
    valueFrom: { secretKeyRef: { name: vaultwarden-oidc, key: client-secret } }
```

The redirect URI and the names of the settings are the application's, not `ssolo`'s, and they
change between applications and sometimes between versions. Check the application's own
documentation for the exact redirect URI it uses; Keycloak rejects a sign-in whose redirect URI
does not match one of the `--redirect-uri` values.

## The gopskit bridge

Keycloak usually has no address outside the cluster that an administrator should use. The gopskit
bridge solves that: a small SOCKS5 server deployed into the cluster once, which `ssolo` reaches
through a port-forward. Commands then use the services' cluster-internal names, such as
`https://keycloak.keycloak.svc:8443`. The bridge is shared by `ssolo`, `waltr` and `steppa`; any of
them can set it up.

```sh
ssolo proxy setup                       # deploy or update; safe to repeat
ssolo proxy check                       # deployed, ready, and answering?
ssolo proxy check --target keycloak.keycloak.svc:8443   # ... and can it reach Keycloak?
ssolo proxy cleanup                     # remove it
```

`proxy setup` takes these flags:

| Flag                  | Meaning                                                                                                                               |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| `--network-policy`    | Also deploy a NetworkPolicy: no ingress, egress to DNS and the allowed namespaces.                                                    |
| `--allow-namespace`   | Namespaces that policy lets the bridge reach (repeatable; default: all).                                                              |
| `--image`             | The image to run. Default: `ghcr.io/adnoctem/gopskit-bridge` tagged with the CLI's release version, `latest` for a development build. |
| `--bridge-namespace`  | The namespace to run in (default `gopskit-bridge`). Accepted by every `proxy` command.                                                |
| `--wait`, `--timeout` | Wait for the bridge to become ready (default `true`), and for how long (`2m`).                                                        |

Running `setup` again without `--network-policy` removes the policy. `cleanup` deletes the
namespace only if `setup` created it; in a namespace that existed before, it removes only the
bridge's own resources.

The bridge Pod meets the "restricted" Pod Security Standard and listens on its loopback interface
only, so the only way in is a port-forward. Who can use it is therefore decided by who has the
RBAC permission to port-forward to Pods in its namespace.

`--via` on the Keycloak commands decides, per URL, whether to use it:

- `auto` (default): through the bridge for hosts ending in `.svc` or `.cluster.local`, directly
  otherwise.
- `direct`: never, for example for an Ingress or a local container.
- `bridge`: always.

A command that needs the bridge fails, and says to run `ssolo proxy setup`, if it is not running.

## Configuration file

Every flag can be set in `ssolo.yaml`: `--config`, otherwise the first of `./ssolo.yaml`,
`~/.config/ssolo/ssolo.yaml` and `/etc/ssolo/ssolo.yaml`. Keys are flag names; a mapping under a
subcommand's name (or alias) applies only to that subcommand. The command line overrides the
environment, which overrides the file. Unknown keys are an error. See
[`docs/configuration.md`](../../docs/configuration.md) for the details.

```yaml
# Connecting: shared by `initialize` and `client apply`.
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

With that file in place, the commands shrink to what differs per application:

```sh
ssolo init
ssolo client apply vaultwarden \
  --redirect-uri https://vault.example.org/identity/connect/oidc-signin \
  --secret-namespace vaultwarden
```

## Environment variables

| Variable                  | Flag                         |
| ------------------------- | ---------------------------- |
| `KEYCLOAK_URL`            | `--url`                      |
| `KEYCLOAK_ADMIN`          | `--username`                 |
| `KEYCLOAK_ADMIN_PASSWORD` | `--password`                 |
| `VAULT_ADDR`              | `client apply --vault-addr`  |
| `VAULT_TOKEN`             | `client apply --vault-token` |

`KUBECONFIG` is honoured as kubectl honours it.
