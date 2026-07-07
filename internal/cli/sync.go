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
	"github.com/RomanAgaltsev/gantry/internal/ledger"
	"github.com/RomanAgaltsev/gantry/internal/notify"
	"github.com/RomanAgaltsev/gantry/internal/pin"
	"github.com/RomanAgaltsev/gantry/internal/verify"
)

type deps struct {
	cfg      *config.Config
	forge    forge.Forge
	exec     executor.Executor
	verify   verify.Verifier
	store    engine.PinStore
	ledger   ledger.Ledger
	notifier notify.Dispatcher
	env      string
}

// buildDeps wires the engine's collaborators from config. The forge is built only when
// needForge is true (sync/plan/status), and the SSH executor only when needExec is true
// (an actual deploy). Read-only or forge-free commands (history, promote, rollback) skip
// the secrets they do not use, so e.g. `gantry history` never resolves a forge token or
// registry credential.
// newForgeFunc builds the forge client; overridable in tests to inject a fake forge without
// standing up an HTTP forge server.
var newForgeFunc = newForge

func buildDeps(cmd *cobra.Command, envName string, needForge, needExec bool) (*deps, error) {
	path, err := cmd.Flags().GetString("config")
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}

	var env config.Environment
	if envName != "" {
		e, ok := cfg.Environment(envName)
		if !ok {
			return nil, fmt.Errorf("environment %q not found", envName)
		}
		env = *e
	}
	res := config.DefaultResolver()
	resolveVaultDefaults(&res, cfg)

	var f forge.Forge
	if needForge {
		token, err := res.Resolve(cfg.Forge.Token)
		if err != nil {
			return nil, err
		}
		f, err = newForgeFunc(cfg.Forge, token)
		if err != nil {
			return nil, err
		}
	}

	ex, vf, err := buildExecAndVerify(res, cfg, &env, needExec)
	if err != nil {
		return nil, err
	}

	// Pin files are tracked in the git repo alongside gantry.yaml.
	store, err := engine.NewGitStore(filepath.Dir(path),
		object.Signature{Name: cfg.Git.AuthorName, Email: cfg.Git.AuthorEmail})
	if err != nil {
		return nil, err
	}
	led, err := ledger.NewGitLedger(filepath.Dir(path),
		object.Signature{Name: cfg.Git.AuthorName, Email: cfg.Git.AuthorEmail})
	if err != nil {
		return nil, err
	}
	notifier, err := buildNotifier(res, cfg.Notifications)
	if err != nil {
		return nil, err
	}
	return &deps{cfg: cfg, forge: f, exec: ex, verify: vf, store: store, ledger: led, notifier: notifier, env: envName}, nil
}

// resolveVaultDefaults resolves the ambient secrets.vault address/token onto res so any
// ${vault:…} ref can use them. Resolution is best-effort: if a vault ref is never used the
// ambient vars need not be set; a ${vault:…} ref whose token is unset errors clearly at use.
func resolveVaultDefaults(res *config.SecretResolver, cfg *config.Config) {
	if addr, err := res.Resolve(cfg.Secrets.Vault.Address); err == nil {
		res.Vault.Address = addr
	}
	if tok, err := res.Resolve(cfg.Secrets.Vault.Token); err == nil {
		res.Vault.Token = tok
	}
}

// buildExecAndVerify builds the SSH executor and verifiers for env when needExec is set and
// the connection has an ssh block; otherwise it returns nils (read-only or forge-free
// commands skip the SSH secrets they do not use).
func buildExecAndVerify(res config.SecretResolver, cfg *config.Config, env *config.Environment, needExec bool) (executor.Executor, verify.Verifier, error) {
	conn := cfg.Connections[env.Executor.Connection]
	if !needExec || conn.SSH == nil {
		return nil, nil, nil
	}
	logins, err := resolveLogins(res, cfg.Registries)
	if err != nil {
		return nil, nil, err
	}
	key, err := res.Resolve(conn.SSH.Key)
	if err != nil {
		return nil, nil, err
	}
	known, err := res.Resolve(conn.SSH.KnownHosts)
	if err != nil {
		return nil, nil, err
	}
	runner, err := composessh.NewSSHRunner(conn.Address, conn.SSH.User, key, known)
	if err != nil {
		return nil, nil, err
	}
	ex, err := newExecutor(*env, runner, logins)
	if err != nil {
		return nil, nil, err
	}
	return ex, buildVerifiers(env.Verify, runner, ex), nil
}

// execFor returns a factory that builds the executor + verifier for one environment,
// resolving secrets with res. Shared by the CLI verbs (via buildDeps) and the daemon.
func execFor(res config.SecretResolver, cfg *config.Config) func(config.Environment) (executor.Executor, verify.Verifier, error) {
	return func(env config.Environment) (executor.Executor, verify.Verifier, error) {
		return buildExecAndVerify(res, cfg, &env, true)
	}
}

// resolveLogins resolves each registry's credentials for the executor to log in with.
func resolveLogins(res config.SecretResolver, registries map[string]config.Registry) ([]composessh.RegistryLogin, error) {
	var logins []composessh.RegistryLogin
	for host, reg := range registries {
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
	return logins, nil
}

func newSyncCmd() *cobra.Command {
	var envName string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Consume releases, pin, and deploy an environment",
		RunE: func(cmd *cobra.Command, _ []string) error {
			release, err := acquireServeLock(cmd)
			if err != nil {
				return err
			}
			defer release()
			d, err := buildDeps(cmd, envName, true, !dryRun)
			if err != nil {
				return err
			}
			res, err := engine.Sync(cmd.Context(), d.cfg, d.env, d.forge, d.exec, d.verify, d.store, d.ledger, engine.SyncOptions{DryRun: dryRun})
			d.notifier.Dispatch(cmd.Context(), syncEvents(envName, res, err)...)
			if err != nil {
				if note := autoRollbackNote(envName, res.RolledBackTo); note != "" {
					cmd.PrintErrln(note)
				}
				return err
			}
			printChanges(cmd, res.Changes, res.Deployed, res.Recovered)
			return nil
		},
	}
	cmd.Flags().StringVar(&envName, "env", "", "environment name")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show changes without writing/deploying")
	mustRequireFlag(cmd, "env")
	return cmd
}

// mustRequireFlag marks the named flag required. MarkFlagRequired only errors when the
// flag is undefined, which is a programming error here (the flag is registered just
// above), so a failure should crash loudly rather than be ignored.
func mustRequireFlag(cmd *cobra.Command, name string) {
	if err := cmd.MarkFlagRequired(name); err != nil {
		panic(err)
	}
}

func printChanges(cmd *cobra.Command, changes []pin.Change, deployed, recovered bool) {
	if len(changes) == 0 {
		cmd.Println(upToDateMessage(deployed, recovered))
		return
	}
	for _, c := range changes {
		cmd.Printf("%s: %s -> %s\n", c.Key, c.Old, c.New)
	}
	if deployed {
		cmd.Println("deployed")
	}
}

// upToDateMessage renders the no-changes line for sync/plan. A recovery with no deploy is
// a dry run (plan, or sync --dry-run): it must read as a prediction, not a past action.
func upToDateMessage(deployed, recovered bool) string {
	switch {
	case recovered && deployed:
		return "recovered: redeployed the last committed pin set"
	case recovered:
		return "would redeploy the last committed pin set (not yet green)"
	default:
		return "up to date; no changes"
	}
}
