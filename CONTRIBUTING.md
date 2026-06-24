# Contributing to gantry

Thanks for contributing! gantry is a single-module Go CLI
(`github.com/RomanAgaltsev/gantry`) — a non-Kubernetes release orchestrator.

## Prerequisites

- Go 1.26+
- [Task](https://taskfile.dev): `go install github.com/go-task/task/v3/cmd/task@latest`

`task setup` installs the pinned dev tools (golangci-lint, gofumpt, gci) into `./bin`.

## Everyday commands

All commands run through `Taskfile.yml` so local == CI:

```bash
task            # list available tasks
task ci         # full local gate: tidy, vet, lint, race tests
task lint       # golangci-lint (strict ruleset)
task format     # gofumpt + gci
task test       # race + shuffled tests
task build      # build ./bin/gantry
```

## Commit & PR conventions

- We **squash-merge** PRs. The **PR title** becomes the commit on `main` and drives
  release-please, so it **must** be a
  [Conventional Commit](https://www.conventionalcommits.org/): `feat: ...`, `fix: ...`,
  `chore: ...`, `docs: ...`, `refactor: ...`, `test: ...`, `build: ...`, `ci: ...`,
  `perf: ...`. Scope optional: `feat(engine): ...`.
- Breaking changes: add `!` (`feat!: ...`) or a `BREAKING CHANGE:` footer. Pre-1.0 this
  bumps the minor version.
- A `pr-title` check enforces the convention.

## Before opening a PR

Run `task ci` and make sure it is green.

## Releasing

Releases are automated by
[release-please](https://github.com/googleapis/release-please-action). Merging
Conventional-Commit PRs to `main` keeps a standing **release PR** that updates
`CHANGELOG.md` and the version. Merging that PR tags `vX.Y.Z`, and GoReleaser publishes
the binaries.


## Maintainer setup (one-time)

These steps require repo-admin access and cannot be committed as code:

- [ ] **Codecov:** add the repo at codecov.io and set the `CODECOV_TOKEN` repo secret
  (`Settings → Secrets and variables → Actions`).
- [ ] **Dependabot:** enable Dependabot (`Settings → Code security`); it picks up
  `.github/dependabot.yml` automatically.
- [ ] **Actions permissions:** `Settings → Actions → General → Workflow permissions` =
  "Read and write" + "Allow GitHub Actions to create and approve pull requests" (so
  release-please can open release PRs).
- [ ] **Code scanning:** ensure CodeQL/Code scanning is enabled (default once
  `security.yml` runs).
- [ ] **Branch protection:** require the `lint`, `test`, `codeql`, `govulncheck`, and
  `pr-title` checks on `main`.
