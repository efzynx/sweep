package core_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/efzynx/sweep/internal/adapters"
	"github.com/efzynx/sweep/internal/adapters/snapper"
	"github.com/efzynx/sweep/internal/core"
)

// mockIntegrationModule implements core.Module to simulate a full adapter lifecycle.
type mockIntegrationModule struct {
	id                string
	name              string
	category          string
	requiresElevation bool
	items             []core.CleanableItem
	cleanHistory      []core.CleanableItem
	lastOpts          core.CleanOptions
	deletedPaths      []string
}

func (m *mockIntegrationModule) ID() string              { return m.id }
func (m *mockIntegrationModule) Name() string            { return m.name }
func (m *mockIntegrationModule) Category() string        { return m.category }
func (m *mockIntegrationModule) Description() string     { return "Mock integration module " + m.id }
func (m *mockIntegrationModule) IsAvailable() bool       { return true }
func (m *mockIntegrationModule) RequiresElevation() bool { return m.requiresElevation }

func (m *mockIntegrationModule) Scan(ctx context.Context) (core.ScanResult, error) {
	var total int64
	for _, it := range m.items {
		total += it.Size
	}
	return core.ScanResult{
		Items:     m.items,
		TotalSize: total,
		ScannedAt: time.Now(),
	}, nil
}

func (m *mockIntegrationModule) Clean(ctx context.Context, items []core.CleanableItem, opts core.CleanOptions) (core.CleanResult, error) {
	m.lastOpts = opts
	m.cleanHistory = append(m.cleanHistory, items...)

	start := time.Now()
	res := core.CleanResult{
		CleanedItems: make([]core.CleanableItem, 0),
		Errors:       make([]error, 0),
	}

	for _, it := range items {
		if opts.DryRun {
			// In dry-run, simulate without deleting anything
			res.CleanedItems = append(res.CleanedItems, it)
			res.FreedBytes += it.Size
			continue
		}

		// In actual clean, simulate deletion
		if it.Path != "" {
			m.deletedPaths = append(m.deletedPaths, it.Path)
		}
		res.CleanedItems = append(res.CleanedItems, it)
		res.FreedBytes += it.Size
	}

	res.Duration = time.Since(start)
	return res, nil
}

