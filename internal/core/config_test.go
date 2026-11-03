package core

import (
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	if !cfg.DryRunDefault {
		t.Error("expected DryRunDefault to be true by default")
	}
	if !cfg.SnapshotSafeguard {
		t.Error("expected SnapshotSafeguard to be true by default")
	}
	if cfg.PacmanRetention != 1 {
		t.Errorf("expected PacmanRetention=1, got %d", cfg.PacmanRetention)
	}
}

func TestLoadAndSaveConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "sweep", "config.yaml")

	// Test loading when file does not exist -> returns default
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig on non-existent path failed: %v", err)
	}
	if cfg.Version != CurrentConfigVersion {
		t.Errorf("expected version %q, got %q", CurrentConfigVersion, cfg.Version)
	}

	// Modify and save
	cfg.LogLevel = "debug"
	cfg.PacmanRetention = 2
	cfg.DryRunDefault = false

	if err := SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Reload and verify
	loaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed after save: %v", err)
	}

	if loaded.LogLevel != "debug" {
		t.Errorf("expected LogLevel 'debug', got %q", loaded.LogLevel)
	}
	if loaded.PacmanRetention != 2 {
		t.Errorf("expected PacmanRetention 2, got %d", loaded.PacmanRetention)
	}
	if loaded.DryRunDefault != false {
		t.Error("expected DryRunDefault false")
	}
}

func TestDefaultConfigPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/test-xdg")
	path, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath error: %v", err)
	}
	expected := "/tmp/test-xdg/sweep/config.yaml"
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}
