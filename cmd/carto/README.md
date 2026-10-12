# carto

Install and upgrade the Helm charts that make up a cluster's backbone - Vault, Keycloak,
cert-manager and the like - from one declaration file and a directory of values files.

carto uses the Helm SDK directly, so it needs neither the `helm` binary nor plugins. Releases are
stored the way `helm` stores them (as Secrets in the release namespace), so `helm list` and
`helm history` see what carto installed, and carto picks up releases that `helm` or Helmfile
installed under the same name. It is meant to replace our Helmfile workflows: the values of a chart
are always the same files merged in the same order, and every command works on one chart at a time.

## Quick start

```sh
# Build it, from the repository root.
bazel build //cmd/carto:carto

# A backbone.yaml declaring the charts, and a charts/ directory with their values.
cat backbone.yaml
# charts:
#   vault:
#     version: 0.29.1
#     repository: https://helm.releases.hashicorp.com
#     namespace: vault
ls charts/vault
# values.yaml  values.prod.yaml

# See the merged values for production. No cluster needed.
carto render vault -e prod

# Compare what would be deployed with what is deployed, then deploy it.
carto diff vault -e prod
carto apply vault -e prod

# Something went wrong: look at the revisions and go back to one.
carto history vault
carto rollback vault 4
```

`--environment` (`-e`) defaults to `dev`, so a `carto.yaml` with `environment: prod` saves typing
it on every call on a production machine; see [Settings](#settings).

## The backbone file

`backbone.yaml` declares every chart carto manages. carto reads it from the working directory, or
from wherever `--backbone` (`-b`) points.

> [!NOTE] `--backbone` used to be `--config`
> The flag naming the backbone file was renamed to `--backbone`. `--config` now names carto's own
> settings file, `carto.yaml`, like it does for every gopskit CLI. A script still passing
> `--config backbone.yaml` will have carto read the backbone file as its settings, which fails on
> the first unknown key.

```yaml
globals:
  - path: globals/common.yaml
    applies: ["*"]
  - path: globals/oidc.yaml
    applies: [vault, keycloak]

charts:
  vault:
    version: 0.29.1
    repository: https://helm.releases.hashicorp.com
    namespace: vault
  keycloak:
    version: 24.4.0
    repository: oci://registry-1.docker.io/bitnamicharts
    namespace: keycloak
```

### `charts`

A map from chart name to its settings. The name does three jobs: it is the name of the chart in its
repository, the name of the Helm release, and the name of the chart's directory under
`--charts-dir`. A release named differently from its chart cannot be declared.

| Key          | Meaning                                                                      |
| ------------ | ---------------------------------------------------------------------------- |
| `version`    | The chart version to install or upgrade to.                                  |
| `repository` | Where the chart comes from - see below.                                      |
| `namespace`  | The namespace the release lives in. carto does not create it; it must exist. |
| `hooks`      | Commands run around `apply` - see [Hooks](#hooks).                           |

`repository` takes three forms:

- A chart repository URL, such as `https://helm.releases.hashicorp.com`. The chart is fetched the
  way `helm install --repo <url> <chart> --version <version>` fetches it.
- An `oci://` registry reference, such as `oci://registry-1.docker.io/bitnamicharts`. carto
  appends the chart name, giving `oci://registry-1.docker.io/bitnamicharts/keycloak`.
- Empty, for a chart on disk. The chart is then the directory named after the chart, relative to
  the working directory - `./vault` for a chart called `vault`.

### `globals`

Values files merged into more than one chart. `applies` lists the charts a file is merged into;
`"*"` means every chart. A relative `path` is resolved against the directory `backbone.yaml` is in,
not the working directory, so `carto -b infra/backbone.yaml` finds `infra/globals/common.yaml`. A
global file that applies to a chart must exist, or every command on that chart fails.

### Hooks

Hooks run commands at three points of `carto apply`:

| Hook            | Runs                                                                 |
| --------------- | -------------------------------------------------------------------- |
| `pre-apply`     | Before every install or upgrade. A failure stops the apply.          |
| `first-install` | After the install, only when the release was not installed before.   |
| `post-apply`    | After every install or upgrade, and after the `first-install` hooks. |

Each hook is a list of actions, run in order; the first one that fails stops the rest and fails
the command. An action sets exactly one of `local` and `exec`:

```yaml
charts:
  vault:
    version: 0.29.1
    repository: https://helm.releases.hashicorp.com
    namespace: vault
    hooks:
      pre-apply:
        - local: ./hooks/vault-snapshot.sh
      first-install:
        - local: ./hooks/vault-init.sh
      post-apply:
        - exec: vault status -format=json | grep -q '"sealed": false'
          container: vault
        - exec: [/bin/vault, version]
```

**`local`** runs a program on the machine carto runs on, with carto's environment and working
directory and without arguments. The value is a path, not a command line: `./hooks/x.sh` is a file
relative to the working directory, a bare `x.sh` is looked up on `$PATH`, and anything after a space
is part of the file name. A script needs to be executable and start with a `#!` line.

**`exec`** runs a command in one of the release's Pods, without a terminal and without input. It
is written either as a string or as a list, as in a Dockerfile:

- A string is run by `/bin/sh -c`, so quoting, pipes and `&&` work.
- A list is run as given, for images without a shell.

`container` selects the Pod's container and is allowed only with `exec`; without it, the Pod's
first container is used. The Pod is chosen the same way [`logs` and `exec`](#logs-and-exec) choose
it, at the moment the hook runs.

A `pre-apply` hook with an `exec` action fails on a first install, as there is no Pod to run it in
yet. The output of successful hooks is not shown.

## Values

Each chart's values come from up to three kinds of files, merged in this order:

```text
backbone.yaml
globals/
  common.yaml               applies: ["*"]
  oidc.yaml                 applies: [vault, keycloak]
charts/
  vault/
    values.yaml             base values, every environment
    values.prod.yaml        overlay for --environment prod
    values.dev.yaml         overlay for --environment dev
```

1. the `globals` files that apply to the chart, in the order `backbone.yaml` declares them
2. `<charts-dir>/<chart>/values.yaml`
3. `<charts-dir>/<chart>/values.<environment>.yaml`

A later file overrides an earlier one. Both files under `<charts-dir>` are optional; a chart with
none of the three gets the chart's own defaults. `--charts-dir` defaults to `charts` and is
relative to the working directory, not to `backbone.yaml`.

The merge is a deep merge of maps:

- A key present in a later file replaces the earlier value, including with `false`, `0` or `""`.
- Maps are merged key by key, so an overlay only needs the keys it changes.
- Lists are replaced whole, never appended to or merged by index.
- A `null` stays `null` in the merged values, which Helm takes to mean "remove the chart's default".

The merged values are what carto hands to Helm, and Helm then merges them over the chart's own
`values.yaml` as usual. `carto render` shows the merged values before that step.

`--environment` is only the name of the overlay file: any name is accepted, and a missing overlay
file is not an error.

## Commands

Every command takes the name of one chart declared in `backbone.yaml`, and fails for any other.
There is no command that works on every chart at once, and carto does not order charts by their
dependencies; call it once per chart, in the order you need.

| Command                            | Needs a cluster | Reads values |
| ---------------------------------- | --------------- | ------------ |
| `render <chart>`                   | no              | yes          |
| `diff <chart>`                     | yes             | yes          |
| `apply <chart>`                    | yes             | yes          |
| `history <chart>`                  | yes             | no           |
| `rollback <chart> <revision>`      | yes             | no           |
| `logs <chart>`                     | yes             | no           |
| `exec <chart> -- <command> [args]` | yes             | no           |

Commands that do not read values ignore `--charts-dir` and `--environment`.

### `render`

```sh
carto render vault -e prod
carto render vault -e prod -o /tmp/vault-prod.yaml
```

Prints the merged values of a chart as YAML, with map keys sorted. `--output` (`-o`) writes them to
a file instead. This is the values, not the rendered Kubernetes manifests, and it neither contacts
the cluster nor downloads the chart.

### `diff`

```sh
carto diff vault -e prod
```

Renders the chart's manifests with the merged values and compares them with the manifest of the
deployed release. When they are identical it prints `no differences`. Otherwise it prints
`Infrastructure Changes:`, followed by the text both have in common, then the rest of the deployed
manifest after `A:` and the rest of the new one after `B:`. It is not a line-by-line diff: the
comparison stops at the first character that differs, and everything after it is printed in full.

When the chart is not installed, carto says so and compares against an empty manifest, so the
output shows everything an install would create. Any other error while reading the release - no
permission, an unreachable cluster - is reported as an error rather than treated as "not
installed".

The render does not contact the cluster, so templates that use `lookup` or check the cluster's API
versions can render differently here than they do during `apply`. Helm hooks of the chart are not
part of the compared manifests.

### `apply`

```sh
carto apply vault -e prod
```

Installs the chart if it has no release yet (or its release was uninstalled), and upgrades it
otherwise. In order:

1. load `backbone.yaml`, merge the values, fetch the chart
2. run the `pre-apply` hooks
3. install or upgrade
4. on an install, run the `first-install` hooks
5. run the `post-apply` hooks

The install or upgrade is atomic and waits for the release's resources to become ready, for at most
10 minutes. A failed install is removed again; a failed upgrade is rolled back to the previous
revision, and resources it created are deleted. A failing hook after step 3 fails the command but
leaves the new release in place.

### `history`

```sh
carto history vault
carto history vault --max 0
```

Lists the release's revisions, newest first, as a table of `REVISION`, `STATUS`, `CHART`,
`APP VERSION` and `DESCRIPTION`. `--max` limits the list and defaults to 10; `0` lists every
revision.

### `rollback`

```sh
carto rollback vault 4
carto rollback vault 4 -y
```

Rolls the release back to the given revision, after asking `roll back release "vault" to revision
4? [y/N]`. Only `y` or `Y` proceeds; anything else, including `yes`, aborts. `--yes` (`-y`) skips
the question, which is needed when there is no terminal to answer it. Revision `0` means the one
before the current revision, as with `helm rollback`.

The rollback recreates the release's Pods, waits for at most 5 minutes for the resources to become
ready, and deletes resources it created if it fails. Hooks are not run, and a rollback is not
atomic.

### `logs` and `exec`

```sh
carto logs vault -f
carto logs keycloak -C keycloak
carto exec vault -- vault status
carto exec vault -C vault -- /bin/sh
```

Both work on one Pod of the release: the Pods in the chart's namespace labelled
`app.kubernetes.io/instance=<chart>`, preferring one that is ready, else one that is running, and
never one that is being deleted. Among equals, the first by name is used, so repeated calls pick the
same Pod. Charts that do not set that label have no Pod carto can find.

`logs` prints the Pod's log, and with `--follow` (`-f`) keeps streaming it. `--container` (`-C`)
selects the container; without it, a Pod with more than one container is an error from Kubernetes.

`exec` runs a command in the Pod with an interactive terminal attached. Put the command after `--`,
so that its own flags are not taken for carto's. `--container` (`-C`) selects the container; without
it, the Pod's first container is used. The terminal size is sent once at the start; resizing the
window afterwards is not passed on.

## Settings

Every flag can also be set in `carto.yaml`. carto uses the file `--config` names, else the first
that exists of `./carto.yaml`, `~/.config/carto/carto.yaml` and `/etc/carto/carto.yaml`. The keys
are the flag names; a key named after a subcommand scopes the settings under it to that command.
The command line wins over the file, and the file over the defaults. No carto flag has an
environment variable of its own. Unknown keys are an error.

```yaml
backbone: /srv/infra/backbone.yaml
charts-dir: /srv/infra/charts
environment: prod
context: prod-admin

history:
  max: 25
logs:
  follow: true
```

[`docs/configuration.md`](../../docs/configuration.md) describes the file in full; it is the same
for every gopskit CLI.

### Choosing the cluster

`--kubeconfig` and `--context` select the cluster. Without them, carto finds the kubeconfig the way
`kubectl` does - `$KUBECONFIG`, else `~/.kube/config` - and then falls back to
`~/.config/gopskit/kubeconfig` and, inside a Pod, the in-cluster configuration. See
[Choosing the cluster](../../docs/configuration.md#choosing-the-cluster).

The connection is only made when a command needs it, so `--help` and `render` work on a machine
without any kubeconfig.

## A worked example

A repository holding the backbone of two clusters, `stage` and `prod`, for one chart:

```text
infra/
  backbone.yaml
  carto.yaml
  globals/
    common.yaml
  charts/
    vault/
      values.yaml
      values.stage.yaml
      values.prod.yaml
  hooks/
    vault-init.sh
```

`backbone.yaml`:

```yaml
globals:
  - path: globals/common.yaml
    applies: ["*"]

charts:
  vault:
    version: 0.29.1
    repository: https://helm.releases.hashicorp.com
    namespace: vault
    hooks:
      first-install:
        - local: ./hooks/vault-init.sh
      post-apply:
        - exec: vault status
          container: vault
```

`globals/common.yaml`, merged into every chart:

```yaml
global:
  imagePullSecrets:
    - name: registry
```

`charts/vault/values.yaml`, the same in every environment:

```yaml
server:
  ha:
    enabled: true
    replicas: 3
    raft:
      enabled: true
  dataStorage:
    size: 10Gi
```

`charts/vault/values.stage.yaml`, only what differs on `stage`:

```yaml
server:
  ha:
    replicas: 1
```

`hooks/vault-init.sh`, run once, after the first install:

```sh
#!/bin/sh
set -eu
waltr init
```

`carto.yaml`, so the commands below need no flags beyond the environment:

```yaml
backbone: backbone.yaml
charts-dir: charts
context: stage-admin
```

From `infra/`:

```console
$ carto render vault -e stage
global:
    imagePullSecrets:
        - name: registry
server:
    dataStorage:
        size: 10Gi
    ha:
        enabled: true
        raft:
            enabled: true
        replicas: 1

$ carto diff vault -e stage
$ carto apply vault -e stage
```

The first `apply` installs Vault, runs `./hooks/vault-init.sh` and then `vault status` in a ready
Vault Pod. Every later `apply` upgrades it and runs only `vault status`. `vault status` exits
non-zero while Vault is sealed, so a post-apply hook like this one fails the command until Vault is
unsealed, which makes it a check rather than a step.

For production, `carto apply vault -e prod --context prod-admin`, or a second `carto.yaml` with
`context: prod-admin` and `environment: prod` on the machine that deploys to production.
