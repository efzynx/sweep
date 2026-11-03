package devcache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/efzynx/sweep/internal/core"
)

func TestDevCacheModule_Metadata(t *testing.T) {
	m := New()

	if m.ID() != "devcache" {
		t.Errorf("expected ID 'devcache', got %q", m.ID())
	}
	if m.Name() != "Developer Build Caches" {
		t.Errorf("expected Name 'Developer Build Caches', got %q", m.Name())
	}
	if m.Category() != "devcache" {
		t.Errorf("expected Category 'devcache', got %q", m.Category())
	}
	if m.RequiresElevation() {
		t.Errorf("expected RequiresElevation to be false")
	}
	if m.Description() == "" {
		t.Errorf("expected non-empty description")
	}
	if !m.IsAvailable() {
		t.Errorf("expected IsAvailable to be true on dev machine")
	}
}

func TestDevCacheModule_ScanAndClean_Mock(t *testing.T) {
	tempHome := t.TempDir()

	// Setup mock directories and files
	npmDir := filepath.Join(tempHome, ".npm", "_cacache")
	goBuildDir := filepath.Join(tempHome, ".cache", "go-build")
	goModDir := filepath.Join(tempHome, "go", "pkg", "mod", "cache")
	cargoRegDir := filepath.Join(tempHome, ".cargo", "registry", "cache")
	pipDir := filepath.Join(tempHome, ".cache", "pip")
	uvDir := filepath.Join(tempHome, ".cache", "uv")

	for _, d := range []string{npmDir, goBuildDir, goModDir, cargoRegDir, pipDir, uvDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}

	// Write mock files
	npmFile := filepath.Join(npmDir, "sample-npm.tgz")
	_ = os.WriteFile(npmFile, []byte("npm-cached-content-12345"), 0o644)

	goBuildFile := filepath.Join(goBuildDir, "build-artifact.a")
	_ = os.WriteFile(goBuildFile, []byte("go-build-artifact-bytes-67890"), 0o644)

	// In Go mod cache, test read-only file handling
	goModFile := filepath.Join(goModDir, "download.zip")
	_ = os.WriteFile(goModFile, []byte("go-mod-download-archive"), 0o444)
	_ = os.Chmod(goModDir, 0o555) // Make parent read-only to test safe unlinking

	cargoFile := filepath.Join(cargoRegDir, "crate-0.1.0.crate")
	_ = os.WriteFile(cargoFile, []byte("cargo-crate-tarball"), 0o644)

	pipFile := filepath.Join(pipDir, "package-wheel.whl")
	_ = os.WriteFile(pipFile, []byte("pip-wheel-bytes"), 0o644)

	uvFile := filepath.Join(uvDir, "uv-cached.whl")
	_ = os.WriteFile(uvFile, []byte("uv-cached-wheel-data"), 0o644)

	m := New()
	m.SetHomeDir(tempHome)

	ctx := context.Background()
	scanRes, err := m.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(scanRes.Items) != 6 {
		t.Fatalf("expected 6 scanned items, got %d", len(scanRes.Items))
	}

	// Verify classifications
	itemMap := make(map[string]core.CleanableItem)
	for _, item := range scanRes.Items {
		itemMap[item.ID] = item
	}

	// 1. npm
	npmItem, ok := itemMap["devcache:npm"]
	if !ok {
		t.Errorf("missing devcache:npm")
	} else {
		if npmItem.Safety != core.SafetySafe {
			t.Errorf("expected npm safety SAFE, got %v", npmItem.Safety)
		}
		if !npmItem.Selected {
			t.Errorf("expected npm Selected to be true")
		}
	}

	// 2. Go build
	goBuildItem, ok := itemMap["devcache:go-build"]
	if !ok {
		t.Errorf("missing devcache:go-build")
	} else {
		if goBuildItem.Safety != core.SafetySafe {
			t.Errorf("expected go-build safety SAFE, got %v", goBuildItem.Safety)
		}
		if !goBuildItem.Selected {
			t.Errorf("expected go-build Selected to be true")
		}
	}

	// 3. Go mod (CAUTION)
	goModItem, ok := itemMap["devcache:go-mod"]
	if !ok {
		t.Errorf("missing devcache:go-mod")
	} else {
		if goModItem.Safety != core.SafetyCaution {
			t.Errorf("expected go-mod safety CAUTION, got %v", goModItem.Safety)
		}
		if goModItem.Selected {
			t.Errorf("expected go-mod Selected to be false (safe-by-default)")
		}
	}

	// 4. Cargo registry
	cargoItem, ok := itemMap["devcache:cargo-registry"]
	if !ok {
		t.Errorf("missing devcache:cargo-registry")
	} else {
		if cargoItem.Safety != core.SafetySafe {
			t.Errorf("expected cargo-registry safety SAFE, got %v", cargoItem.Safety)
		}
		if !cargoItem.Selected {
			t.Errorf("expected cargo-registry Selected to be true")
		}
	}

	// 5. pip
	pipItem, ok := itemMap["devcache:pip"]
	if !ok {
		t.Errorf("missing devcache:pip")
	} else {
		if pipItem.Safety != core.SafetySafe {
			t.Errorf("expected pip safety SAFE, got %v", pipItem.Safety)
		}
		if !pipItem.Selected {
			t.Errorf("expected pip Selected to be true")
		}
	}

	// 6. uv
	uvItem, ok := itemMap["devcache:uv"]
	if !ok {
		t.Errorf("missing devcache:uv")
	} else {
		if uvItem.Safety != core.SafetySafe {
			t.Errorf("expected uv safety SAFE, got %v", uvItem.Safety)
		}
		if !uvItem.Selected {
			t.Errorf("expected uv Selected to be true")
		}
	}

	// Test DryRun
	dryRunOpts := core.CleanOptions{DryRun: true}
	dryRes, err := m.Clean(ctx, scanRes.Items, dryRunOpts)
	if err != nil {
		t.Fatalf("DryRun Clean failed: %v", err)
	}
	if len(dryRes.CleanedItems) != len(scanRes.Items) {
		t.Errorf("expected %d dry-run cleaned items, got %d", len(scanRes.Items), len(dryRes.CleanedItems))
	}
	if dryRes.FreedBytes != scanRes.TotalSize {
		t.Errorf("expected freed bytes %d, got %d", scanRes.TotalSize, dryRes.FreedBytes)
	}

	// Verify files still exist after dry-run
	if _, err := os.Stat(npmFile); err != nil {
		t.Errorf("npmFile was deleted during dry-run!")
	}
	if _, err := os.Stat(goBuildFile); err != nil {
		t.Errorf("goBuildFile was deleted during dry-run!")
	}

	// Test Real Clean
	cleanOpts := core.CleanOptions{DryRun: false}
	cleanRes, err := m.Clean(ctx, scanRes.Items, cleanOpts)
	if err != nil {
		t.Fatalf("Real Clean failed: %v", err)
	}
	if len(cleanRes.Errors) > 0 {
		t.Fatalf("Clean encountered errors: %v", cleanRes.Errors)
	}
	if len(cleanRes.CleanedItems) != len(scanRes.Items) {
		t.Errorf("expected %d cleaned items, got %d", len(scanRes.Items), len(cleanRes.CleanedItems))
	}

	// Verify directory contents were unlinked
	npmEntries, _ := os.ReadDir(npmDir)
	if len(npmEntries) != 0 {
		t.Errorf("expected npmDir to be empty after clean, got %d entries", len(npmEntries))
	}
	goBuildEntries, _ := os.ReadDir(goBuildDir)
	if len(goBuildEntries) != 0 {
		t.Errorf("expected goBuildDir to be empty after clean, got %d entries", len(goBuildEntries))
	}
	goModEntries, _ := os.ReadDir(goModDir)
	if len(goModEntries) != 0 {
		t.Errorf("expected goModDir to be empty after clean, got %d entries", len(goModEntries))
	}
}

