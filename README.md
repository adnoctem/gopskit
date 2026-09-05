<p align="center">
    <!-- Go Operations Toolkit -->
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="https://github.com/adnoctem/artwork/blob/4a43cf6856b3d5d98998c215bc09252c060e56ba/projects/gopskit/icon/white/gopskit-icon-white.png?raw=true">
      <img src="https://github.com/adnoctem/artwork/blob/4a43cf6856b3d5d98998c215bc09252c060e56ba/projects/gopskit/icon/color/gopskit-icon-color.png?raw=true" alt="GOpsKit Logo" width="225">
    </picture>
    <h1 align="center">Go Operations Toolkit</h1>
</p>

[![License](https://img.shields.io/github/license/adnoctem/gopskit?label=License)][license]
[![go.mod version](https://img.shields.io/github/go-mod/go-version/adnoctem/gopskit?logo=go)][go]
[![Language](https://img.shields.io/github/languages/top/adnoctem/gopskit?label=Go&logo=go)][go]
[![Testing](https://github.com/adnoctem/gopskit/actions/workflows/testing.yaml/badge.svg)][ci_testing_workflow]
[![GitHub Release](https://img.shields.io/github/v/release/adnoctem/gopskit?label=Release)][github_releases]
[![GitHub Activity](https://img.shields.io/github/commit-activity/m/adnoctem/gopskit?label=Commits)][github_commits]
[![Bazel](https://img.shields.io/badge/Bazel-built-brightgreen?logo=bazel&logoColor=43A047)][bazel]
[![Renovate](https://img.shields.io/badge/Renovate-enabled-brightgreen?logo=renovate&logoColor=1A1F6C)][renovate]
[![PreCommit](https://img.shields.io/badge/PreCommit-enabled-brightgreen?logo=precommit&logoColor=FAB040)][precommit]

`GOpsKit` (**Go** **Op**erations Tool**kit**) is an open-source [MIT][license]-licensed [Go][go]-based toolkit for
working with [Kubernetes] [kubernetes] Clusters `v1.26` and above. The project is built using Google's [Bazel][bazel]
build system in combination with their first-party [Gazelle][gazelle] `BUILD` file generator.

## 📖 Overview

The toolkit offers a plethora of functionalities like setting up HashCorp's [Vault][vault] with [`waltr`][waltr],
registering various applications for SSO authentication with [Keycloak][keycloak] using [`ssolo`][ssolo]. Never
write [Helmfile][helmfile] `values.yaml` template files to manage applications on your cluster again. Instead, generate
them using [`fillr`][fillr]. Are you running your own custom private Certificate Authority using
[Smallstep's CA][smallstep_certificates]? Then you'd likely want to generate and manage PKI values using
[`steppa`][steppa]. The German KBA delivers data in a custom bespoke text-based format, which purely relies on columns
to separate data. _That ain't SQL..._ So let's swiftly generate some usable SQL import script using [`amtrac`][amtrac].
Managing high-stakes backbone components like Vault or Keycloak themselves with Helm? [`carto`][carto] renders,
diffs, applies and rolls back charts using a deterministic base+overlay values-merge strategy.

## ✨ TL;DR

```shell
# build all projects at once - requires Bazel at .bazelversion
bazel build //...
```

## 🛠️ Tools

Like most modern [Go][go] projects the various executables are located within the [cmd][cmd] directory. Here's a
quick-reference list as an overview:

- [`ssolo`][ssolo]: manage SSO authentication for various apps using Keycloak
- [`waltr`][waltr]: configure and manage [HashCorp's Vault][vault]
- [`fillr`][fillr]: create Helmfile templates automatically
- [`steppa`][steppa]: generate and manage SmallStep PKI values
- [`amtrac`][amtrac]: generate SQL dumps from the German KBA's data files using Docker
- [`carto`][carto]: manage Kubernetes backbone components (Helm charts) with the Helm SDK

### 🔃 Contributing

Refer to our [documentation for contributors][contributing] for contributing guidelines, commit message
formats and versioning tips.

### 📥 Maintainers

This project is owned and maintained by [Ad Noctem Collective][org] refer to the [`AUTHORS`][authors] or [`CODEOWNERS`][owners]
for more information. You may also use the linked contact details to reach out directly.

### ©️ Copyright

- _Assets provided by:_ **[IconScout](https://iconscout.com)**
- _Sources provided by:_ **[Ad Noctem Collective][org]** under the **[MIT License][license]**

<!-- INTERNAL REFERENCES -->

<!-- Project references -->

[cmd]: cmd
[ssolo]: cmd/ssolo
[waltr]: cmd/waltr
[fillr]: cmd/fillr
[steppa]: cmd/steppa
[amtrac]: cmd/amtrac
[carto]: cmd/carto

<!-- File references -->

[license]: LICENSE
[contributing]: docs/CONTRIBUTING.md
[authors]: .github/AUTHORS
[owners]: .github/CODEOWNERS
[ci_testing_workflow]: https://github.com/adnoctem/gopskit/actions/workflows/testing.yaml

<!-- General links -->

[org]: https://github.com/adnoctem
[kubernetes]: https://kubernetes.io
[vault]: https://vaultproject.io
[keycloak]: https://www.keycloak.org/
[go]: https://go.dev
[bazel]: https://bazel.build
[gazelle]: https://github.com/bazelbuild/bazel-gazelle
[helmfile]: https://github.com/helmfile/helmfile
[smallstep_certificates]: https://github.com/smallstep/certificates
[github_releases]: https://github.com/adnoctem/gopskit/releases
[github_commits]: https://github.com/adnoctem/gopskit/commits/main/

<!-- Third-party -->

[renovate]: https://renovatebot.com/
[precommit]: https://pre-commit.com/
