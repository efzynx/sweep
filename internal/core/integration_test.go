package core_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/efzynx/sweep/internal/adapters"
	"github.com/efzynx/sweep/internal/adapters/apt"
	"github.com/efzynx/sweep/internal/adapters/devcache"
	"github.com/efzynx/sweep/internal/adapters/dnf"
	"github.com/efzynx/sweep/internal/adapters/journal"
	"github.com/efzynx/sweep/internal/adapters/snap"
	"github.com/efzynx/sweep/internal/adapters/snapper"
	"github.com/efzynx/sweep/internal/adapters/usercache"
	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/core/distro"
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
		"pacman":    true,
		"apt":       true,
		"dnf":       true,
		"snapper":   true,
		"docker":    false,
		"flatpak":   false,
		"custom":    false,
		"devcache":  false,
		"snap":      true,
		"usercache": false,
		"journal":   true,
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
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

// TestIntegration_Phase2_Devcache_DryRunAndCleanEquivalence verifies devcache
// handles user caches safely: zero elevation requirement, dry-run equivalence,
// read-only chmod handling, and preserving parent cache directories upon clean.
func TestIntegration_Phase2_Devcache_DryRunAndCleanEquivalence(t *testing.T) {
	tmpHome := t.TempDir()

	m := devcache.New()
	m.SetHomeDir(tmpHome)

	// Contract check
	if m.RequiresElevation() {
		t.Errorf("devcache must not require elevation")
	}

	// Populate targets
	targets := []struct {
		relDir   string
		file     string
		content  []byte
		readOnly bool
	}{
		{filepath.Join(".npm", "_cacache"), "npm-cache-1.tgz", []byte("sample npm content"), false},
		{filepath.Join(".cache", "go-build"), "b001.a", []byte("go build archive bytes"), false},
		{filepath.Join("go", "pkg", "mod", "cache"), "mod-v1.zip", []byte("go mod download zip"), true},
		{filepath.Join(".cargo", "registry", "cache"), "pkg-0.1.crate", []byte("cargo crate archive"), false},
		{filepath.Join(".cache", "pip"), "pip-sample.whl", []byte("python wheel pip"), false},
		{filepath.Join(".cache", "uv"), "uv-sample.whl", []byte("uv fast wheel archive"), false},
	}

	var expectedTotalSize int64
	for _, tgt := range targets {
		dir := filepath.Join(tmpHome, tgt.relDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
		filePath := filepath.Join(dir, tgt.file)
		if err := os.WriteFile(filePath, tgt.content, 0o644); err != nil {
			t.Fatalf("failed to write file %s: %v", filePath, err)
		}
		if tgt.readOnly {
			_ = os.Chmod(filePath, 0o444)
			_ = os.Chmod(dir, 0o555) // read-only directory
		}
		expectedTotalSize += int64(len(tgt.content))
	}

	ctx := context.Background()

	// 1. Scan
	scanRes, err := m.Scan(ctx)
	if err != nil {
		t.Fatalf("devcache Scan failed: %v", err)
	}
	if len(scanRes.Items) != len(targets) {
		t.Fatalf("expected %d scanned items, got %d", len(targets), len(scanRes.Items))
	}
	if scanRes.TotalSize != expectedTotalSize {
		t.Errorf("expected scanned total %d, got %d", expectedTotalSize, scanRes.TotalSize)
	}

	// 2. Dry-Run simulation: verify 0 deletions on disk
	dryRes, err := m.Clean(ctx, scanRes.Items, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("devcache dry-run Clean failed: %v", err)
	}
	if dryRes.FreedBytes != expectedTotalSize {
		t.Errorf("expected dry-run freed %d, got %d", expectedTotalSize, dryRes.FreedBytes)
	}
	if len(dryRes.CleanedItems) != len(targets) {
		t.Errorf("expected %d dry-run cleaned items, got %d", len(targets), len(dryRes.CleanedItems))
	}

	// Assert files are still on disk
	for _, tgt := range targets {
		p := filepath.Join(tmpHome, tgt.relDir, tgt.file)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Errorf("CRITICAL: file %s was deleted during dry-run!", p)
		}
	}

	// 3. Real Clean: verify files removed, but parent cache directories preserved
	cleanRes, err := m.Clean(ctx, scanRes.Items, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("devcache real Clean failed: %v", err)
	}
	if cleanRes.FreedBytes != expectedTotalSize {
		t.Errorf("expected real clean freed %d, got %d", expectedTotalSize, cleanRes.FreedBytes)
	}
	if len(cleanRes.Errors) > 0 {
		t.Errorf("devcache clean encountered errors: %v", cleanRes.Errors)
	}

	for _, tgt := range targets {
		p := filepath.Join(tmpHome, tgt.relDir, tgt.file)
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("expected file %s to be unlinked after clean", p)
		}
		// Base directory must still exist!
		dir := filepath.Join(tmpHome, tgt.relDir)
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			t.Errorf("expected base directory %s to be preserved after clean", dir)
		}
	}
}

