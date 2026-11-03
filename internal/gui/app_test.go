package gui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/efzynx/sweep/internal/adapters/custom"
	"github.com/efzynx/sweep/internal/core"
)

type mockModule struct {
	id          string
	name        string
	category    string
	description string
	available   bool
	elevation   bool
	items       []core.CleanableItem
	scanErr     error
	cleanErr    error
	cleaned     []core.CleanableItem
	freed       int64
	cfg         *core.Config
}

func (m *mockModule) ID() string                 { return m.id }
func (m *mockModule) Name() string               { return m.name }
func (m *mockModule) Category() string           { return m.category }
func (m *mockModule) Description() string        { return m.description }
func (m *mockModule) IsAvailable() bool          { return m.available }
func (m *mockModule) RequiresElevation() bool    { return m.elevation }
func (m *mockModule) SetConfig(cfg *core.Config) { m.cfg = cfg }

func (m *mockModule) Scan(ctx context.Context) (core.ScanResult, error) {
	if m.scanErr != nil {
		return core.ScanResult{}, m.scanErr
	}
	return core.ScanResult{
		Items:     m.items,
		TotalSize: 0,
		ScannedAt: time.Now(),
	}, nil
}

func (m *mockModule) Clean(ctx context.Context, items []core.CleanableItem, opts core.CleanOptions) (core.CleanResult, error) {
	if m.cleanErr != nil {
		return core.CleanResult{}, m.cleanErr
	}
	if opts.DryRun {
		var total int64
		for _, it := range items {
			total += it.Size
		}
		return core.CleanResult{
			CleanedItems: items,
			FreedBytes:   total,
		}, nil
	}
	m.cleaned = append(m.cleaned, items...)
	var total int64
	for _, it := range items {
		total += it.Size
	}
	m.freed += total
	return core.CleanResult{
		CleanedItems: items,
		FreedBytes:   total,
	}, nil
}

type mockSnapperModule struct {
	mockModule
	snapshotCreated bool
	snapshotErr     error
}

func (s *mockSnapperModule) CreateSafeguardSnapshot(desc string) error {
	if s.snapshotErr != nil {
		return s.snapshotErr
	}
	s.snapshotCreated = true
	return nil
}

func TestApp_Lifecycle(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	app := NewApp()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app.Startup(ctx)
	defer app.Shutdown(ctx)

	if app.ctx == nil {
		t.Fatal("expected app.ctx to be initialized")
	}
	if app.reg == nil {
		t.Fatal("expected app.reg to be initialized")
	}
	if app.cfg == nil {
		t.Fatal("expected app.cfg to be initialized")
	}

	// Verify modules registered
	mods := app.reg.Modules()
	if len(mods) == 0 {
		t.Error("expected default adapters to be registered on startup")
	}

	// Verify shutdown cancels context
	app.Shutdown(ctx)
	if app.ctx.Err() == nil {
		t.Error("expected app.ctx to be cancelled after shutdown")
	}
}

func TestApp_GetDistroInfo(t *testing.T) {
	app := NewApp()
	info, err := app.GetDistroInfo()
	if err != nil {
		t.Fatalf("unexpected error getting distro info: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil distro info")
	}
	if info.Family == "" {
		t.Error("expected non-empty distro family")
	}
}

func TestApp_GetStorageInfo(t *testing.T) {
	reg := core.NewRegistry()
	cfg := core.DefaultConfig()
	app := NewAppWithDependencies(reg, cfg)

	info, err := app.GetStorageInfo()
	if err != nil {
		t.Logf("GetStorageInfo returned error on current system: %v (tolerated if non-standard mount env)", err)
	}
	// Verify that if no error, root path is populated
	if err == nil && info.Root.Path == "" {
		t.Error("expected root path to be populated in storage info")
	}
}

