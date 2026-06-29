package cli

import (
	"fmt"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/executor/composessh"
	"github.com/RomanAgaltsev/gantry/internal/executor/symlinkrelease"
)

// newExecutor selects the deploy backend by kind, sharing the SSH runner and resolved
// registry logins. Adding an executor is adding a case here; the engine never learns the
// kind.
func newExecutor(env config.Environment, runner composessh.Runner, logins []composessh.RegistryLogin) (executor.Executor, error) {
	switch env.Executor.Kind {
	case "compose-over-ssh":
		return &composessh.Executor{
			Runner:       runner,
			ProjectDir:   env.Executor.ProjectDir,
			ComposeFiles: env.Executor.ComposeFiles,
			EnvFile:      env.Executor.EnvFile,
			Logins:       logins,
		}, nil
	case "symlink-release":
		return &symlinkrelease.Executor{
			Runner:       runner,
			ProjectDir:   env.Executor.ProjectDir,
			ComposeFiles: env.Executor.ComposeFiles,
			Logins:       logins,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported executor.kind %q", env.Executor.Kind)
	}
}