// TestIntegration_Phase2_Usercache_BrowserProfilesAndTrashPreserved verifies:
//  1. usercache does not require elevation.
//  2. Dry-run deletes zero files.
//  3. Clean removes trash, thumbnails, and browser HTTP cache files.
//  4. CRITICAL: Browser user profile data (places.sqlite, key4.db, prefs.js, Bookmarks, Cookies)
//     is NEVER deleted or corrupted.
//  5. Active Firefox lock skips cleaning.
func TestIntegration_Phase2_Usercache_BrowserProfilesAndTrashPreserved(t *testing.T) {
	tmpHome := t.TempDir()
	m := usercache.NewWithHome(tmpHome)

	if m.RequiresElevation() {
		t.Errorf("usercache must not require elevation")
	}

	// Setup Trash
	trashFilesDir := filepath.Join(tmpHome, ".local", "share", "Trash", "files")
	trashInfoDir := filepath.Join(tmpHome, ".local", "share", "Trash", "info")
	_ = os.MkdirAll(trashFilesDir, 0o755)
	_ = os.MkdirAll(trashInfoDir, 0o755)
	trashFile := filepath.Join(trashFilesDir, "unwanted.pdf")
	_ = os.WriteFile(trashFile, []byte("unwanted trash content"), 0o644)
	_ = os.WriteFile(filepath.Join(trashInfoDir, "unwanted.pdf.trashinfo"), []byte("[Trash Info]"), 0o644)

	// Setup Thumbnails
	thumbDir := filepath.Join(tmpHome, ".cache", "thumbnails", "normal")
	_ = os.MkdirAll(thumbDir, 0o755)
	thumbFile := filepath.Join(thumbDir, "preview.png")
	_ = os.WriteFile(thumbFile, []byte("png thumbnail preview"), 0o644)

	// Setup Firefox Profile (Config / DB vs Cache)
	ffProfileDir := filepath.Join(tmpHome, ".mozilla", "firefox", "default.release")
	_ = os.MkdirAll(ffProfileDir, 0o755)
	placesDB := filepath.Join(ffProfileDir, "places.sqlite")
	keyDB := filepath.Join(ffProfileDir, "key4.db")
	prefsJS := filepath.Join(ffProfileDir, "prefs.js")
	_ = os.WriteFile(placesDB, []byte("CRITICAL BROWSER BOOKMARKS AND HISTORY"), 0o644)
	_ = os.WriteFile(keyDB, []byte("CRITICAL PASSWORDS KEY"), 0o644)
	_ = os.WriteFile(prefsJS, []byte("user_pref('browser.startup', 1);"), 0o644)

	ffCacheDir := filepath.Join(tmpHome, ".cache", "mozilla", "firefox", "default.release", "cache2", "entries")
	_ = os.MkdirAll(ffCacheDir, 0o755)
	ffCacheFile := filepath.Join(ffCacheDir, "cached_http_blob_1")
	_ = os.WriteFile(ffCacheFile, []byte("cached http web data"), 0o644)

	// Setup Chromium Profile (Config vs Cache)
	chromConfigDir := filepath.Join(tmpHome, ".config", "chromium", "Default")
	_ = os.MkdirAll(chromConfigDir, 0o755)
	chromBookmarks := filepath.Join(chromConfigDir, "Bookmarks")
	chromCookies := filepath.Join(chromConfigDir, "Cookies")
	_ = os.WriteFile(chromBookmarks, []byte("CRITICAL CHROMIUM BOOKMARKS JSON"), 0o644)
	_ = os.WriteFile(chromCookies, []byte("CRITICAL CHROMIUM SESSION COOKIES"), 0o644)

	chromCacheDir := filepath.Join(tmpHome, ".cache", "chromium", "Default", "Cache")
	_ = os.MkdirAll(chromCacheDir, 0o755)
	chromCacheFile := filepath.Join(chromCacheDir, "data_0")
	_ = os.WriteFile(chromCacheFile, []byte("cached chromium web data"), 0o644)

	ctx := context.Background()

	// 1. Scan
	scanRes, err := m.Scan(ctx)
	if err != nil {
		t.Fatalf("usercache Scan failed: %v", err)
	}

	// Verify that NO scanned item targets browser user profile directories or databases
	for _, it := range scanRes.Items {
		if strings.Contains(it.Path, "places.sqlite") ||
			strings.Contains(it.Path, "key4.db") ||
			strings.Contains(it.Path, "prefs.js") ||
			strings.Contains(it.Path, "Bookmarks") ||
			strings.Contains(it.Path, "Cookies") {
			t.Fatalf("CRITICAL SECURITY VIOLATION: user profile database included in scan items: %s", it.Path)
		}
	}

	// 2. Dry-Run simulation: verify 0 deletions
	dryRes, err := m.Clean(ctx, scanRes.Items, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	if dryRes.FreedBytes != scanRes.TotalSize {
		t.Errorf("expected dry-run freed %d, got %d", scanRes.TotalSize, dryRes.FreedBytes)
	}

	// All files must be untouched
	for _, f := range []string{trashFile, thumbFile, ffCacheFile, chromCacheFile, placesDB, keyDB, prefsJS, chromBookmarks, chromCookies} {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			t.Errorf("file %s was unlinked during dry run!", f)
		}
	}

	// 3. Real Clean
	cleanRes, err := m.Clean(ctx, scanRes.Items, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("clean failed: %v", err)
	}
	if len(cleanRes.Errors) > 0 {
		t.Errorf("clean errors: %v", cleanRes.Errors)
	}

	// Cache & trash files must be deleted
	if _, err := os.Stat(trashFile); !os.IsNotExist(err) {
		t.Errorf("expected trash file %s to be deleted", trashFile)
	}
	if _, err := os.Stat(ffCacheFile); !os.IsNotExist(err) {
		t.Errorf("expected firefox cache file %s to be deleted", ffCacheFile)
	}
	if _, err := os.Stat(chromCacheFile); !os.IsNotExist(err) {
		t.Errorf("expected chromium cache file %s to be deleted", chromCacheFile)
	}

	// CRITICAL VERIFICATION: User profile database files MUST STILL EXIST!
	for _, profFile := range []string{placesDB, keyDB, prefsJS, chromBookmarks, chromCookies} {
		data, err := os.ReadFile(profFile)
		if err != nil {
			t.Fatalf("CRITICAL DATA LOSS: user profile file %s was deleted or damaged: %v", profFile, err)
		}
		if len(data) == 0 {
			t.Fatalf("CRITICAL DATA LOSS: user profile file %s was truncated to 0 bytes", profFile)
		}
	}

	// 4. Test Lockfile Protection
	// Recreate a cache file
	_ = os.MkdirAll(ffCacheDir, 0o755)
	_ = os.WriteFile(ffCacheFile, []byte("new cache blob"), 0o644)
	// Create .parentlock in firefox profile
	lockFile := filepath.Join(ffProfileDir, ".parentlock")
	_ = os.WriteFile(lockFile, []byte("lock"), 0o644)

	lockScan, _ := m.Scan(ctx)
	lockClean, _ := m.Clean(ctx, lockScan.Items, core.CleanOptions{DryRun: false})
	_ = lockClean

	// Because locked, ffCacheFile must NOT be deleted!
	if _, err := os.Stat(ffCacheFile); os.IsNotExist(err) {
		t.Errorf("CRITICAL: locked Firefox profile cache was cleaned while lockfile was active!")
	}
}

