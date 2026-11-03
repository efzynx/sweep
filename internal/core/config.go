package core

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	CurrentConfigVersion = "1.0"
	DefaultConfigDir     = "sweep"
	DefaultConfigFile    = "config.yaml"
	SystemConfigFile     = "/etc/sweep/config.yaml"
)

// GeneralConfig holds top-level operational switches and logging options.
type GeneralConfig struct {
	DryRunDefault     bool   `yaml:"dry_run_default" json:"dry_run_default"`
	SnapshotSafeguard bool   `yaml:"snapshot_safeguard" json:"snapshot_safeguard"`
	LogLevel          string `yaml:"log_level" json:"log_level"`
}

// ModuleConfig controls the state of an individual cleanup module.
type ModuleConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// UnmarshalYAML implements custom unmarshaling to support both boolean shorthand
// (e.g. `pacman: true`) and mapping structures (e.g. `pacman: { enabled: true }`).
func (m *ModuleConfig) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		var b bool
		if err := node.Decode(&b); err != nil {
			return err
		}
		m.Enabled = b
		return nil
	}
	type rawModuleConfig ModuleConfig
	var raw rawModuleConfig
	if err := node.Decode(&raw); err != nil {
		return err
	}
	*m = ModuleConfig(raw)
	return nil
}

// RetentionConfig configures data retention and cache vacuum thresholds.
type RetentionConfig struct {
	SnapperMinAgeDays  int `yaml:"snapper_min_age_days" json:"snapper_min_age_days"`
	PacmanKeepVersions int `yaml:"pacman_keep_versions" json:"pacman_keep_versions"`
	JournalVacuumDays  int `yaml:"journal_vacuum_days" json:"journal_vacuum_days"`
	DevCacheIdleDays   int `yaml:"dev_cache_idle_days" json:"dev_cache_idle_days"`
}

// CustomScanRule defines a user-specified path and filter pattern for custom cleanup.
type CustomScanRule struct {
	ID         string      `yaml:"id" json:"id"`
	Name       string      `yaml:"name" json:"name"`
	Path       string      `yaml:"path" json:"path"`
	Pattern    string      `yaml:"pattern" json:"pattern"`
	Recursive  bool        `yaml:"recursive" json:"recursive"`
	MinAgeDays int         `yaml:"min_age_days" json:"min_age_days"`
	Safety     SafetyLevel `yaml:"safety" json:"safety"`
}

// ResolvedPath returns the rule's Path with tilde (~) and environment variables expanded.
func (r CustomScanRule) ResolvedPath() string {
	return ExpandPath(r.Path)
}

// EffectiveSafety normalizes and returns the safety level for this rule.
func (r CustomScanRule) EffectiveSafety() SafetyLevel {
	switch strings.ToUpper(strings.TrimSpace(string(r.Safety))) {
	case "SAFE":
		return SafetySafe
	case "HIGH":
		return SafetyHigh
	case "CAUTION":
		return SafetyCaution
	default:
		if r.Safety != "" {
			return r.Safety
		}
		return SafetyCaution
	}
}