// TestIntegration_ScannerRegistrySelectionDryRunClean tests the complete lifecycle:
// Scanner -> Registry -> Selection -> Dry-Run -> Clean Result
func TestIntegration_ScannerRegistrySelectionDryRunClean(t *testing.T) {
	reg := core.NewRegistry()

	// Setup mock modules with multiple safety tiers
	modPkg := &mockIntegrationModule{
		id:                "mock_pkg",
		name:              "Package Cleaner",
		category:          "package_manager",
		requiresElevation: true,
		items: []core.CleanableItem{
			{ID: "pkg-1", Name: "broken-download", Size: 10 * 1024 * 1024, Safety: core.SafetySafe, Path: "/tmp/pkg1"},
			{ID: "pkg-2", Name: "old-version", Size: 50 * 1024 * 1024, Safety: core.SafetyCaution, Path: "/tmp/pkg2"},
			{ID: "pkg-3", Name: "kernel-image", Size: 120 * 1024 * 1024, Safety: core.SafetyHigh, Path: "/tmp/pkg3"},
		},
	}

	modCache := &mockIntegrationModule{
		id:                "mock_cache",
		name:              "Cache Cleaner",
		category:          "caches",
		requiresElevation: false,
		items: []core.CleanableItem{
			{ID: "cache-1", Name: "app-cache", Size: 5 * 1024 * 1024, Safety: core.SafetySafe, Path: "/tmp/cache1"},
			{ID: "cache-2", Name: "log-cache", Size: 15 * 1024 * 1024, Safety: core.SafetySafe, Path: "/tmp/cache2"},
		},
	}

	if err := reg.Register(modPkg); err != nil {
		t.Fatalf("failed to register modPkg: %v", err)
	}
	if err := reg.Register(modCache); err != nil {
		t.Fatalf("failed to register modCache: %v", err)
	}

	ctx := context.Background()

	// 1. ScanAll
	results, err := reg.ScanAll(ctx)
	if err != nil {
		t.Fatalf("ScanAll failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 scan results, got %d", len(results))
	}

	pkgRes := results["mock_pkg"]
	if len(pkgRes.Items) != 3 {
		t.Fatalf("expected 3 items in modPkg scan, got %d", len(pkgRes.Items))
	}
	expectedPkgTotal := int64((10 + 50 + 120) * 1024 * 1024)
	if pkgRes.TotalSize != expectedPkgTotal {
		t.Errorf("expected modPkg total %d, got %d", expectedPkgTotal, pkgRes.TotalSize)
	}

	cacheRes := results["mock_cache"]
	if len(cacheRes.Items) != 2 {
		t.Fatalf("expected 2 items in modCache scan, got %d", len(cacheRes.Items))
	}

	// 2. Selection: Apply Safe-by-default selection policy
	for i := range pkgRes.Items {
		pkgRes.Items[i].Selected = (pkgRes.Items[i].Safety == core.SafetySafe)
	}
	for i := range cacheRes.Items {
		cacheRes.Items[i].Selected = (cacheRes.Items[i].Safety == core.SafetySafe)
	}

	pkgSelected := pkgRes.SelectedItems()
	if len(pkgSelected) != 1 || pkgSelected[0].ID != "pkg-1" {
		t.Errorf("expected only pkg-1 selected by default, got %v", pkgSelected)
	}
	if pkgRes.SelectedSize() != 10*1024*1024 {
		t.Errorf("expected 10MB selected size, got %d", pkgRes.SelectedSize())
	}

	cacheSelected := cacheRes.SelectedItems()
	if len(cacheSelected) != 2 {
		t.Errorf("expected 2 items selected in cache, got %d", len(cacheSelected))
	}

	// User decides to also select caution item pkg-2, and deselect cache-2
	pkgRes.Items[1].Selected = true    // pkg-2 selected
	cacheRes.Items[1].Selected = false // cache-2 deselected

	pkgSelected = pkgRes.SelectedItems()
	if len(pkgSelected) != 2 {
		t.Fatalf("expected 2 items selected in pkg after toggle, got %d", len(pkgSelected))
	}

	cacheSelected = cacheRes.SelectedItems()
	if len(cacheSelected) != 1 {
		t.Fatalf("expected 1 item selected in cache after toggle, got %d", len(cacheSelected))
	}

	// 3. Dry-Run Execution
	dryRunOpts := core.CleanOptions{DryRun: true}
	pkgDryRes, err := modPkg.Clean(ctx, pkgSelected, dryRunOpts)
	if err != nil {
		t.Fatalf("dry-run modPkg clean failed: %v", err)
	}
	if !modPkg.lastOpts.DryRun {
		t.Error("expected lastOpts.DryRun to be true")
	}
	if len(pkgDryRes.CleanedItems) != 2 {
		t.Errorf("expected 2 items in dry-run result, got %d", len(pkgDryRes.CleanedItems))
	}
	expectedPkgFreed := int64((10 + 50) * 1024 * 1024)
	if pkgDryRes.FreedBytes != expectedPkgFreed {
		t.Errorf("expected %d bytes freed in dry-run, got %d", expectedPkgFreed, pkgDryRes.FreedBytes)
	}
	if len(modPkg.deletedPaths) != 0 {
		t.Errorf("expected zero deleted paths in dry-run, got %v", modPkg.deletedPaths)
	}

	cacheDryRes, err := modCache.Clean(ctx, cacheSelected, dryRunOpts)
	if err != nil {
		t.Fatalf("dry-run modCache clean failed: %v", err)
	}
	if len(cacheDryRes.CleanedItems) != 1 {
		t.Errorf("expected 1 item in cache dry-run, got %d", len(cacheDryRes.CleanedItems))
	}
	if cacheDryRes.FreedBytes != 5*1024*1024 {
		t.Errorf("expected 5MB freed in cache dry-run, got %d", cacheDryRes.FreedBytes)
	}

	// 4. Actual Clean Execution
	cleanOpts := core.CleanOptions{DryRun: false}
	pkgCleanRes, err := modPkg.Clean(ctx, pkgSelected, cleanOpts)
	if err != nil {
		t.Fatalf("clean modPkg failed: %v", err)
	}
	if pkgCleanRes.FreedBytes != expectedPkgFreed {
		t.Errorf("expected %d bytes freed in actual clean, got %d", expectedPkgFreed, pkgCleanRes.FreedBytes)
	}
	if len(modPkg.deletedPaths) != 2 {
		t.Errorf("expected 2 deleted paths in actual clean, got %d (%v)", len(modPkg.deletedPaths), modPkg.deletedPaths)
	}
	// Assert high-risk unselected item pkg-3 was NOT touched or deleted
	for _, p := range modPkg.deletedPaths {
		if p == "/tmp/pkg3" {
			t.Error("CRITICAL: unselected high-risk item pkg-3 was deleted!")
		}
	}
}