func TestApp_GetModules(t *testing.T) {
	reg := core.NewRegistry()
	cfg := core.DefaultConfig()
	cfg.Modules["mock1"] = core.ModuleConfig{Enabled: true}
	cfg.Modules["mock2"] = core.ModuleConfig{Enabled: false}

	m1 := &mockModule{id: "mock1", name: "Mock 1", category: "test", available: true}
	m2 := &mockModule{id: "mock2", name: "Mock 2", category: "test", available: true}
	_ = reg.Register(m1)
	_ = reg.Register(m2)

	app := NewAppWithDependencies(reg, cfg)
	summaries, err := app.GetModules()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 module summaries, got %d", len(summaries))
	}

	for _, s := range summaries {
		if s.ID == "mock1" && !s.Enabled {
			t.Errorf("expected mock1 to be enabled")
		}
		if s.ID == "mock2" && s.Enabled {
			t.Errorf("expected mock2 to be disabled")
		}
	}
}

func TestApp_ScanAll_and_ScanModule(t *testing.T) {
	reg := core.NewRegistry()
	cfg := core.DefaultConfig()
	cfg.Modules["mock_active"] = core.ModuleConfig{Enabled: true}
	cfg.Modules["mock_disabled"] = core.ModuleConfig{Enabled: false}

	item1 := core.CleanableItem{ID: "mock_active:1", Name: "item1", Path: "/tmp/cache", Size: 1024}
	item2 := core.CleanableItem{ID: "mock_disabled:2", Name: "item2", Path: "/tmp/old", Size: 2048}

	m1 := &mockModule{id: "mock_active", name: "Active", available: true, items: []core.CleanableItem{item1}}
	m2 := &mockModule{id: "mock_disabled", name: "Disabled", available: true, items: []core.CleanableItem{item2}}
	_ = reg.Register(m1)
	_ = reg.Register(m2)

	app := NewAppWithDependencies(reg, cfg)

	// ScanAll should only scan enabled modules
	results, err := app.ScanAll()
	if err != nil {
		t.Fatalf("ScanAll failed: %v", err)
	}
	if _, ok := results["mock_active"]; !ok {
		t.Error("expected mock_active in scan results")
	}
	if _, ok := results["mock_disabled"]; ok {
		t.Error("expected mock_disabled to be skipped in ScanAll")
	}

	res := results["mock_active"]
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}
	if res.Items[0].Metadata["module"] != "mock_active" {
		t.Errorf("expected module tag 'mock_active', got %s", res.Items[0].Metadata["module"])
	}
	if res.Items[0].Metadata["storage"] == "" {
		t.Error("expected storage metadata tag to be set")
	}

	// ScanModule specific
	modRes, err := app.ScanModule("mock_disabled")
	if err != nil {
		t.Fatalf("ScanModule failed: %v", err)
	}
	if len(modRes.Items) != 1 {
		t.Fatalf("expected 1 item from mock_disabled scan, got %d", len(modRes.Items))
	}

	// ScanModule non-existent
	_, err = app.ScanModule("unknown_mod")
	if err == nil {
		t.Error("expected error scanning non-existent module")
	}
}

func TestApp_CleanItems(t *testing.T) {
	reg := core.NewRegistry()
	cfg := core.DefaultConfig()

	m1 := &mockModule{id: "mock1", name: "Mock 1", available: true}
	snapperMod := &mockSnapperModule{
		mockModule: mockModule{id: "snapper", name: "Snapper", available: true},
	}

	_ = reg.Register(m1)
	_ = reg.Register(snapperMod)

	app := NewAppWithDependencies(reg, cfg)

	items := []core.CleanableItem{
		{ID: "mock1:item1", Name: "Item 1", Size: 500, Metadata: map[string]string{"module": "mock1"}},
		{ID: "mock1:item2", Name: "Item 2", Size: 1000}, // module inferred from ID prefix
	}

	// 1. Dry run clean with safeguard
	dryRes, err := app.CleanItems(items, true, true)
	if err != nil {
		t.Fatalf("CleanItems dryRun failed: %v", err)
	}
	if len(dryRes.CleanedItems) != 2 {
		t.Errorf("expected 2 cleaned items, got %d", len(dryRes.CleanedItems))
	}
	if dryRes.FreedBytes != 1500 {
		t.Errorf("expected 1500 freed bytes, got %d", dryRes.FreedBytes)
	}
	if snapperMod.snapshotCreated {
		t.Error("expected snapper snapshot to NOT be created during dry-run")
	}

	// 2. Real clean with safeguard
	cleanRes, err := app.CleanItems(items, false, true)
	if err != nil {
		t.Fatalf("CleanItems real clean failed: %v", err)
	}
	if len(cleanRes.CleanedItems) != 2 {
		t.Errorf("expected 2 cleaned items, got %d", len(cleanRes.CleanedItems))
	}
	if cleanRes.FreedBytes != 1500 {
		t.Errorf("expected 1500 freed bytes, got %d", cleanRes.FreedBytes)
	}
	if !snapperMod.snapshotCreated {
		t.Error("expected snapper snapshot to be created during real clean")
	}

	// 3. Clean empty items
	emptyRes, err := app.CleanItems(nil, false, false)
	if err != nil {
		t.Fatalf("CleanItems empty failed: %v", err)
	}
	if len(emptyRes.CleanedItems) != 0 {
		t.Errorf("expected 0 cleaned items, got %d", len(emptyRes.CleanedItems))
	}
}