func TestDevCacheModule_ScanRealEnv(t *testing.T) {
	m := New()
	start := time.Now()
	res, err := m.Scan(context.Background())
	dur := time.Since(start)
	t.Logf("Real Scan completed in %v, found %d items, total %d bytes", dur, len(res.Items), res.TotalSize)
	for _, it := range res.Items {
		t.Logf("  - %s: %s (%d bytes)", it.ID, it.Name, it.Size)
	}
	if err != nil {
		t.Fatalf("real scan failed: %v", err)
	}
}

func TestDevCacheModule_ContextCancellation(t *testing.T) {
	tempHome := t.TempDir()
	npmDir := filepath.Join(tempHome, ".npm", "_cacache")
	_ = os.MkdirAll(npmDir, 0o755)
	_ = os.WriteFile(filepath.Join(npmDir, "file.dat"), []byte("test"), 0o644)

	m := New()
	m.SetHomeDir(tempHome)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel context

	_, err := m.Scan(ctx)
	if err == nil {
		t.Errorf("expected error from canceled context on Scan")
	}

	items := []core.CleanableItem{
		{ID: "devcache:npm", Path: npmDir, Size: 4},
	}
	_, err = m.Clean(ctx, items, core.CleanOptions{DryRun: false})
	if err == nil {
		t.Errorf("expected error from canceled context on Clean")
	}
}

func TestDevCacheModule_EmptyDirectorySkipped(t *testing.T) {
	tempHome := t.TempDir()
	emptyDir := filepath.Join(tempHome, ".npm", "_cacache")
	_ = os.MkdirAll(emptyDir, 0o755)

	m := New()
	m.SetHomeDir(tempHome)

	res, err := m.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if len(res.Items) != 0 {
		t.Errorf("expected 0 items for empty directories, got %d", len(res.Items))
	}
}

func TestDevCacheModule_CustomTargetsAndConfig(t *testing.T) {
	cfg := &core.Config{
		Retention: core.RetentionConfig{
			DevCacheIdleDays: 14,
		},
	}
	m := NewWithConfig(cfg)
	if m.cfg == nil || m.cfg.Retention.DevCacheIdleDays != 14 {
		t.Errorf("expected retention dev cache idle days 14, got %v", m.cfg)
	}

	customTargets := []Target{
		{
			ID:          "custom:cache",
			Name:        "Custom Test Cache",
			Ecosystem:   "custom",
			Path:        "/tmp/custom_cache",
			Safety:      core.SafetySafe,
			Description: "Custom path",
		},
	}
	m.SetTargets(customTargets)
	targets := m.Targets()
	if len(targets) != 1 || targets[0].ID != "custom:cache" {
		t.Errorf("SetTargets failed to update targets")
	}
}
