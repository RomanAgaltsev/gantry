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
	"github.com/RomanAgaltsev/gantry/internal/forge"
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

// firstNonEmpty returns the first non-empty argument, or def when all are empty.
func firstNonEmpty(s, def string) string {
	if s != "" {
		return s
	}
	return def
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
	resolveVaultDefaults(cmd.Context(), &res, cfg)

	deps, err := serveDeps(cmd.Context(), cfg, path, res)
	if err != nil {
		return err
	}

	obs, metricsHandler := daemon.NewPrometheusObserver(Version)
	deps.Metrics = obs

	bell, bellMount, err := buildDoorbell(cmd.Context(), res, cfg)
	if err != nil {
		return err
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

	// The HTTP server + mux and the metrics registry are built once and reused across reloads
	// so /metrics continuity holds and the listen address does not flap.
	srv := startServeHTTP(cmd.Context(), cfg.Daemon.Listen, buildServeMux(metricsHandler, bellMount))

	// SIGHUP reloads the config without dropping the single-writer lock: the current loop is
	// cancelled and restarted with freshly-built deps (review §9 item 9). SIGINT/SIGTERM stop.
	reload := make(chan os.Signal, 1)
	signal.Notify(reload, syscall.SIGHUP)
	defer signal.Stop(reload)

	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var loopErr error
	for {
		runCtx, cancelRun := context.WithCancel(ctx)
		go func() {
			select {
			case <-reload:
				logging.From(ctx).Info("SIGHUP: reloading config")
				cancelRun() // stop the current loop; the outer for rebuilds deps
			case <-ctx.Done():
				cancelRun()
			}
		}()
		loopErr = daemon.Run(runCtx, *deps, daemon.Options{Interval: ivl, Doorbell: bell})
		if ctx.Err() != nil {
			break // real shutdown (SIGINT/SIGTERM)
		}
		// Reload: rebuild deps from disk; a parse/wiring error keeps the previous deps and logs.
		deps, ivl = applyReload(ctx, path, res, interval, deps, ivl)
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx) //nolint:gosec // best-effort shutdown; the loop result is what matters
	if loopErr != nil && !errors.Is(loopErr, context.Canceled) {
		return loopErr
	}
	return nil
}

// applyReload rebuilds the daemon deps from disk for a SIGHUP reload and returns the new deps
// and interval. On any failure (parse/wiring/invalid interval) it logs and returns the
// previous deps and interval unchanged, so a bad edit never takes down a running daemon.
func applyReload(ctx context.Context, path string, res config.SecretResolver, interval string, prev *daemon.Deps, prevIvl time.Duration) (*daemon.Deps, time.Duration) {
	nd, cfg, err := reloadDeps(ctx, path, res)
	if err != nil {
		logging.From(ctx).Error("reload failed; keeping previous config", "error", err)
		return prev, prevIvl
	}
	nd.Metrics = prev.Metrics // keep the same registry so /metrics continuity holds
	ivl := prevIvl
	if newIvl, err := resolveInterval(interval, cfg.Daemon.Interval.Duration()); err != nil {
		logging.From(ctx).Error("reload interval invalid; keeping previous interval", "error", err)
	} else {
		ivl = newIvl
	}
	return nd, ivl
}

// reloadDeps loads the config from path and rebuilds the daemon deps, resolving the ambient
// Vault defaults onto res so ${vault:…} refs work after a reload. It returns the new deps and
// config, or an error so the caller can keep the previous deps (a parse or wiring failure
// must never take down a running daemon).
func reloadDeps(ctx context.Context, path string, res config.SecretResolver) (*daemon.Deps, *config.Config, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return nil, nil, err
	}
	resolveVaultDefaults(ctx, &res, cfg)
	deps, err := serveDeps(ctx, cfg, path, res)
	if err != nil {
		return nil, nil, err
	}
	return deps, cfg, nil
}

