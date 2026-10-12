# steppa

Create a private certificate authority for Smallstep's [step-ca] and its Helm chart,
[`smallstep/step-certificates`][chart].

steppa produces what `step ca init --helm` produces - a root and intermediate CA, a JWK provisioner,
`ca.json` and `defaults.json`, and optionally the SSH CA keys - but natively, with step's own crypto
library, so no `step` binary is needed. It then writes the CA to where the chart can use it: a values
file, Secrets in the cluster, Vault, or several of these at once.

steppa creates a CA. It does not install the chart, run step-ca, or rotate an existing CA.

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [What is generated](#what-is-generated)
- [Passwords](#passwords)
- [Outputs](#outputs)
  - [`values`](#values)
  - [`kubernetes`](#kubernetes)
  - [`vault`](#vault)
- [The root key](#the-root-key)
- [A CA is created once](#a-ca-is-created-once)
- [DNS names](#dns-names)
- [Installing the chart](#installing-the-chart)
  - [Inject mode](#inject-mode)
  - [Existing Secrets mode](#existing-secrets-mode)
- [Configuration](#configuration)
- [Reaching the cluster and Vault](#reaching-the-cluster-and-vault)
- [What is verified](#what-is-verified)
- [Not covered](#not-covered)

## Install

```sh
# From the repository root.
go build -o steppa ./cmd/steppa

# Or with Bazel.
bazel build //cmd/steppa:steppa
```

steppa needs no other binary. Helm is only needed to install the chart afterwards.

## Quick start

A CA whose values are written to a file for the chart's inject mode, with the root key kept in an
offline backup:

```sh
steppa initialize \
  --name "Example" \
  --dns ca.example.org \
  --namespace pki \
  --root-backup example-root.yaml

# step-certificates-values.yaml now holds the whole CA for the chart, including the
# intermediate key and its passwords. example-root.yaml holds the root certificate,
# its encrypted key and that key's password. Move it somewhere offline.

helm repo add smallstep https://smallstep.github.io/helm-charts
helm install step-certificates smallstep/step-certificates \
  --namespace pki --create-namespace \
  --values step-certificates-values.yaml \
  --wait
```

`initialize` has the aliases `init` and `generate`; `steppa init` is the same command. The root
certificate's SHA-256 fingerprint, which clients pin, is logged at the end of the run and is also in
`defaults.json`. Diagnostics go to standard error, so `--values-file -` can write the values to
standard output for a pipe.

## What is generated

| Part                  | Details                                                                                                      |
| --------------------- | ------------------------------------------------------------------------------------------------------------ |
| Root CA               | EC P-256, called `<name> Root CA`, valid for 10 years                                                        |
| Intermediate CA       | EC P-256, called `<name> Intermediate CA`, signed by the root, valid for 10 years; step-ca signs with it     |
| JWK provisioner       | named `admin` (`--provisioner`); its private key is stored in `ca.json`, encrypted with its own password     |
| SSH CA keys (`--ssh`) | an SSH host and an SSH user CA key, an `sshpop` provisioner, and SSH enabled on the JWK provisioner          |
| ACME (`--acme`)       | an ACME provisioner named `acme`                                                                             |
| `ca.json`             | step-ca's configuration, with the paths where the chart mounts the files, listening on `--address` (`:9000`) |
| `defaults.json`       | the CA URL (`https://` and the first `--dns` name), the root certificate's path and its fingerprint          |

Both certificates carry `--name` as their organization. `--name` and at least one `--dns` are
required.

## Passwords

Every key is encrypted with its own password:

| Key                   | Flag                     | Environment variable          |
| --------------------- | ------------------------ | ----------------------------- |
| root CA               | `--root-password`        | `STEPPA_ROOT_PASSWORD`        |
| intermediate CA       | `--password`             | `STEPPA_PASSWORD`             |
| JWK provisioner       | `--provisioner-password` | `STEPPA_PROVISIONER_PASSWORD` |
| SSH host CA (`--ssh`) | `--ssh-host-password`    | `STEPPA_SSH_HOST_PASSWORD`    |
| SSH user CA (`--ssh`) | `--ssh-user-password`    | `STEPPA_SSH_USER_PASSWORD`    |

A password that is not given is generated, 48 characters long. Generating them is the normal case:
the outputs carry every password step-ca needs, so there is nothing to type in later.

`step ca init` encrypts everything with one password. steppa does not, because the chart accepts the
CA, provisioner and both SSH passwords separately, and its template advises a root key password that
is used for nothing else. step-ca never needs the root key, so its password never needs to be in
the cluster either.

## Outputs

`--output` (`-o`) names where the CA goes. It is repeatable and takes a comma-separated list, and
the default is `values`. With more than one, the root backup is written first, then Vault, then the
Secrets, then the values file, so that if a later output fails the root key is already safe.

### `values`

Chart values, written to `--values-file`, which defaults to `<release>-values.yaml` in the working
directory (`step-certificates-values.yaml` for the default release). `-` writes them to standard
output instead.

On its own, `values` holds the entire CA for the chart's **inject** mode: the certificates,
`ca.json`, `defaults.json`, the encrypted intermediate and SSH keys, and the CA, provisioner and SSH
passwords. The file is therefore created with mode `0600`. Treat it as a secret: do not commit it
unencrypted.

Together with `kubernetes`, the values hold no key material at all - only the settings that switch
the chart to its **existingSecrets** mode (`existingSecrets.enabled`, `inject.enabled: false`, and
`bootstrap` disabled), since the CA itself is in the Secrets.

### `kubernetes`

The Secrets the chart's existingSecrets mode reads, created in `--namespace`, which this output
requires. Use it when the CA must exist before the chart is installed, for instance in a GitOps
setup. Cluster access follows `--kubeconfig` and `--context`.

The Secrets are named for the release the way the chart names its resources (without a
`fullnameOverride` or `nameOverride`): `<fullname>` is `--release` itself if it contains
`step-certificates`, and `<release>-step-certificates` otherwise, cut to 63 characters. For the
default release `step-certificates`, `<fullname>` is `step-certificates`; for `--release pki` it is
`pki-step-certificates`.

| Secret                            | Type                                 | Keys                                                                                                                 |
| --------------------------------- | ------------------------------------ | -------------------------------------------------------------------------------------------------------------------- |
| `<fullname>-secrets`              | `smallstep.com/private-keys`         | `intermediate_ca_key`; `root_ca_key` with `--include-root-key`; `ssh_host_ca_key` and `ssh_user_ca_key` with `--ssh` |
| `<fullname>-certs`                | `Opaque`                             | `root_ca.crt`, `intermediate_ca.crt`; `ssh_host_ca_key.pub` and `ssh_user_ca_key.pub` with `--ssh`                   |
| `<fullname>-config`               | `Opaque`                             | `ca.json`, `defaults.json`                                                                                           |
| `<fullname>-ca-password`          | `smallstep.com/ca-password`          | `password`                                                                                                           |
| `<fullname>-provisioner-password` | `smallstep.com/provisioner-password` | `password`                                                                                                           |
| `<fullname>-ssh-host-ca-password` | `smallstep.com/ssh-host-ca-password` | `password` (only with `--ssh`)                                                                                       |
| `<fullname>-ssh-user-ca-password` | `smallstep.com/ssh-user-ca-password` | `password` (only with `--ssh`)                                                                                       |

Each carries the labels `app.kubernetes.io/name: step-certificates`,
`app.kubernetes.io/instance: <release>` and `app.kubernetes.io/managed-by: steppa`. The root key's
password is never put into a Secret.

The chart must then be told to use these Secrets. Either add `values` to the outputs, as below, or
set the existingSecrets values yourself.

### `vault`

Everything, as one KV-v2 secret: the root and intermediate certificates and keys, the fingerprint,
`ca.json`, `defaults.json`, and every password - including the root key's. With `--ssh`, also both
SSH CA keys, their public keys and their passwords. It is everything needed to recover the CA, and
the place to keep the root key if it is not kept offline.

| Flag                               | Default                   |
| ---------------------------------- | ------------------------- |
| `--vault-addr`                     | `$VAULT_ADDR` (required)  |
| `--vault-token`                    | `$VAULT_TOKEN` (required) |
| `--vault-mount`                    | `kv`                      |
| `--vault-path`                     | `<release>/ca`            |
| `--vault-ca-cert`                  | the system's trusted CAs  |
| `--vault-insecure-skip-tls-verify` | off                       |

The token needs to read and write the secret at `--vault-path`: steppa reads it first to make sure
no CA is there already.

## The root key

step-ca signs with the intermediate key and never needs the root key, so the root key stays out of
the cluster: it is not in the values file or the Secrets unless `--include-root-key` is given.

Without the root key, no new intermediate CA can ever be issued, so steppa refuses to run unless the
root key ends up somewhere:

- `--output vault`, which stores it with its password, or
- `--root-backup FILE`, a YAML file with the root certificate, its encrypted key, its password and
  the fingerprint, created with mode `0600`. Keep it offline, not next to the cluster, or
- `--include-root-key`, which puts the encrypted key into the values (inject mode) or into
  `<fullname>-secrets`.

`--include-root-key` does not put the root key's **password** anywhere. So if it is the only place
the root key goes, steppa insists on `--root-password` (or `$STEPPA_ROOT_PASSWORD`) and refuses to
generate one that would be stored nowhere; keep that password safe yourself.

## A CA is created once

Running `initialize` again creates a different CA, with a different root. Every certificate the
first one issued no longer chains to the root that clients are given from then on. So steppa looks at every output before it generates
anything, and refuses if any of them already holds a CA:

- the `--root-backup` file, and the `--values-file` when `values` is an output,
- any of the chart's Secrets (all seven names listed above) in `--namespace` when `kubernetes` is an
  output - a leftover of an earlier CA is reason enough, since replacing part of a CA breaks it,
- the secret at `--vault-mount`/`--vault-path` when `vault` is an output.

`--force` replaces them. Use it only when invalidating the old CA is what you want.

## DNS names

`--dns` lists the names step-ca serves under and is repeatable. The first one is the URL clients use
(`ca-url` in `defaults.json`), so it should be the name clients outside the cluster reach.

`--namespace` adds the chart Service's in-cluster names, `<fullname>.<namespace>.svc.cluster.local`
and `<fullname>.<namespace>.svc`, after those, so workloads in the cluster can reach step-ca by its
Service as well. Pass it whenever the CA runs in the chart, also for `values` alone.

## Installing the chart

Add the repository once:

```sh
helm repo add smallstep https://smallstep.github.io/helm-charts
helm repo update
```

The release name must be the `--release` steppa was given (default `step-certificates`), and the
namespace the `--namespace`, since the Secrets and DNS names are derived from both.

### Inject mode

Everything goes into the release through the values file.

```sh
steppa init --name "Example" --dns ca.example.org --ssh --acme \
  --namespace pki --root-backup example-root.yaml

helm install step-certificates smallstep/step-certificates \
  --namespace pki --create-namespace \
  --values step-certificates-values.yaml \
  --wait
```

### Existing Secrets mode

The CA is created as Secrets first, and the values only point the chart at them.

```sh
kubectl create namespace pki

steppa init --name "Example" --dns ca.example.org --ssh --acme \
  --namespace pki --output kubernetes,values --root-backup example-root.yaml

helm install step-certificates smallstep/step-certificates \
  --namespace pki \
  --values step-certificates-values.yaml \
  --wait
```

The values file of this mode holds no secrets, so it can be committed alongside the release.

In both modes step-ca only passes the chart's readiness probe once it has decrypted every key with
its password, so `helm install --wait` succeeding means the CA was accepted.

## Configuration

Every flag can also be set in `steppa.yaml`: the file `--config` names, or else the first of
`./steppa.yaml`, `~/.config/steppa/steppa.yaml` and `/etc/steppa/steppa.yaml`. The keys are the flag
names; a mapping named for a subcommand, or one of its aliases, scopes the settings in it to that
subcommand. The command line wins over an environment variable, which wins over the file, which wins
over the default. Unknown keys are an error. The details are in
[`docs/configuration.md`](../../docs/configuration.md).

```yaml
# steppa.yaml
initialize:
  name: Example
  dns: [ca.example.org]
  namespace: pki
  ssh: true
  acme: true
  output: [kubernetes, values, vault]
  vault-addr: https://vault.vault.svc:8200
  vault-ca-cert: [/etc/ssl/private-ca.pem]
  # vault-token: better from $VAULT_TOKEN than from a file
```

```sh
export VAULT_TOKEN=...
steppa init
```

Passwords can be set in the file too, but the environment variables or generated passwords are
the better choice: the file is then not a secret itself.

## Reaching the cluster and Vault

`--kubeconfig` and `--context` select the cluster for `--output kubernetes`; they default to
`$KUBECONFIG`, then `~/.kube/config`, and the current context.

A Vault that is only reachable inside the cluster can be reached through the gopskit bridge, a
small SOCKS5 proxy in the cluster. Deploy it once with `steppa proxy setup`; with the default
`--via auto`, a `--vault-addr` under `*.svc` or `*.cluster.local` then goes through it, `--via
bridge` sends every connection through it and `--via direct` none. `steppa proxy check` and
`steppa proxy cleanup` verify and remove it. The bridge is the same for every gopskit CLI; see
[`docs/configuration.md`](../../docs/configuration.md#bootstrapping-a-cluster).

## What is verified

The tests run the real software against a generated CA:

- **step-ca 0.30.2** (`smallstep/step-ca:0.30.2`) starts with a generated CA - with SSH enabled,
  laid out and started the way the chart's Deployment does it - serves the generated root, and
  issues a certificate through the JWK provisioner that chains to that root.
- **step-certificates chart 1.30.0** installs in a k3s cluster and becomes ready, in both inject
  mode (`--output values`) and existingSecrets mode (`--output kubernetes,values`), with `--ssh`
  and `--acme`.

Other versions of step-ca and the chart are not tested.

## Not covered

- The registration authority (RA) mode of the chart, and the "certificate issuer" key and password
  it needs, are not generated.
- An existing CA is never rotated or extended: there is no command to issue a new intermediate from
  a kept root key.

[step-ca]: https://smallstep.com/docs/step-ca/
[chart]: https://github.com/smallstep/helm-charts/tree/master/step-certificates
