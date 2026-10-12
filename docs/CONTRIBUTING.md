# Ad Noctem Collective - `gopskit` Repository Contributing Guidelines

This file is still growing. Please open an [issue][issues] if something you need is missing.

## Building and testing

[Bazel][bazel] at the version in `.bazelversion` builds and tests everything:

```shell
bazel build //...
bazel test //...
```

Several test suites start real services in containers through [testcontainers][testcontainers]:
Keycloak, Vault, step-ca and a k3s cluster. They need a Docker daemon and skip themselves without
one, or when run with `go test -short`.

After adding or moving Go files, regenerate the `BUILD.bazel` files and the module's repository
list:

```shell
bazel run //:gazelle
bazel mod tidy
```

## Scripts and libsh

The scripts in [`scripts/`](../scripts) use [libsh][libsh], Ad Noctem's shared Bash library, from an
installation on the machine rather than a vendored copy. They need **libsh 0.7.0 or newer** for its
intent logging (`lib::log::print_*`), option parser (`lib::opt::parse`) and
`lib::os::is_executable`, and find it through `LIBSH_DIR`:

```shell
curl -fsSL https://raw.githubusercontent.com/adnoctem/libsh/v0.7.0/bin/install | bash -s -- --init-shell bash
export LIBSH_DIR="$HOME/.local/lib/libsh"
```

| Script                           | Purpose                                                            | Needs libsh |
| -------------------------------- | ------------------------------------------------------------------ | ----------- |
| `scripts/release-create.sh`      | Build the release tarballs and checksums; semantic-release runs it | yes         |
| `scripts/bazel-create-status.sh` | The workspace status Bazel stamps binaries with, on every build    | no          |

`bazel-create-status.sh` deliberately depends on nothing but Bash, Git and coreutils: Bazel runs it
for every build, in CI too, and reads everything it prints as stamp values.

The release workflow installs the same pinned libsh release before semantic-release runs, taking the
installer from that release's commit. Update both together when moving to a newer libsh.

Script names follow libsh's [nomenclature][libsh_nomenclature]: `<domain>-<verb>[-<object>].sh`.

## Commits

Commit messages follow [Conventional Commits][conventional_commits]; semantic-release derives the
version and the changelog from them.

<!-- INTERNAL REFERENCES -->

<!-- General links -->

[issues]: https://github.com/adnoctem/gopskit/issues
[bazel]: https://bazel.build
[testcontainers]: https://testcontainers.com
[libsh]: https://github.com/adnoctem/libsh
[libsh_nomenclature]: https://github.com/adnoctem/libsh/blob/main/docs/NOMENCLATURE.md
[conventional_commits]: https://www.conventionalcommits.org