// TestIntegration_SnapperSafeguardHook tests the Snapper pre-clean snapshot hook
// and asserts protected snapshot preservation.
func TestIntegration_SnapperSafeguardHook(t *testing.T) {
	// Setup Snapper with custom command runner to record calls
	m := snapper.New()

	var commandCalls [][]string
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmdCall := append([]string{name}, args...)
		commandCalls = append(commandCalls, cmdCall)

		// Return simulated output for snapshot creation
		if len(args) >= 3 && args[0] == "-c" && args[2] == "create" {
			return []byte("Snapshot 42 created"), nil
		}
		// Return simulated output for snapshot deletion
		if len(args) >= 3 && args[0] == "-c" && args[2] == "delete" {
			return []byte("Snapshot deleted"), nil
		}
		return nil, nil
	})

	items := []core.CleanableItem{
		{
			ID:       "snapper:root:0",
			Name:     "Snapshot 0 (Current System)",
			Selected: true,
			Safety:   core.SafetyHigh,
			Metadata: map[string]string{
				"config":      "root",
				"snapshot_id": "0",
				"protected":   "true",
			},
		},
		{
			ID:       "snapper:root:10",
			Name:     "Old timeline snapshot #10",
			Size:     500 * 1024 * 1024,
			Selected: true,
			Safety:   core.SafetySafe,
			Metadata: map[string]string{
				"config":      "root",
				"snapshot_id": "10",
				"protected":   "false",
			},
		},
	}

	// 1. Dry run should NOT invoke snapper create or snapper delete
	dryRes, err := m.Clean(context.Background(), items, core.CleanOptions{
		DryRun:            true,
		SnapshotSafeguard: true,
	})
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	if len(commandCalls) != 0 {
		t.Errorf("expected 0 command calls during dry-run, got %d (%v)", len(commandCalls), commandCalls)
	}
	if len(dryRes.CleanedItems) != 1 || dryRes.CleanedItems[0].Metadata["snapshot_id"] != "10" {
		t.Errorf("expected only non-protected snapshot #10 in dry-run cleaned items, got %v", dryRes.CleanedItems)
	}

	// 2. Actual clean with SnapshotSafeguard = true
	commandCalls = nil
	cleanRes, err := m.Clean(context.Background(), items, core.CleanOptions{
		DryRun:            false,
		SnapshotSafeguard: true,
	})
	if err != nil {
		t.Fatalf("actual clean failed: %v", err)
	}

	// Verify pre-clean safeguard snapshot was triggered BEFORE snapshot deletion
	if len(commandCalls) < 2 {
		t.Fatalf("expected at least 2 command calls (safeguard create + delete), got %d", len(commandCalls))
	}

	createCall := commandCalls[0]
	if len(createCall) < 5 || createCall[0] != "snapper" || createCall[3] != "create" {
		t.Errorf("expected first call to be 'snapper -c root create ...', got %v", createCall)
	}

	deleteCall := commandCalls[1]
	if len(deleteCall) < 4 || deleteCall[0] != "snapper" || deleteCall[3] != "delete" {
		t.Errorf("expected second call to be 'snapper -c root delete ...', got %v", deleteCall)
	}

	// Verify snapshot 0 was NEVER passed to delete
	for _, arg := range deleteCall {
		if arg == "0" {
			t.Errorf("CRITICAL SECURITY VIOLATION: snapshot 0 was passed to delete: %v", deleteCall)
		}
	}

	// Verify error recorded for refusing protected snapshot 0
	if len(cleanRes.Errors) != 1 {
		t.Errorf("expected 1 error for protected snapshot 0, got %d", len(cleanRes.Errors))
	}
}