// TestIntegration_Phase2_Journal_ElevationAndParameterizedClean verifies:
// 1. journal module requires elevation.
// 2. Command calls are strictly parameterized without shell.
// 3. Dry-run does not invoke vacuum command.
// 4. Real clean invokes journalctl with exact parameterized flags.
func TestIntegration_Phase2_Journal_ElevationAndParameterizedClean(t *testing.T) {
	m := journal.New()

	if !m.RequiresElevation() {
		t.Errorf("journal module must require elevation")
	}

	tmpLogDir := t.TempDir()
	archivedJournal := filepath.Join(tmpLogDir, "system@0000000000000000-0000000000000000.journal")
	_ = os.WriteFile(archivedJournal, make([]byte, 5*1024*1024), 0o644)
	// Set mtime to 14 days ago
	oldTime := time.Now().AddDate(0, 0, -14)
	_ = os.Chtimes(archivedJournal, oldTime, oldTime)

	m.SetJournalDirs([]string{tmpLogDir})
	m.SetSystemdRunning(func() bool { return true })

	var commandCalls [][]string
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		call := append([]string{name}, args...)
		commandCalls = append(commandCalls, call)
		if len(args) > 0 && args[0] == "--disk-usage" {
			return []byte("Archived and active journals take up 50.0M in the file system."), nil
		}
		if len(args) > 0 && strings.HasPrefix(args[0], "--vacuum-time=") {
			return []byte("Vacuuming done, freed 5.0M of archived journals."), nil
		}
		return nil, nil
	})

	ctx := context.Background()

	// 1. Scan
	scanRes, err := m.Scan(ctx)
	if err != nil {
		t.Fatalf("journal Scan failed: %v", err)
	}
	if len(scanRes.Items) != 1 {
		t.Fatalf("expected 1 vacuumable item, got %d", len(scanRes.Items))
	}
	if scanRes.Items[0].ID != "journal:vacuum" {
		t.Errorf("expected item ID 'journal:vacuum', got %s", scanRes.Items[0].ID)
	}

	// 2. Dry-Run
	commandCalls = nil
	dryRes, err := m.Clean(ctx, scanRes.Items, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("journal dry-run failed: %v", err)
	}
	if len(commandCalls) != 0 {
		t.Errorf("expected 0 commands executed in dry-run, got %d (%v)", len(commandCalls), commandCalls)
	}
	if dryRes.FreedBytes != scanRes.Items[0].Size {
		t.Errorf("expected dry-run freed %d, got %d", scanRes.Items[0].Size, dryRes.FreedBytes)
	}

	// 3. Real Clean
	commandCalls = nil
	cleanRes, err := m.Clean(ctx, scanRes.Items, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("journal Clean failed: %v", err)
	}
	if len(commandCalls) != 1 {
		t.Fatalf("expected 1 command call, got %d (%v)", len(commandCalls), commandCalls)
	}
	vacuumCall := commandCalls[0]
	if vacuumCall[0] != "journalctl" {
		t.Errorf("expected binary journalctl, got %s", vacuumCall[0])
	}
	// Assert parameterized args
	hasVacuumTime := false
	hasVacuumSize := false
	for _, arg := range vacuumCall[1:] {
		if strings.HasPrefix(arg, "--vacuum-time=") {
			hasVacuumTime = true
		}
		if strings.HasPrefix(arg, "--vacuum-size=") {
			hasVacuumSize = true
		}
	}
	if !hasVacuumTime || !hasVacuumSize {
		t.Errorf("expected parameterized vacuum flags, got %v", vacuumCall)
	}
	if cleanRes.FreedBytes <= 0 {
		t.Errorf("expected positive freed bytes, got %d", cleanRes.FreedBytes)
	}
}