// Scan inspects the target path for this rule and returns cleanable items matching filters.
func (r CustomScanRule) Scan(ctx context.Context) ([]CleanableItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	targetPath := r.ResolvedPath()
	if targetPath == "" {
		return nil, nil
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			return nil, nil
		}
		return nil, err
	}

	safety := r.EffectiveSafety()
	minAge := time.Duration(r.MinAgeDays) * 24 * time.Hour
	now := time.Now()

	var items []CleanableItem

	if !info.IsDir() {
		if matchesPattern(r.Pattern, info.Name()) && matchesAge(info.ModTime(), minAge, now) {
			items = append(items, CleanableItem{
				ID:          fmt.Sprintf("custom:%s:%s", r.ID, targetPath),
				Name:        info.Name(),
				Description: fmt.Sprintf("[%s] %s", r.Name, targetPath),
				Path:        targetPath,
				Size:        info.Size(),
				Selected:    safety == SafetySafe,
				Safety:      safety,
				ModTime:     info.ModTime(),
				Metadata: map[string]string{
					"rule_id":   r.ID,
					"rule_name": r.Name,
				},
			})
		}
		return items, nil
	}

	if r.Recursive {
		err = filepath.WalkDir(targetPath, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}

			if !matchesPattern(r.Pattern, d.Name()) {
				return nil
			}

			fInfo, err := d.Info()
			if err != nil {
				return nil
			}

			if !matchesAge(fInfo.ModTime(), minAge, now) {
				return nil
			}

			rel, _ := filepath.Rel(targetPath, path)
			if rel == "" {
				rel = d.Name()
			}

			items = append(items, CleanableItem{
				ID:          fmt.Sprintf("custom:%s:%s", r.ID, path),
				Name:        rel,
				Description: fmt.Sprintf("[%s] %s", r.Name, path),
				Path:        path,
				Size:        fInfo.Size(),
				Selected:    safety == SafetySafe,
				Safety:      safety,
				ModTime:     fInfo.ModTime(),
				Metadata: map[string]string{
					"rule_id":   r.ID,
					"rule_name": r.Name,
				},
			})
			return nil
		})
		if err != nil && ctx.Err() != nil {
			return items, ctx.Err()
		}
	} else {
		entries, err := os.ReadDir(targetPath)
		if err != nil {
			return nil, nil
		}

		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return items, err
			}
			if entry.IsDir() {
				continue
			}

			if !matchesPattern(r.Pattern, entry.Name()) {
				continue
			}

			fInfo, err := entry.Info()
			if err != nil {
				continue
			}

			if !matchesAge(fInfo.ModTime(), minAge, now) {
				continue
			}

			filePath := filepath.Join(targetPath, entry.Name())
			items = append(items, CleanableItem{
				ID:          fmt.Sprintf("custom:%s:%s", r.ID, filePath),
				Name:        entry.Name(),
				Description: fmt.Sprintf("[%s] %s", r.Name, filePath),
				Path:        filePath,
				Size:        fInfo.Size(),
				Selected:    safety == SafetySafe,
				Safety:      safety,
				ModTime:     fInfo.ModTime(),
				Metadata: map[string]string{
					"rule_id":   r.ID,
					"rule_name": r.Name,
				},
			})
		}
	}

	return items, nil
}

// Config represents user-configurable options for Sweep stored in ~/.config/sweep/config.yaml.
type Config struct {
	Version      string                  `yaml:"version" json:"version"`
	General      GeneralConfig           `yaml:"general" json:"general"`
	Modules      map[string]ModuleConfig `yaml:"modules" json:"modules"`
	Retention    RetentionConfig         `yaml:"retention" json:"retention"`
	CustomRules  []CustomScanRule        `yaml:"custom_rules" json:"custom_rules"`
	ExcludePaths []string                `yaml:"exclude_paths,omitempty" json:"exclude_paths,omitempty"`

	// Deprecated top-level fields maintained for backward compatibility.
	DryRunDefault     bool   `yaml:"dry_run_default,omitempty" json:"dry_run_default,omitempty"`
	SnapshotSafeguard bool   `yaml:"snapshot_safeguard,omitempty" json:"snapshot_safeguard,omitempty"`
	PacmanRetention   int    `yaml:"pacman_retention,omitempty" json:"pacman_retention,omitempty"`
	LogLevel          string `yaml:"log_level,omitempty" json:"log_level,omitempty"`
}

