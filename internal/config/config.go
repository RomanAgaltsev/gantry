// Package config defines gantry's declarative configuration model and loader.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the whole gantry.yaml.
type Config struct {
	Forge         ForgeConfig           `yaml:"forge"`
	Connections   map[string]Connection `yaml:"connections"`
	Components    []Component           `yaml:"components"`
	Environments  []Environment         `yaml:"environments"`
	Registries    map[string]Registry   `yaml:"registries"`
	Git           GitConfig             `yaml:"git"`
	Drift         DriftConfig           `yaml:"drift"`
	Promote       PromoteConfig         `yaml:"promote"`
	Notifications []NotifyChannel       `yaml:"notifications"`
	Daemon        DaemonConfig          `yaml:"daemon"`
	Secrets       SecretsConfig         `yaml:"secrets"`
}

// PromoteConfig tunes the promotion gate. Optional; require_healthy defaults false so
// enabling verification never retroactively breaks an existing config.
type PromoteConfig struct {
	RequireHealthy bool `yaml:"require_healthy"`
}

// NotifyChannel is one configured notification destination.
type NotifyChannel struct {
	Kind   string     `yaml:"kind"`    // webhook | email
	URL    SecretRef  `yaml:"url"`     // webhook (holds the Telegram bot token)
	ChatID SecretRef  `yaml:"chat_id"` // webhook, optional (Telegram)
	SMTP   SMTPConfig `yaml:"smtp"`    // email
	From   string     `yaml:"from"`    // email
	To     []string   `yaml:"to"`      // email
	Events []string   `yaml:"events"`  // subscribed kinds; empty = all
}

// SMTPConfig configures the email backend's SMTP transport.
type SMTPConfig struct {
	Host     string    `yaml:"host"`
	Port     int       `yaml:"port"`
	Username string    `yaml:"username"`
	Password SecretRef `yaml:"password"`
	TLS      string    `yaml:"tls"` // "" | "starttls" | "implicit"; default starttls
}

// VerifyProbe is one post-deploy health check for an environment.
type VerifyProbe struct {
	Kind         string `yaml:"kind"`          // "http" | "compose-ps" | "command"
	URL          string `yaml:"url"`           // http
	ExpectStatus int    `yaml:"expect_status"` // http, default 200
	Command      string `yaml:"command"`       // command
}

// GitConfig sets the identity gantry stamps on the pin commits it makes, and optionally
// turns the daemon into a fleet-safe worker that fast-forward-pulls and pushes a remote so
// multiple clones of the same repo converge (review D1).
type GitConfig struct {
	AuthorName  string       `yaml:"author_name"`
	AuthorEmail string       `yaml:"author_email"`
	Remote      RemoteConfig `yaml:"remote"` // optional; when unset the daemon works the local clone only
}

// RemoteConfig turns the daemon into a fleet-safe worker: when Pull/Push are enabled it
// fast-forward-pulls before each reconcile cycle and pushes after each cycle that committed,
// so multiple clones of the same repo converge instead of splitting the ledger (review D1).
type RemoteConfig struct {
	Name     string    `yaml:"name"`     // remote name; default "origin"
	Branch   string    `yaml:"branch"`   // branch to pull/push; default the current HEAD branch
	Pull     bool      `yaml:"pull"`     // ff-only pull at the top of each reconcile cycle
	Push     bool      `yaml:"push"`     // push after each cycle that committed
	Username string    `yaml:"username"` // HTTPS basic-auth username (e.g. a token name); optional
	Token    SecretRef `yaml:"token"`    // HTTPS auth token/password; a SecretRef
}

// ForgeConfig selects and configures the forge adapter.
type ForgeConfig struct {
	Kind           string    `yaml:"kind"`
	BaseURL        string    `yaml:"base_url"`
	Token          SecretRef `yaml:"token"`
	MetadataMarker string    `yaml:"metadata_marker"`
}

// Connection is a named deploy target in the inventory.
type Connection struct {
	Address string   `yaml:"address"`
	SSH     *SSHConn `yaml:"ssh"`
}

