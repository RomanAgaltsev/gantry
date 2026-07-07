# Upgrade notes

Human-readable notes for changes that need operator attention, complementing the
commit-level [CHANGELOG](../CHANGELOG.md). Add a line here when a plan lands a
behaviour change, a new config field, or a default that operators may want to review.

## Recent changes

- **GitLab "latest" now excludes prereleases**, matching GitHub's `/releases/latest`. A
  team relying on RC tags auto-deploying must promote stable releases instead.
  (Design D5.)
- **New daemon config:** `daemon.reconcile_timeout` (default `5m`) bounds one
  environment's reconcile — a wedged remote command (e.g. a stuck `docker compose pull`)
  fails that env after the timeout instead of blocking the loop; `/healthz` keeps
  answering `ok`. `daemon.reconcile_failed_repeat` (default `1h`) collapses a flapping
  host's `reconcile_failed` notifications to one per window.
- **`git.remote` (opt-in pull/push):** set `git.remote.pull`/`push` (with a token) so the
  daemon ff-only-pulls before each cycle and pushes after a committing one, enabling
  fleet-safe shared state across multiple daemons. (Design D1.)
- **Email `smtp.tls`:** accepts `starttls` or `implicit` for the email notification channel.
- **Notification kinds `slack` and `telegram`:** in addition to `webhook` and `email`.
  `slack`/`telegram` are webhook-shaped (set `url`; `telegram` also needs `chat_id`).
- **Drift gauge now clears on resolve:** `gantry_drift_age_seconds` is written every
  reconcile pass and resets to `0` when an environment is no longer drifted, so a
  `GantryDriftStuck`-style alert auto-resolves. See the example rules in
  [observability.md](observability.md).
