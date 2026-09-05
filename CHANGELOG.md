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