// SSHConn holds SSH connection settings; credentials are SecretRefs.
type SSHConn struct {
	User       string    `yaml:"user"`
	Key        SecretRef `yaml:"key"`
	KnownHosts SecretRef `yaml:"known_hosts"`
}

// Component is a buildable repo whose image is pinned.
type Component struct {
	ID      string          `yaml:"id"`
	Project string          `yaml:"project"`
	PinKey  string          `yaml:"pin_key"`
	Source  ComponentSource `yaml:"source"`
}

// IsExplicit reports whether the component's pin is maintained in the pin file.
func (c Component) IsExplicit() bool { return c.Source.Pin == "explicit" }

// IsForgeRelease reports whether the component's pin is derived from a Release.
func (c Component) IsForgeRelease() bool { return c.Source.Forge == "release" }

// Environment is one deploy target environment.
type Environment struct {
	Name            string         `yaml:"name"`
	Source          Source         `yaml:"source"`
	PinFile         string         `yaml:"pin_file"`
	Executor        ExecutorConfig `yaml:"executor"`
	Verify          []VerifyProbe  `yaml:"verify"`
	VerifyOnFailure string         `yaml:"verify_on_failure"` // "" (hold) | "hold" | "rollback"
}

// RollbackOnVerifyFailure reports whether a failed post-deploy verify should auto-roll-back
// this environment to its last known-good pin set. Default (unset/"hold") holds the failure.
func (e Environment) RollbackOnVerifyFailure() bool {
	return e.VerifyOnFailure == "rollback"
}

// Source declares how an environment's pins are computed.
type Source struct {
	Track       string `yaml:"track"`        // e.g. "latest"
	PromoteFrom string `yaml:"promote_from"` // upstream env name
}

// ExecutorConfig configures the deploy backend for an environment.
type ExecutorConfig struct {
	Kind         string                `yaml:"kind"`
	Connection   string                `yaml:"connection"`
	ProjectDir   string                `yaml:"project_dir"`
	ComposeFiles []string              `yaml:"compose_files"`
	EnvFile      string                `yaml:"env_file"`
	Slots        map[string]SlotConfig `yaml:"slots"`   // blue-green only
	Pointer      PointerConfig         `yaml:"pointer"` // blue-green only
}

// SlotConfig is one blue-green slot's compose project.
type SlotConfig struct {
	ProjectDir   string   `yaml:"project_dir"`
	ComposeFiles []string `yaml:"compose_files"`
}

// PointerConfig declares the switchable pointer: a symlink (Link) flipped between the
// per-slot targets (Blue/Green), followed by Reload.
type PointerConfig struct {
	Link   string `yaml:"link"`
	Blue   string `yaml:"blue"`
	Green  string `yaml:"green"`
	Reload string `yaml:"reload"`
}

// Registry holds credentials for one container registry host.
type Registry struct {
	User     SecretRef `yaml:"user"`
	Password SecretRef `yaml:"password"`
}

// ComponentSource declares how a component's desired pin is resolved:
// {forge: release} = derived by the poller from the latest Release (default);
// {pin: explicit}  = maintained directly in the pin file by a human/Renovate.
type ComponentSource struct {
	Forge string `yaml:"forge"`
	Pin   string `yaml:"pin"`
}

// DaemonConfig configures `gantry serve`. Optional; every field defaults so an existing
// config runs the daemon with sane values.
type DaemonConfig struct {
	Interval              Duration `yaml:"interval"`                // reconcile period; default 60s
	Listen                string   `yaml:"listen"`                  // HTTP bind address; default "127.0.0.1:9713" (S1 — localhost by default)
	ReconcileTimeout      Duration `yaml:"reconcile_timeout"`       // per-env reconcile deadline; default 5m
	ReconcileFailedRepeat Duration `yaml:"reconcile_failed_repeat"` // suppress repeat reconcile_failed alerts; default 1h
	Doorbell              Doorbell `yaml:"doorbell"`                // C3c; disabled by default
}

