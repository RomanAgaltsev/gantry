package cli

import (
	"fmt"
	"path/filepath"

	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/engine"
	"github.com/RomanAgaltsev/gantry/internal/executor"
	"github.com/RomanAgaltsev/gantry/internal/executor/composessh"
	"github.com/RomanAgaltsev/gantry/internal/forge"
	"github.com/RomanAgaltsev/gantry/internal/forge/gitlab"
	"github.com/RomanAgaltsev/gantry/internal/pin"
)

type deps struct {
	cfg   *config.Config
	forge forge.Forge
	exec  executor.Executor
	store engine.PinStore
	env   string
}

// buildDeps wires the engine's collaborators from config. The SSH executor is
// built only when needExec is true (an actual deploy); read-only commands
// (plan, status, sync --dry-run) skip it so they never require usable SSH creds.
func buildDeps(cmd *cobra.Command, envName string, needExec bool) (*deps, error) {
	path, _ := cmd.Flags().GetString("config")
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	env, ok := cfg.Environment(envName)
	if !ok {
		return nil, fmt.Errorf("environment %q not found", envName)
	}
	res := config.DefaultResolver()
	token, err := res.Resolve(cfg.Forge.Token)
	if err != nil {
		return nil, err
	}
	f := gitlab.New(cfg.Forge.BaseURL, token, cfg.Forge.MetadataMarker, nil)

	var logins []composessh.RegistryLogin
	for host, reg := range cfg.Registries {
		u, err := res.Resolve(reg.User)
		if err != nil {
			return nil, err
		}
		pw, err := res.Resolve(reg.Password)
		if err != nil {
			return nil, err
		}
		logins = append(logins, composessh.RegistryLogin{Registry: host, Username: u, Password: pw})
	}

	conn := cfg.Connections[env.Executor.Connection]
	var ex executor.Executor
	if needExec && conn.SSH != nil {
		key, err := res.Resolve(conn.SSH.Key)
		if err != nil {
			return nil, err
		}
		known, err := res.Resolve(conn.SSH.KnownHosts)
		if err != nil {
			return nil, err
		}
		runner, err := composessh.NewSSHRunner(conn.Address, conn.SSH.User, key, known)
		if err != nil {
			return nil, err
		}
		ex = &composessh.Executor{
			Runner:       runner,
			ProjectDir:   env.Executor.ProjectDir,
			ComposeFiles: env.Executor.ComposeFiles,
			EnvFile:      env.Executor.EnvFile,
			Logins:       logins,
		}
	}

	// Pin files are tracked in the git repo alongside gantry.yaml.
	store, err := engine.NewGitStore(filepath.Dir(path),
		object.Signature{Name: cfg.Git.AuthorName, Email: cfg.Git.AuthorEmail})
	if err != nil {
		return nil, err
	}
	return &deps{cfg: cfg, forge: f, exec: ex, store: store, env: envName}, nil
}

func newSyncCmd() *cobra.Command {
	var envName string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Consume releases, pin, and deploy an environment",
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := buildDeps(cmd, envName, !dryRun)
			if err != nil {
				return err
			}
			res, err := engine.Sync(cmd.Context(), d.cfg, d.env, d.forge, d.exec, d.store, engine.SyncOptions{DryRun: dryRun})
			if err != nil {
				return err
			}
			printChanges(cmd, res.Changes, res.Deployed)
			return nil
		},
	}
	cmd.Flags().StringVar(&envName, "env", "", "environment name")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show changes without writing/deploying")
	_ = cmd.MarkFlagRequired("env")
	return cmd
}

func printChanges(cmd *cobra.Command, changes []pin.Change, deployed bool) {
	if len(changes) == 0 {
		cmd.Println("up to date; no changes")
		return
	}
	for _, c := range changes {
		cmd.Printf("%s: %s -> %s\n", c.Key, c.Old, c.New)
	}
	if deployed {
		cmd.Println("deployed")
	}
}
