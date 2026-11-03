package core

import (
	"context"
	"os"
	"os/user"
	"path/filepath"
	"strings"
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

	if err := SaveConfig(cfg, configPath); err != nil {
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

func TestGetUserHomeDir_StandardUser(t *testing.T) {
	origGetuid := getuid
	defer func() { getuid = origGetuid }()

	// Simulate standard non-root user (uid 1000)
	getuid = func() int { return 1000 }
	t.Setenv("SUDO_USER", "ignored_for_nonroot")

	home, err := GetUserHomeDir()
	if err != nil {
		t.Fatalf("GetUserHomeDir error: %v", err)
	}

	expected, _ := os.UserHomeDir()
	if home != expected {
		t.Errorf("expected %q, got %q", expected, home)
	}
}

func TestGetUserHomeDir_SudoUserSimulation(t *testing.T) {
	origGetuid := getuid
	origLookup := lookupUser
	defer func() {
		getuid = origGetuid
		lookupUser = origLookup
	}()

	// 1. Simulate root with SUDO_USER resolved via lookup
	getuid = func() int { return 0 }
	t.Setenv("SUDO_USER", "alice")

	lookupUser = func(username string) (*user.User, error) {
		if username == "alice" {
			return &user.User{
				Username: "alice",
				HomeDir:  "/home/alice",
			}, nil
		}
		return nil, user.UnknownUserError(username)
	}

	home, err := GetUserHomeDir()
	if err != nil {
		t.Fatalf("GetUserHomeDir failed: %v", err)
	}
	if home != "/home/alice" {
		t.Errorf("expected /home/alice, got %q", home)
	}

	// Verify ExpandPath and DefaultConfigPath resolve correctly under SUDO_USER
	expanded := ExpandPath("~/projects")
	if expanded != "/home/alice/projects" {
		t.Errorf("expected ExpandPath to use /home/alice, got %q", expanded)
	}

	cfgPath, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath failed: %v", err)
	}
	expectedCfg := "/home/alice/.config/sweep/config.yaml"
	if cfgPath != expectedCfg {
		t.Errorf("expected DefaultConfigPath %q, got %q", expectedCfg, cfgPath)
	}

	// 2. Simulate root with SUDO_USER fallback when user lookup fails
	lookupUser = func(username string) (*user.User, error) {
		return nil, user.UnknownUserError(username)
	}
	t.Setenv("SUDO_USER", "bob")
	homeBob, err := GetUserHomeDir()
	if err != nil {
		t.Fatalf("GetUserHomeDir with fallback failed: %v", err)
	}
	if homeBob != "/home/bob" {
		t.Errorf("expected fallback /home/bob, got %q", homeBob)
	}

	// 3. Simulate root with SUDO_USER=root (ignored, should fallback to standard home)
	t.Setenv("SUDO_USER", "root")
	stdHome, _ := os.UserHomeDir()
	homeRoot, err := GetUserHomeDir()
	if err != nil {
		t.Fatalf("GetUserHomeDir failed: %v", err)
	}
	if homeRoot != stdHome {
		t.Errorf("expected standard home %q when SUDO_USER=root, got %q", stdHome, homeRoot)
	}

	// 4. Simulate root with empty SUDO_USER (standard home)
	t.Setenv("SUDO_USER", "")
	homeEmpty, err := GetUserHomeDir()
	if err != nil {
		t.Fatalf("GetUserHomeDir failed: %v", err)
	}
	if homeEmpty != stdHome {
		t.Errorf("expected standard home %q when SUDO_USER empty, got %q", stdHome, homeEmpty)
	}
}

func TestLoadConfig_DoesNotClobberExistingConfigUnderSudo(t *testing.T) {
	origGetuid := getuid
	origLookup := lookupUser
	defer func() {
		getuid = origGetuid
		lookupUser = origLookup
	}()

	tempDir := t.TempDir()
	userHome := filepath.Join(tempDir, "user_alice")
	userConfigDir := filepath.Join(userHome, ".config", "sweep")
	if err := os.MkdirAll(userConfigDir, 0755); err != nil {
		t.Fatalf("failed to create user config dir: %v", err)
	}
	userConfigFile := filepath.Join(userConfigDir, "config.yaml")

	// Pre-create custom user configuration with custom properties
	customYAML := `# Custom User Sweep Configuration
version: "1.0"
general:
  dry_run_default: false
  snapshot_safeguard: false
  log_level: "debug"
custom_rules:
  - id: "custom_rule_alice"
    name: "Alice Build Dirs"
    path: "~/projects/build"
    pattern: "*.o"
    recursive: true
    safety: "SAFE"
`
	if err := os.WriteFile(userConfigFile, []byte(customYAML), 0644); err != nil {
		t.Fatalf("failed to write initial user config: %v", err)
	}

	initialStat, err := os.Stat(userConfigFile)
	if err != nil {
		t.Fatalf("failed to stat user config: %v", err)
	}

	// Simulate running under sudo: root uid 0, SUDO_USER="alice"
	getuid = func() int { return 0 }
	t.Setenv("SUDO_USER", "alice")
	lookupUser = func(username string) (*user.User, error) {
		if username == "alice" {
			return &user.User{
				Username: "alice",
				Uid:      "1001",
				Gid:      "1001",
				HomeDir:  userHome,
			}, nil
		}
		return nil, user.UnknownUserError(username)
	}

	// Verify DefaultConfigPath() points to user's config
	cfgPath, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath failed: %v", err)
	}
	if cfgPath != userConfigFile {
		t.Fatalf("expected DefaultConfigPath to be %q, got %q", userConfigFile, cfgPath)
	}

	// LoadConfig should read user config and NOT clobber it
	loadedCfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if loadedCfg.General.DryRunDefault != false {
		t.Errorf("expected DryRunDefault == false from custom config, got true")
	}
	if loadedCfg.General.LogLevel != "debug" {
		t.Errorf("expected LogLevel == 'debug', got %q", loadedCfg.General.LogLevel)
	}
	if len(loadedCfg.CustomRules) != 1 || loadedCfg.CustomRules[0].ID != "custom_rule_alice" {
		t.Errorf("expected custom rule 'custom_rule_alice' loaded, got %+v", loadedCfg.CustomRules)
	}

	// Ensure file on disk was NOT clobbered or modified
	postStat, err := os.Stat(userConfigFile)
	if err != nil {
		t.Fatalf("failed to stat user config post-load: %v", err)
	}
	if postStat.Size() != initialStat.Size() {
		t.Errorf("expected config file size to be unchanged, initial: %d, post: %d", initialStat.Size(), postStat.Size())
	}

	postData, _ := os.ReadFile(userConfigFile)
	if string(postData) != customYAML {
		t.Errorf("config file content was clobbered:\n%s", string(postData))
	}
}