// DefaultConfig returns the recommended default configuration for Sweep.
func DefaultConfig() *Config {
	cfg := &Config{
		Version: CurrentConfigVersion,
		General: GeneralConfig{
			DryRunDefault:     true,
			SnapshotSafeguard: true,
			LogLevel:          "info",
		},
		Modules: map[string]ModuleConfig{
			"pacman":    {Enabled: true},
			"docker":    {Enabled: true},
			"flatpak":   {Enabled: true},
			"snapper":   {Enabled: true},
			"apt":       {Enabled: true},
			"dnf":       {Enabled: true},
			"custom":    {Enabled: true},
			"usercache": {Enabled: true},
			"journal":   {Enabled: true},
			"devcache":  {Enabled: true},
		},
		Retention: RetentionConfig{
			SnapperMinAgeDays:  3,
			PacmanKeepVersions: 1,
			JournalVacuumDays:  7,
			DevCacheIdleDays:   30,
		},
		CustomRules:  make([]CustomScanRule, 0),
		ExcludePaths: make([]string, 0),

		// Backward-compatible mirror fields
		DryRunDefault:     true,
		SnapshotSafeguard: true,
		PacmanRetention:   1,
		LogLevel:          "info",
	}
	return cfg
}

// IsModuleEnabled checks whether a given module ID is enabled in the configuration.
func (c *Config) IsModuleEnabled(moduleID string) bool {
	if c == nil || c.Modules == nil {
		return true
	}
	if mod, ok := c.Modules[moduleID]; ok {
		return mod.Enabled
	}
	if mod, ok := c.Modules[strings.ToLower(moduleID)]; ok {
		return mod.Enabled
	}
	return true
}

// SetModuleEnabled toggles the enabled state of a module.
func (c *Config) SetModuleEnabled(moduleID string, enabled bool) {
	if c == nil {
		return
	}
	if c.Modules == nil {
		c.Modules = make(map[string]ModuleConfig)
	}
	c.Modules[moduleID] = ModuleConfig{Enabled: enabled}
}

// syncFields synchronizes legacy top-level attributes and structured child configs.
func (c *Config) syncFields() {
	// Sync LogLevel
	if c.LogLevel != "info" && c.General.LogLevel == "info" {
		c.General.LogLevel = c.LogLevel
	} else if c.General.LogLevel != "info" && c.LogLevel == "info" {
		c.LogLevel = c.General.LogLevel
	} else if c.General.LogLevel != "" {
		c.LogLevel = c.General.LogLevel
	} else if c.LogLevel != "" {
		c.General.LogLevel = c.LogLevel
	} else {
		c.General.LogLevel = "info"
		c.LogLevel = "info"
	}

	// Sync Pacman retention
	if c.PacmanRetention != 1 && c.Retention.PacmanKeepVersions == 1 {
		c.Retention.PacmanKeepVersions = c.PacmanRetention
	} else if c.Retention.PacmanKeepVersions != 1 && c.PacmanRetention == 1 {
		c.PacmanRetention = c.Retention.PacmanKeepVersions
	} else if c.Retention.PacmanKeepVersions > 0 {
		c.PacmanRetention = c.Retention.PacmanKeepVersions
	} else if c.PacmanRetention > 0 {
		c.Retention.PacmanKeepVersions = c.PacmanRetention
	} else {
		c.Retention.PacmanKeepVersions = 1
		c.PacmanRetention = 1
	}

	if c.Retention.SnapperMinAgeDays <= 0 {
		c.Retention.SnapperMinAgeDays = 3
	}
	if c.Retention.JournalVacuumDays <= 0 {
		c.Retention.JournalVacuumDays = 7
	}
	if c.Retention.DevCacheIdleDays <= 0 {
		c.Retention.DevCacheIdleDays = 30
	}

	// Sync DryRunDefault
	if !c.DryRunDefault && c.General.DryRunDefault {
		c.General.DryRunDefault = false
	} else if !c.General.DryRunDefault && c.DryRunDefault {
		c.DryRunDefault = false
	} else {
		c.DryRunDefault = c.General.DryRunDefault
	}

	// Sync SnapshotSafeguard
	if !c.SnapshotSafeguard && c.General.SnapshotSafeguard {
		c.General.SnapshotSafeguard = false
	} else if !c.General.SnapshotSafeguard && c.SnapshotSafeguard {
		c.SnapshotSafeguard = false
	} else {
		c.SnapshotSafeguard = c.General.SnapshotSafeguard
	}
}

