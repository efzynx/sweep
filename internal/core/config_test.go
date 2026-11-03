package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	if !cfg.DryRunDefault || !cfg.General.DryRunDefault {
		t.Error("expected DryRunDefault to be true by default")
	}
	if !cfg.SnapshotSafeguard || !cfg.General.SnapshotSafeguard {
		t.Error("expected SnapshotSafeguard to be true by default")
	}
	if cfg.PacmanRetention != 1 || cfg.Retention.PacmanKeepVersions != 1 {
		t.Errorf("expected PacmanRetention=1, got %d (retention=%d)", cfg.PacmanRetention, cfg.Retention.PacmanKeepVersions)
	}
	if cfg.Retention.SnapperMinAgeDays != 3 {
		t.Errorf("expected SnapperMinAgeDays=3, got %d", cfg.Retention.SnapperMinAgeDays)
	}
	if cfg.Retention.JournalVacuumDays != 7 {
		t.Errorf("expected JournalVacuumDays=7, got %d", cfg.Retention.JournalVacuumDays)
	}
	if cfg.Retention.DevCacheIdleDays != 30 {
		t.Errorf("expected DevCacheIdleDays=30, got %d", cfg.Retention.DevCacheIdleDays)
	}
	if cfg.General.LogLevel != "info" || cfg.LogLevel != "info" {
		t.Errorf("expected LogLevel 'info', got general=%q top=%q", cfg.General.LogLevel, cfg.LogLevel)
	}

	// Verify default modules are enabled
	for _, mod := range []string{"pacman", "docker", "flatpak", "snapper", "apt", "dnf", "custom", "usercache", "journal", "devcache"} {
		if !cfg.IsModuleEnabled(mod) {
			t.Errorf("expected module %q to be enabled by default", mod)
		}
	}
}

func TestLoadAndSaveConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "sweep", "config.yaml")

	// Test loading when file does not exist -> auto-creates and returns default
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig on non-existent path failed: %v", err)
	}
	if cfg.Version != CurrentConfigVersion {
		t.Errorf("expected version %q, got %q", CurrentConfigVersion, cfg.Version)
	}

	// Check file was auto-created on disk
	if _, err := os.Stat(configPath); err != nil {
		t.Errorf("expected auto-created config file at %s, got error: %v", configPath, err)
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

	if loaded.LogLevel != "debug" || loaded.General.LogLevel != "debug" {
		t.Errorf("expected LogLevel 'debug', got %q", loaded.LogLevel)
	}
	if loaded.PacmanRetention != 2 || loaded.Retention.PacmanKeepVersions != 2 {
		t.Errorf("expected PacmanRetention 2, got %d", loaded.PacmanRetention)
	}
	if loaded.DryRunDefault != false || loaded.General.DryRunDefault != false {
		t.Error("expected DryRunDefault false")
	}
}

func TestAutoCreateDefaultConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "auto_create", "config.yaml")

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	// Verify file content has comments
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read auto-created config: %v", err)
	}
	content := string(data)
	if !filepath.IsAbs(configPath) {
		t.Errorf("expected absolute path")
	}
	if len(content) == 0 {
		t.Fatal("auto-created config is empty")
	}
	if !containsSubstring(content, "# Sweep Configuration File") {
		t.Error("expected auto-created config to contain header comment")
	}
	if !containsSubstring(content, "dry_run_default: true") {
		t.Error("expected auto-created config to contain dry_run_default")
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("skipping expand path test: home directory unavailable")
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "tilde only",
			input:    "~",
			expected: home,
		},
		{
			name:     "tilde slash path",
			input:    "~/foo/bar",
			expected: filepath.Join(home, "foo", "bar"),
		},
		{
			name:     "absolute path",
			input:    "/var/log",
			expected: "/var/log",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandPath(tt.input)
			if got != tt.expected {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}

	// Test env var expansion
	t.Setenv("SWEEP_TEST_DIR", "/opt/sweep_test")
	expanded := ExpandPath("$SWEEP_TEST_DIR/cache")
	if expanded != "/opt/sweep_test/cache" {
		t.Errorf("expected env var expansion, got %q", expanded)
	}
}

