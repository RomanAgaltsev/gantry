package cli

import (
	"fmt"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/executor/bluegreen"
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
			Keep:         env.Executor.Keep,
		}, nil
	case "blue-green":
		ec := env.Executor
		return &bluegreen.Executor{
			Runner: runner,
			SlotMap: map[string]bluegreen.Slot{
				"blue":  {ProjectDir: ec.Slots["blue"].ProjectDir, ComposeFiles: ec.Slots["blue"].ComposeFiles},
				"green": {ProjectDir: ec.Slots["green"].ProjectDir, ComposeFiles: ec.Slots["green"].ComposeFiles},
			},
			Order: [2]string{"blue", "green"},
			Pointer: bluegreen.Pointer{
				Link:   ec.Pointer.Link,
				Target: map[string]string{"blue": ec.Pointer.Blue, "green": ec.Pointer.Green},
				Reload: ec.Pointer.Reload,
			},
			Logins: logins,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported executor.kind %q", env.Executor.Kind)
	}
}
