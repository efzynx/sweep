package zypper

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/core/distro"
)

func TestZypperModuleContract(t *testing.T) {
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

	// Host system is CachyOS / Arch
	cur := distro.Current()
	if cur.IsSuse() || distro.HasZypper() {
		if !m.IsAvailable() {
			t.Error("expected IsAvailable() to be true on openSUSE host")
		}
	} else {
		if m.IsAvailable() {
			t.Error("expected IsAvailable() to be false on non-openSUSE host without zypper")
		}
	}
}

func TestParseUnneededPackages(t *testing.T) {
	output := `Loading repository data...
Reading installed packages...
S | Repository | Name         | Version   | Arch
--+------------+--------------+-----------+-------
i | repo-oss   | libjpeg8     | 8.0.2-1.1 | x86_64
i+| repo-oss   | libpng12-0   | 1.2.59-3  | x86_64
v | repo-oss   | not-installed| 1.0.0     | noarch
`

	unneeded := ParseUnneededPackages(output)
	if len(unneeded) != 2 {
		t.Fatalf("expected 2 unneeded packages, got %d", len(unneeded))
	}

	if unneeded[0].Name != "libjpeg8" {
		t.Errorf("expected first package name libjpeg8, got %q", unneeded[0].Name)
	}
	if unneeded[0].Version != "8.0.2-1.1" {
		t.Errorf("expected first package version 8.0.2-1.1, got %q", unneeded[0].Version)
	}
	if unneeded[0].Repo != "repo-oss" {
		t.Errorf("expected first package repo repo-oss, got %q", unneeded[0].Repo)
	}
	if unneeded[0].Arch != "x86_64" {
		t.Errorf("expected first package arch x86_64, got %q", unneeded[0].Arch)
	}

	if unneeded[1].Name != "libpng12-0" {
		t.Errorf("expected second package name libpng12-0, got %q", unneeded[1].Name)
	}
}

func TestZypperScanAndDryRun(t *testing.T) {
	defer distro.ResetLookPathMock()

	distro.SetLookPathMock(func(binary string) (string, error) {
		if binary == "zypper" {
			return "/usr/bin/zypper", nil
		}
		return "", os.ErrNotExist
	})

	m := New()
	m.SetAvailable(true)

	tmpDir := t.TempDir()
	m.SetCachePaths([]string{tmpDir})

	subDir := filepath.Join(tmpDir, "repo-oss", "packages")
	_ = os.MkdirAll(subDir, 0755)

	rpm1 := filepath.Join(subDir, "app-1.0.rpm")
	rpm2 := filepath.Join(subDir, "lib-2.0.rpm")
	_ = os.WriteFile(rpm1, make([]byte, 1024*100), 0644) // 100 KB
	_ = os.WriteFile(rpm2, make([]byte, 1024*200), 0644) // 200 KB

	var runnerCalls [][]string
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		runnerCalls = append(runnerCalls, append([]string{name}, args...))
		if len(args) >= 3 && args[1] == "packages" && args[2] == "--unneeded" {
			sampleOutput := `S | Repository | Name | Version | Arch
--+------------+------+---------+-----
i | repo-oss   | unneeded-lib | 1.0.0 | x86_64
`
			return []byte(sampleOutput), nil
		}
		if len(args) >= 3 && args[0] == "-q" && args[1] == "--queryformat" {
			return []byte("524288\n"), nil // 512 KB
		}
		return nil, nil
	})

	ctx := context.Background()
	scanRes, err := m.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(scanRes.Items) != 2 {
		t.Fatalf("expected 2 items (1 package cache, 1 unneeded), got %d", len(scanRes.Items))
	}

	var pkgItem, unneededItem *core.CleanableItem
	for i := range scanRes.Items {
		if scanRes.Items[i].ID == "zypper-packages" {
			pkgItem = &scanRes.Items[i]
		} else if strings.HasPrefix(scanRes.Items[i].ID, "zypper-unneeded:") {
			unneededItem = &scanRes.Items[i]
		}
	}

	if pkgItem == nil {
		t.Fatal("expected zypper-packages item")
	}
	if pkgItem.Size != 1024*300 {
		t.Errorf("expected 300KB cache size, got %d", pkgItem.Size)
	}

	if unneededItem == nil {
		t.Fatal("expected zypper-unneeded item")
	}
	if unneededItem.Size != 524288 {
		t.Errorf("expected 524288 bytes for unneeded package, got %d", unneededItem.Size)
	}

	// Test Dry-Run Clean
	runnerCalls = nil
	dryRes, err := m.Clean(ctx, scanRes.Items, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Clean dry-run failed: %v", err)
	}
	if len(dryRes.CleanedItems) != 2 {
		t.Errorf("expected 2 cleaned items in dry-run, got %d", len(dryRes.CleanedItems))
	}
	expectedFreed := int64(1024*300 + 524288)
	if dryRes.FreedBytes != expectedFreed {
		t.Errorf("expected %d freed bytes, got %d", expectedFreed, dryRes.FreedBytes)
	}
	if len(runnerCalls) != 0 {
		t.Errorf("expected 0 command calls in dry-run, got %d", len(runnerCalls))
	}
}

func TestZypperCleanExecution(t *testing.T) {
	m := New()
	m.SetAvailable(true)

	var executedCommands [][]string
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		executedCommands = append(executedCommands, append([]string{name}, args...))
		return []byte("done"), nil
	})

	items := []core.CleanableItem{
		{
			ID:   "zypper-packages",
			Name: "Cached .rpm packages",
			Size: 2048,
		},
		{
			ID:       "zypper-unneeded:libold",
			Name:     "Unneeded package: libold",
			Size:     4096,
			Metadata: map[string]string{"package_name": "libold"},
		},
	}

	ctx := context.Background()
	res, err := m.Clean(ctx, items, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("Clean execution failed: %v", err)
	}

	if len(res.CleanedItems) != 2 {
		t.Errorf("expected 2 cleaned items, got %d", len(res.CleanedItems))
	}
	if res.FreedBytes != 6144 {
		t.Errorf("expected 6144 freed bytes, got %d", res.FreedBytes)
	}

	// Verify zypper clean and remove commands were triggered
	if len(executedCommands) != 2 {
		t.Fatalf("expected 2 commands executed, got %d (%v)", len(executedCommands), executedCommands)
	}

	// Check clean command: zypper clean --all
	cleanCmd := strings.Join(executedCommands[0], " ")
	if !strings.Contains(cleanCmd, "zypper clean --all") {
		t.Errorf("unexpected clean command: %s", cleanCmd)
	}

	// Check remove command: zypper remove --clean-deps -y libold
	rmCmd := strings.Join(executedCommands[1], " ")
	if !strings.Contains(rmCmd, "remove --clean-deps -y libold") {
		t.Errorf("unexpected remove command: %s", rmCmd)
	}
}

func TestZypperContextCancellation(t *testing.T) {
	m := New()
	m.SetAvailable(true)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	res, err := m.Scan(ctx)
	if err == nil && len(res.Items) > 0 {
		t.Error("expected error or empty items on canceled scan context")
	}

	_, err = m.Clean(ctx, []core.CleanableItem{{ID: "zypper-packages"}}, core.CleanOptions{DryRun: false})
	if err == nil {
		t.Error("expected error on canceled clean context")
	}
}

func TestZypperIsAvailableDistroFamily(t *testing.T) {
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