func TestApp_CleanItems_Errors(t *testing.T) {
	reg := core.NewRegistry()
	cfg := core.DefaultConfig()
	mErr := &mockModule{
		id:        "err_mod",
		name:      "Err Mod",
		available: true,
		cleanErr:  errors.New("clean operation failed"),
	}
	_ = reg.Register(mErr)

	app := NewAppWithDependencies(reg, cfg)
	res, _ := app.CleanItems([]core.CleanableItem{{ID: "err_mod:1"}}, false, false)
	if len(res.Errors) == 0 {
		t.Error("expected clean error in res.Errors")
	}
}

func TestApp_CustomRule_Add_and_Remove(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	reg := core.NewRegistry()
	cfg := core.DefaultConfig()
	customMod := &mockModule{id: "custom", name: "Custom Rules", available: true}
	_ = reg.Register(customMod)

	app := NewAppWithDependencies(reg, cfg)

	// Add custom rule
	err := app.AddCustomRule("Test Rule", "/tmp/test", "*.log", true, "SAFE")
	if err != nil {
		t.Fatalf("AddCustomRule failed: %v", err)
	}

	retrievedCfg, err := app.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if len(retrievedCfg.CustomRules) != 1 {
		t.Fatalf("expected 1 custom rule, got %d", len(retrievedCfg.CustomRules))
	}
	rule := retrievedCfg.CustomRules[0]
	if rule.Name != "Test Rule" || rule.Pattern != "*.log" || !rule.Recursive || rule.Safety != core.SafetySafe {
		t.Errorf("rule fields mismatch: %+v", rule)
	}

	// Verify custom adapter received the updated config
	if customMod.cfg == nil || len(customMod.cfg.CustomRules) != 1 {
		t.Error("expected custom module to have updated config synced")
	}

	// Remove custom rule
	err = app.RemoveCustomRule(rule.ID)
	if err != nil {
		t.Fatalf("RemoveCustomRule failed: %v", err)
	}

	retrievedCfg, err = app.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if len(retrievedCfg.CustomRules) != 0 {
		t.Errorf("expected 0 custom rules after removal, got %d", len(retrievedCfg.CustomRules))
	}
}

func TestApp_UninitializedErrors(t *testing.T) {
	app := NewApp() // Uninitialized

	if _, err := app.GetModules(); err == nil {
		t.Error("expected error from GetModules on uninitialized app")
	}
	if _, err := app.ScanAll(); err == nil {
		t.Error("expected error from ScanAll on uninitialized app")
	}
	if _, err := app.ScanModule("any"); err == nil {
		t.Error("expected error from ScanModule on uninitialized app")
	}
	if _, err := app.CleanItems([]core.CleanableItem{{ID: "1"}}, false, false); err == nil {
		t.Error("expected error from CleanItems on uninitialized app")
	}
	if err := app.AddCustomRule("test", "/tmp", "", false, "SAFE"); err == nil {
		t.Error("expected error from AddCustomRule on uninitialized app")
	}
	if err := app.RemoveCustomRule("test"); err == nil {
		t.Error("expected error from RemoveCustomRule on uninitialized app")
	}
	if _, err := app.GetConfig(); err == nil {
		t.Error("expected error from GetConfig on uninitialized app")
	}
}

