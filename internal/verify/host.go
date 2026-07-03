package verify

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Runner runs a shell command on the deploy host. *composessh.Executor's Runner
// satisfies it structurally, so verify never imports the executor package.
type Runner interface {
	Run(ctx context.Context, cmd string, stdin []byte) (string, error)
}

// CommandVerifier runs an arbitrary command on the host; a non-zero exit fails.
type CommandVerifier struct {
	Runner  Runner
	Command string
}

// Verify runs the command; any error (non-zero exit) fails verification.
func (v CommandVerifier) Verify(ctx context.Context) error {
	if _, err := v.Runner.Run(ctx, v.Command, nil); err != nil {
		return fmt.Errorf("command %q: %w", v.Command, err)
	}
	return nil
}

// ComposePSVerifier runs `docker compose … ps --format json` on the host and fails if any
// service is not running, or (when it declares a healthcheck) not healthy. The target compose
// project is resolved at verify time via Target, so a blue-green probe checks the idle slot.
type ComposePSVerifier struct {
	Runner Runner
	Target func(ctx context.Context) (ComposeTarget, error)
}

// Verify queries compose service status and asserts every service is up and healthy.
func (v ComposePSVerifier) Verify(ctx context.Context) error {
	target, err := v.Target(ctx)
	if err != nil {
		return fmt.Errorf("resolve compose target: %w", err)
	}
	var files strings.Builder
	for _, f := range target.ComposeFiles {
		fmt.Fprintf(&files, " -f %s", shellQuote(f))
	}
	cmd := fmt.Sprintf("cd %s && docker compose%s --env-file %s ps --format json",
		shellQuote(target.ProjectDir), files.String(), shellQuote(target.EnvFile))
	out, err := v.Runner.Run(ctx, cmd, nil)
	if err != nil {
		return fmt.Errorf("compose ps: %w", err)
	}
	svcs, err := parseComposePS(out)
	if err != nil {
		return fmt.Errorf("compose ps: %w", err)
	}
	if len(svcs) == 0 {
		return errors.New("compose ps: no services reported")
	}
	for _, s := range svcs {
		name := s.Service
		if name == "" {
			name = s.Name
		}
		if s.State != "running" {
			return fmt.Errorf("service %s is %q, not running", name, s.State)
		}
		if s.Health != "" && s.Health != "healthy" {
			return fmt.Errorf("service %s health is %q", name, s.Health)
		}
	}
	return nil
}

type composeService struct {
	Service string `json:"Service"`
	Name    string `json:"Name"`
	State   string `json:"State"`
	Health  string `json:"Health"`
}

// parseComposePS tolerates both compose output styles: a JSON array, or newline-delimited
// JSON objects (one per service).
func parseComposePS(out string) ([]composeService, error) {
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	if strings.HasPrefix(out, "[") {
		var arr []composeService
		if err := json.Unmarshal([]byte(out), &arr); err != nil {
			return nil, fmt.Errorf("parse json array: %w", err)
		}
		return arr, nil
	}
	var svcs []composeService
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var s composeService
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			return nil, fmt.Errorf("parse json line: %w", err)
		}
		svcs = append(svcs, s)
	}
	return svcs, sc.Err()
}

// shellQuote single-quotes s for safe interpolation into a remote shell command. It mirrors
// the executor's quoting; verify keeps its own copy to avoid importing the executor package.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