// Doorbell configures the optional forge-webhook trigger (C3c).
type Doorbell struct {
	Enabled bool      `yaml:"enabled"`
	Path    string    `yaml:"path"`   // default "/hooks/forge"
	HMAC    bool      `yaml:"hmac"`   // verify X-Hub-Signature-256 body HMAC instead of a token header
	Secret  SecretRef `yaml:"secret"` // required when Enabled; token or HMAC key
}

// SecretsConfig holds backend-wide secret settings (currently just Vault). Optional.
type SecretsConfig struct {
	Vault VaultRefs `yaml:"vault"`
}

// VaultRefs are the ambient Vault address and token, each a SecretRef so they can come from
// env or a file. Both default to the standard VAULT_ADDR/VAULT_TOKEN env vars.
type VaultRefs struct {
	Address SecretRef `yaml:"address"`
	Token   SecretRef `yaml:"token"`
}

// Environment returns the named environment.
func (c *Config) Environment(name string) (*Environment, bool) {
	for i := range c.Environments {
		if c.Environments[i].Name == name {
			return &c.Environments[i], true
		}
	}
	return nil, false
}

// Load reads, defaults, and validates a gantry.yaml.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if c.Forge.MetadataMarker == "" {
		c.Forge.MetadataMarker = "gantry-release-metadata"
	}
	if c.Git.AuthorName == "" {
		c.Git.AuthorName = "gantry"
	}
	if c.Git.AuthorEmail == "" {
		c.Git.AuthorEmail = "gantry@local"
	}
	if c.Git.Remote.Name == "" {
		c.Git.Remote.Name = "origin"
	}
	if c.Forge.Kind == "github" && c.Forge.BaseURL == "" {
		c.Forge.BaseURL = "https://api.github.com"
	}

	for i := range c.Components {
		s := &c.Components[i].Source
		if s.Forge == "" && s.Pin == "" {
			s.Forge = "release"
		}
	}

	for i := range c.Environments {
		for j := range c.Environments[i].Verify {
			p := &c.Environments[i].Verify[j]
			if p.Kind == "http" && p.ExpectStatus == 0 {
				p.ExpectStatus = 200
			}
		}
	}

	c.defaultDaemon()
	c.defaultSecrets()

	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validate() error {
	switch c.Forge.Kind {
	case "gitlab", "github":
	default:
		return fmt.Errorf("unsupported forge.kind %q (supported: gitlab, github)", c.Forge.Kind)
	}
	if err := c.validateComponents(); err != nil {
		return err
	}
	if err := c.validateEnvironments(); err != nil {
		return err
	}
	if err := c.validateNotifications(); err != nil {
		return err
	}
	if err := c.validateRemote(); err != nil {
		return err
	}
	return c.validateDaemon()
}

// validateRemote checks the optional git.remote block: push/pull over HTTPS needs a token to
// authenticate (SSH remotes can leave it unset, but gantry cannot see the remote URL from
// config, so it requires the token whenever pull/push is enabled and lets SSH ignore it).
func (c *Config) validateRemote() error {
	if (c.Git.Remote.Pull || c.Git.Remote.Push) && strings.TrimSpace(c.Git.Remote.Token.Raw) == "" {
		return errors.New("git.remote.pull/push requires git.remote.token (HTTPS auth)")
	}
	return nil
}

func (c *Config) validateComponents() error {
	seen := map[string]bool{}
	for _, comp := range c.Components {
		if comp.PinKey == "" {
			return fmt.Errorf("component %q: pin_key required", comp.ID)
		}
		if seen[comp.PinKey] {
			return fmt.Errorf("duplicate pin_key %q", comp.PinKey)
		}
		seen[comp.PinKey] = true
		if comp.Source.Forge != "" && comp.Source.Pin != "" {
			return fmt.Errorf("component %q: source must set exactly one of forge/pin", comp.ID)
		}
		if comp.Source.Pin != "" && comp.Source.Pin != "explicit" {
			return fmt.Errorf("component %q: unsupported source.pin %q (want \"explicit\")", comp.ID, comp.Source.Pin)
		}
		if comp.Source.Forge != "" && comp.Source.Forge != "release" {
			return fmt.Errorf("component %q: unsupported source.forge %q (want \"release\")", comp.ID, comp.Source.Forge)
		}
		if comp.IsExplicit() && comp.Project != "" {
			return fmt.Errorf("component %q: explicit-pin component must not set project", comp.ID)
		}
		if comp.IsForgeRelease() && comp.Project == "" {
			return fmt.Errorf("component %q: forge-release component requires project", comp.ID)
		}
	}
	return nil
}

