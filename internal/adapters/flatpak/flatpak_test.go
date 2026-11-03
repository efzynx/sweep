package flatpak

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/efzynx/sweep/internal/core"
)

func TestFlatpakModule_Metadata(t *testing.T) {
	m := New()

	if m.ID() != "flatpak" {
		t.Errorf("expected ID 'flatpak', got %q", m.ID())
	}
	if m.Name() != "Flatpak Packages" {
		t.Errorf("expected Name 'Flatpak Packages', got %q", m.Name())
	}
	if m.Category() != "sandboxed_apps" {
		t.Errorf("expected Category 'sandboxed_apps', got %q", m.Category())
	}
	if m.RequiresElevation() {
		t.Errorf("expected RequiresElevation to be false")
	}
	if m.Description() == "" {
		t.Errorf("expected non-empty description")
	}
}

func TestFlatpakModule_Scan(t *testing.T) {
	tempVarApp := t.TempDir()

	// Setup fake app cache directories
	app1Cache := filepath.Join(tempVarApp, "org.test.AppOne", "cache")
	_ = os.MkdirAll(app1Cache, 0755)
	_ = os.WriteFile(filepath.Join(app1Cache, "cached_data.bin"), []byte("app1 cached data 50 bytes"), 0644)

	app2Cache := filepath.Join(tempVarApp, "org.test.AppTwo", "cache")
	_ = os.MkdirAll(app2Cache, 0755)
	_ = os.WriteFile(filepath.Join(app2Cache, "thumb.jpg"), []byte("app2 thumbnail 30 bytes"), 0644)

	m := New()
	m.SetVarAppDir(tempVarApp)
	m.SetAvailable(true)

	mockRunner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmdStr := strings.Join(args, " ")
		if strings.Contains(cmdStr, "list --unused") {
			// Mock standard flatpak list --unused --columns=application,ref,size,options
			return []byte("org.kde.Platform\torg.kde.Platform/x86_64/6.10\t1.1\u00a0GB\tsystem,runtime\norg.gnome.Sdk\torg.gnome.Sdk/x86_64/45\t800.5\u00a0MB\tsystem,runtime\n"), nil
		}
		return nil, fmt.Errorf("unexpected command: %s %v", name, args)
	}

	m.SetCommandRunner(mockRunner)

	ctx := context.Background()
	res, err := m.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(res.Items) != 4 {
		t.Fatalf("expected 4 items (2 runtimes, 2 caches), got %d", len(res.Items))
	}

	itemMap := make(map[string]core.CleanableItem)
	for _, it := range res.Items {
		itemMap[it.ID] = it
	}

	// Verify runtimes
	rt1, ok := itemMap["flatpak-runtime:org.kde.Platform/x86_64/6.10"]
	if !ok {
		t.Errorf("expected runtime org.kde.Platform not found")
	} else {
		if rt1.Size == 0 {
			t.Errorf("expected parsed size for runtime")
		}
		if rt1.Safety != core.SafetySafe {
			t.Errorf("expected SafetySafe for unused runtime")
		}
	}

	// Verify caches
	c1, ok := itemMap["flatpak-cache:org.test.AppOne"]
	if !ok {
		t.Errorf("expected cache for AppOne not found")
	} else {
		if c1.Size == 0 {
			t.Errorf("expected non-zero size for AppOne cache")
		}
		if c1.Safety != core.SafetySafe {
			t.Errorf("expected SafetySafe for cache")
		}
	}
}

func TestFlatpakModule_Clean(t *testing.T) {
	tempVarApp := t.TempDir()

	cacheDir := filepath.Join(tempVarApp, "org.test.AppOne", "cache")
	_ = os.MkdirAll(cacheDir, 0755)
	fileInCache := filepath.Join(cacheDir, "sample.dat")
	_ = os.WriteFile(fileInCache, []byte("data to be cleaned"), 0644)

	items := []core.CleanableItem{
		{
			ID:       "flatpak-runtime:org.kde.Platform/x86_64/6.10",
			Size:     1024 * 1024 * 100,
			Selected: true,
			Metadata: map[string]string{
				"type": "runtime",
				"ref":  "org.kde.Platform/x86_64/6.10",
			},
		},
		{
			ID:       "flatpak-cache:org.test.AppOne",
			Path:     cacheDir,
			Size:     100,
			Selected: true,
			Metadata: map[string]string{
				"type": "cache",
				"app":  "org.test.AppOne",
			},
		},
	}

	var executedCommands [][]string
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmdLine := append([]string{name}, args...)
		executedCommands = append(executedCommands, cmdLine)
		return []byte("ok"), nil
	}

	m := New()
	m.SetVarAppDir(tempVarApp)
	m.SetCommandRunner(runner)

	ctx := context.Background()

	// 1. Dry run
	dryRes, err := m.Clean(ctx, items, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("DryRun failed: %v", err)
	}
	if len(dryRes.CleanedItems) != 2 {
		t.Errorf("expected 2 cleaned items in dry-run, got %d", len(dryRes.CleanedItems))
	}
	if len(executedCommands) != 0 {
		t.Errorf("DryRun should not execute any commands, executed: %v", executedCommands)
	}
	if _, err := os.Stat(fileInCache); os.IsNotExist(err) {
		t.Errorf("DryRun should not delete files")
	}

	// 2. Real clean
	cleanRes, err := m.Clean(ctx, items, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("Clean failed: %v", err)
	}
	if len(cleanRes.CleanedItems) != 2 {
		t.Errorf("expected 2 cleaned items, got %d", len(cleanRes.CleanedItems))
	}
	if len(cleanRes.Errors) != 0 {
		t.Errorf("unexpected errors in clean: %v", cleanRes.Errors)
	}

	// Verify command executed
	if len(executedCommands) != 1 {
		t.Fatalf("expected 1 command executed, got %d", len(executedCommands))
	}
	cmd := executedCommands[0]
	if cmd[0] != "flatpak" || cmd[1] != "uninstall" || cmd[2] != "--unused" || cmd[3] != "-y" {
		t.Errorf("unexpected command: %v", cmd)
	}

	// Verify cache contents removed
	if _, err := os.Stat(fileInCache); !os.IsNotExist(err) {
		t.Errorf("expected file inside cache to be removed")
	}
	// Verify directory still exists
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		t.Errorf("cache parent directory should still exist")
	}
}
