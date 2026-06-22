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
	ID      string `yaml:"id"`
	Project string `yaml:"project"`
	PinKey  string `yaml:"pin_key"`
}

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
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validate() error {
	if c.Forge.Kind != "gitlab" {
		return fmt.Errorf("unsupported forge.kind %q (slice 1: gitlab)", c.Forge.Kind)
	}
	seen := map[string]bool{}
	for _, comp := range c.Components {
		if comp.PinKey == "" {
			return fmt.Errorf("component %q: pin_key required", comp.ID)
		}
		if seen[comp.PinKey] {
			return fmt.Errorf("duplicate pin_key %q", comp.PinKey)
		}
		seen[comp.PinKey] = true
	}
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
		if _, ok := c.Connections[env.Executor.Connection]; !ok {
			return fmt.Errorf("environment %q: connection %q not found", env.Name, env.Executor.Connection)
		}
	}
	return nil
}
