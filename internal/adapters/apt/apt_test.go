package apt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	// Verify IsAvailable matches runtime host detection
	cur := distro.Current()
	if cur.IsDebian() || distro.HasApt() {
		if !m.IsAvailable() {
			t.Error("expected IsAvailable() to be true on Debian/Ubuntu host")
		}
	} else {
		if m.IsAvailable() {
			t.Error("expected IsAvailable() to be false on non-Debian host without apt")
		}
	}
}

func TestParseAptAutoremoveOutput(t *testing.T) {
	rawOutput := `Reading package lists... Done
Building dependency tree... Done
Reading state information... Done
The following packages will be REMOVED:
  linux-headers-6.5.0-28 linux-headers-6.5.0-28-generic linux-image-6.5.0-28-generic
  linux-modules-6.5.0-28-generic linux-modules-extra-6.5.0-28-generic
0 upgraded, 0 newly installed, 5 to remove and 12 not upgraded.
After this operation, 482 MB disk space will be freed.
Remv linux-headers-6.5.0-28-generic [6.5.0-28.29]
Remv linux-headers-6.5.0-28 [6.5.0-28.29]
Remv linux-modules-extra-6.5.0-28-generic [6.5.0-28.29]
Remv linux-modules-6.5.0-28-generic [6.5.0-28.29]
Remv linux-image-6.5.0-28-generic [6.5.0-28.29]
`

	pkgs, freedBytes := ParseAptAutoremoveOutput(rawOutput)

	expectedCount := 5
	if len(pkgs) != expectedCount {
		t.Fatalf("expected %d packages, got %d: %v", expectedCount, len(pkgs), pkgs)
	}

	expectedBytes := int64(482 * 1024 * 1024)
	if freedBytes != expectedBytes {
		t.Errorf("expected %d freed bytes, got %d", expectedBytes, freedBytes)
	}

	// Test zero output
	zeroOutput := `Reading package lists... Done
Building dependency tree... Done
0 upgraded, 0 newly installed, 0 to remove and 0 not upgraded.
`
	zeroPkgs, zeroFreed := ParseAptAutoremoveOutput(zeroOutput)
	if len(zeroPkgs) != 0 || zeroFreed != 0 {
		t.Errorf("expected 0 packages and 0 bytes for clean system, got %d and %d", len(zeroPkgs), zeroFreed)
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

	// Create test .deb files and subdirectories
	deb1 := filepath.Join(tmpDir, "pkg1_1.0_amd64.deb")
	deb2 := filepath.Join(tmpDir, "pkg2_2.0_amd64.deb")
	lockFile := filepath.Join(tmpDir, "lock")
	partialDir := filepath.Join(tmpDir, "partial")
	_ = os.MkdirAll(partialDir, 0755)
	_ = os.WriteFile(filepath.Join(partialDir, "in-flight.deb"), []byte("partial"), 0644)

	_ = os.WriteFile(deb1, []byte("payload-1-bytes"), 0644)
	_ = os.WriteFile(deb2, []byte("payload-22-bytes"), 0644)
	_ = os.WriteFile(lockFile, []byte(""), 0644)

	// Mock CommandRunner for apt-get --dry-run autoremove
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "apt-get" && len(args) >= 2 && args[0] == "--dry-run" && args[1] == "autoremove" {
			return []byte(`Reading package lists... Done
The following packages will be REMOVED:
  libdummy-orphan1 libdummy-orphan2
0 upgraded, 0 newly installed, 2 to remove and 0 not upgraded.
After this operation, 2,048 kB disk space will be freed.
Remv libdummy-orphan1 [1.0]
Remv libdummy-orphan2 [2.0]
`), nil
		}
		return nil, fmt.Errorf("unexpected command: %s %v", name, args)
	})

	res, err := m.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan returned unexpected error: %v", err)
	}

	// 1 archive item + 2 orphan items = 3 items total
	if len(res.Items) != 3 {
		t.Fatalf("expected 3 items, got %d: %+v", len(res.Items), res.Items)
	}

	expectedArchiveSize := int64(len("payload-1-bytes") + len("payload-22-bytes"))
	if res.Items[0].Size != expectedArchiveSize {
		t.Errorf("expected archive size %d, got %d", expectedArchiveSize, res.Items[0].Size)
	}

	expectedOrphanTotal := int64(2048 * 1024)
	var orphanSizeSum int64
	for _, it := range res.Items[1:] {
		orphanSizeSum += it.Size
	}
	if orphanSizeSum != expectedOrphanTotal {
		t.Errorf("expected orphan size sum %d, got %d", expectedOrphanTotal, orphanSizeSum)
	}

	// Test Clean with dry-run
	cleanRes, err := m.Clean(context.Background(), res.Items, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Clean returned error: %v", err)
	}

	expectedTotal := expectedArchiveSize + expectedOrphanTotal
	if cleanRes.FreedBytes != expectedTotal {
		t.Errorf("expected %d freed bytes in dry-run, got %d", expectedTotal, cleanRes.FreedBytes)
	}

	// Ensure files were not deleted during dry run
	if _, err := os.Stat(deb1); os.IsNotExist(err) {
		t.Error("deb file was deleted during dry run!")
	}
}

