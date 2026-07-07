// Package composessh deploys via `docker compose pull && up -d` over SSH.
package composessh

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/pin"
	"github.com/RomanAgaltsev/gantry/internal/verify"
)

// compile-time check
var _ verify.ComposeVerifiable = (*Executor)(nil)

// Runner executes a shell command on the target host, optionally feeding stdin.
type Runner interface {
	Run(ctx context.Context, cmd string, stdin []byte) (string, error)
}

// Closer is an optional Runner capability: a runner holding a pooled connection (the SSH
// runner) implements it so long-running callers (the daemon) can release the connection
// between reconciles. A test/stub runner without a connection need not implement it.
type Closer interface {
	Close() error
}

// Executor implements executor.Executor over an SSH Runner.
type Executor struct {
	Runner       Runner
	ProjectDir   string
	ComposeFiles []string
	EnvFile      string
	Logins       []RegistryLogin // registries to docker-login before pull
}

// Deploy writes the env file on the host, then pulls and brings up the stack.
func (e *Executor) Deploy(ctx context.Context, p executor.Plan) (executor.Result, error) {
	envPath := path.Join(e.ProjectDir, e.EnvFile)
	if _, err := e.Runner.Run(ctx, "cat > "+ShellQuote(envPath), pin.Render(p.Pins)); err != nil {
		return executor.Result{}, fmt.Errorf("write env file: %w", err)
	}
	if err := RunCompose(ctx, e.Runner, ComposeOpts{
		ProjectDir:   e.ProjectDir,
		ComposeFiles: e.ComposeFiles,
		EnvFile:      e.EnvFile,
		Logins:       e.Logins,
	}, p.Pins); err != nil {
		return executor.Result{}, err
	}
	return executor.Result{Changed: true, Detail: "compose pull && up -d"}, nil
}

// ComposeTarget reports where this executor runs compose, for compose-ps verification.
func (e *Executor) ComposeTarget(context.Context) (verify.ComposeTarget, error) {
	return verify.ComposeTarget{ProjectDir: e.ProjectDir, ComposeFiles: e.ComposeFiles, EnvFile: e.EnvFile}, nil
}

// CloseRunner releases the executor's runner connection if the runner supports it.
func (e *Executor) CloseRunner() error {
	if c, ok := e.Runner.(Closer); ok {
		return c.Close()
	}
	return nil
}

// ComposeOpts configures a compose pull+up run.
type ComposeOpts struct {
	ProjectDir   string
	ComposeFiles []string
	EnvFile      string // value passed to --env-file (relative to ProjectDir cwd)
	Logins       []RegistryLogin
}

// RunCompose logs in to the registries used by pins, then runs `compose pull` and
// `compose up -d` over runner. Shared by the compose-over-ssh and symlink-release executors.
func RunCompose(ctx context.Context, runner Runner, o ComposeOpts, pins pin.Set) error {
	var fileFlags strings.Builder
	for _, f := range o.ComposeFiles {
		fmt.Fprintf(&fileFlags, " -f %s", ShellQuote(f))
	}
	base := fmt.Sprintf("cd %s && docker compose%s --env-file %s",
		ShellQuote(o.ProjectDir), fileFlags.String(), ShellQuote(o.EnvFile))

	hosts := pinHosts(pins)
	for _, lg := range o.Logins {
		if !hosts[lg.Registry] {
			continue
		}
		cmd := fmt.Sprintf("docker login %s -u %s --password-stdin",
			ShellQuote(lg.Registry), ShellQuote(lg.Username))
		if _, err := runner.Run(ctx, cmd, []byte(lg.Password)); err != nil {
			return fmt.Errorf("docker login %s: %w", lg.Registry, err)
		}
	}
	if _, err := runner.Run(ctx, base+" pull", nil); err != nil {
		return fmt.Errorf("compose pull: %w", err)
	}
	if _, err := runner.Run(ctx, base+" up -d", nil); err != nil {
		return fmt.Errorf("compose up: %w", err)
	}
	return nil
}

// ShellQuote single-quotes s for safe interpolation into a remote shell command, escaping
// embedded single quotes. Every config- or release-derived value reaching the host shell
// must pass through this.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// RegistryLogin holds resolved credentials for one registry host.
type RegistryLogin struct {
	Registry string
	Username string
	Password string
}

// registryHostOf returns the registry host for an image reference, using
// Docker's rule: the first path segment is the host iff it contains "." or ":"
// or equals "localhost"; otherwise the registry is docker.io.
func registryHostOf(ref string) string {
	slash := strings.IndexByte(ref, '/')
	if slash < 0 {
		return "docker.io"
	}
	first := ref[:slash]
	if first == "localhost" || strings.ContainsAny(first, ".:") {
		return first
	}
	return "docker.io"
}

func pinHosts(s pin.Set) map[string]bool {
	h := make(map[string]bool, len(s))
	for _, ref := range s {
		h[registryHostOf(ref)] = true
	}
	return h
}