type rawConfig struct {
	Version           string                  `yaml:"version"`
	General           *GeneralConfig          `yaml:"general"`
	Modules           map[string]ModuleConfig `yaml:"modules"`
	Retention         *RetentionConfig        `yaml:"retention"`
	CustomRules       []CustomScanRule        `yaml:"custom_rules"`
	ExcludePaths      []string                `yaml:"exclude_paths"`
	DryRunDefault     *bool                   `yaml:"dry_run_default"`
	SnapshotSafeguard *bool                   `yaml:"snapshot_safeguard"`
	PacmanRetention   *int                    `yaml:"pacman_retention"`
	LogLevel          *string                 `yaml:"log_level"`
}

// UnmarshalYAMLData populates the configuration from raw YAML bytes.
func (c *Config) UnmarshalYAMLData(data []byte) error {
	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}

	if raw.Version != "" {
		c.Version = raw.Version
	}

	if raw.General != nil {
		c.General = *raw.General
	}
	if raw.DryRunDefault != nil {
		c.DryRunDefault = *raw.DryRunDefault
		if raw.General == nil {
			c.General.DryRunDefault = *raw.DryRunDefault
		}
	}
	if raw.SnapshotSafeguard != nil {
		c.SnapshotSafeguard = *raw.SnapshotSafeguard
		if raw.General == nil {
			c.General.SnapshotSafeguard = *raw.SnapshotSafeguard
		}
	}
	if raw.LogLevel != nil {
		c.LogLevel = *raw.LogLevel
		if raw.General == nil || raw.General.LogLevel == "" {
			c.General.LogLevel = *raw.LogLevel
		}
	}

	if raw.Modules != nil {
		for k, v := range raw.Modules {
			c.Modules[k] = v
		}
	}

	if raw.Retention != nil {
		if raw.Retention.SnapperMinAgeDays > 0 {
			c.Retention.SnapperMinAgeDays = raw.Retention.SnapperMinAgeDays
		}
		if raw.Retention.PacmanKeepVersions > 0 {
			c.Retention.PacmanKeepVersions = raw.Retention.PacmanKeepVersions
			c.PacmanRetention = raw.Retention.PacmanKeepVersions
		}
		if raw.Retention.JournalVacuumDays > 0 {
			c.Retention.JournalVacuumDays = raw.Retention.JournalVacuumDays
		}
		if raw.Retention.DevCacheIdleDays > 0 {
			c.Retention.DevCacheIdleDays = raw.Retention.DevCacheIdleDays
		}
	}
	if raw.PacmanRetention != nil {
		c.PacmanRetention = *raw.PacmanRetention
		if raw.Retention == nil || raw.Retention.PacmanKeepVersions == 0 {
			c.Retention.PacmanKeepVersions = *raw.PacmanRetention
		}
	}

	if raw.CustomRules != nil {
		c.CustomRules = raw.CustomRules
	}
	if raw.ExcludePaths != nil {
		c.ExcludePaths = raw.ExcludePaths
	}

	c.syncFields()
	return nil
}

// DefaultConfigPath returns the canonical path to the config file:
// $XDG_CONFIG_HOME/sweep/config.yaml or ~/.config/sweep/config.yaml.
// When running as root under sudo, it resolves the original user's config directory.
func DefaultConfigPath() (string, error) {
	if getuid() == 0 && os.Getenv("SUDO_USER") != "" && os.Getenv("SUDO_USER") != "root" {
		home, err := GetUserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not determine user home directory: %w", err)
		}
		return filepath.Join(home, ".config", DefaultConfigDir, DefaultConfigFile), nil
	}

	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := GetUserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not determine user home directory: %w", err)
		}
		configHome = filepath.Join(home, ".config")
	}

	return filepath.Join(configHome, DefaultConfigDir, DefaultConfigFile), nil
}