func TestAptCleanExecution(t *testing.T) {
	tmpDir := t.TempDir()
	m := New()
	m.SetCachePath(tmpDir)

	deb1 := filepath.Join(tmpDir, "test.deb")
	_ = os.WriteFile(deb1, []byte("data"), 0644)

	var executedCommands [][]string
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := append([]string{name}, args...)
		executedCommands = append(executedCommands, cmd)
		return []byte("done"), nil
	})

	items := []core.CleanableItem{
		{
			ID:       "apt-archives",
			Path:     tmpDir,
			Size:     4,
			Metadata: map[string]string{"type": "archives"},
		},
		{
			ID:       "apt-orphan:dummy-pkg",
			Size:     100,
			Metadata: map[string]string{"type": "orphan", "package": "dummy-pkg"},
		},
	}

	cleanRes, err := m.Clean(context.Background(), items, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("Clean failed: %v", err)
	}

	if cleanRes.FreedBytes != 104 {
		t.Errorf("expected 104 freed bytes, got %d", cleanRes.FreedBytes)
	}

	// Verify executed commands: apt-get clean, apt-get autoclean, apt-get autoremove -y
	expectedCalls := [][]string{
		{"apt-get", "clean"},
		{"apt-get", "autoclean"},
		{"apt-get", "autoremove", "-y"},
	}

	if len(executedCommands) != len(expectedCalls) {
		t.Fatalf("expected %d commands executed, got %d: %v", len(expectedCalls), len(executedCommands), executedCommands)
	}

	for i, exp := range expectedCalls {
		gotStr := strings.Join(executedCommands[i], " ")
		expStr := strings.Join(exp, " ")
		if gotStr != expStr {
			t.Errorf("command %d mismatch: got %q, want %q", i, gotStr, expStr)
		}
	}

	// Ensure .deb file was cleaned from test tmpDir
	if _, err := os.Stat(deb1); !os.IsNotExist(err) {
		t.Error("expected deb1 to be removed from test cachePath")
	}
}

func TestAptContextCancellation(t *testing.T) {
	m := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	items := []core.CleanableItem{
		{ID: "apt-archives", Size: 100},
	}

	_, err := m.Clean(ctx, items, core.CleanOptions{DryRun: false})
	if err == nil {
		t.Error("expected error on cancelled context, got nil")
	}
}

func TestAptIsAvailableDistroFamily(t *testing.T) {
	m := New()
	m.SetAvailable(true)
	if !m.IsAvailable() {
		t.Error("expected IsAvailable() to be true when explicitly overridden")
	}
	m.SetAvailable(false)
	if m.IsAvailable() {
		t.Error("expected IsAvailable() to be false when explicitly overridden")
	}
}