// serveDeps builds the daemon's long-lived collaborators from config: the forge client, the
// git-backed pin store and ledger, the notification dispatcher, and the per-env executor
// factory shared with the CLI verbs. Secret refs resolve under ctx.
func serveDeps(ctx context.Context, cfg *config.Config, path string, res config.SecretResolver) (*daemon.Deps, error) {
	token, err := res.Resolve(ctx, cfg.Forge.Token)
	if err != nil {
		return nil, err
	}
	forgeClient, err := newForge(cfg.Forge, token)
	if err != nil {
		return nil, err
	}
	// Wrap the forge in a per-cycle release cache so a reconcile fetches each component's latest
	// release once (shared by every env's Sync and drift observation) instead of once per env plus
	// again for drift (P1). The cache TTL is the reconcile interval; the daemon clears it at the
	// top of each cycle so a doorbell-rung run reuses the snapshot until the next clear.
	cachedForge := forge.NewCache(forgeClient, cfg.Daemon.Interval.Duration())
	sig := object.Signature{Name: cfg.Git.AuthorName, Email: cfg.Git.AuthorEmail}
	store, err := engine.NewGitStore(filepath.Dir(path), sig)
	if err != nil {
		return nil, err
	}
	led, err := ledger.NewGitLedger(filepath.Dir(path), sig)
	if err != nil {
		return nil, err
	}
	disp, err := notify.FromConfig(ctx, cfg, res)
	if err != nil {
		return nil, err
	}
	// When git.remote is configured, give the store transport auth (HTTPS token) so the daemon
	// can pull/push it (D1). The store is a PinStore interface here; reach SetRemoteAuth via an
	// anonymous interface type-assert so PinStore need not widen for the local-only case.
	remotePull, remotePush := cfg.Git.Remote.Pull, cfg.Git.Remote.Push
	if remotePull || remotePush {
		token, err := res.Resolve(ctx, cfg.Git.Remote.Token)
		if err != nil {
			return nil, err
		}
		if setter, ok := store.(interface {
			SetRemoteAuth(username, token, remote, branch string)
		}); ok {
			setter.SetRemoteAuth(cfg.Git.Remote.Username, token, firstNonEmpty(cfg.Git.Remote.Name, "origin"), cfg.Git.Remote.Branch)
		}
	}
	eng := engine.New(cfg, cachedForge, store, led)
	return &daemon.Deps{
		Engine:   eng,
		Dispatch: disp, ExecFor: execFor(res, cfg),
		ReconcileTimeout:      cfg.Daemon.ReconcileTimeout.Duration(),
		ReconcileFailedRepeat: cfg.Daemon.ReconcileFailedRepeat.Duration(),
		RemotePull:            remotePull, RemotePush: remotePush,
	}, nil
}

// buildDoorbell constructs the doorbell trigger channel and its HTTP mount from config. When
// the doorbell is disabled it returns a nil channel and an empty mount (mounts nothing). The
// doorbell is built once at startup and reused across SIGHUP reloads with the HTTP mux.
func buildDoorbell(ctx context.Context, res config.SecretResolver, cfg *config.Config) (<-chan struct{}, doorbellMount, error) {
	if !cfg.Daemon.Doorbell.Enabled {
		return nil, doorbellMount{}, nil
	}
	secret, err := res.Resolve(ctx, cfg.Daemon.Doorbell.Secret)
	if err != nil {
		return nil, doorbellMount{}, err
	}
	handler, ch := daemon.NewDoorbell(secret, cfg.Daemon.Doorbell.HMAC)
	logging.From(ctx).Info("doorbell enabled", "path", cfg.Daemon.Doorbell.Path)
	return ch, doorbellMount{Path: cfg.Daemon.Doorbell.Path, Handler: handler}, nil
}

// doorbellMount binds a doorbell handler to the path it is served at. A zero value (empty
// Path) mounts nothing, so callers without a doorbell pass doorbellMount{}.
type doorbellMount struct {
	Path    string
	Handler http.Handler
}

// startServeHTTP builds the daemon HTTP server and serves it in a background goroutine. A
// non-ErrServerClosed error is logged; the returned server is Shutdown by the caller on exit.
func startServeHTTP(ctx context.Context, addr string, handler *http.ServeMux) *http.Server {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logging.From(ctx).Error("http server stopped", "error", err)
		}
	}()
	return srv
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
