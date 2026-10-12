# plattr

Bootstrap the platform pieces of a Kubernetes cluster: provider credentials for AWS and Cloudflare,
the Secrets and StorageClasses for encrypted Hetzner and Longhorn volumes, and Diffie-Hellman
parameters for ingress-nginx.

Every command creates a fixed, well-known object and then leaves it alone. Running a command twice
is safe: an object that already exists is reported and kept, unless you ask for it to be replaced
and the command allows that. Charts and controllers can then refer to these names without caring how
they were created.

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [Global flags](#global-flags)
- [Commands](#commands)
  - [`aws`](#aws)
  - [`cloudflare`](#cloudflare)
  - [`hetzner-encryption`](#hetzner-encryption)
  - [`longhorn-encryption`](#longhorn-encryption)
  - [`ingress-nginx`](#ingress-nginx)
  - [`generate passphrase`](#generate-passphrase)
  - [`generate dhparams`](#generate-dhparams)
- [Existing objects](#existing-objects)
- [Reflection](#reflection)
- [Credentials from the environment](#credentials-from-the-environment)
- [Configuration](#configuration)
- [Cluster access](#cluster-access)

## Install

```sh
# From the repository root.
go build -o plattr ./cmd/plattr

# Or with Bazel, which is what CI uses.
bazel build //cmd/plattr:plattr
```

## Quick start

```sh
# Credentials come from the environment, so they stay out of shell history.
export AWS_ACCESS_KEY_ID=AKIA...
export AWS_SECRET_ACCESS_KEY=...
export CLOUDFLARE_API_TOKEN=...

# Provider credentials in kube-system, mirrored by Reflector into the namespaces that use them.
plattr aws --reflect --reflect-namespaces external-dns --reflect-namespaces velero
plattr cloudflare --reflect --reflect-namespaces cert-manager --reflect-namespaces external-dns

# Encrypted volumes. The passphrase is generated and only ever stored in the Secret.
plattr hetzner-encryption --generate-passphrase
plattr longhorn-encryption --generate-passphrase

# Diffie-Hellman parameters for ingress-nginx. 4096 bits can take many minutes.
plattr ingress-nginx --reflect --reflect-namespaces ingress-nginx

# What was created.
kubectl -n kube-system get secret aws-credentials cloudflare-api-credentials \
  hetzner-volume-encryption longhorn-crypto dhparam
kubectl get storageclass hcloud-encrypted-volumes longhorn-encrypted
```

Run the same commands again and nothing changes: each object already exists, and `plattr` says so.

> [!IMPORTANT] Back up the encryption passphrases
> `--generate-passphrase` does not print the passphrase. It exists only in the Secret, and losing
> the Secret means losing every volume encrypted with it. Copy it somewhere safe straight away:
>
> ```sh
> kubectl -n kube-system get secret hetzner-volume-encryption \
>   -o jsonpath='{.data.encryption-passphrase}' | base64 -d
> kubectl -n kube-system get secret longhorn-crypto \
>   -o jsonpath='{.data.CRYPTO_KEY_VALUE}' | base64 -d
> ```

## Global flags

These apply to every command.

| Flag                | Default       | Meaning                                           |
| ------------------- | ------------- | ------------------------------------------------- |
| `-n`, `--namespace` | `kube-system` | The namespace the Secrets are created in          |
| `--reflect`         | off           | Add Reflector annotations; see [Reflection][refl] |
| `--config`          | see below     | The configuration file; see [Configuration][conf] |
| `--kubeconfig`      | see below     | The kubeconfig file; see [Cluster access][kube]   |
| `--context`         | current       | The kubeconfig context                            |

[refl]: #reflection
[conf]: #configuration
[kube]: #cluster-access

StorageClasses are cluster-scoped and have no namespace. The encryption StorageClasses refer to
their Secret by namespace and name, so they point at whatever `--namespace` was when they were
created.

## Commands

### `aws`

Creates the Secret `aws-credentials`:

| Key                 | Value                                              |
| ------------------- | -------------------------------------------------- |
| `access_key_id`     | `--access-key-id`, or `$AWS_ACCESS_KEY_ID`         |
| `secret_access_key` | `--secret-access-key`, or `$AWS_SECRET_ACCESS_KEY` |

Both values are required; without either, the command fails before touching the cluster.

| Flag                   | Default                                                     |
| ---------------------- | ----------------------------------------------------------- |
| `--access-key-id`      | `$AWS_ACCESS_KEY_ID`                                        |
| `--secret-access-key`  | `$AWS_SECRET_ACCESS_KEY`                                    |
| `--overwrite`          | off; replaces an existing Secret, to rotate the credentials |
| `--reflect-namespaces` | none; required with `--reflect`                             |

```sh
# Rotate the key pair.
AWS_ACCESS_KEY_ID=AKIA... AWS_SECRET_ACCESS_KEY=... plattr aws --reflect --reflect-namespaces external-dns --overwrite
```

### `cloudflare`

Creates the Secret `cloudflare-api-credentials` with one key, `cloudflare_api_token`, from `--token`
or `$CLOUDFLARE_API_TOKEN`. The token is required.

| Flag                   | Default                                               |
| ---------------------- | ----------------------------------------------------- |
| `--token`              | `$CLOUDFLARE_API_TOKEN`                               |
| `--overwrite`          | off; replaces an existing Secret, to rotate the token |
| `--reflect-namespaces` | none; required with `--reflect`                       |

```sh
plattr cloudflare --reflect --reflect-namespaces cert-manager --reflect-namespaces external-dns
```

### `hetzner-encryption`

For volume encryption with the Hetzner Cloud CSI driver. Creates two objects:

- the Secret `hetzner-volume-encryption`, with the passphrase under the key `encryption-passphrase`;
- the StorageClass `hcloud-encrypted-volumes`: provisioner `csi.hetzner.cloud`, reclaim policy
  `Delete`, binding mode `WaitForFirstConsumer`, volume expansion allowed, and
  `csi.storage.k8s.io/node-publish-secret-name` / `-namespace` pointing at the Secret.

Creating the Secret needs a passphrase: give one with `--passphrase` or
`$PLATTR_ENCRYPTION_PASSPHRASE`, or have one generated with `--generate-passphrase` - not both, which
is an error. Once the Secret exists, a re-run needs none.

| Flag                    | Default                         |
| ----------------------- | ------------------------------- |
| `--passphrase`          | `$PLATTR_ENCRYPTION_PASSPHRASE` |
| `--generate-passphrase` | off                             |
| `--passphrase-length`   | `48`                            |
| `--passphrase-charset`  | `A`-`Z`, `a`-`z`, `0`-`9`       |
| `--reflect-namespaces`  | none; required with `--reflect` |

There is no `--overwrite`; see [Existing objects](#existing-objects) for why.

```sh
plattr hetzner-encryption --generate-passphrase --passphrase-length 64
```

### `longhorn-encryption`

For Longhorn volume encryption. Creates two objects:

- the Secret `longhorn-crypto`, labelled `component=longhorn` and `use=encryption`, with these keys:

  | Key                   | Value             |
  | --------------------- | ----------------- |
  | `CRYPTO_KEY_VALUE`    | the passphrase    |
  | `CRYPTO_KEY_PROVIDER` | `secret`          |
  | `CRYPTO_KEY_CIPHER`   | `aes-xts-plain64` |
  | `CRYPTO_KEY_HASH`     | `sha256`          |
  | `CRYPTO_KEY_SIZE`     | `256`             |
  | `CRYPTO_PBKDF`        | `argon2i`         |

- the StorageClass `longhorn-encrypted`: provisioner `driver.longhorn.io`, volume expansion allowed,
  and the parameters `encrypted: "true"`, `numberOfReplicas`, `staleReplicaTimeout`, `fromBackup`,
  and the provisioner, node-publish and node-stage secret name and namespace, all pointing at the
  Secret.

The passphrase works as for `hetzner-encryption`.

| Flag                      | Default                                                        |
| ------------------------- | -------------------------------------------------------------- |
| `--passphrase`            | `$PLATTR_ENCRYPTION_PASSPHRASE`                                |
| `--generate-passphrase`   | off                                                            |
| `--passphrase-length`     | `48`                                                           |
| `--passphrase-charset`    | `A`-`Z`, `a`-`z`, `0`-`9`                                      |
| `--encryption-replicas`   | `3`; becomes `numberOfReplicas`                                |
| `--stale-replica-timeout` | `2880`; becomes `staleReplicaTimeout` (Longhorn reads minutes) |
| `--from-backup`           | empty; becomes `fromBackup`                                    |
| `--reflect-namespaces`    | none; required with `--reflect`                                |

There is no `--overwrite`.

```sh
export PLATTR_ENCRYPTION_PASSPHRASE="$(plattr generate passphrase)"
plattr longhorn-encryption --encryption-replicas 2
```

### `ingress-nginx`

Creates the Secret `dhparam` with one key, `dhparam.pem`, holding PEM-encoded Diffie-Hellman
parameters (`-----BEGIN DH PARAMETERS-----`), which is what ingress-nginx's `ssl-dh-param` setting
expects.

Generating the parameters is slow: at the default 4096 bits it can take many minutes, and the time
varies a lot between runs. `plattr` therefore checks whether the Secret exists first, and generates
nothing if it does and `--overwrite` is not set. Ctrl+C stops a generation in progress without
creating the Secret, and `plattr` exits with an error.

| Flag                   | Default                                                          |
| ---------------------- | ---------------------------------------------------------------- |
| `-b`, `--bits`         | `4096`                                                           |
| `--overwrite`          | off; replaces an existing Secret with newly generated parameters |
| `--reflect-namespaces` | none; required with `--reflect`                                  |

```sh
# Faster, for a test cluster.
plattr ingress-nginx --bits 2048

# Fresh parameters for an existing Secret.
plattr ingress-nginx --reflect --reflect-namespaces ingress-nginx --overwrite
```

To generate the parameters ahead of time, outside the cluster, use
[`generate dhparams`](#generate-dhparams).

### `generate passphrase`

Prints a random passphrase to stdout. Needs no cluster.

| Flag        | Default                   |
| ----------- | ------------------------- |
| `--length`  | `48`                      |
| `--charset` | `A`-`Z`, `a`-`z`, `0`-`9` |

Characters outside ASCII work too; each is used whole. The passphrase comes from the operating
system's cryptographic random number generator.

```sh
plattr generate passphrase --length 32
```

### `generate dhparams`

Prints PEM-encoded Diffie-Hellman parameters to stdout, the same as `ingress-nginx` stores. Needs no
cluster, and is just as slow.

| Flag           | Default |
| -------------- | ------- |
| `-b`, `--bits` | `4096`  |

```sh
plattr generate dhparams > dhparam.pem
```

## Existing objects

Before creating anything, `plattr` looks the object up. Only a "not found" answer counts as missing.
Any other error - no permission to read Secrets, an unreachable API server - is reported and the
command fails, rather than being mistaken for a missing object and leading to an attempt to create
it.

When the object exists, what happens depends on the kind:

**Credentials and DH parameters** (`aws`, `cloudflare`, `ingress-nginx`). The Secret is kept as it
is, and `plattr` says that `--overwrite` would replace it. With `--overwrite`, its data, labels and
annotations are replaced; the Secret's type is kept, since Kubernetes does not allow changing it.
That is how credentials are rotated. Annotations are replaced too, so an `--overwrite` without
`--reflect` removes Reflector annotations the Secret had.

**Encryption passphrases** (both encryption commands). The Secret is never replaced, and these
commands have no `--overwrite`. Every volume created through the encrypted StorageClass is encrypted
with the passphrase that was in the Secret at the time, and the CSI driver reads the Secret again
whenever it unlocks a volume. Putting a new passphrase in the Secret would not re-encrypt anything;
it would only make every existing volume impossible to unlock. If you really need to replace it,
migrate the data off those volumes first and delete the Secret yourself.

A re-run needs no passphrase once the Secret exists, since it would not be used.

**StorageClasses** (`hcloud-encrypted-volumes`, `longhorn-encrypted`). An existing StorageClass is
never changed: Kubernetes does not allow changing a StorageClass's provisioner or parameters. The
StorageClass is created even when its Secret already existed. To change one, for instance a
different replica count, delete it and run the command again; existing volumes are unaffected.

## Reflection

The credential and DH parameter Secrets live in one namespace, but the workloads that use them live
in others. `plattr` can annotate the Secrets for emberstack's
[Reflector](https://github.com/emberstack/kubernetes-reflector), which then mirrors them into the
namespaces listed.

Reflection is off unless you pass the global `--reflect`. With it, every Secret a command creates
gets these annotations, where `<namespaces>` is the command's `--reflect-namespaces`, joined with
commas:

```yaml
reflector.v1.k8s.emberstack.com/reflection-allowed: "true"
reflector.v1.k8s.emberstack.com/reflection-allowed-namespaces: <namespaces>
reflector.v1.k8s.emberstack.com/reflection-auto-enabled: "true"
reflector.v1.k8s.emberstack.com/reflection-auto-namespaces: <namespaces>
```

Without `--reflect` there are no annotations at all, and `--reflect-namespaces` has no effect. With
it, `--reflect-namespaces` is required: there is no default list, since which namespaces need a
credential depends on the cluster, and Reflector would read an empty list as every namespace. Give
the flag once per namespace on the command line; in the configuration file, give a list. Reflector is not installed by `plattr`; it
has to be running in the cluster for the annotations to do anything.

## Credentials from the environment

| Flag                  | Commands                 | Variable                       |
| --------------------- | ------------------------ | ------------------------------ |
| `--access-key-id`     | `aws`                    | `AWS_ACCESS_KEY_ID`            |
| `--secret-access-key` | `aws`                    | `AWS_SECRET_ACCESS_KEY`        |
| `--token`             | `cloudflare`             | `CLOUDFLARE_API_TOKEN`         |
| `--passphrase`        | both encryption commands | `PLATTR_ENCRYPTION_PASSPHRASE` |

Prefer the variables to the flags. A secret given as a flag ends up in your shell history and is
visible to other users in the process list while `plattr` runs; a variable avoids both. The AWS and
Cloudflare variables are the ones the providers' own tools read, so an environment set up for those
works unchanged. A flag on the command line still wins over the variable, and an empty variable
counts as unset.

## Configuration

Every flag can also be set in a YAML file: the one named by `--config`, or else the first that
exists of `./plattr.yaml`, `~/.config/plattr/plattr.yaml` and `/etc/plattr/plattr.yaml`. Keys are
flag names without the dashes. A key naming a subcommand scopes the settings inside it to that
command. For each flag, the command line wins over its environment variable, which wins over the
file, which wins over the default. Unknown keys are an error. The details are in
[`docs/configuration.md`](../../docs/configuration.md).

```yaml
# plattr.yaml
namespace: kube-system
reflect: true # every command below then needs its reflect-namespaces

aws:
  reflect-namespaces: [external-dns, velero]

cloudflare:
  reflect-namespaces: [cert-manager, external-dns]

longhorn-encryption:
  generate-passphrase: true
  encryption-replicas: 2
  reflect-namespaces: [longhorn-system]

ingress-nginx:
  bits: 4096
  reflect-namespaces: [ingress-nginx]

generate:
  passphrase:
    length: 64
```

Keep the credentials themselves out of this file and in the environment.

## Cluster access

`--kubeconfig` and `--context` select the cluster. Without them, `plattr` follows kubectl's rules
(`$KUBECONFIG`, then `~/.kube/config`), then falls back to `~/.config/gopskit/kubeconfig`, and
finally to the in-cluster configuration when it runs in a Pod.

The client is only created when a command needs the cluster, so `--help` and both `generate`
commands work on a machine without any kubeconfig. The commands that create objects need permission
to get, create and update Secrets in the target namespace, and to get and create StorageClasses for
the two encryption commands.
