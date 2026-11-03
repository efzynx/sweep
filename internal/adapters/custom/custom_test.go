package custom

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/efzynx/sweep/internal/core"
)

func TestCustomModule_Interface(t *testing.T) {
	m := New()
	if m.ID() != "custom" {
		t.Errorf("expected ID custom, got %s", m.ID())
	}
	if m.Name() != "Custom Paths & Rules" {
		t.Errorf("expected Name 'Custom Paths & Rules', got %s", m.Name())
	}
	if m.Category() != "custom" {
		t.Errorf("expected Category custom, got %s", m.Category())
	}
	if !m.IsAvailable() {
		t.Error("expected IsAvailable true")
	}
	if m.RequiresElevation() {
		t.Error("expected RequiresElevation false")
	}
}

func TestCustomModule_ScanEmpty(t *testing.T) {
	m := New()
	res, err := m.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan on empty rules failed: %v", err)
	}
	if len(res.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(res.Items))
	}
}

func TestCustomModule_ScanAndClean_DryRun(t *testing.T) {
	tempDir := t.TempDir()

	f1 := filepath.Join(tempDir, "app.log")
	f2 := filepath.Join(tempDir, "temp.cache")
	f3 := filepath.Join(tempDir, "keep.txt")

	if err := os.WriteFile(f1, []byte("log data 123"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("cache data 456"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f3, []byte("keep this"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &core.Config{
		CustomRules: []core.CustomScanRule{
			{
				ID:        "rule-logs",
				Name:      "App Logs",
				Path:      tempDir,
				Pattern:   "*.log",
				Recursive: false,
				Safety:    core.SafetySafe,
			},
			{
				ID:        "rule-cache",
				Name:      "Temp Cache",
				Path:      tempDir,
				Pattern:   "*.cache",
				Recursive: false,
				Safety:    core.SafetyCaution,
			},
		},
	}

	m := NewWithConfig(cfg)

	ctx := context.Background()
	res, err := m.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(res.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(res.Items))
	}

	expectedSize := int64(len("log data 123") + len("cache data 456"))
	if res.TotalSize != expectedSize {
		t.Errorf("expected total size %d, got %d", expectedSize, res.TotalSize)
	}

	// Clean with DryRun = true
	cleanRes, err := m.Clean(ctx, res.Items, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Clean dry-run failed: %v", err)
	}

	if len(cleanRes.CleanedItems) != 2 {
		t.Errorf("expected 2 dry-run cleaned items, got %d", len(cleanRes.CleanedItems))
	}
	if cleanRes.FreedBytes != expectedSize {
		t.Errorf("expected %d freed bytes in dry-run, got %d", expectedSize, cleanRes.FreedBytes)
	}

	// Verify all files still exist
	for _, path := range []string{f1, f2, f3} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("file %s should still exist after dry-run, got err: %v", path, err)
		}
	}
}

func TestCustomModule_Clean_Real(t *testing.T) {
	tempDir := t.TempDir()

	f1 := filepath.Join(tempDir, "delete_me.tmp")
	f2 := filepath.Join(tempDir, "keep_me.txt")

	if err := os.WriteFile(f1, []byte("temporary bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("important"), 0644); err != nil {
		t.Fatal(err)
	}

	m := New()
	m.SetRules([]core.CustomScanRule{
		{
			ID:        "delete-tmp",
			Name:      "Delete Tmp",
			Path:      tempDir,
			Pattern:   "*.tmp",
			Recursive: false,
			Safety:    core.SafetySafe,
		},
	})

	ctx := context.Background()
	scanRes, err := m.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(scanRes.Items) != 1 {
		t.Fatalf("expected 1 item to delete, got %d", len(scanRes.Items))
	}

	cleanRes, err := m.Clean(ctx, scanRes.Items, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("Real clean failed: %v", err)
	}

	if len(cleanRes.CleanedItems) != 1 {
		t.Errorf("expected 1 cleaned item, got %d", len(cleanRes.CleanedItems))
	}
	if cleanRes.FreedBytes != int64(len("temporary bytes")) {
		t.Errorf("expected %d freed bytes, got %d", len("temporary bytes"), cleanRes.FreedBytes)
	}

	// Verify f1 was deleted
	if _, err := os.Stat(f1); !os.IsNotExist(err) {
		t.Errorf("file %s should have been deleted, err: %v", f1, err)
	}
	// Verify f2 still exists
	if _, err := os.Stat(f2); err != nil {
		t.Errorf("file %s should still exist, err: %v", f2, err)
	}
}

func TestCustomModule_SafetyProtections(t *testing.T) {
	m := New()

	items := []core.CleanableItem{
		{
			ID:   "danger-root",
			Path: "/",
			Size: 1000,
		},
		{
			ID:   "danger-dot",
			Path: ".",
			Size: 1000,
		},
	}

	cleanRes, err := m.Clean(context.Background(), items, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cleanRes.Errors) != 2 {
		t.Fatalf("expected 2 safety errors, got %d", len(cleanRes.Errors))
	}
	if len(cleanRes.CleanedItems) != 0 {
		t.Errorf("expected 0 cleaned items for dangerous paths, got %d", len(cleanRes.CleanedItems))
	}
}

func TestCustomModule_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	f1 := filepath.Join(tempDir, "cancel.tmp")
	_ = os.WriteFile(f1, []byte("test"), 0644)

	m := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	items := []core.CleanableItem{
		{ID: "item1", Path: f1, Size: 4},
	}

	_, err := m.Clean(ctx, items, core.CleanOptions{DryRun: false})
	if err == nil {
		t.Error("expected error due to cancelled context")
	}
}