// TestIntegration_Phase2_Snap_ElevationAndParameterizedClean verifies:
// 1. snap module requires elevation.
// 2. Dry-run does not invoke removal commands.
// 3. Real clean executes parameterized `snap remove <name> --revision=<rev>`.
func TestIntegration_Phase2_Snap_ElevationAndParameterizedClean(t *testing.T) {
	distro.SetLookPathMock(func(binary string) (string, error) {
		if binary == "snap" {
			return "/usr/bin/snap", nil
		}
		return "", os.ErrNotExist
	})
	defer distro.ResetLookPathMock()

	m := snap.New()

	if !m.RequiresElevation() {
		t.Errorf("snap module must require elevation")
	}

	tmpSnapsDir := t.TempDir()
	m.SetSnapsDir(tmpSnapsDir)

	mockSnapFile := filepath.Join(tmpSnapsDir, "core20_1822.snap")
	_ = os.WriteFile(mockSnapFile, []byte(strings.Repeat("S", 20*1024*1024)), 0o644)

	var commandCalls [][]string
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		commandCalls = append(commandCalls, append([]string{name}, args...))
		if len(args) >= 2 && args[0] == "list" && args[1] == "--all" {
			out := "Name    Version    Rev    Tracking  Publisher  Notes\n" +
				"core20  20230503   1822   latest/stable canonical*  disabled\n" +
				"core20  20230622   1974   latest/stable canonical*  base\n"
			return []byte(out), nil
		}
		return []byte("snap removed"), nil
	})

	ctx := context.Background()

	// 1. Scan
	scanRes, err := m.Scan(ctx)
	if err != nil {
		t.Fatalf("snap Scan failed: %v", err)
	}
	if len(scanRes.Items) != 1 {
		t.Fatalf("expected 1 disabled snap item, got %d", len(scanRes.Items))
	}
	item := scanRes.Items[0]
	if item.Metadata["name"] != "core20" || item.Metadata["revision"] != "1822" {
		t.Errorf("unexpected snap item metadata: %v", item.Metadata)
	}

	// 2. Dry-Run
	commandCalls = nil
	dryRes, err := m.Clean(ctx, scanRes.Items, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	if len(commandCalls) != 0 {
		t.Errorf("expected 0 commands in dry-run, got %d (%v)", len(commandCalls), commandCalls)
	}
	if dryRes.FreedBytes != item.Size {
		t.Errorf("expected dry-run freed %d, got %d", item.Size, dryRes.FreedBytes)
	}

	// 3. Real Clean
	commandCalls = nil
	cleanRes, err := m.Clean(ctx, scanRes.Items, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("snap Clean failed: %v", err)
	}
	if len(commandCalls) != 1 {
		t.Fatalf("expected 1 snap remove command, got %d (%v)", len(commandCalls), commandCalls)
	}
	expectedCmd := []string{"snap", "remove", "core20", "--revision=1822"}
	cmd := commandCalls[0]
	if len(cmd) != len(expectedCmd) || cmd[0] != expectedCmd[0] || cmd[1] != expectedCmd[1] || cmd[2] != expectedCmd[2] || cmd[3] != expectedCmd[3] {
		t.Errorf("expected parameterized command %v, got %v", expectedCmd, cmd)
	}
	if cleanRes.FreedBytes != item.Size {
		t.Errorf("expected %d bytes freed, got %d", item.Size, cleanRes.FreedBytes)
	}
}

