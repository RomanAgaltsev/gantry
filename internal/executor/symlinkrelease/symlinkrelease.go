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
)

const (
	releasesDir  = "releases"
	currentLink  = "current"
	versionFile  = ".version"
	envInRelease = currentLink + "/.env"
)

// Executor deploys each pin set into releases/<commit> and atomically flips `current` to it.
type Executor struct {
	Runner       composessh.Runner
	ProjectDir   string
	ComposeFiles []string
	Logins       []composessh.RegistryLogin
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
	return executor.Result{Changed: true, Detail: "symlink-release " + p.Commit}, nil
}