func TestSaveConfig_SudoUserOwnershipAndPermissions(t *testing.T) {
	origGetuid := getuid
	origLookup := lookupUser
	origChown := chownFile
	defer func() {
		getuid = origGetuid
		lookupUser = origLookup
		chownFile = origChown
	}()

	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config_test", "sweep")
	configPath := filepath.Join(configDir, "config.yaml")

	var chownedPaths []string
	var chownedUIDs, chownedGIDs []int
	chownFile = func(name string, uid, gid int) error {
		chownedPaths = append(chownedPaths, name)
		chownedUIDs = append(chownedUIDs, uid)
		chownedGIDs = append(chownedGIDs, gid)
		return nil
	}

	// Simulate root running with SUDO_USER="alice"
	getuid = func() int { return 0 }
	t.Setenv("SUDO_USER", "alice")
	lookupUser = func(username string) (*user.User, error) {
		if username == "alice" {
			return &user.User{
				Username: "alice",
				Uid:      "1005",
				Gid:      "1005",
				HomeDir:  tempDir,
			}, nil
		}
		return nil, user.UnknownUserError(username)
	}

	cfg := DefaultConfig()
	if err := SaveConfig(cfg, configPath); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Verify file was written with 0644 mode
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("failed to stat configPath: %v", err)
	}
	perm := info.Mode().Perm()
	if perm != 0644 {
		t.Errorf("expected permissions 0644, got %#o", perm)
	}

	// Verify chown was invoked for dir and file with UID/GID 1005
	if len(chownedPaths) < 2 {
		t.Fatalf("expected at least 2 chown calls (dir and file), got %d: %v", len(chownedPaths), chownedPaths)
	}
	foundDir := false
	foundFile := false
	for i, p := range chownedPaths {
		if p == configDir && chownedUIDs[i] == 1005 && chownedGIDs[i] == 1005 {
			foundDir = true
		}
		if p == configPath && chownedUIDs[i] == 1005 && chownedGIDs[i] == 1005 {
			foundFile = true
		}
	}
	if !foundDir {
		t.Errorf("expected config directory %q to be chowned to 1005:1005", configDir)
	}
	if !foundFile {
		t.Errorf("expected config file %q to be chowned to 1005:1005", configPath)
	}
}