// TestIntegration_Phase2_AptAndDnf_ElevationAndParameterizedExecution verifies:
// apt and dnf declare RequiresElevation == true, parameterize all commands,
// and never execute destructive commands in dry-run.
func TestIntegration_Phase2_AptAndDnf_ElevationAndParameterizedExecution(t *testing.T) {
	aptMod := apt.New()
	dnfMod := dnf.New()

	if !aptMod.RequiresElevation() {
		t.Errorf("apt module must require elevation")
	}
	if !dnfMod.RequiresElevation() {
		t.Errorf("dnf module must require elevation")
	}

	// Apt dry-run & parameterized test
	var aptCalls [][]string
	aptMod.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		aptCalls = append(aptCalls, append([]string{name}, args...))
		return []byte("ok"), nil
	})

	items := []core.CleanableItem{
		{ID: "apt-archives", Name: "APT Archives", Size: 1024, Selected: true, Safety: core.SafetySafe, Metadata: map[string]string{"type": "archives"}},
		{ID: "apt-orphan:libunused", Name: "libunused", Size: 2048, Selected: true, Safety: core.SafetySafe, Metadata: map[string]string{"type": "orphan", "package": "libunused"}},
	}

	// Apt Dry-Run
	ctx := context.Background()
	dryRes, err := aptMod.Clean(ctx, items, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("apt dry-run failed: %v", err)
	}
	if len(aptCalls) != 0 {
		t.Errorf("expected 0 apt commands in dry-run, got %d (%v)", len(aptCalls), aptCalls)
	}
	if dryRes.FreedBytes != 3072 {
		t.Errorf("expected 3072 bytes freed in dry-run, got %d", dryRes.FreedBytes)
	}

	// Apt Clean
	aptCalls = nil
	_, err = aptMod.Clean(ctx, items, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("apt clean failed: %v", err)
	}
	if len(aptCalls) != 3 {
		t.Fatalf("expected 3 apt calls (clean, autoclean, autoremove), got %d (%v)", len(aptCalls), aptCalls)
	}
	// Verify commands are parameterized
	if aptCalls[0][0] != "apt-get" || aptCalls[0][1] != "clean" {
		t.Errorf("expected 'apt-get clean', got %v", aptCalls[0])
	}
	if aptCalls[1][0] != "apt-get" || aptCalls[1][1] != "autoclean" {
		t.Errorf("expected 'apt-get autoclean', got %v", aptCalls[1])
	}
	if aptCalls[2][0] != "apt-get" || aptCalls[2][1] != "autoremove" || aptCalls[2][2] != "-y" {
		t.Errorf("expected 'apt-get autoremove -y', got %v", aptCalls[2])
	}

	// DNF Dry-Run & Clean
	var dnfCalls [][]string
	dnfMod.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		dnfCalls = append(dnfCalls, append([]string{name}, args...))
		return []byte("ok"), nil
	})

	dnfItems := []core.CleanableItem{
		{ID: "dnf-packages", Name: "DNF Cache", Size: 4096, Selected: true, Safety: core.SafetySafe, Metadata: map[string]string{"type": "packages"}},
	}

	dnfDryRes, err := dnfMod.Clean(ctx, dnfItems, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dnf dry-run failed: %v", err)
	}
	if len(dnfCalls) != 0 {
		t.Errorf("expected 0 dnf commands in dry-run, got %d (%v)", len(dnfCalls), dnfCalls)
	}
	if dnfDryRes.FreedBytes != 4096 {
		t.Errorf("expected 4096 bytes freed in dnf dry-run, got %d", dnfDryRes.FreedBytes)
	}

	dnfCalls = nil
	_, err = dnfMod.Clean(ctx, dnfItems, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("dnf clean failed: %v", err)
	}
	if len(dnfCalls) != 1 {
		t.Fatalf("expected 1 dnf command, got %d (%v)", len(dnfCalls), dnfCalls)
	}
	if dnfCalls[0][0] != "dnf" || dnfCalls[0][1] != "clean" || dnfCalls[0][2] != "all" {
		t.Errorf("expected 'dnf clean all', got %v", dnfCalls[0])
	}
}