func TestApp_IPC_SecurityAudit_PathValidation(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	reg := core.NewRegistry()
	cfg := core.DefaultConfig()
	app := NewAppWithDependencies(reg, cfg)

	maliciousTestCases := []struct {
		name    string
		path    string
		wantErr string
	}{
		{
			name:    "path traversal with .. components",
			path:    "../../etc/shadow",
			wantErr: "directory traversal",
		},
		{
			name:    "path traversal in middle of path",
			path:    "/tmp/../etc/passwd",
			wantErr: "directory traversal",
		},
		{
			name:    "null byte injection",
			path:    "/tmp/test\x00payload",
			wantErr: "null character",
		},
		{
			name:    "system root directory",
			path:    "/",
			wantErr: "dangerous or system root",
		},
		{
			name:    "system etc directory",
			path:    "/etc",
			wantErr: "dangerous or system root",
		},
		{
			name:    "system boot directory",
			path:    "/boot",
			wantErr: "dangerous or system root",
		},
		{
			name:    "system dev directory",
			path:    "/dev",
			wantErr: "dangerous or system root",
		},
		{
			name:    "user home root",
			path:    "~",
			wantErr: "dangerous or system root",
		},
		{
			name:    "relative path without leading tilde or slash",
			path:    "relative/subdir/cache",
			wantErr: "absolute path or begin with '~'",
		},
	}

	for _, tc := range maliciousTestCases {
		t.Run(tc.name, func(t *testing.T) {
			err := app.AddCustomRule("Exploit Attempt", tc.path, "*", false, "SAFE")
			if err == nil {
				t.Fatalf("expected AddCustomRule to reject malicious path %q, but got nil", tc.path)
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.wantErr)) {
				t.Errorf("error %q does not contain expected substring %q", err.Error(), tc.wantErr)
			}
		})
	}

	// Verify valid paths succeed
	validTestCases := []string{
		"/tmp/valid_test_cache",
		"~/Downloads/old_builds",
	}
	for _, vp := range validTestCases {
		if err := app.AddCustomRule("Valid Rule", vp, "*.tmp", false, "SAFE"); err != nil {
			t.Errorf("expected valid path %q to succeed, got: %v", vp, err)
		}
	}
}

func TestApp_DualStorageTelemetryAndPartitionFilter(t *testing.T) {
	app := NewApp()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	app.Startup(ctx)
	defer app.Shutdown(ctx)

	storageInfo, err := app.GetStorageInfo()
	if err != nil {
		t.Fatalf("GetStorageInfo failed: %v", err)
	}

	expectedStorage, err := core.DetectSystemStorage()
	if err == nil {
		if storageInfo.Root.Path != expectedStorage.Root.Path {
			t.Errorf("expected root path %s, got %s", expectedStorage.Root.Path, storageInfo.Root.Path)
		}
		if storageInfo.HasSeparateHome != expectedStorage.HasSeparateHome {
			t.Errorf("expected hasSeparateHome %v, got %v", expectedStorage.HasSeparateHome, storageInfo.HasSeparateHome)
		}
	}

	// Test partition tagging and filtering
	items := []core.CleanableItem{
		{ID: "item-root", Path: "/var/cache/pacman/pkg/test.pkg.tar.zst", Size: 1024},
		{ID: "item-home", Path: "/home/user/.cache/npm/package.tgz", Size: 2048},
	}

	for i := range items {
		core.TagItemStorage(&items[i], storageInfo)
		items[i].Metadata["storage_target"] = items[i].Metadata["storage"]
	}

	if items[0].Metadata["storage_target"] != "root" || items[0].Metadata["storage"] != "root" {
		t.Errorf("expected item-root to have target 'root', got %s", items[0].Metadata["storage_target"])
	}

	if storageInfo.HasSeparateHome {
		if items[1].Metadata["storage_target"] != "home" || items[1].Metadata["storage"] != "home" {
			t.Errorf("expected item-home to have target 'home', got %s", items[1].Metadata["storage_target"])
		}
	}

	// Test partitioning filter logic
	filterByScope := func(itemList []core.CleanableItem, scope string) []core.CleanableItem {
		if scope == "all" {
			return itemList
		}
		var filtered []core.CleanableItem
		for _, it := range itemList {
			target := strings.ToLower(it.Metadata["storage_target"])
			if scope == "root" && (target == "root" || target == "") {
				filtered = append(filtered, it)
			} else if scope == "home" && target == "home" {
				filtered = append(filtered, it)
			}
		}
		return filtered
	}

	allItems := filterByScope(items, "all")
	if len(allItems) != 2 {
		t.Errorf("scope 'all' should return 2 items, got %d", len(allItems))
	}

	rootItems := filterByScope(items, "root")
	if len(rootItems) != 1 || rootItems[0].ID != "item-root" {
		t.Errorf("scope 'root' should return only item-root, got %v", rootItems)
	}

	if storageInfo.HasSeparateHome {
		homeItems := filterByScope(items, "home")
		if len(homeItems) != 1 || homeItems[0].ID != "item-home" {
			t.Errorf("scope 'home' should return only item-home, got %v", homeItems)
		}
	}
}

