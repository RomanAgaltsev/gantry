# Changelog

## [0.6.0](https://github.com/RomanAgaltsev/gantry/compare/v0.5.0...v0.6.0) (2026-06-29)


### Features

* **cli:** build per-env verifiers and pass them into the engine ([c4d6562](https://github.com/RomanAgaltsev/gantry/commit/c4d656267bf07e1e4aeda30589bd1cbaa379762f))
* **config:** add per-env verify probes and promote.require_healthy ([7d847b1](https://github.com/RomanAgaltsev/gantry/commit/7d847b171f93642be35d256603397ea0d003d93e))
* **engine:** gate promotion on healthy when promote.require_healthy is ([7a1dcc6](https://github.com/RomanAgaltsev/gantry/commit/7a1dcc6d5f2050f15071085e91ad8132b60fa629))
* **engine:** run verification after deploy and record the healthy ([e6f688d](https://github.com/RomanAgaltsev/gantry/commit/e6f688dbbc0df3fbf2b2eb6fd38ddc541b65bcd1))
* health verification ([fa0bf03](https://github.com/RomanAgaltsev/gantry/commit/fa0bf037a696ca58049986acec382c3c5a166546))
* **ledger:** add LatestHealthy (most recent ok+healthy entry) ([75b4b03](https://github.com/RomanAgaltsev/gantry/commit/75b4b0379bffa7fdb5329e74904bacda0ba6529b))
* **verify:** add host-side command and compose-ps probes ([1fa9762](https://github.com/RomanAgaltsev/gantry/commit/1fa97628fe26f5ec8ee832b7a0ca36257b8ca053))
* **verify:** add Verifier interface, Composite, and gantry-side HTTP ([d9003dc](https://github.com/RomanAgaltsev/gantry/commit/d9003dcf57272729dfb05956d649b9d1f7e8c66c))

## [0.5.0](https://github.com/RomanAgaltsev/gantry/compare/v0.4.0...v0.5.0) (2026-06-29)


### Features

* **cli:** select forge adapter by kind via a newForge factory ([3fa3553](https://github.com/RomanAgaltsev/gantry/commit/3fa3553f2b1a6a5026cee0c04af27fa2771acf8a))
* **config:** accept forge.kind github and default its API base_url ([4109929](https://github.com/RomanAgaltsev/gantry/commit/410992943442e350799ab09fb234381d4ae66b21))
* **forge:** add GitHub Releases adapter ([01422ff](https://github.com/RomanAgaltsev/gantry/commit/01422ff340dcd77b6002834d927625fa26c1cd53))
* gh forge adapter ([a290f35](https://github.com/RomanAgaltsev/gantry/commit/a290f3505545b0e6fbe3548ad1d8997887774841))

## [0.4.0](https://github.com/RomanAgaltsev/gantry/compare/v0.3.1...v0.4.0) (2026-06-28)


### Features

* **cli:** add gantry drift command with exit code 3 on detected drift ([f39e3e7](https://github.com/RomanAgaltsev/gantry/commit/f39e3e708b3c2e72e9fd3ee71008f6c0034fa732))
* **config:** add drift.threshold with a day-aware duration type ([5c05c9f](https://github.com/RomanAgaltsev/gantry/commit/5c05c9f56bb5e576461f463a924725fea464bfab))
* drift detector ([7a8244b](https://github.com/RomanAgaltsev/gantry/commit/7a8244bc83641cb7827132347f9a5a8bf76133b1))
* **engine:** add read-only Drift detector keyed on release built_at age ([a18fa9f](https://github.com/RomanAgaltsev/gantry/commit/a18fa9f7aad6dcd8f09ceae33474642f20c5f560))

## [0.3.1](https://github.com/RomanAgaltsev/gantry/compare/v0.3.0...v0.3.1) (2026-06-27)


### Bug Fixes

* **cli:** only resolve forge and registry secrets when a command needs them ([e3c0d88](https://github.com/RomanAgaltsev/gantry/commit/e3c0d88c1db24c1be70ba2e4d1adf8f2342a2236))
* dry-run recovery wording, ledger doc typos, runbook, test filename ([299b3b6](https://github.com/RomanAgaltsev/gantry/commit/299b3b69df24fedede68aaee2863dbc7ea20bb1d))
* **engine,cli:** surface post-commit deploy failures and off-DAG promotes ([cbd6f3f](https://github.com/RomanAgaltsev/gantry/commit/cbd6f3f70dc7deddf31e37d190dd4735644936d4))
* **git:** refuse to commit when other files are staged ([1c84f36](https://github.com/RomanAgaltsev/gantry/commit/1c84f36f0b1e8b0f02762f8cc763748a6a135369))
* **promote:** accept short SHAs for --sha ([85782b8](https://github.com/RomanAgaltsev/gantry/commit/85782b84c538b7cde9427dcaff2945ef97d64734))
* review findings ([ca53c55](https://github.com/RomanAgaltsev/gantry/commit/ca53c555430153b71a0f47f3ec3ecf446123e0cd))
* **rollback:** target the last known-good ledger entry, not the parent commit ([4957d99](https://github.com/RomanAgaltsev/gantry/commit/4957d990264265998d64c72deff2aa7d8707e5db))

## [0.3.0](https://github.com/RomanAgaltsev/gantry/compare/v0.2.0...v0.3.0) (2026-06-25)


### Features

* add gated SHA-frozen promote (engine + CLI) ([ef15cdf](https://github.com/RomanAgaltsev/gantry/commit/ef15cdf688c9381a19b461ad5bf321a037d0caf2))
* add logical-revert rollback (engine + CLI) ([1fd6dfa](https://github.com/RomanAgaltsev/gantry/commit/1fd6dfab2efdddf836ed282dfd9311132967c497))
* **cli:** add history command over the deploy-outcome ledger ([96a5096](https://github.com/RomanAgaltsev/gantry/commit/96a50967de1aac3b8ff889f9a5ee9c0f876cc9d3))
* **engine:** add ReadAt/LatestCommit/ParentOf seams; WriteAndCommit ([6b20a54](https://github.com/RomanAgaltsev/gantry/commit/6b20a54143a9e193d3a3c7ced92767fb2c62e4d0))
* **engine:** make sync/deploy ledger-aware and self-healing ([844fd4d](https://github.com/RomanAgaltsev/gantry/commit/844fd4d2119b122001a831655afd4a5daa40208c))
* **ledger:** add Entry, Ledger interface, and pure query helpers ([fce9072](https://github.com/RomanAgaltsev/gantry/commit/fce90724f00230becde97a56e98da70b2a83ee7f))
* **ledger:** add git-backed .gantry/deploys.jsonl implementation ([98d5e0f](https://github.com/RomanAgaltsev/gantry/commit/98d5e0f3ab44eae8deac42f0919e712b59a5ba42))
* promote and rollback ([9dd97da](https://github.com/RomanAgaltsev/gantry/commit/9dd97da199d8336fa34ec2bee58cdbe5b210fc05))


### Bug Fixes

* linter issues ([ea8de5c](https://github.com/RomanAgaltsev/gantry/commit/ea8de5c875ede18bcfbbfb3832461a3b474e4b8d))

## [0.2.0](https://github.com/RomanAgaltsev/gantry/compare/v0.1.0...v0.2.0) (2026-06-24)


### Features

* bootstrap gantry CLI with version command ([2097e2a](https://github.com/RomanAgaltsev/gantry/commit/2097e2a874233bcdb5d5628472ecb85661c1264a))
* **cli:** status shows latest=(untracked) for explicit components ([1d67003](https://github.com/RomanAgaltsev/gantry/commit/1d67003d5a403cdadb38614dfadfa8b019d6c481))
* **cli:** wire sync, plan, and status commands ([4993466](https://github.com/RomanAgaltsev/gantry/commit/49934660c7814bf9f9e2eade6b238ff9a49a3a62))
* **config:** add component source discriminator (forge-release | ([d4c0fd1](https://github.com/RomanAgaltsev/gantry/commit/d4c0fd175c57c599ef9c7cdbf66dcb1fe643394a))
* **config:** add config model, loader, and validation ([b466d82](https://github.com/RomanAgaltsev/gantry/commit/b466d821b1008954e8023974a4a310cbaf113c65))
* **config:** add SecretRef indirection (env/file, no inline secrets) ([9e5d453](https://github.com/RomanAgaltsev/gantry/commit/9e5d4530a70ba410a74b3992abe48c1838bc2745))
* consume pin deploy ([f9bb7d9](https://github.com/RomanAgaltsev/gantry/commit/f9bb7d9be21f761291a1b9ae7e11b43692737321))
* **engine:** add Deploy reconcile + gantry deploy command ([602f814](https://github.com/RomanAgaltsev/gantry/commit/602f81497d617ddfa452cb143180c6bfbe5e29db))
* **engine:** add Sync flow and go-git pin store ([313071f](https://github.com/RomanAgaltsev/gantry/commit/313071f852362e39840d27aeedc91fd09d5850d1))
* **engine:** poller skips explicit-pin components ([5ac2c14](https://github.com/RomanAgaltsev/gantry/commit/5ac2c14110b2632bad3ae074247fef2a4b897cfe))
* **executor:** add compose-over-ssh executor with Runner seam ([8ae8a2b](https://github.com/RomanAgaltsev/gantry/commit/8ae8a2b3f4090c30f4b27eb42e3267c488a42456))
* **executor:** docker login private registries before pull (Unit B) ([e529536](https://github.com/RomanAgaltsev/gantry/commit/e529536c44e1285579e3759ea64eb09da67d5308))
* **forge:** add GitLab releases adapter ([d7dfd9b](https://github.com/RomanAgaltsev/gantry/commit/d7dfd9bd665f077fb638c2625661faef081f0659))
* **forge:** add Release type and metadata-block parser ([a0c2f48](https://github.com/RomanAgaltsev/gantry/commit/a0c2f48aa671ff514401b7ce5a2eb229e160b5a7))
* **pin:** add dotenv pin read/render and diff ([18657c4](https://github.com/RomanAgaltsev/gantry/commit/18657c487c41658e449eae0a775798ce4795f03f))


### Bug Fixes

* coverage command ([fddfab8](https://github.com/RomanAgaltsev/gantry/commit/fddfab82cc71a0475c045f16defc8508e4c3ad22))
* linter issues ([b7fb0cf](https://github.com/RomanAgaltsev/gantry/commit/b7fb0cfff742e00547571f0c0d76a0208f5c874c))
* review issues ([5b5a9f8](https://github.com/RomanAgaltsev/gantry/commit/5b5a9f8097da4b545b33102c384d545a76511488))

## Changelog

All notable changes to this project are documented here. This file is maintained
automatically by [release-please](https://github.com/googleapis/release-please-action);
do not edit it by hand.