func TestYAMLMarshalingAndUnmarshaling(t *testing.T) {
	yamlContent := `
version: "1.0"
general:
  dry_run_default: false
  snapshot_safeguard: false
  log_level: "warn"
modules:
  pacman: false
  docker:
    enabled: true
  custom:
    enabled: true
retention:
  snapper_min_age_days: 10
  pacman_keep_versions: 3
  journal_vacuum_days: 14
  dev_cache_idle_days: 45
custom_rules:
  - id: "test_rule"
    name: "Temporary Test Files"
    path: "~/test_cache"
    pattern: "*.cache"
    recursive: true
    min_age_days: 5
    safety: "SAFE"
`
	cfg := DefaultConfig()
	if err := cfg.UnmarshalYAMLData([]byte(yamlContent)); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if cfg.General.DryRunDefault != false || cfg.DryRunDefault != false {
		t.Error("expected DryRunDefault false")
	}
	if cfg.General.SnapshotSafeguard != false || cfg.SnapshotSafeguard != false {
		t.Error("expected SnapshotSafeguard false")
	}
	if cfg.General.LogLevel != "warn" || cfg.LogLevel != "warn" {
		t.Errorf("expected LogLevel warn, got %q", cfg.General.LogLevel)
	}

	// Shorthand boolean module
	if cfg.IsModuleEnabled("pacman") {
		t.Error("expected pacman to be disabled")
	}
	// Mapping module
	if !cfg.IsModuleEnabled("docker") {
		t.Error("expected docker to be enabled")
	}
	if !cfg.IsModuleEnabled("custom") {
		t.Error("expected custom to be enabled")
	}
	// Unspecified module defaults to true
	if !cfg.IsModuleEnabled("flatpak") {
		t.Error("expected unspecified flatpak to default to enabled")
	}

	// Retention thresholds
	if cfg.Retention.SnapperMinAgeDays != 10 {
		t.Errorf("expected SnapperMinAgeDays=10, got %d", cfg.Retention.SnapperMinAgeDays)
	}
	if cfg.Retention.PacmanKeepVersions != 3 || cfg.PacmanRetention != 3 {
		t.Errorf("expected PacmanKeepVersions=3, got %d (pacman_retention=%d)", cfg.Retention.PacmanKeepVersions, cfg.PacmanRetention)
	}
	if cfg.Retention.JournalVacuumDays != 14 {
		t.Errorf("expected JournalVacuumDays=14, got %d", cfg.Retention.JournalVacuumDays)
	}
	if cfg.Retention.DevCacheIdleDays != 45 {
		t.Errorf("expected DevCacheIdleDays=45, got %d", cfg.Retention.DevCacheIdleDays)
	}

	// Custom rules
	if len(cfg.CustomRules) != 1 {
		t.Fatalf("expected 1 custom rule, got %d", len(cfg.CustomRules))
	}
	rule := cfg.CustomRules[0]
	if rule.ID != "test_rule" || rule.Name != "Temporary Test Files" {
		t.Errorf("unexpected rule metadata: %+v", rule)
	}
	if rule.Pattern != "*.cache" || !rule.Recursive || rule.MinAgeDays != 5 {
		t.Errorf("unexpected rule filter properties: %+v", rule)
	}
	if rule.EffectiveSafety() != SafetySafe {
		t.Errorf("expected SafetySafe, got %v", rule.EffectiveSafety())
	}
}

