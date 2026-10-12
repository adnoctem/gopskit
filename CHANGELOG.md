## [1.2.0](https://github.com/adnoctem/gopskit/compare/v1.1.2...v1.2.0) (2026-10-12)

### Features

* **bridge:** add gopskit-bridge SOCKS5 server ([92f8b00](https://github.com/adnoctem/gopskit/commit/92f8b00103c5b28d7daea1d2ab62ec32f9128ae7))
* **bridge:** add proxy setup, check and cleanup commands ([28da46c](https://github.com/adnoctem/gopskit/commit/28da46c7b2f53b47ff26aa674b498cfb2c16ea8f))
* **bridge:** build and publish the gopskit-bridge image ([2814977](https://github.com/adnoctem/gopskit/commit/2814977c380090cdac90fca3ac72f088c0925f46))
* **pkg/api/vault:** add WithTransport option ([05a24c5](https://github.com/adnoctem/gopskit/commit/05a24c5781203976ae627699fa13db58dd021405))
* **pkg/config:** add configuration files mirroring CLI flags ([701da82](https://github.com/adnoctem/gopskit/commit/701da8232acb82e6a6f091b009e0fa77c2a84b9b))
* **pkg/log:** add WithOutputPaths option ([d269054](https://github.com/adnoctem/gopskit/commit/d26905440f3553345ae22db19a3141b3b4dd9df6))

### Bug Fixes

* **bridge:** answer slow dials instead of hanging up ([59addc2](https://github.com/adnoctem/gopskit/commit/59addc26f0d8cfd01648a73c6fb0e43efa6d8450))
* **build:** stamp the release version into binaries ([75e8202](https://github.com/adnoctem/gopskit/commit/75e8202f37e109bcdeb36f873c2fc36a2404964e))
* **pkg/api/keycloak:** keep login settings when loading persisted tokens ([8d4d901](https://github.com/adnoctem/gopskit/commit/8d4d901517340969fe128b907a9bae299db563cf))
* **pkg/kube:** stop racing on port-forward output buffers ([b65db23](https://github.com/adnoctem/gopskit/commit/b65db234f1b21f207264d5e7e4daa23e4a574e9b))
* **ssolo:** create OIDC clients and reach Keycloak by URL ([e1e29d5](https://github.com/adnoctem/gopskit/commit/e1e29d5dc5b9be3674f331ada1ef8357a3e86c98))
* **ssolo:** read settings from a configuration file ([c66fbdd](https://github.com/adnoctem/gopskit/commit/c66fbdd652a3c08166d17e9da9f82eb100d8be37))
* **ssolo:** reject an unparseable --vault-ca-cert up front ([dc913a3](https://github.com/adnoctem/gopskit/commit/dc913a35b492c3e88a96359ea43a784eb6fcd057))
* **waltr:** reach Vault by address and read settings from a config file ([a40c4f5](https://github.com/adnoctem/gopskit/commit/a40c4f50d38d37c8be34e122cdcd8a48213b1d21))

## [1.1.2](https://github.com/adnoctem/gopskit/compare/v1.1.1...v1.1.2) (2026-09-05)

### Bug Fixes

* add test suite for pkg/ and internal/, fixing bugs it surfaced ([0645539](https://github.com/adnoctem/gopskit/commit/0645539a8742fafea26133f88c181c3021aaf88e))

## [1.1.1](https://github.com/adnoctem/gopskit/compare/v1.1.0...v1.1.1) (2026-09-05)

### Bug Fixes

* bump transitive dependencies to avoid CVEs ([7441367](https://github.com/adnoctem/gopskit/commit/744136713dd0d1588baaf8abedc31f8f01fcee63))

## [1.1.0](https://github.com/adnoctem/gopskit/compare/v1.0.2...v1.1.0) (2026-09-05)

### Features

* **plattr:** add `plattr`  implementation ([e011a60](https://github.com/adnoctem/gopskit/commit/e011a60ac3d3a8c918c172ed949e7cb1d63d3340))

### Bug Fixes

* **pkg/log:** add global `Logger` ([483741d](https://github.com/adnoctem/gopskit/commit/483741de3931684ec57718ed6cf8dbb23d3d6543))
* **pkg/proc:** prevent read from closed `std` pipes ([ae55266](https://github.com/adnoctem/gopskit/commit/ae552667de48d6ed798ef3373ce3b80c8ed05250))
* **plattr:** fix bug causing panic ([119db49](https://github.com/adnoctem/gopskit/commit/119db49f9d0536f490c95d66c9938f5cd5631fef))
* **waltr:** fix bug causing panic ([acc06da](https://github.com/adnoctem/gopskit/commit/acc06daa1f0a371b1a62847c2fa3087d1be5eddf))

## [1.0.2](https://github.com/adnoctem/gopskit/compare/v1.0.1...v1.0.2) (2024-11-21)

### Bug Fixes

- **waltr:** add missing `transit` command ([4a4e68b](https://github.com/adnoctem/gopskit/commit/4a4e68bef58c09326650fb53c4482962f340f5bc))

## [1.0.1](https://github.com/adnoctem/gopskit/compare/v1.0.0...v1.0.1) (2024-11-18)

### Bug Fixes

- **waltr:** fixed some issues causing goroutine deadlocks ([f5d521c](https://github.com/adnoctem/gopskit/commit/f5d521c715bb094c495611f93cdc436f701accdc))

### Performance Improvements

- **proc:** rename `cmd` package to `proc` ([3f9aa65](https://github.com/adnoctem/gopskit/commit/3f9aa653dadc87ee96ca17df3eebe81495a816b6))
- **waltr:** update `waltr` commands to new `app` APIs ([9fc9025](https://github.com/adnoctem/gopskit/commit/9fc90254ae35ec47f6f179a33a4d861ccc73a610))
- **waltr:** update the central `app` package ([a59b2d5](https://github.com/adnoctem/gopskit/commit/a59b2d52594311c0a1a0b69bee8cfd6a526629d7))

## 1.0.0 (2024-09-24)

### Features

- add `common` central application type ([c0478ac](https://github.com/adnoctem/gopskit/commit/c0478aca177a76e31a080757a3af393fca89e44d))
- add `steppa` executable ([90cfa78](https://github.com/adnoctem/gopskit/commit/90cfa78c0bf72b1dbc6a4197b83d8ed271c54d51))
- add beta `fillr` implementation ([ed61f74](https://github.com/adnoctem/gopskit/commit/ed61f743fd3a6dc924655f1e4e77a865b0e072e2))
- add root and subcommands for `waltr` ([431e825](https://github.com/adnoctem/gopskit/commit/431e825e9b986ec9e9c3aa152e9ed4a32fe8115d))
- implement new central `App` type ([376ed0a](https://github.com/adnoctem/gopskit/commit/376ed0aab9c7a0568847709f1a9842520f80666a))
- implement root command for `ssolo` ([50341db](https://github.com/adnoctem/gopskit/commit/50341db06c89aafbd9eef805c72d7e3681fea4b9))
- implement root command for `steppa` ([92b7467](https://github.com/adnoctem/gopskit/commit/92b746700defdc8ec3e9379e3e05f55f4865184c))
- overhauled `core` package ([436060a](https://github.com/adnoctem/gopskit/commit/436060ac78604f4e39c91504e2d1666514cd0c77))
- **pkg/kube:** add `apply` implementation ([a2a1c88](https://github.com/adnoctem/gopskit/commit/a2a1c88aa0207c8427c7248fdbc50f59c827968b))
- **pkg/kube:** add `create` methods ([9a77d9c](https://github.com/adnoctem/gopskit/commit/9a77d9c76ee4f985bcd900d438aefa77b4bc14b0))
- **pkg/kube:** add `get` methods ([acde2b1](https://github.com/adnoctem/gopskit/commit/acde2b1e9a0dbbcbe418a101410b427122b9d45a))
- **pkg/kube:** add `port-forward` implementation ([b0d247a](https://github.com/adnoctem/gopskit/commit/b0d247ac80d3a0a5e3b5ab00e600f0b72cc374e3))
- **pkg/tools:** add helm functions ([17e00e5](https://github.com/adnoctem/gopskit/commit/17e00e526227ed0f79ee80672685bc0205dc5961))
- **pkg/util:** new changes ([5931ea0](https://github.com/adnoctem/gopskit/commit/5931ea09e559d23cdead8af86c4644a8afaf9377))
- rename `opsctl` to `ssolo` ([1f3663f](https://github.com/adnoctem/gopskit/commit/1f3663f2c0e32afd85dfdaf006fce607cb4a1484))
- renamed and rewrote `core` package to `cmd` ([48b7d73](https://github.com/adnoctem/gopskit/commit/48b7d739d4a8427ef477fb5b75f3751775287926))
- switch to `semantic-release` ([430a672](https://github.com/adnoctem/gopskit/commit/430a672d41258dc63861c8a1b581aa06768e85ce))
- update `pkg` modules to new `cmd` package API ([35a2f23](https://github.com/adnoctem/gopskit/commit/35a2f230f25d890576c5e034a36a45c735391523))
- **waltr:** add `configure` command ([8ebbe59](https://github.com/adnoctem/gopskit/commit/8ebbe5954c84548ffebef5db0657d4c41a4d285a))
- **waltr:** add `methods` command ([7b837f3](https://github.com/adnoctem/gopskit/commit/7b837f384d88a62e4e765187de52b49a3cc663f4))
- **waltr:** add `prepare keycloak` subcommand ([4865220](https://github.com/adnoctem/gopskit/commit/486522062e586a2def96350503e37492df70fa23))
- **waltr:** add `transit` command ([4df3d14](https://github.com/adnoctem/gopskit/commit/4df3d1490b88a39f267ddf387743b42e1325d792))

### Bug Fixes

- add secondary `init()` call ([4ca03bb](https://github.com/adnoctem/gopskit/commit/4ca03bbadaca283e4314cb7bc03fac005db0991c))
- **pkg/env:** fix invalid indexing of `String` method for `Environment` ([2d4b756](https://github.com/adnoctem/gopskit/commit/2d4b756351053196af68148943b8af77ced6ce4e))
- **pkg/filesystem:** fix inconsistent file reads ([888ee85](https://github.com/adnoctem/gopskit/commit/888ee85c71cdea9c873a9d81a9219050a516c30b))
- **pre-commit:** the `excludes` to pre-commit config ([c628fbf](https://github.com/adnoctem/gopskit/commit/c628fbf77a1646bfc8f268720743f3845d0d9898))
- **release:** add multi-arch/platform support ([0753c6b](https://github.com/adnoctem/gopskit/commit/0753c6b5f211323f1655dd5ad218662f2cfcd67f))