// SystemConfigPath returns the system-wide configuration path (/etc/sweep/config.yaml).
func SystemConfigPath() string {
	return SystemConfigFile
}

// ExpandPath expands leading ~ or ~/ to the current user's home directory
// and expands any environment variables ($VAR or ${VAR}).
// Resolves the real interactive user's home directory even when running under sudo.
func ExpandPath(path string) string {
	if path == "" {
		return ""
	}

	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := GetUserHomeDir()
		if err == nil {
			if path == "~" {
				path = home
			} else {
				path = filepath.Join(home, path[2:])
			}
		}
	}

	return os.ExpandEnv(path)
}

// GenerateDefaultConfigYAML returns the default configuration serialized as YAML with comments.
func GenerateDefaultConfigYAML() string {
	return `# Sweep Configuration File
# Version: 1.0

version: "1.0"

# General application settings
general:
  # Enable dry-run mode by default (simulates cleanup without deleting files)
  dry_run_default: true

  # Create a Btrfs/Snapper snapshot before performing destructive actions
  snapshot_safeguard: true

  # Logging level: debug, info, warn, error
  log_level: "info"

# Module toggles (enable or disable specific cleanup adapters)
modules:
  pacman:
    enabled: true
  docker:
    enabled: true
  flatpak:
    enabled: true
  snapper:
    enabled: true
  apt:
    enabled: true
  dnf:
    enabled: true
  custom:
    enabled: true
  usercache:
    enabled: true
  journal:
    enabled: true
  devcache:
    enabled: true

# Data retention and vacuum thresholds
retention:
  # Minimum age (in days) before Btrfs/Snapper snapshots are eligible for cleanup
  snapper_min_age_days: 3

  # Number of old package versions to keep in Pacman cache (paccache -rk)
  pacman_keep_versions: 1

  # Number of days of systemd journal logs to retain
  journal_vacuum_days: 7

  # Minimum idle age (in days) for developer build caches
  dev_cache_idle_days: 30

# Custom directory scan and cleanup rules
custom_rules: []
# Example custom rule:
# - id: "my_temp_builds"
#   name: "My Temporary Builds"
#   path: "~/tmp/builds"
#   pattern: "*.tmp"
#   recursive: true
#   min_age_days: 7
#   safety: "SAFE"
`
}

// SaveDefaultConfig writes the default commented YAML configuration to the given path.
func SaveDefaultConfig(path string) error {
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
	_ = ChownToSudoUser(dir)

	content := GenerateDefaultConfigYAML()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write default config file %q: %w", path, err)
	}
	_ = ChownToSudoUser(path)

	return nil
}

// LoadConfig reads and parses the configuration file at the given path.
// If path is empty, it checks:
//  1. $XDG_CONFIG_HOME/sweep/config.yaml or ~/.config/sweep/config.yaml
//  2. /etc/sweep/config.yaml
//  3. Auto-creates ~/.config/sweep/config.yaml with defaults if missing.
func LoadConfig(path string) (*Config, error) {
	targetPath := path
	if targetPath == "" {
		defaultPath, err := DefaultConfigPath()
		if err != nil {
			return nil, err
		}

		if _, err := os.Stat(defaultPath); err == nil {
			targetPath = defaultPath
		} else if _, err := os.Stat(SystemConfigFile); err == nil {
			targetPath = SystemConfigFile
		} else {
			// Auto-create default config in user configuration directory
			_ = SaveDefaultConfig(defaultPath)
			return DefaultConfig(), nil
		}
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_ = SaveDefaultConfig(targetPath)
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config file %q: %w", targetPath, err)
	}

	cfg := DefaultConfig()
	if err := cfg.UnmarshalYAMLData(data); err != nil {
		return nil, fmt.Errorf("failed to parse config yaml from %q: %w", targetPath, err)
	}

	return cfg, nil
}

