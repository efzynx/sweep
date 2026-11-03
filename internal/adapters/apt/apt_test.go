package apt

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/core/distro"
)

func TestAptModuleContract(t *testing.T) {
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

	// On host system (CachyOS), apt is not installed
	if m.IsAvailable() {
		t.Error("expected IsAvailable() to be false on CachyOS host")
	}
}

func TestAptScanAndDryRun(t *testing.T) {
	defer distro.ResetLookPathMock()

	// Mock apt-get presence
	distro.SetLookPathMock(func(binary string) (string, error) {
		if binary == "apt-get" {
			return "/usr/bin/apt-get", nil
		}
		return "", os.ErrNotExist
	})

	m := New()
	if !m.IsAvailable() {
		t.Fatal("expected IsAvailable() to be true when mocked")
	}

	// Create simulated apt cache directory
	tmpDir := t.TempDir()
	m.SetCachePath(tmpDir)

	// Create test .deb files
	deb1 := filepath.Join(tmpDir, "pkg1_1.0_amd64.deb")
	deb2 := filepath.Join(tmpDir, "pkg2_2.0_amd64.deb")
	nonDeb := filepath.Join(tmpDir, "lock")

	_ = os.WriteFile(deb1, []byte("payload-1-bytes"), 0644)
	_ = os.WriteFile(deb2, []byte("payload-22-bytes"), 0644)
	_ = os.WriteFile(nonDeb, []byte(""), 0644)

	res, err := m.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan returned unexpected error: %v", err)
	}

	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item group, got %d", len(res.Items))
	}

	expectedSize := int64(len("payload-1-bytes") + len("payload-22-bytes"))
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
	if _, err := os.Stat(deb1); os.IsNotExist(err) {
		t.Error("deb file was deleted during dry run!")
	}
}
