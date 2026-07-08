# Changelog

## [0.19.0](https://github.com/RomanAgaltsev/gantry/compare/v0.18.1...v0.19.0) (2026-07-08)


### Features

* **cli:** --output json on status/history/drift/plan (§9.2) ([b372196](https://github.com/RomanAgaltsev/gantry/commit/b372196e4a72afef76f17115117026e41e3e31e2))
* **cli:** gantry diff between two environments' pins (§9.11) ([0ac36b9](https://github.com/RomanAgaltsev/gantry/commit/0ac36b9349b72c437b0eb98a692370e555a6c186))
* **cli:** gantry validate — schema, secrets, and pin/config coherence (§9.1) ([c170af4](https://github.com/RomanAgaltsev/gantry/commit/c170af41fe2ea2e35777ae8eefa88c4e370fcc99))
* **drift:** per-component drift_threshold overriding the global (§9.12) ([a57245b](https://github.com/RomanAgaltsev/gantry/commit/a57245be341c2592493d74a0746e088b88beb54b))
* **promote:** --only for subset promotion with an unvalidated-unit warning (§9.5) ([62d14f0](https://github.com/RomanAgaltsev/gantry/commit/62d14f0a7945b18ec2d6040404832cf3729f953c))
* review features ([a9a9eda](https://github.com/RomanAgaltsev/gantry/commit/a9a9eda9aa3ad7d544c3696186877a03cc652e47))
* **rollback:** --fast symlink flip to the previous release (§9.7) ([5935998](https://github.com/RomanAgaltsev/gantry/commit/5935998815a715a80e8ce1c28e5553abb9230fac))
* **secrets:** native in-process vault-http backend for the distroless image (§9.13) ([71e1603](https://github.com/RomanAgaltsev/gantry/commit/71e1603dfaa39e795e021ad4ea11e464ea4c9354))
* **serve:** reload config on SIGHUP without restarting the process (§9.9) ([567f8a7](https://github.com/RomanAgaltsev/gantry/commit/567f8a73018232a6fe364f0a0599fd15fbdd5177))
* **status:** --watch refresh loop over the status matrix (§9.18) ([c628a20](https://github.com/RomanAgaltsev/gantry/commit/c628a20bafc0ab66407dc3154ab443dc7a30f6db))


### Bug Fixes

* TestStatusCmd_WatchRendersUntilCancelled ([ea9ec53](https://github.com/RomanAgaltsev/gantry/commit/ea9ec532a4b1ee00116ce4a0c4363bcf2464ae1d))

## [0.18.1](https://github.com/RomanAgaltsev/gantry/compare/v0.18.0...v0.18.1) (2026-07-07)


### Bug Fixes

* performance ([050a698](https://github.com/RomanAgaltsev/gantry/commit/050a698ffafcbb9cb7999d2a2400aa00bbd902ed))


### Performance Improvements

* fetch releases once per cycle and in bounded parallel (P1) ([5794f53](https://github.com/RomanAgaltsev/gantry/commit/5794f53f905b90d6c02c8bc01147ce4e155dcefe))
* **ledger:** cache parsed entries on file mtime, invalidate on record (P2/D4) ([52b273f](https://github.com/RomanAgaltsev/gantry/commit/52b273fb2bb0e08f5ec55230cdc48c976c9c8a92))
* **notify:** dispatch a single event's channels concurrently (P4) ([8045614](https://github.com/RomanAgaltsev/gantry/commit/8045614f5bd1c88056a6d7907f2d1525e65d36da))
* **symlink-release:** prune old release dirs with keep:N (P3) ([97acaa8](https://github.com/RomanAgaltsev/gantry/commit/97acaa8b389f9523e545900bbbee8f3da6d48157))

## [0.18.0](https://github.com/RomanAgaltsev/gantry/compare/v0.17.2...v0.18.0) (2026-07-07)


### Features

* **daemon:** HMAC-SHA256 doorbell signatures (X-Hub-Signature-256) (S1) ([d11ed3a](https://github.com/RomanAgaltsev/gantry/commit/d11ed3a42f1d362da05b0366124ab79c29dd8f27))


### Bug Fixes

* **daemon:** bind HTTP server to localhost by default (S1) ([7d27af1](https://github.com/RomanAgaltsev/gantry/commit/7d27af1df7579790ac6aec80dd4de8682152e4a8))
* security ([7195ccd](https://github.com/RomanAgaltsev/gantry/commit/7195ccdd247fc5d22c3c76cd9bc38487fef5f2f9))

## [0.17.2](https://github.com/RomanAgaltsev/gantry/compare/v0.17.1...v0.17.2) (2026-07-07)


### Bug Fixes

* consistency cleanup ([a33980c](https://github.com/RomanAgaltsev/gantry/commit/a33980c3303d02accf205c18fabc0417291ed594))
* **daemon:** make serve lock race-free via kernel flock (C6) ([87c1049](https://github.com/RomanAgaltsev/gantry/commit/87c1049e1bf39c8a951353010fdd371a668ef6a3))

## [0.17.1](https://github.com/RomanAgaltsev/gantry/compare/v0.17.0...v0.17.1) (2026-07-07)


### Bug Fixes

* architecture ([2835598](https://github.com/RomanAgaltsev/gantry/commit/28355982f9c34418087abff5664af83930c30f8e))

## [0.17.0](https://github.com/RomanAgaltsev/gantry/compare/v0.16.0...v0.17.0) (2026-07-07)


### Features

* **daemon:** reconcile_failed alert with repeat suppression (D7) ([7ac5ea8](https://github.com/RomanAgaltsev/gantry/commit/7ac5ea8bf45a22ff74ba9de01ea79ac3e91b8805))
* **engine:** warn when a declared pin_key is missing from the deployed set (D3) ([aa5dafe](https://github.com/RomanAgaltsev/gantry/commit/aa5dafe12a19a7d7def11e67e961e59306e82321))
* git.remote pull/push topology for fleet-safe daemon operation (D1) ([13f07a3](https://github.com/RomanAgaltsev/gantry/commit/13f07a32e6fc391cd66fd4b02969293d0d8a8743))
* **notify:** first-class slack and telegram kinds over the webhook core (D6) ([2c10375](https://github.com/RomanAgaltsev/gantry/commit/2c10375c29143c09ea7e9bc6e08de77a963fc997))
* report orphan pins and add gantry prune for pin-file subtraction (D2) ([1c1f51f](https://github.com/RomanAgaltsev/gantry/commit/1c1f51f06778798a78fc339414aad122f4bb11f9))


### Bug Fixes

* design - domain model ([370fe04](https://github.com/RomanAgaltsev/gantry/commit/370fe04bee350420f7df4f2cc4655989828548c3))
* **gitlab:** skip prereleases so latest matches GitHub semantics (D5) ([eb02440](https://github.com/RomanAgaltsev/gantry/commit/eb024404e8fab9d2acac2d7c589506df1aa3396c))

## [0.16.0](https://github.com/RomanAgaltsev/gantry/compare/v0.15.0...v0.16.0) (2026-07-07)


### Features

* **notify:** support implicit-TLS SMTP and an explicit smtp.tls choice (C9) ([b7e9543](https://github.com/RomanAgaltsev/gantry/commit/b7e9543a4aae33d1b85df340adf1dc0ce98a0834))


### Bug Fixes

* **cli:** error on a bad serve --interval and accept the day suffix (C4) ([edb2341](https://github.com/RomanAgaltsev/gantry/commit/edb23410941d9755402b2b0fa2c89f908418b01b))
* correctness ([ab3a174](https://github.com/RomanAgaltsev/gantry/commit/ab3a174bf826611949b720cdf657029f34905a4b))
* **daemon:** bound each environment's reconcile with daemon.reconcile_timeout (C2) ([0ddea70](https://github.com/RomanAgaltsev/gantry/commit/0ddea700d7177b979cf8f4f7a14e2ee03068cf6c))
* **daemon:** close pooled SSH connection after each reconcile (C3) ([e4801ed](https://github.com/RomanAgaltsev/gantry/commit/e4801ed3c1fd89448d7f5a198e62d39c4abf2110))
* **daemon:** reset drift gauge on resolve and report oldest drifted component (C1) ([0efd1b1](https://github.com/RomanAgaltsev/gantry/commit/0efd1b15455db0c8ff506f064b2ee4bef782bb5f))
* **lock:** close TOCTOU in stale-lock steal so exactly one racer wins (C6) ([f0caa9f](https://github.com/RomanAgaltsev/gantry/commit/f0caa9f5154bd4fc866e286e64725b9263ff0280))
* **lock:** mutating verbs acquire the lock; steal a stale lock atomically (C6) ([0126ad9](https://github.com/RomanAgaltsev/gantry/commit/0126ad9ef844d4827a3306d0ffe27de454b5575e))
* **notify:** render drift-alarm durations with one shared formatter (C8) ([5855d86](https://github.com/RomanAgaltsev/gantry/commit/5855d8683ee56ed1ceec99986d8c4d08ae0b3cfe))
* **pin:** reject duplicate keys in a pin file instead of silently keeping the last (C7) ([53171c9](https://github.com/RomanAgaltsev/gantry/commit/53171c9d55edf08c58845a0d5caa72ca17a76d63))
* **status:** degrade a failing component to (error) instead of failing the matrix (C5) ([50c49c7](https://github.com/RomanAgaltsev/gantry/commit/50c49c7ebc3b02e8215327fb98cc359f34d5f3f4))

## [0.15.0](https://github.com/RomanAgaltsev/gantry/compare/v0.14.0...v0.15.0) (2026-07-03)


### Features

* **config:** add ${cmd:…} secret backend ([2376cac](https://github.com/RomanAgaltsev/gantry/commit/2376cac65adf02ae165f9958a6d8ee1da422b831))
* **config:** add ${sops:path#key} secret backend ([3a1dcde](https://github.com/RomanAgaltsev/gantry/commit/3a1dcdee47645216b7c531f2264fc771865389c9))
* **config:** add ${vault:…} secret backend with secrets.vault defaults ([072bf76](https://github.com/RomanAgaltsev/gantry/commit/072bf76cb8e750bfaa68a48be72962fdcef4a3b6))
* **config:** add an injectable command runner to SecretResolver ([9806a25](https://github.com/RomanAgaltsev/gantry/commit/9806a25297d1e2dbd13415adc54fa250febfe9cc))
* secret backends ([17ffc9b](https://github.com/RomanAgaltsev/gantry/commit/17ffc9b9076fdf38309d366be3fd814ac3088285))

## [0.14.0](https://github.com/RomanAgaltsev/gantry/compare/v0.13.0...v0.14.0) (2026-07-03)


### Features

* **cli:** mount the doorbell and feed it into the reconcile loop ([1dd8375](https://github.com/RomanAgaltsev/gantry/commit/1dd8375c6a3784ea6831257464943c6e7828a3bc))
* **daemon:** add authenticated debounced doorbell handler ([15cbd47](https://github.com/RomanAgaltsev/gantry/commit/15cbd47d09ff99165330a47f800f2c8eafff9886))
* doorbell ([2a59283](https://github.com/RomanAgaltsev/gantry/commit/2a59283d2ad67573336a94f009be7403992c9292))

## [0.13.0](https://github.com/RomanAgaltsev/gantry/compare/v0.12.0...v0.13.0) (2026-07-03)


### Features

* **cli:** expose /metrics and wire the Prometheus observer into serve ([06026a6](https://github.com/RomanAgaltsev/gantry/commit/06026a620969e2aa667cec1a9f8b88e3e3b71e77))
* daemon metrics ([939b4fc](https://github.com/RomanAgaltsev/gantry/commit/939b4fc94542b59feff32199bd5e498301b8dc49))
* **daemon:** add Prometheus observer with reconcile/deploy/drift metrics ([1e16e68](https://github.com/RomanAgaltsev/gantry/commit/1e16e6822e9af80410e794eca5c49db851036a0d))

## [0.12.0](https://github.com/RomanAgaltsev/gantry/compare/v0.11.0...v0.12.0) (2026-07-03)


### Features

* **cli:** add serve command and guard mutating verbs against a running ([2566d89](https://github.com/RomanAgaltsev/gantry/commit/2566d893cc048848384f5ba0b5519dbea2e3b92e))
* **config:** add optional daemon block with defaults and validation ([3f00e5d](https://github.com/RomanAgaltsev/gantry/commit/3f00e5d08ad6c8e03884b13478231a822ee1169d))
* daemon core ([1032b06](https://github.com/RomanAgaltsev/gantry/commit/1032b063427825ac943aecc74cc3ff394df8a32f))
* **daemon:** add advisory single-writer lockfile with staleness reclaim ([fb92c3e](https://github.com/RomanAgaltsev/gantry/commit/fb92c3e3c6e086b8ecefb2445271d81a0297f542))
* **daemon:** add reconcile loop over track-mode environments ([f31e0ab](https://github.com/RomanAgaltsev/gantry/commit/f31e0abdef3cc1afd90ccb3f14da210a1ea6fe77))
* **notify:** build a Dispatcher from config (shared by daemon and CLI) ([e5473f5](https://github.com/RomanAgaltsev/gantry/commit/e5473f5fbe6c4a0e03cf77c56e2f4992196de739))

## [0.11.0](https://github.com/RomanAgaltsev/gantry/compare/v0.10.0...v0.11.0) (2026-07-03)


### Features

* **cli:** wire compose-ps verify to the executor's kind-aware target ([7ff1ccd](https://github.com/RomanAgaltsev/gantry/commit/7ff1ccd4701193a4fd95464dcec6934ae8c1059b))
* **config:** allow compose-ps on all executor kinds (kind-aware verify) ([9581bb5](https://github.com/RomanAgaltsev/gantry/commit/9581bb562d7b060b68cbd433daa8967a9602b879))
* **engine:** blue-green deploys hold on failed verify (no auto-flip) ([b44d9d1](https://github.com/RomanAgaltsev/gantry/commit/b44d9d1792965150763d222d5943ca2dff23dda9))
* **engine:** gate switch on a fresh idle-slot verify ([4960fed](https://github.com/RomanAgaltsev/gantry/commit/4960fed8eda18e6537a185a33cc6a0b83ed45a4a))
* **executor:** implement ComposeTarget for all three executor kinds ([8740a84](https://github.com/RomanAgaltsev/gantry/commit/8740a84245e3bf9030a3a4cf9eb4416594e1df71))
* kind aware verification ([57b91c0](https://github.com/RomanAgaltsev/gantry/commit/57b91c0b60652928ca5f3c4a05727c9bd1cc9e0d))
* **verify:** resolve compose-ps target at verify time via ([ec86852](https://github.com/RomanAgaltsev/gantry/commit/ec868529028ae13e70fca9914633984ee6d417aa))

## [0.10.0](https://github.com/RomanAgaltsev/gantry/compare/v0.9.0...v0.10.0) (2026-07-03)


### Features

* **cli:** dispatch notifications for deploy/promote/rollback/drift ([c00b19b](https://github.com/RomanAgaltsev/gantry/commit/c00b19b4e38e79072938f2a18f96bd9f8fc993ca))
* **cli:** map engine results to notification events ([6f7701b](https://github.com/RomanAgaltsev/gantry/commit/6f7701b65c3f6f663338578417feafdcdfbab4d3))
* **config:** add notifications block (webhook|email) with validation ([feb8ffb](https://github.com/RomanAgaltsev/gantry/commit/feb8ffbd5ecdd88b758addcc747a80d62a3ab1ad))
* **engine:** expose VerifyFailed on deploy/sync/promote results ([e4018ce](https://github.com/RomanAgaltsev/gantry/commit/e4018cea53570482a3810c6998dc12d3037cfdcd))
* notifications ([a9d751e](https://github.com/RomanAgaltsev/gantry/commit/a9d751edb8fe6fbf7add74ecd0fe9ac324b5ce4d))
* **notify:** add Event, Notifier, and best-effort Dispatcher ([b36a066](https://github.com/RomanAgaltsev/gantry/commit/b36a06609e05f5a9845b89f5281fcabcc2788968))
* **notify:** add SMTP email backend ([8964e1c](https://github.com/RomanAgaltsev/gantry/commit/8964e1cf8509c7087f4fb1c52259840b292ff79b))
* **notify:** add Telegram-compatible webhook backend ([04bfdf9](https://github.com/RomanAgaltsev/gantry/commit/04bfdf995f4ea52652cdcff10d0d7cc7e071be05))

## [0.9.0](https://github.com/RomanAgaltsev/gantry/compare/v0.8.1...v0.9.0) (2026-07-03)


### Features

* auto rollback ([a0e6142](https://github.com/RomanAgaltsev/gantry/commit/a0e6142e77ac36b9b872ef8eff68b0b0c08db164))
* **cli:** note the auto-rollback when a verify-triggered rollback ([48e0198](https://github.com/RomanAgaltsev/gantry/commit/48e019838fd2141357c0912922344b287191dd3d))
* **config:** add per-env verify_on_failure (hold|rollback) ([4fed230](https://github.com/RomanAgaltsev/gantry/commit/4fed23015eb5fcc95d2bc726e743d5d95d996ce5))
* **engine:** auto-rollback on failed verify when verify_on_failure=rollback ([14e09d0](https://github.com/RomanAgaltsev/gantry/commit/14e09d0fb8fb8c5aafbd7d30459e6094b5a95ac5))


### Bug Fixes

* fmt ([35bc252](https://github.com/RomanAgaltsev/gantry/commit/35bc2528672d97d1ef988e74fdd880654ec6b0bd))

## [0.8.1](https://github.com/RomanAgaltsev/gantry/compare/v0.8.0...v0.8.1) (2026-07-01)


### Bug Fixes

* **bluegreen:** surface pointer-read errors instead of assuming bootstrap ([2b68130](https://github.com/RomanAgaltsev/gantry/commit/2b68130523b38e8afdcbaabcebaa87f42d24aa8c))
* **composessh:** honor context cancellation for remote commands ([817f414](https://github.com/RomanAgaltsev/gantry/commit/817f414c4c67dfcfebe9faee24bd5bec4f39951e))
* **config:** error when a referenced env secret is unset ([73a86b1](https://github.com/RomanAgaltsev/gantry/commit/73a86b14ddb918bfa830fdbf860cb37cfcc6343a))
* **config:** reject compose-ps verify on non-compose-over-ssh executors ([d16f253](https://github.com/RomanAgaltsev/gantry/commit/d16f2539783815d1a98d916cba130a7e0a6937ca))
* **engine:** run the configured verifier during deploy ([c203cdd](https://github.com/RomanAgaltsev/gantry/commit/c203cdd3786a4b3efb5b26d36a6331af7afe4e63))
* **forge:** bound error-response body reads and fix doc typos ([6f5c50c](https://github.com/RomanAgaltsev/gantry/commit/6f5c50c07b416dba102e9721871148b0bca6b734))
* review issues ([5ec90ec](https://github.com/RomanAgaltsev/gantry/commit/5ec90ec89b3885eaa4e21d09ef0090c834df49f5))

## [0.8.0](https://github.com/RomanAgaltsev/gantry/compare/v0.7.0...v0.8.0) (2026-06-29)


### Features

* blue-green executor ([a56aea9](https://github.com/RomanAgaltsev/gantry/commit/a56aea9ae0bf26a21c0f019c6c33f2d53e626d77))
* **cli:** add --log-format/--log-level and inject logger into context ([c94222c](https://github.com/RomanAgaltsev/gantry/commit/c94222c10599614918e24d16a8996cb939a2ea26))
* **cli:** add gantry switch and blue-green executor wiring ([5a6ac07](https://github.com/RomanAgaltsev/gantry/commit/5a6ac07f674febb23092821d192e8f14d8adf912))
* **cli:** add status --all matrix and make buildDeps env-optional ([06ec9de](https://github.com/RomanAgaltsev/gantry/commit/06ec9def8550762875c2f5ef67798b0ba0967e6f))
* **config:** add blue-green executor (slots + pointer) block and ([df84454](https://github.com/RomanAgaltsev/gantry/commit/df8445484db4e30337fff9dcdcb919a40de9a178))
* **engine:** add FormatMatrix renderer ([995cfd2](https://github.com/RomanAgaltsev/gantry/commit/995cfd24fbd20b52a4048239b17184f34c92d51b))
* **engine:** add StatusMatrix read model ([03495e4](https://github.com/RomanAgaltsev/gantry/commit/03495e4f978af298bf0e151eb3b451db9727e4a7))
* **engine:** add Switch (gate the idle slot, flip the blue-green ([dc6bdf9](https://github.com/RomanAgaltsev/gantry/commit/dc6bdf951d877c79cf8c3aa2822d4cdaaf40e73e))
* **engine:** emit structured logs via context-carried logger ([327e625](https://github.com/RomanAgaltsev/gantry/commit/327e62553b24ccdeb09fce7afd835fbbd0d0f1d8))
* **engine:** roll back blue-green by flipping the pointer to the prior ([48bb628](https://github.com/RomanAgaltsev/gantry/commit/48bb6281fc46c116b13732f0e94f186ee4375128))
* **executor:** add blue-green pointer switch (atomic flip + reload) ([e4e88e3](https://github.com/RomanAgaltsev/gantry/commit/e4e88e3c033c83cdd220320f545863b7fc4f50df))
* **executor:** add SlotExecutor capability and blue-green ([f35835e](https://github.com/RomanAgaltsev/gantry/commit/f35835e3cb5899e56c4e40a95f32c4a1cf1f2f85))
* **logging:** add slog seam with context carrier and discard fallback ([a10063f](https://github.com/RomanAgaltsev/gantry/commit/a10063fa0623e7e0e2c15e73746b917a72512c16))
* status matrix ([20b7960](https://github.com/RomanAgaltsev/gantry/commit/20b7960d3a5a320a622ed2cc9bc88e93c37b215f))

## [0.7.0](https://github.com/RomanAgaltsev/gantry/compare/v0.6.0...v0.7.0) (2026-06-29)


### Features

* **cli:** select executor by kind via a newExecutor factory ([d736941](https://github.com/RomanAgaltsev/gantry/commit/d73694137dd5201dcc673b5aba50c68736cc0648))
* **executor:** add symlink-release executor (versioned dir + atomic ([b7515a5](https://github.com/RomanAgaltsev/gantry/commit/b7515a5d102ac8719072edd6138ead156ad1ce46))
* **executor:** carry the pin commit SHA on Plan ([3dc7eb1](https://github.com/RomanAgaltsev/gantry/commit/3dc7eb11e76055294bc4b4bfa68e170240845581))
* symlink release executor ([dc97abf](https://github.com/RomanAgaltsev/gantry/commit/dc97abf22804f67f52d879bb6fd6ca4e560b0b9d))

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
