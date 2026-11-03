package dnf

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/core/distro"
)

func TestDnfModuleContract(t *testing.T) {
	m := New()

	if m.ID() != ModuleID {
		t.Errorf("expected ID %q, got %q", ModuleID, m.ID())
	}
	if m.Name() != ModuleName {
		t.Errorf("expected Name %q, got %q", ModuleName, m.Name())
	}
	if m.Category() != ModuleCategory {
		t.Errorf("expected Category %q, got %q", ModuleCategory, m.Category())
	}
	if !m.RequiresElevation() {
		t.Error("expected RequiresElevation() to be true")
	}

	// On host system (CachyOS), DNF is not installed
	if m.IsAvailable() {
		t.Error("expected IsAvailable() to be false on CachyOS host")
	}
}

func TestDnfScanAndDryRun(t *testing.T) {
	defer distro.ResetLookPathMock()

	// Mock dnf presence
	distro.SetLookPathMock(func(binary string) (string, error) {
		if binary == "dnf" {
			return "/usr/bin/dnf", nil
		}
		return "", os.ErrNotExist
	})

	m := New()
	if !m.IsAvailable() {
		t.Fatal("expected IsAvailable() to be true when mocked")
	}

	// Create simulated DNF cache directory
	tmpDir := t.TempDir()
	m.SetCachePath(tmpDir)

	subDir := filepath.Join(tmpDir, "repo-fedora", "packages")
	_ = os.MkdirAll(subDir, 0755)

	rpm1 := filepath.Join(subDir, "package-a.rpm")
	rpm2 := filepath.Join(subDir, "package-b.rpm")
	other := filepath.Join(subDir, "repomd.xml")

	_ = os.WriteFile(rpm1, []byte("rpm-content-1"), 0644)
	_ = os.WriteFile(rpm2, []byte("rpm-content-222"), 0644)
	_ = os.WriteFile(other, []byte("<xml></xml>"), 0644)

	res, err := m.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan returned unexpected error: %v", err)
	}

	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item group, got %d", len(res.Items))
	}

	expectedSize := int64(len("rpm-content-1") + len("rpm-content-222"))
	if res.Items[0].Size != expectedSize {
		t.Errorf("expected item size %d, got %d", expectedSize, res.Items[0].Size)
	}

	// Test Clean with dry-run
	cleanRes, err := m.Clean(context.Background(), res.Items, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Clean returned error: %v", err)
	}

	if cleanRes.FreedBytes != expectedSize {
		t.Errorf("expected %d freed bytes in dry-run, got %d", expectedSize, cleanRes.FreedBytes)
	}

	// Ensure files were not deleted during dry run
	if _, err := os.Stat(rpm1); os.IsNotExist(err) {
		t.Error("rpm file was deleted during dry run!")
	}
}