//nolint:gocognit // -
func (c *Config) validateEnvironments() error {
	for _, env := range c.Environments {
		if env.Source.Track == "" && env.Source.PromoteFrom == "" {
			return fmt.Errorf("environment %q: source must set track or promote_from", env.Name)
		}
		if env.Source.PromoteFrom != "" {
			if _, ok := c.Environment(env.Source.PromoteFrom); !ok {
				return fmt.Errorf("environment %q: promote_from %q not found", env.Name, env.Source.PromoteFrom)
			}
		}
		switch env.Executor.Kind {
		case "compose-over-ssh", "symlink-release":
		case "blue-green":
			if err := validateBlueGreen(env); err != nil {
				return err
			}
		default:
			return fmt.Errorf("environment %q: unsupported executor.kind %q (want compose-over-ssh|symlink-release|blue-green)", env.Name, env.Executor.Kind)
		}
		conn, ok := c.Connections[env.Executor.Connection]
		if !ok {
			return fmt.Errorf("environment %q: connection %q not found", env.Name, env.Executor.Connection)
		}
		if conn.SSH == nil {
			return fmt.Errorf("environment %q: connection %q requires an ssh block", env.Name, env.Executor.Connection)
		}
		if err := validateVerifyProbes(env); err != nil {
			return err
		}
		if err := validateVerifyOnFailure(env); err != nil {
			return err
		}
	}
	return nil
}

func validateVerifyProbes(env Environment) error {
	for _, p := range env.Verify {
		switch p.Kind {
		case "http":
			if p.URL == "" {
				return fmt.Errorf("environment %q: http verify probe requires url", env.Name)
			}
		case "command":
			if p.Command == "" {
				return fmt.Errorf("environment %q: command verify probe requires command", env.Name)
			}
		case "compose-ps":
			// No extra fields; the executor's kind-aware ComposeTarget resolves the compose
			// project (current/.env for symlink-release, the idle slot for blue-green) at
			// verify time.
		default:
			return fmt.Errorf("environment %q: unsupported verify kind %q (want http|compose-ps|command)", env.Name, p.Kind)
		}
	}
	return nil
}

func validateVerifyOnFailure(env Environment) error {
	switch env.VerifyOnFailure {
	case "", "hold", "rollback":
	default:
		return fmt.Errorf("environment %q: unsupported verify_on_failure %q (want hold|rollback)", env.Name, env.VerifyOnFailure)
	}
	if env.VerifyOnFailure == "rollback" && len(env.Verify) == 0 {
		return fmt.Errorf("environment %q: verify_on_failure: rollback requires at least one verify probe", env.Name)
	}
	return nil
}

func validateBlueGreen(env Environment) error {
	if len(env.Executor.Slots) != 2 {
		return fmt.Errorf("environment %q: blue-green requires exactly slots blue and green", env.Name)
	}
	for _, name := range []string{"blue", "green"} {
		s, ok := env.Executor.Slots[name]
		if !ok || s.ProjectDir == "" {
			return fmt.Errorf("environment %q: blue-green slot %q requires project_dir", env.Name, name)
		}
	}
	p := env.Executor.Pointer
	if p.Link == "" || p.Blue == "" || p.Green == "" || p.Reload == "" {
		return fmt.Errorf("environment %q: blue-green pointer requires link, blue, green, reload", env.Name)
	}
	return nil
}

