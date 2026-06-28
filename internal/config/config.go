// Package config defines gantry's declarative configuration model and loader.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the whole gantry.yaml.
type Config struct {
	Forge        ForgeConfig           `yaml:"forge"`
	Connections  map[string]Connection `yaml:"connections"`
	Components   []Component           `yaml:"components"`
	Environments []Environment         `yaml:"environments"`
	Registries   map[string]Registry   `yaml:"registries"`
	Git          GitConfig             `yaml:"git"`
	Drift        DriftConfig           `yaml:"drift"`
}

// GitConfig sets the identity gantry stamps on the pin commits it makes.
// Both fields default (see Load) so the block is optional.
type GitConfig struct {
	AuthorName  string `yaml:"author_name"`
	AuthorEmail string `yaml:"author_email"`
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
	Name     string         `yaml:"name"`
	Source   Source         `yaml:"source"`
	PinFile  string         `yaml:"pin_file"`
	Executor ExecutorConfig `yaml:"executor"`
}

// Source declares how an environment's pins are computed.
type Source struct {
	Track       string `yaml:"track"`        // e.g. "latest"
	PromoteFrom string `yaml:"promote_from"` // upstream env name
}

// ExecutorConfig configures the deploy backend for an environment.
type ExecutorConfig struct {
	Kind         string   `yaml:"kind"`
	Connection   string   `yaml:"connection"`
	ProjectDir   string   `yaml:"project_dir"`
	ComposeFiles []string `yaml:"compose_files"`
	EnvFile      string   `yaml:"env_file"`
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

	for i := range c.Components {
		s := &c.Components[i].Source
		if s.Forge == "" && s.Pin == "" {
			s.Forge = "release"
		}
	}

	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validate() error {
	if c.Forge.Kind != "gitlab" {
		return fmt.Errorf("unsupported forge.kind %q (slice 1: gitlab)", c.Forge.Kind)
	}
	if err := c.validateComponents(); err != nil {
		return err
	}
	return c.validateEnvironments()
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
		if env.Executor.Kind != "compose-over-ssh" {
			return fmt.Errorf("environment %q: unsupported executor.kind %q (slice 1: compose-over-ssh)", env.Name, env.Executor.Kind)
		}
		conn, ok := c.Connections[env.Executor.Connection]
		if !ok {
			return fmt.Errorf("environment %q: connection %q not found", env.Name, env.Executor.Connection)
		}
		if env.Executor.Kind == "compose-over-ssh" && conn.SSH == nil {
			return fmt.Errorf("environment %q: connection %q requires an ssh block for compose-over-ssh",
				env.Name, env.Executor.Connection)
		}
	}
	return nil
}
