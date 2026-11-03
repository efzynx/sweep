package dnf

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

func TestParseOldKernels(t *testing.T) {
	runningKernel := "6.8.9-300.fc40.x86_64"
	rpmOutput := `kernel-6.8.5-301.fc40.x86_64 85000000
kernel-6.8.7-300.fc40.x86_64 86000000
kernel-6.8.9-300.fc40.x86_64 87000000
`

	oldKernels := ParseOldKernels(runningKernel, rpmOutput)

	if len(oldKernels) != 2 {
		t.Fatalf("expected 2 old kernels, got %d", len(oldKernels))
	}

	if oldKernels[0].PackageName != "kernel-6.8.5-301.fc40.x86_64" {
		t.Errorf("unexpected first old kernel: %s", oldKernels[0].PackageName)
	}
	if oldKernels[0].Size != 85000000 {
		t.Errorf("unexpected first old kernel size: %d", oldKernels[0].Size)
	}

	if oldKernels[1].PackageName != "kernel-6.8.7-300.fc40.x86_64" {
		t.Errorf("unexpected second old kernel: %s", oldKernels[1].PackageName)
	}

	// Test without sizes in rpm output (fallback size default)
	noSizeOutput := `kernel-6.8.5-301.fc40.x86_64
kernel-6.8.9-300.fc40.x86_64
`
	oldKernels2 := ParseOldKernels(runningKernel, noSizeOutput)
	if len(oldKernels2) != 1 {
		t.Fatalf("expected 1 old kernel, got %d", len(oldKernels2))
	}
	if oldKernels2[0].Size != 80*1024*1024 {
		t.Errorf("expected default size 80MB, got %d", oldKernels2[0].Size)
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

	// Create simulated DNF cache directories (dnf & libdnf5)
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()
	m.SetCachePaths([]string{tmpDir1, tmpDir2})

	subDir1 := filepath.Join(tmpDir1, "repo-fedora", "packages")
	_ = os.MkdirAll(subDir1, 0755)
	subDir2 := filepath.Join(tmpDir2, "repo-updates", "packages")
	_ = os.MkdirAll(subDir2, 0755)

	rpm1 := filepath.Join(subDir1, "package-a.rpm")
	rpm2 := filepath.Join(subDir2, "package-b.rpm")
	other := filepath.Join(subDir1, "repomd.xml")

	_ = os.WriteFile(rpm1, []byte("rpm-content-1"), 0644)
	_ = os.WriteFile(rpm2, []byte("rpm-content-222"), 0644)
	_ = os.WriteFile(other, []byte("<xml></xml>"), 0644)

	// Mock CommandRunner for uname and rpm
	m.SetRunningKernel("6.8.9-300.fc40.x86_64")
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "rpm" && len(args) >= 2 && args[0] == "-q" {
			return []byte(`kernel-6.8.5-301.fc40.x86_64 85000000
kernel-6.8.9-300.fc40.x86_64 87000000
`), nil
		}
		return nil, fmt.Errorf("unexpected command: %s %v", name, args)
	})

	res, err := m.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan returned unexpected error: %v", err)
	}

	// 1 package cache item + 1 old kernel item = 2 items total
	if len(res.Items) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(res.Items), res.Items)
	}

	expectedPackageSize := int64(len("rpm-content-1") + len("rpm-content-222"))
	if res.Items[0].Size != expectedPackageSize {
		t.Errorf("expected package cache size %d, got %d", expectedPackageSize, res.Items[0].Size)
	}

	expectedKernelSize := int64(85000000)
	if res.Items[1].Size != expectedKernelSize {
		t.Errorf("expected kernel item size %d, got %d", expectedKernelSize, res.Items[1].Size)
	}

	// Test Clean with dry-run
	cleanRes, err := m.Clean(context.Background(), res.Items, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Clean returned error: %v", err)
	}

	expectedTotal := expectedPackageSize + expectedKernelSize
	if cleanRes.FreedBytes != expectedTotal {
		t.Errorf("expected %d freed bytes in dry-run, got %d", expectedTotal, cleanRes.FreedBytes)
	}

	// Ensure files were not deleted during dry run
	if _, err := os.Stat(rpm1); os.IsNotExist(err) {
		t.Error("rpm file was deleted during dry run!")
	}
}

func TestDnfCleanExecution(t *testing.T) {
	tmpDir := t.TempDir()
	m := New()
	m.SetCachePath(tmpDir)

	rpm1 := filepath.Join(tmpDir, "pkg1.rpm")
	_ = os.WriteFile(rpm1, []byte("rpm data"), 0644)

	var executedCommands [][]string
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := append([]string{name}, args...)
		executedCommands = append(executedCommands, cmd)
		return []byte("done"), nil
	})

	items := []core.CleanableItem{
		{
			ID:       "dnf-packages",
			Path:     tmpDir,
			Size:     8,
			Metadata: map[string]string{"type": "packages"},
		},
		{
			ID:       "dnf-kernel:kernel-6.8.5-301.fc40.x86_64",
			Size:     85000000,
			Metadata: map[string]string{"type": "kernel", "package": "kernel-6.8.5-301.fc40.x86_64"},
		},
		{
			ID:       "dnf-orphan:unused-dep",
			Size:     1000,
			Metadata: map[string]string{"type": "orphan", "package": "unused-dep"},
		},
	}

	cleanRes, err := m.Clean(context.Background(), items, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("Clean failed: %v", err)
	}

	expectedBytes := int64(8 + 85000000 + 1000)
	if cleanRes.FreedBytes != expectedBytes {
		t.Errorf("expected %d freed bytes, got %d", expectedBytes, cleanRes.FreedBytes)
	}

	// Verify parameterized execution of dnf clean all, dnf remove -y, dnf autoremove -y
	expectedCalls := [][]string{
		{"dnf", "clean", "all"},
		{"dnf", "remove", "-y", "kernel-6.8.5-301.fc40.x86_64"},
		{"dnf", "autoremove", "-y"},
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

	// Ensure .rpm file was cleaned from test tmpDir
	if _, err := os.Stat(rpm1); !os.IsNotExist(err) {
		t.Error("expected rpm1 to be removed from test cachePath")
	}
}

func TestDnfContextCancellation(t *testing.T) {
	m := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	items := []core.CleanableItem{
		{ID: "dnf-packages", Size: 100},
	}

	_, err := m.Clean(ctx, items, core.CleanOptions{DryRun: false})
	if err == nil {
		t.Error("expected error on cancelled context, got nil")
	}
}

func TestDnfIsAvailableDistroFamily(t *testing.T) {
	m := New()
	if m.IsAvailable() {
		t.Error("expected IsAvailable() to be false on non-Fedora host without dnf")
	}
}
