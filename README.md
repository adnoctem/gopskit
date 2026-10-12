<p align="center">
    <!-- Go Operations Toolkit -->
    <img src="https://github.com/adnoctem/artwork/blob/8a9d84ca8d91a61bd1f69fdc1ba6362ccbcb5060/projects/gopskit/icon/gopskit.svg?raw=true" alt="GOpsKit Logo" width="256">
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

Managing high-stakes backbone components like Vault or Keycloak themselves with Helm? [`carto`][carto] renders,
diffs, applies and rolls back charts using a deterministic base+overlay values-merge strategy - it's the intended
replacement for our [Helmfile][helmfile] workflows. The toolkit also offers a plethora of
other functionalities like setting up HashiCorp's [Vault][vault] with [`waltr`][waltr], registering various
applications for SSO authentication with [Keycloak][keycloak] using [`ssolo`][ssolo]. Are you running your own custom
private Certificate Authority using [Smallstep's CA][smallstep_certificates]? Then you'd likely want to generate and
manage PKI values using [`steppa`][steppa].

## ✨ TL;DR

```shell
# build all projects at once - requires Bazel at .bazelversion
bazel build //...
```

## 🛠️ Tools

Like most modern [Go][go] projects the various executables are located within the [cmd][cmd] directory, each with
its own README documenting how to use it. Here's a quick-reference list as an overview:

- [`carto`][carto]: manage Kubernetes backbone components (Helm charts) with the Helm SDK
- [`plattr`][plattr]: create a platform cluster's provider credentials, volume encryption and DH parameter Secrets
- [`ssolo`][ssolo]: set up Keycloak realms and the OIDC clients applications sign their users in with
- [`steppa`][steppa]: create a private certificate authority for [Smallstep's step-ca][smallstep_certificates]
- [`waltr`][waltr]: initialize, unseal and configure [HashiCorp's Vault][vault]

Every flag can also be set in a configuration file or, for secrets, the environment - see
[configuring the CLIs][configuration]. Services inside a cluster are reached through the [gopskit
bridge][bridge], which the CLIs set up with `<cli> proxy setup`.

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
[steppa]: cmd/steppa
[plattr]: cmd/plattr
[carto]: cmd/carto

<!-- File references -->

[license]: LICENSE
[contributing]: docs/CONTRIBUTING.md
[configuration]: docs/configuration.md
[bridge]: cmd/gopskit-bridge
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
