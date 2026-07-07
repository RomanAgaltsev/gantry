// Package symlinkrelease deploys via a versioned release directory and an atomic `current`
// symlink, running compose from the active release. It composes the compose-over-ssh Runner
// and RunCompose helper rather than reinventing SSH (roadmap B2-D3).
package symlinkrelease

import (
	"context"
	"fmt"
	"path"
	"time"

	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/executor/composessh"
	"github.com/RomanAgaltsev/gantry/internal/pin"
	"github.com/RomanAgaltsev/gantry/internal/verify"
)

var _ verify.ComposeVerifiable = (*Executor)(nil)

const (
	releasesDir  = "releases"
	currentLink  = "current"
	versionFile  = ".version"
	envInRelease = currentLink + "/.env"
)

// Executor deploys each pin set into releases/<commit> and atomically flips `current` to it.
// Keep, when >0, bounds retained release dirs: after a successful deploy the oldest dirs beyond
// the newest Keep are removed (never the active one). Keep==0 retains all (today's behavior).
type Executor struct {
	Runner       composessh.Runner
	ProjectDir   string
	ComposeFiles []string
	Logins       []composessh.RegistryLogin
	Keep         int
}

// Deploy lays down the release directory, flips the current symlink atomically, then runs
// compose from the active release.
func (e *Executor) Deploy(ctx context.Context, p executor.Plan) (executor.Result, error) {
	if p.Commit == "" {
		return executor.Result{}, fmt.Errorf("symlink-release requires a pin commit; none set for %q", p.Env)
	}
	rel := releasesDir + "/" + p.Commit
	relAbs := path.Join(e.ProjectDir, rel)

	if _, err := e.Runner.Run(ctx, "mkdir -p "+composessh.ShellQuote(relAbs), nil); err != nil {
		return executor.Result{}, fmt.Errorf("mkdir release dir: %w", err)
	}
	if _, err := e.Runner.Run(ctx, "cat > "+composessh.ShellQuote(path.Join(relAbs, ".env")), pin.Render(p.Pins)); err != nil {
		return executor.Result{}, fmt.Errorf("write release env: %w", err)
	}
	verLine := fmt.Sprintf("%s %s\n", p.Commit, time.Now().UTC().Format(time.RFC3339))
	if _, err := e.Runner.Run(ctx, "cat > "+composessh.ShellQuote(path.Join(relAbs, versionFile)), []byte(verLine)); err != nil {
		return executor.Result{}, fmt.Errorf("write %s: %w", versionFile, err)
	}

	// Atomic flip: a relative-target temp symlink, then mv -T rename over `current`.
	tmp := path.Join(e.ProjectDir, ".current.tmp")
	cur := path.Join(e.ProjectDir, currentLink)
	flip := fmt.Sprintf("ln -sfn %s %s && mv -Tf %s %s",
		composessh.ShellQuote(rel), composessh.ShellQuote(tmp),
		composessh.ShellQuote(tmp), composessh.ShellQuote(cur))
	if _, err := e.Runner.Run(ctx, flip, nil); err != nil {
		return executor.Result{}, fmt.Errorf("flip current symlink: %w", err)
	}

	if err := composessh.RunCompose(ctx, e.Runner, composessh.ComposeOpts{
		ProjectDir:   e.ProjectDir,
		ComposeFiles: e.ComposeFiles,
		EnvFile:      envInRelease,
		Logins:       e.Logins,
	}, p.Pins); err != nil {
		return executor.Result{}, err
	}
	// Pruning is best-effort housekeeping; a failure must not fail an otherwise-good deploy.
	// The command's own stderr is captured by the runner error if it ever surfaces.
	_ = e.prune(ctx) //nolint:gosec // best-effort prune; never fails a good deploy
	return executor.Result{Changed: true, Detail: "symlink-release " + p.Commit}, nil
}

// prune removes release directories beyond the newest Keep, never the one `current` points
// at. A no-op when Keep<=0. One shell command over the runner keeps it a single round-trip.
// cur and d are shell-internal values derived from ls/readlink on the host, never
// gantry-interpolated config data, so this stays within the ShellQuote discipline.
func (e *Executor) prune(ctx context.Context) error {
	if e.Keep <= 0 {
		return nil
	}
	relDir := composessh.ShellQuote(path.Join(e.ProjectDir, releasesDir))
	curLink := composessh.ShellQuote(path.Join(e.ProjectDir, currentLink))
	// List release dirs by mtime (newest first), drop the active target, keep the newest Keep,
	// rm -rf the remainder. `readlink -f` resolves `current` to its release dir basename.
	cmd := fmt.Sprintf(
		`cur=$(basename "$(readlink -f %s)"); `+
			`ls -1t %s | grep -v -x "$cur" | tail -n +%d | while read d; do rm -rf %s/"$d"; done`,
		curLink, relDir, e.Keep+1, relDir)
	_, err := e.Runner.Run(ctx, cmd, nil)
	return err
}

// ComposeTarget verifies the active release, which runs compose from current/.env.
func (e *Executor) ComposeTarget(context.Context) (verify.ComposeTarget, error) {
	return verify.ComposeTarget{ProjectDir: e.ProjectDir, ComposeFiles: e.ComposeFiles, EnvFile: envInRelease}, nil
}

// CloseRunner releases the executor's runner connection if the runner supports it.
func (e *Executor) CloseRunner() error {
	if c, ok := e.Runner.(composessh.Closer); ok {
		return c.Close()
	}
	return nil
}
