// Package composessh deploys via `docker compose pull && up -d` over SSH.
package composessh

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

// Runner executes a shell command on the target host, optionally feeding stdin.
type Runner interface {
	Run(ctx context.Context, cmd string, stdin []byte) (string, error)
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
	if _, err := e.Runner.Run(ctx, fmt.Sprintf("cat > %s", envPath), pin.Render(p.Pins)); err != nil {
		return executor.Result{}, fmt.Errorf("write env file: %w", err)
	}

	var fileFlags strings.Builder
	for _, f := range e.ComposeFiles {
		fmt.Fprintf(&fileFlags, " -f %s", f)
	}
	base := fmt.Sprintf("cd %s && docker compose%s --env-file %s", e.ProjectDir, fileFlags.String(), e.EnvFile)

	hosts := pinHosts(p.Pins)
	for _, lg := range e.Logins {
		if !hosts[lg.Registry] {
			continue
		}
		cmd := fmt.Sprintf("docker login %s -u %s --password-stdin", lg.Registry, lg.Username)
		if _, err := e.Runner.Run(ctx, cmd, []byte(lg.Password)); err != nil {
			return executor.Result{}, fmt.Errorf("docker login %s: %w", lg.Registry, err)
		}
	}

	if _, err := e.Runner.Run(ctx, base+" pull", nil); err != nil {
		return executor.Result{}, fmt.Errorf("compose pull: %w", err)
	}
	if _, err := e.Runner.Run(ctx, base+" up -d", nil); err != nil {
		return executor.Result{}, fmt.Errorf("compose up: %w", err)
	}
	return executor.Result{Changed: true, Detail: "compose pull && up -d"}, nil
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
