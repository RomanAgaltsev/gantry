package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/spf13/cobra"

	"github.com/RomanAgaltsev/gantry/internal/config"
	"github.com/RomanAgaltsev/gantry/internal/daemon"
	"github.com/RomanAgaltsev/gantry/internal/engine"
	"github.com/RomanAgaltsev/gantry/internal/ledger"
	"github.com/RomanAgaltsev/gantry/internal/logging"
	"github.com/RomanAgaltsev/gantry/internal/notify"
)

// lockPath returns the serve lockfile path for a repo given its config path. It accepts either
// the repo directory or the gantry.yaml file path; both resolve to <repo>/.gantry/serve.lock.
func lockPath(repoOrConfig string) string {
	dir := repoOrConfig
	if filepath.Ext(repoOrConfig) != "" { // a file path → its dir
		dir = filepath.Dir(repoOrConfig)
	}
	return filepath.Join(dir, ".gantry", "serve.lock")
}

// acquireServeLock takes the single-writer serve lock so a mutating CLI verb cannot run
// concurrently with `gantry serve` or another mutating verb (C6 — verbs now Acquire the
// same lock the daemon does, rather than merely peeking with CheckFree). The returned
// release func drops the lock and must be deferred by the caller.
func acquireServeLock(cmd *cobra.Command) (func(), error) {
	path, err := cmd.Flags().GetString("config")
	if err != nil {
		return nil, err
	}
	lp := lockPath(path)
	if err := os.MkdirAll(filepath.Dir(lp), 0o750); err != nil {
		return nil, err
	}
	lock, err := daemon.Acquire(lp)
	if err != nil {
		return nil, err
	}
	return func() { _ = lock.Release() }, nil //nolint:gosec // best-effort lock release
}

// resolveInterval picks the daemon interval: the --interval flag when set (parsed with the
// day-suffix-aware config.ParseDuration so "1d" works and a typo errors), else cfgDefault.
func resolveInterval(flag string, cfgDefault time.Duration) (time.Duration, error) {
	if flag == "" {
		return cfgDefault, nil
	}
	d, err := config.ParseDuration(flag)
	if err != nil {
		return 0, fmt.Errorf("--interval %q: %w", flag, err)
	}
	return d, nil
}

func newServeCmd() *cobra.Command {
	var interval string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the reconcile loop continuously (daemon)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd, interval)
		},
	}
	cmd.Flags().StringVar(&interval, "interval", "", "override daemon.interval (e.g. 30s)")
	return cmd
}

// runServe loads config, takes the single-writer lock, serves /healthz, and drives the
// reconcile loop until the process is interrupted. A reconcile error never returns from here.
func runServe(cmd *cobra.Command, interval string) error {
	path, err := cmd.Flags().GetString("config")
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	res := config.DefaultResolver()
	resolveVaultDefaults(&res, cfg)

	deps, err := serveDeps(cfg, path, res)
	if err != nil {
		return err
	}

	obs, metricsHandler := daemon.NewPrometheusObserver(Version)
	deps.Metrics = obs

	var bell <-chan struct{}
	var bellMount doorbellMount
	if cfg.Daemon.Doorbell.Enabled {
		secret, err := res.Resolve(cfg.Daemon.Doorbell.Secret)
		if err != nil {
			return err
		}
		handler, ch := daemon.NewDoorbell(secret)
		bell, bellMount = ch, doorbellMount{Path: cfg.Daemon.Doorbell.Path, Handler: handler}
		logging.From(cmd.Context()).Info("doorbell enabled", "path", cfg.Daemon.Doorbell.Path)
	}

	if err := os.MkdirAll(filepath.Dir(lockPath(path)), 0o750); err != nil {
		return err
	}
	lock, err := daemon.Acquire(lockPath(path))
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }() //nolint:gosec // best-effort lock release on shutdown

	ivl, err := resolveInterval(interval, cfg.Daemon.Interval.Duration())
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.Daemon.Listen,
		Handler:           buildServeMux(metricsHandler, bellMount),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.From(cmd.Context()).Error("http server stopped", "error", err)
		}
	}()

	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	err = daemon.Run(ctx, *deps, daemon.Options{Interval: ivl, Doorbell: bell})

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx) //nolint:gosec // best-effort shutdown; the loop result is what matters
	return err
}

// serveDeps builds the daemon's long-lived collaborators from config: the forge client, the
// git-backed pin store and ledger, the notification dispatcher, and the per-env executor
// factory shared with the CLI verbs.
func serveDeps(cfg *config.Config, path string, res config.SecretResolver) (*daemon.Deps, error) {
	token, err := res.Resolve(cfg.Forge.Token)
	if err != nil {
		return nil, err
	}
	forgeClient, err := newForge(cfg.Forge, token)
	if err != nil {
		return nil, err
	}
	sig := object.Signature{Name: cfg.Git.AuthorName, Email: cfg.Git.AuthorEmail}
	store, err := engine.NewGitStore(filepath.Dir(path), sig)
	if err != nil {
		return nil, err
	}
	led, err := ledger.NewGitLedger(filepath.Dir(path), sig)
	if err != nil {
		return nil, err
	}
	disp, err := notify.FromConfig(cfg, res)
	if err != nil {
		return nil, err
	}
	return &daemon.Deps{
		Cfg: cfg, Forge: forgeClient, Store: store, Ledger: led,
		Dispatch: disp, ExecFor: execFor(res, cfg),
		ReconcileTimeout:      cfg.Daemon.ReconcileTimeout.Duration(),
		ReconcileFailedRepeat: cfg.Daemon.ReconcileFailedRepeat.Duration(),
	}, nil
}

// doorbellMount binds a doorbell handler to the path it is served at. A zero value (empty
// Path) mounts nothing, so callers without a doorbell pass doorbellMount{}.
type doorbellMount struct {
	Path    string
	Handler http.Handler
}

// buildServeMux builds the HTTP mux for the daemon. /healthz is always registered; /metrics is
// registered when metrics != nil (C3b); the doorbell is registered at doorbell.Path when
// non-empty (C3c). One helper both slices share; C3c only supplies the doorbell mount.
func buildServeMux(metrics http.Handler, doorbell doorbellMount) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok")) //nolint:gosec // a fixed "ok" body; a write failure is not actionable
	})
	if metrics != nil {
		mux.Handle("/metrics", metrics)
	}
	if doorbell.Path != "" {
		mux.Handle(doorbell.Path, doorbell.Handler)
	}
	return mux
}
