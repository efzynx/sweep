package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	CurrentConfigVersion = "1.0"
	DefaultConfigDir     = "sweep"
	DefaultConfigFile    = "config.yaml"
)

// Config represents user-configurable options for Sweep stored in ~/.config/sweep/config.yaml.
type Config struct {
	Version           string          `yaml:"version" json:"version"`
	DryRunDefault     bool            `yaml:"dry_run_default" json:"dry_run_default"`
	SnapshotSafeguard bool            `yaml:"snapshot_safeguard" json:"snapshot_safeguard"`
	PacmanRetention   int             `yaml:"pacman_retention" json:"pacman_retention"`
	Modules           map[string]bool `yaml:"modules" json:"modules"`
	ExcludePaths      []string        `yaml:"exclude_paths" json:"exclude_paths"`
	LogLevel          string          `yaml:"log_level" json:"log_level"`
}

// DefaultConfig returns the recommended default configuration for Sweep.
func DefaultConfig() *Config {
	return &Config{
		Version:           CurrentConfigVersion,
		DryRunDefault:     true,
		SnapshotSafeguard: true,
		PacmanRetention:   1,
		Modules: map[string]bool{
			"pacman":  true,
			"docker":  true,
			"flatpak": true,
			"snapper": true,
		},
		ExcludePaths: make([]string, 0),
		LogLevel:     "info",
	}
}

// DefaultConfigPath returns the canonical path to the config file:
// $XDG_CONFIG_HOME/sweep/config.yaml or ~/.config/sweep/config.yaml.
func DefaultConfigPath() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not determine user home directory: %w", err)
		}
		configHome = filepath.Join(home, ".config")
	}

	return filepath.Join(configHome, DefaultConfigDir, DefaultConfigFile), nil
}

// LoadConfig reads and parses the configuration file at the given path.
// If path is empty, DefaultConfigPath() is used.
// If the configuration file does not exist, DefaultConfig() is returned.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return nil, err
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config yaml from %q: %w", path, err)
	}

	return cfg, nil
}

// SaveConfig marshals the configuration into YAML and writes it to path.
// If path is empty, DefaultConfigPath() is used.
func SaveConfig(path string, cfg *Config) error {
	if cfg == nil {
		return errors.New("cannot save nil config")
	}

	if path == "" {
		var err error
		path, err = DefaultConfigPath()
		if err != nil {
			return err
		}
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory %q: %w", dir, err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config to yaml: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file %q: %w", path, err)
	}

	return nil
}