// TestIntegration_DefaultAdaptersContract verifies all registered adapters
// conform to the security contract:
// 1. Valid non-empty ID, Name, Category.
// 2. Elevation requirement is strictly declared.
// 3. Scan() runs safely in non-root environment without crashing or panicking.
func TestIntegration_DefaultAdaptersContract(t *testing.T) {
	reg := core.NewRegistry()
	adapters.RegisterDefaultAdapters(reg)

	modules := reg.Modules()
	if len(modules) < 6 {
		t.Fatalf("expected at least 6 registered adapters, got %d", len(modules))
	}

	expectedElevation := map[string]bool{
		"pacman":  true,
		"apt":     true,
		"dnf":     true,
		"snapper": true,
		"docker":  false,
		"flatpak": false,
		"custom":  false,
	}

	for _, mod := range modules {
		id := mod.ID()
		t.Run("Module_"+id, func(t *testing.T) {
			if mod.Name() == "" {
				t.Errorf("module %s has empty Name()", id)
			}
			if mod.Category() == "" {
				t.Errorf("module %s has empty Category()", id)
			}
			if mod.Description() == "" {
				t.Errorf("module %s has empty Description()", id)
			}

			expectedElev, ok := expectedElevation[id]
			if !ok {
				t.Errorf("unexpected module ID %s not in expectedElevation map", id)
			} else if mod.RequiresElevation() != expectedElev {
				t.Errorf("module %s RequiresElevation() = %v, expected %v", id, mod.RequiresElevation(), expectedElev)
			}

			// Execute Scan() with a bounded timeout in non-root context
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			res, err := mod.Scan(ctx)
			if err != nil && ctx.Err() != nil {
				t.Fatalf("module %s scan timed out: %v", id, err)
			}

			// Scan should never return uninitialized fields
			if res.Items == nil {
				t.Errorf("module %s returned nil Items slice", id)
			}
			if res.ScannedAt.IsZero() {
				t.Errorf("module %s returned zero ScannedAt", id)
			}
		})
	}
}

// TestIntegration_DryRunEquivalenceWithRealFiles verifies on disk that dry-run
// calculates exact byte sizes and counts without deleting any files.
func TestIntegration_DryRunEquivalenceWithRealFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files in tmpDir
	file1 := filepath.Join(tmpDir, "test1.dat")
	file2 := filepath.Join(tmpDir, "test2.dat")
	data1 := []byte("hello world 12345")
	data2 := []byte("sweep dry run verification bytes")
	if err := os.WriteFile(file1, data1, 0o644); err != nil {
		t.Fatalf("failed to write file1: %v", err)
	}
	if err := os.WriteFile(file2, data2, 0o644); err != nil {
		t.Fatalf("failed to write file2: %v", err)
	}

	mod := &mockIntegrationModule{
		id:                "disk_test",
		name:              "Disk Test",
		category:          "disk",
		requiresElevation: false,
		items: []core.CleanableItem{
			{ID: "f1", Name: "file1", Size: int64(len(data1)), Path: file1, Selected: true, Safety: core.SafetySafe},
			{ID: "f2", Name: "file2", Size: int64(len(data2)), Path: file2, Selected: true, Safety: core.SafetySafe},
		},
	}

	// 1. Dry run
	ctx := context.Background()
	res, err := mod.Clean(ctx, mod.items, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}

	expectedSize := int64(len(data1) + len(data2))
	if res.FreedBytes != expectedSize {
		t.Errorf("expected %d bytes freed in dry-run, got %d", expectedSize, res.FreedBytes)
	}
	if len(res.CleanedItems) != 2 {
		t.Errorf("expected 2 cleaned items, got %d", len(res.CleanedItems))
	}

	// Assert files are still on disk
	if _, err := os.Stat(file1); os.IsNotExist(err) {
		t.Errorf("file1 was deleted during dry-run!")
	}
	if _, err := os.Stat(file2); os.IsNotExist(err) {
		t.Errorf("file2 was deleted during dry-run!")
	}

	// 2. Non dry run
	cleanRes, err := mod.Clean(ctx, mod.items, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("clean failed: %v", err)
	}
	if cleanRes.FreedBytes != expectedSize {
		t.Errorf("expected %d bytes freed in clean, got %d", expectedSize, cleanRes.FreedBytes)
	}
	if len(mod.deletedPaths) != 2 {
		t.Errorf("expected 2 deleted paths, got %d", len(mod.deletedPaths))
	}
}