func TestCustomScanRule_Scan(t *testing.T) {
	tempDir := t.TempDir()

	// Create test file structure:
	//   tempDir/
	//     file1.tmp (5 bytes)
	//     file2.log (10 bytes)
	//     sub/
	//       file3.tmp (15 bytes)
	//       file4.txt (20 bytes)
	subDir := filepath.Join(tempDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("failed to create sub dir: %v", err)
	}

	f1 := filepath.Join(tempDir, "file1.tmp")
	f2 := filepath.Join(tempDir, "file2.log")
	f3 := filepath.Join(subDir, "file3.tmp")
	f4 := filepath.Join(subDir, "file4.txt")

	if err := os.WriteFile(f1, []byte("12345"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("1234567890"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f3, []byte("123456789012345"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f4, []byte("12345678901234567890"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Recursive scan for *.tmp
	recRule := CustomScanRule{
		ID:        "rule-rec",
		Name:      "Recursive TMP",
		Path:      tempDir,
		Pattern:   "*.tmp",
		Recursive: true,
		Safety:    SafetySafe,
	}

	items, err := recRule.Scan(ctx)
	if err != nil {
		t.Fatalf("recRule.Scan failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items for recursive tmp scan, got %d", len(items))
	}
	var totalSize int64
	for _, it := range items {
		totalSize += it.Size
		if it.Safety != SafetySafe {
			t.Errorf("expected item safety SAFE, got %v", it.Safety)
		}
		if !it.Selected {
			t.Errorf("expected safe item to be selected")
		}
	}
	if totalSize != 20 { // 5 + 15
		t.Errorf("expected total size 20, got %d", totalSize)
	}

	// 2. Non-recursive scan for *.tmp
	nonRecRule := CustomScanRule{
		ID:        "rule-nonrec",
		Name:      "Non-Recursive TMP",
		Path:      tempDir,
		Pattern:   "*.tmp",
		Recursive: false,
		Safety:    SafetyCaution,
	}

	items, err = nonRecRule.Scan(ctx)
	if err != nil {
		t.Fatalf("nonRecRule.Scan failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item for non-recursive tmp scan, got %d", len(items))
	}
	if items[0].Path != f1 {
		t.Errorf("expected path %s, got %s", f1, items[0].Path)
	}
	if items[0].Selected {
		t.Error("caution item should not be selected by default")
	}

	// 3. MinAgeDays filter
	// Make f1 modified 10 days ago
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	_ = os.Chtimes(f1, oldTime, oldTime)

	ageRule := CustomScanRule{
		ID:         "rule-age",
		Name:       "Old TMP",
		Path:       tempDir,
		Pattern:    "*.tmp",
		Recursive:  true,
		MinAgeDays: 5,
		Safety:     SafetySafe,
	}

	items, err = ageRule.Scan(ctx)
	if err != nil {
		t.Fatalf("ageRule.Scan failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected only 1 old item, got %d", len(items))
	}
	if items[0].Path != f1 {
		t.Errorf("expected old item %s, got %s", f1, items[0].Path)
	}
}

func TestRegistryEnabledModules(t *testing.T) {
	reg := NewRegistry()
	cfg := DefaultConfig()

	// Register dummy modules
	m1 := &mockModule{id: "m1", name: "M1", available: true}
	m2 := &mockModule{id: "m2", name: "M2", available: true}
	m3 := &mockModule{id: "m3", name: "M3", available: false}

	_ = reg.Register(m1)
	_ = reg.Register(m2)
	_ = reg.Register(m3)

	// All available
	avail := reg.AvailableModules()
	if len(avail) != 2 {
		t.Fatalf("expected 2 available modules, got %d", len(avail))
	}

	// Disable m1 in config
	cfg.SetModuleEnabled("m1", false)

	enabled := reg.EnabledModules(cfg)
	if len(enabled) != 1 {
		t.Fatalf("expected 1 enabled module, got %d", len(enabled))
	}
	if enabled[0].ID() != "m2" {
		t.Errorf("expected enabled module to be m2, got %s", enabled[0].ID())
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

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || filepath.Base(s) != "" && (len(s) > 0 && filepath.Clean(s) != "" && stringContains(s, substr)))
}

func stringContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