// AddCustomRule appends or updates a custom scan rule in the configuration,
// validates its target path, and atomically persists the updated configuration to DefaultConfigPath().
func (c *Config) AddCustomRule(rule CustomScanRule) error {
	if c == nil {
		return errors.New("cannot add custom rule to nil config")
	}

	trimmedPath := strings.TrimSpace(rule.Path)
	if trimmedPath == "" {
		return errors.New("custom rule path cannot be empty")
	}

	// Validate path pattern syntax if glob wildcards are present
	if strings.ContainsAny(trimmedPath, "*?[]") {
		if _, err := filepath.Match(filepath.Base(trimmedPath), ""); err != nil {
			return fmt.Errorf("invalid custom rule path pattern %q: %w", trimmedPath, err)
		}
	}

	// Validate pattern syntax if specified
	if rule.Pattern != "" {
		if _, err := filepath.Match(rule.Pattern, ""); err != nil {
			return fmt.Errorf("invalid custom rule filter pattern %q: %w", rule.Pattern, err)
		}
	}

	if rule.ID == "" {
		if rule.Name != "" {
			rule.ID = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(rule.Name), " ", "_"))
		} else {
			rule.ID = fmt.Sprintf("custom_%d", time.Now().UnixNano())
		}
	}
	if rule.Name == "" {
		rule.Name = rule.ID
	}
	if rule.Safety == "" {
		rule.Safety = SafetySafe
	}

	// Check if a rule with this ID already exists, and replace it; otherwise append
	replaced := false
	for i, existing := range c.CustomRules {
		if existing.ID == rule.ID {
			c.CustomRules[i] = rule
			replaced = true
			break
		}
	}
	if !replaced {
		c.CustomRules = append(c.CustomRules, rule)
	}

	return SaveConfig(c, "")
}

// RemoveCustomRule removes the custom scan rule matching the given ID
// and atomically persists the updated configuration to DefaultConfigPath().
func (c *Config) RemoveCustomRule(id string) error {
	if c == nil {
		return errors.New("cannot remove rule from nil config")
	}
	if strings.TrimSpace(id) == "" {
		return errors.New("custom rule id cannot be empty")
	}

	found := false
	filtered := make([]CustomScanRule, 0, len(c.CustomRules))
	for _, rule := range c.CustomRules {
		if rule.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, rule)
	}

	if !found {
		return fmt.Errorf("custom rule with id %q not found", id)
	}

	c.CustomRules = filtered
	return SaveConfig(c, "")
}

// Save marshals and writes the configuration to disk. If path is empty, DefaultConfigPath() is used.
func (c *Config) Save(path string) error {
	return SaveConfig(c, path)
}

// SaveConfig marshals the configuration into YAML and atomically writes it to path.
// If path is empty, DefaultConfigPath() is used.
func SaveConfig(cfg *Config, path string) error {
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
	_ = ChownToSudoUser(dir)

	cfg.syncFields()

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config to yaml: %w", err)
	}

	// Write atomically using a temporary file in the destination directory
	tmpFile, err := os.CreateTemp(dir, ".sweep_config_*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp config file in %q: %w", dir, err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	_ = tmpFile.Chmod(0644)

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write config to temp file %q: %w", tmpName, err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to sync temp file %q: %w", tmpName, err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file %q: %w", tmpName, err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to atomically replace config file %q: %w", path, err)
	}

	_ = ChownToSudoUser(path)

	return nil
}

func matchesPattern(pattern, name string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	matched, err := filepath.Match(pattern, name)
	if err == nil && matched {
		return true
	}
	matchedLower, errLower := filepath.Match(strings.ToLower(pattern), strings.ToLower(name))
	if errLower == nil && matchedLower {
		return true
	}
	return false
}

func matchesAge(modTime time.Time, minAge time.Duration, now time.Time) bool {
	if minAge <= 0 {
		return true
	}
	return now.Sub(modTime) >= minAge
}