func TestApp_DryRun_NoAlterationsAndAccurateStats(t *testing.T) {
	tempDir := t.TempDir()
	f1 := filepath.Join(tempDir, "file1.cache")
	f2 := filepath.Join(tempDir, "file2.cache")

	content1 := []byte("cache file number 1 test data")
	content2 := []byte("cache file number 2 longer test data payload")
	if err := os.WriteFile(f1, content1, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, content2, 0644); err != nil {
		t.Fatal(err)
	}

	reg := core.NewRegistry()
	cfg := core.DefaultConfig()

	// Register real custom module
	customMod := custom.NewWithConfig(cfg)
	_ = reg.Register(customMod)

	app := NewAppWithDependencies(reg, cfg)

	items := []core.CleanableItem{
		{ID: "custom:1", Path: f1, Size: int64(len(content1)), Metadata: map[string]string{"module": "custom"}},
		{ID: "custom:2", Path: f2, Size: int64(len(content2)), Metadata: map[string]string{"module": "custom"}},
	}
	expectedFreed := int64(len(content1) + len(content2))

	// 1. Dry run execution
	dryRes, err := app.CleanItems(items, true, false)
	if err != nil {
		t.Fatalf("CleanItems dry-run failed: %v", err)
	}
	if dryRes.FreedBytes != expectedFreed {
		t.Errorf("expected dry-run freed bytes %d, got %d", expectedFreed, dryRes.FreedBytes)
	}
	if len(dryRes.CleanedItems) != 2 {
		t.Errorf("expected 2 cleaned items in dry-run, got %d", len(dryRes.CleanedItems))
	}

	// Verify NO filesystem alterations occurred during dry-run
	if _, err := os.Stat(f1); err != nil {
		t.Errorf("dry run altered filesystem: file %s missing: %v", f1, err)
	}
	if _, err := os.Stat(f2); err != nil {
		t.Errorf("dry run altered filesystem: file %s missing: %v", f2, err)
	}

	// 2. Real clean execution
	cleanRes, err := app.CleanItems(items, false, false)
	if err != nil {
		t.Fatalf("CleanItems real clean failed: %v", err)
	}
	if cleanRes.FreedBytes != expectedFreed {
		t.Errorf("expected real clean freed bytes %d, got %d", expectedFreed, cleanRes.FreedBytes)
	}
	if len(cleanRes.CleanedItems) != 2 {
		t.Errorf("expected 2 cleaned items in real clean, got %d", len(cleanRes.CleanedItems))
	}

	// Verify filesystem alterations DID occur after real clean
	if _, err := os.Stat(f1); !os.IsNotExist(err) {
		t.Errorf("file %s should have been removed after real clean", f1)
	}
	if _, err := os.Stat(f2); !os.IsNotExist(err) {
		t.Errorf("file %s should have been removed after real clean", f2)
	}
}
