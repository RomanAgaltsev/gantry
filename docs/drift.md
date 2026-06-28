# Drift detection

gantry's poller closes the gap between a component's latest published Release and its
pin by writing the pin. **Drift** is what gantry calls the situation where that gap has
been left open too long — a Release was published but never consumed. Silent drift is
the incident gantry exists to prevent, so `gantry drift` makes it loud.

## What counts as drift

For each **track-mode** environment, and each component pinned from a forge Release
(explicit-pin components are skipped — gantry has no notion of their "latest"):

- the component **has drifted** when its latest published Release's image reference
  differs from the current pin **and** that Release was published more than
  `drift.threshold` ago (measured from the Release's own `built_at`).

A newer Release that is still within the threshold is **not** drift — the poller simply
has not run yet, which is normal.

Promote-target environments (those with `promote_from`) are never checked: they
intentionally lag their upstream, so alarming on that would fire forever.

## Configuration

```yaml
drift:
  threshold: 7d   # default 7d; accepts d/h/m (e.g. 72h, 14d)
```

## Usage

```bash
gantry drift --env test   # check one environment
gantry drift --all        # check every track-mode environment
```

Output is one line per drifted component:

```
DRIFT test/api: pinned reg/api:v1.4.2, latest v1.5.0 published 9d ago (>7d)
```

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | no drift |
| `3` | drift detected (gate CI on this) |
| `1` | operational error (forge unreachable, bad config) |
| `2` | usage error (bad flags) |

Run it in CI to turn drift into a red build:

```bash
gantry drift --all   # non-zero (3) fails the job
```
