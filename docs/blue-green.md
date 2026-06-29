# Blue/green environments

A `blue-green` environment runs two slots of the same service behind a switchable pointer
(typically an nginx upstream). gantry stages a new version on the **idle** slot, leaves the
**live** slot serving, and promotes by flipping the pointer — instant, and instantly
reversible.

## Model

- One environment, one pin file (the staged/desired pins), two slots (`blue`, `green`).
- `gantry sync --env front` (or `deploy`) deploys the pins to the **idle** slot — the one
  the pointer is *not* routing to. The live slot is untouched.
- `gantry switch --env front` flips the pointer to the freshly-staged slot, gated on its
  deploy being `ok`. Idle and live swap.
- `gantry rollback --env front` flips the pointer back to the other slot, which still runs
  the previous version.

The pointer is a **symlink gantry owns**: it flips `pointer.link` between two host-provided
per-slot targets and runs `pointer.reload`. gantry never parses or templates nginx — you
provide the two upstream confs; gantry decides which is active.

## Configuration

```yaml
environments:
  - name: front
    source: { track: latest }
    pin_file: .env.versions.front
    executor:
      kind: blue-green
      connection: front-host
      slots:
        blue:  { project_dir: /opt/front-blue,  compose_files: [compose.yaml] }
        green: { project_dir: /opt/front-green, compose_files: [compose.yaml] }
      pointer:
        link:   /etc/nginx/conf.d/front-upstream.conf   # gantry flips this symlink
        blue:   /etc/nginx/conf.d/upstream-blue.conf      # you provide these two
        green:  /etc/nginx/conf.d/upstream-green.conf
        reload: "nginx -s reload"
```

## Bootstrap

On a fresh host the pointer link does not exist yet: the first `sync` stages `blue`, and
the first `switch` creates the link pointing at it. From then on slots alternate.

## Workflow

```bash
gantry sync   --env front   # stage the new version on the idle slot
gantry switch --env front   # promote it (flip the pointer), gated on an ok deploy
gantry rollback --env front # flip back to the prior slot if needed
gantry history --env front  # sync/switch/rollback are all recorded
```