// NotifyEventKinds is the set of valid notify event kinds, for config validation. It is the
// single source of truth mirrored by the notify.Kind* string constants.
var NotifyEventKinds = map[string]bool{
	"deployed": true, "promoted": true, "rolled_back": true,
	"verify_failed": true, "drift_alarm": true, "reconcile_failed": true,
}

//nolint:gocognit // the per-kind switch is naturally a flat dispatch; splitting it hurts readability
func (c *Config) validateNotifications() error {
	for i, ch := range c.Notifications {
		switch ch.Kind {
		case "webhook", "slack", "telegram":
			if strings.TrimSpace(ch.URL.Raw) == "" {
				return fmt.Errorf("notifications[%d]: %s requires url", i, ch.Kind)
			}
			if ch.Kind == "telegram" && strings.TrimSpace(ch.ChatID.Raw) == "" {
				return fmt.Errorf("notifications[%d]: telegram requires chat_id", i)
			}
		case "email":
			if ch.SMTP.Host == "" || ch.From == "" || len(ch.To) == 0 {
				return fmt.Errorf("notifications[%d]: email requires smtp.host, from, and at least one to", i)
			}
			switch ch.SMTP.TLS {
			case "", "starttls", "implicit":
			default:
				return fmt.Errorf("notifications[%d]: unsupported smtp.tls %q (want starttls|implicit)", i, ch.SMTP.TLS)
			}
		default:
			return fmt.Errorf("notifications[%d]: unsupported kind %q (want webhook|slack|telegram|email)", i, ch.Kind)
		}
		for _, ev := range ch.Events {
			if !NotifyEventKinds[ev] {
				return fmt.Errorf("notifications[%d]: unknown event %q (want deployed|promoted|rolled_back|verify_failed|drift_alarm|reconcile_failed)", i, ev)
			}
		}
	}
	return nil
}

// defaultDaemon fills in the optional daemon block's defaults so an existing config runs the
// daemon with sane values.
func (c *Config) defaultDaemon() {
	if c.Daemon.Interval.Duration() == 0 {
		c.Daemon.Interval = DurationOf(60 * time.Second)
	}
	if c.Daemon.Listen == "" {
		c.Daemon.Listen = "127.0.0.1:9713" // localhost by default; binding wide is an explicit choice (S1)
	}
	if c.Daemon.ReconcileTimeout.Duration() == 0 {
		c.Daemon.ReconcileTimeout = DurationOf(5 * time.Minute)
	}
	if c.Daemon.ReconcileFailedRepeat.Duration() == 0 {
		c.Daemon.ReconcileFailedRepeat = DurationOf(1 * time.Hour)
	}
	if c.Daemon.Doorbell.Path == "" {
		c.Daemon.Doorbell.Path = "/hooks/forge"
	}
}

// defaultSecrets defaults the ambient Vault address/token to the standard env vars so a
// ${vault:…} ref works without an explicit secrets.vault block.
func (c *Config) defaultSecrets() {
	if c.Secrets.Vault.Address.Raw == "" {
		c.Secrets.Vault.Address = SecretRef{Raw: "${env:VAULT_ADDR}"}
	}
	if c.Secrets.Vault.Token.Raw == "" {
		c.Secrets.Vault.Token = SecretRef{Raw: "${env:VAULT_TOKEN}"}
	}
}

func (c *Config) validateDaemon() error {
	if c.Daemon.Interval.Duration() < time.Second {
		return errors.New("daemon.interval must be at least 1s")
	}
	if c.Daemon.Listen == "" {
		return errors.New("daemon.listen must not be empty")
	}
	if c.Daemon.ReconcileTimeout.Duration() < time.Second {
		return errors.New("daemon.reconcile_timeout must be at least 1s")
	}
	if c.Daemon.Doorbell.Enabled && strings.TrimSpace(c.Daemon.Doorbell.Secret.Raw) == "" {
		return errors.New("daemon.doorbell.enabled requires a doorbell.secret")
	}
	return nil
}