func TestAddAndRemoveCustomRule(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempDir)

	configPath := filepath.Join(tempDir, "sweep", "config.yaml")

	cfg := DefaultConfig()
	if err := SaveConfig(cfg, configPath); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// 1. Validation: empty path should fail
	err := cfg.AddCustomRule(CustomScanRule{
		Name: "Empty Path Rule",
		Path: "   ",
	})
	if err == nil {
		t.Fatal("expected error when adding rule with empty path, got nil")
	}

	// 2. Validation: invalid glob pattern should fail
	err = cfg.AddCustomRule(CustomScanRule{
		Name:    "Invalid Pattern Rule",
		Path:    "/tmp/test",
		Pattern: "[a-",
	})
	if err == nil {
		t.Fatal("expected error when adding rule with invalid pattern, got nil")
	}

	// 3. Add valid rule
	rule1 := CustomScanRule{
		ID:        "rule_downloads",
		Name:      "Old Downloads",
		Path:      "~/Downloads/old",
		Pattern:   "*.iso",
		Recursive: true,
		Safety:    SafetyCaution,
	}

	if err := cfg.AddCustomRule(rule1); err != nil {
		t.Fatalf("AddCustomRule failed: %v", err)
	}

	// Verify in-memory
	if len(cfg.CustomRules) != 1 {
		t.Fatalf("expected 1 custom rule, got %d", len(cfg.CustomRules))
	}
	if cfg.CustomRules[0].ID != "rule_downloads" {
		t.Errorf("expected rule ID rule_downloads, got %s", cfg.CustomRules[0].ID)
	}

	// Verify persisted on disk
	reloaded, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(reloaded.CustomRules) != 1 {
		t.Fatalf("expected 1 persisted custom rule, got %d", len(reloaded.CustomRules))
	}
	if reloaded.CustomRules[0].Name != "Old Downloads" {
		t.Errorf("expected Old Downloads, got %s", reloaded.CustomRules[0].Name)
	}

	// 4. Update existing rule (same ID replaces)
	rule1Updated := rule1
	rule1Updated.Pattern = "*.img"
	if err := cfg.AddCustomRule(rule1Updated); err != nil {
		t.Fatalf("AddCustomRule update failed: %v", err)
	}
	if len(cfg.CustomRules) != 1 {
		t.Errorf("expected rule count to stay 1 after updating same ID, got %d", len(cfg.CustomRules))
	}

	// 5. Add second rule with auto-generated ID & name defaults
	rule2 := CustomScanRule{
		Path: "/var/tmp/builds",
	}
	if err := cfg.AddCustomRule(rule2); err != nil {
		t.Fatalf("AddCustomRule with auto-ID failed: %v", err)
	}
	if len(cfg.CustomRules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(cfg.CustomRules))
	}
	autoID := cfg.CustomRules[1].ID
	if autoID == "" {
		t.Errorf("expected generated ID for rule2, got empty")
	}

	// 6. Remove non-existent rule should fail
	if err := cfg.RemoveCustomRule("non_existent_id"); err == nil {
		t.Fatal("expected error removing non-existent rule, got nil")
	}

	// 7. Remove empty ID should fail
	if err := cfg.RemoveCustomRule(""); err == nil {
		t.Fatal("expected error removing empty rule id, got nil")
	}

	// 8. Remove rule1
	if err := cfg.RemoveCustomRule("rule_downloads"); err != nil {
		t.Fatalf("RemoveCustomRule failed: %v", err)
	}
	if len(cfg.CustomRules) != 1 {
		t.Fatalf("expected 1 rule remaining, got %d", len(cfg.CustomRules))
	}
	if cfg.CustomRules[0].ID != autoID {
		t.Errorf("expected remaining rule ID %s, got %s", autoID, cfg.CustomRules[0].ID)
	}

	// Verify persistence after removal
	reloaded2, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if len(reloaded2.CustomRules) != 1 || reloaded2.CustomRules[0].ID != autoID {
		t.Errorf("persistence mismatch after removal")
	}
}

func TestSaveConfig_AtomicAndNil(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "sweep", "config.yaml")

	// Nil config returns error
	if err := SaveConfig(nil, configPath); err == nil {
		t.Fatal("expected error saving nil config, got nil")
	}

	cfg := DefaultConfig()
	cfg.General.LogLevel = "debug"
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read written config: %v", err)
	}
	if !strings.Contains(string(data), "debug") {
		t.Errorf("expected written file to contain 'debug'")
	}
}
