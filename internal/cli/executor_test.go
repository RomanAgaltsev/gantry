package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/executor/composessh"
	"github.com/RomanAgaltsev/gantry/internal/executor/symlinkrelease"
)

func TestNewExecutor(t *testing.T) {
	env := func(kind string) config.Environment {
		return config.Environment{Executor: config.ExecutorConfig{Kind: kind, ProjectDir: "/o", ComposeFiles: []string{"c.yaml"}}}
	}
	cs, err := newExecutor(env("compose-over-ssh"), nil, nil)
	require.NoError(t, err)
	require.IsType(t, &composessh.Executor{}, cs)

	sr, err := newExecutor(env("symlink-release"), nil, nil)
	require.NoError(t, err)
	require.IsType(t, &symlinkrelease.Executor{}, sr)

	_, err = newExecutor(env("nomad"), nil, nil)
	require.Error(t, err)
}
