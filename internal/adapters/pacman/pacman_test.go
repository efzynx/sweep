package pacman

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/efzynx/sweep/internal/core"
)

func TestPacmanModule_Metadata(t *testing.T) {
	m := New()

	if m.ID() != "pacman" {
		t.Errorf("expected ID 'pacman', got %q", m.ID())
	}
	if m.Name() != "Pacman & AUR" {
		t.Errorf("expected Name 'Pacman & AUR', got %q", m.Name())
	}
	if m.Category() != "package_manager" {
		t.Errorf("expected Category 'package_manager', got %q", m.Category())
	}
	if !m.RequiresElevation() {
		t.Errorf("expected RequiresElevation to be true")
	}
	if m.Description() == "" {
		t.Errorf("expected non-empty description")
	}
}

func TestParseArchPackageName(t *testing.T) {
	tests := []struct {
		filename string
		expected string
		ok       bool
	}{
		{"7zip-26.05-1-x86_64.pkg.tar.zst", "7zip", true},
		{"alsa-card-profiles-1:1.6.10-1-x86_64.pkg.tar.zst", "alsa-card-profiles", true},
		{"linux-cachyos-6.14.3-1-x86_64.pkg.tar.xz", "linux-cachyos", true},
		{"adwaita-cursors-51.0-1-any.pkg.tar.zst", "adwaita-cursors", true},
		{"invalid-file.txt", "", false},
		{"short-1-x86_64.pkg.tar.zst", "", false}, // only 3 parts
	}

	for _, tt := range tests {
		got, ok := parseArchPackageName(tt.filename)
		if ok != tt.ok {
			t.Errorf("parseArchPackageName(%q) ok = %v, expected %v", tt.filename, ok, tt.ok)
		}
		if got != tt.expected {
			t.Errorf("parseArchPackageName(%q) = %q, expected %q", tt.filename, got, tt.expected)
		}
	}
}

func TestPacmanModule_ScanCacheAndBrokenDownloads(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files:
	// pkgA v1 (older) and v2 (newer)
	pkgA1 := filepath.Join(tmpDir, "pkgA-1.0.0-1-x86_64.pkg.tar.zst")
	pkgA1Sig := filepath.Join(tmpDir, "pkgA-1.0.0-1-x86_64.pkg.tar.zst.sig")
	pkgA2 := filepath.Join(tmpDir, "pkgA-2.0.0-1-x86_64.pkg.tar.zst")

	// pkgB v1 (only 1 version, should NOT be cleaned)
	pkgB1 := filepath.Join(tmpDir, "pkgB-1.0.0-1-x86_64.pkg.tar.zst")

	// Broken downloads
	broken1 := filepath.Join(tmpDir, "download-incomplete.part")
	broken2 := filepath.Join(tmpDir, "pkgC-1.0.0-1-x86_64.pkg.tar.zst.part")

	_ = os.WriteFile(pkgA1, []byte("pkgA v1 contents 100 bytes padding"), 0644)
	_ = os.WriteFile(pkgA1Sig, []byte("sig1"), 0644)
	// Make pkgA1 older
	_ = os.Chtimes(pkgA1, time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour))

	_ = os.WriteFile(pkgA2, []byte("pkgA v2 contents 200 bytes"), 0644)
	_ = os.Chtimes(pkgA2, time.Now().Add(-1*time.Hour), time.Now().Add(-1*time.Hour))

	_ = os.WriteFile(pkgB1, []byte("pkgB single version"), 0644)
	_ = os.WriteFile(broken1, []byte("broken1"), 0644)
	_ = os.WriteFile(broken2, []byte("broken2"), 0644)

	m := New()
	m.SetAvailable(true)
	m.SetCacheDir(tmpDir)
	m.SetAURDirs(nil)
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("no command")
	})

	ctx := context.Background()
	res, err := m.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan returned unexpected error: %v", err)
	}

	var foundOldPkgA, foundBroken1, foundBroken2, foundPkgB bool
	for _, item := range res.Items {
		if strings.Contains(item.ID, "pkgA-1.0.0") {
			foundOldPkgA = true
			if item.Safety != core.SafetyCaution {
				t.Errorf("expected SafetyCaution for old package, got %v", item.Safety)
			}
		}
		if strings.Contains(item.ID, "download-incomplete") {
			foundBroken1 = true
			if item.Safety != core.SafetySafe {
				t.Errorf("expected SafetySafe for broken download, got %v", item.Safety)
			}
		}
		if strings.Contains(item.ID, "pkgC") {
			foundBroken2 = true
		}
		if strings.Contains(item.ID, "pkgB") {
			foundPkgB = true
		}
	}

	if !foundOldPkgA {
		t.Errorf("expected to find old pkgA-1.0.0 in scan items, but not found")
	}
	if !foundBroken1 {
		t.Errorf("expected to find broken download 1, but not found")
	}
	if !foundBroken2 {
		t.Errorf("expected to find broken download 2, but not found")
	}
	if foundPkgB {
		t.Errorf("pkgB has only 1 version and should not be flagged for pruning")
	}
}

func TestPacmanModule_ScanOrphansAndAUR(t *testing.T) {
	cacheDir := t.TempDir()
	aurDir := t.TempDir()

	// Mock AUR subdirectories
	paruCloneDir := filepath.Join(aurDir, "paru", "clone")
	_ = os.MkdirAll(filepath.Join(paruCloneDir, "antigravity"), 0755)
	_ = os.WriteFile(filepath.Join(paruCloneDir, "antigravity", "PKGBUILD"), []byte("pkgbuild content"), 0644)

	m := New()
	m.SetAvailable(true)
	m.SetCacheDir(cacheDir)
	m.SetAURDirs([]string{paruCloneDir})

	// Mock pacman command runner
	m.SetCommandRunner(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "pacman" {
			if len(args) == 1 && args[0] == "-Qtdq" {
				return []byte("orphan-lib\nunused-tool\n"), nil
			}
			if len(args) == 2 && args[0] == "-Qi" && args[1] == "orphan-lib" {
				return []byte("Name           : orphan-lib\nInstalled Size : 15.50 MiB\n"), nil
			}
			if len(args) == 2 && args[0] == "-Qi" && args[1] == "unused-tool" {
				return []byte("Name           : unused-tool\nInstalled Size : 2.50 MiB\n"), nil
			}
		}
		return nil, fmt.Errorf("unexpected command: %s %v", name, args)
	})

	ctx := context.Background()
	res, err := m.Scan(ctx)
	if err != nil {
		t.Fatalf("Scan returned unexpected error: %v", err)
	}

	var foundOrphanLib, foundUnusedTool, foundAUR bool
	for _, item := range res.Items {
		if item.ID == "pacman-orphan:orphan-lib" {
			foundOrphanLib = true
			if item.Size == 0 {
				t.Errorf("expected non-zero size for orphan-lib")
			}
		}
		if item.ID == "pacman-orphan:unused-tool" {
			foundUnusedTool = true
		}
		if strings.HasPrefix(item.ID, "aur-cache:clone:antigravity") {
			foundAUR = true
		}
	}

	if !foundOrphanLib {
		t.Errorf("expected pacman-orphan:orphan-lib to be found")
	}
	if !foundUnusedTool {
		t.Errorf("expected pacman-orphan:unused-tool to be found")
	}
	if !foundAUR {
		t.Errorf("expected AUR cache item to be found")
	}
}

func TestPacmanModule_Clean(t *testing.T) {
	cacheDir := t.TempDir()
	aurDir := t.TempDir()

	oldPkg := filepath.Join(cacheDir, "oldpkg-1.0-1-x86_64.pkg.tar.zst")
	oldSig := filepath.Join(cacheDir, "oldpkg-1.0-1-x86_64.pkg.tar.zst.sig")
	_ = os.WriteFile(oldPkg, []byte("old package contents"), 0644)
	_ = os.WriteFile(oldSig, []byte("sig"), 0644)

	aurPkgDir := filepath.Join(aurDir, "some-aur-app")
	_ = os.MkdirAll(aurPkgDir, 0755)
	_ = os.WriteFile(filepath.Join(aurPkgDir, "PKGBUILD"), []byte("pkgbuild"), 0644)

	items := []core.CleanableItem{
		{
			ID:       "pacman-cache:oldpkg-1.0-1-x86_64.pkg.tar.zst",
			Path:     oldPkg,
			Size:     20,
			Selected: true,
			Metadata: map[string]string{
				"sig_file": "oldpkg-1.0-1-x86_64.pkg.tar.zst.sig",
			},
		},
		{
			ID:       "pacman-orphan:orphan-pkg",
			Name:     "Orphan package: orphan-pkg",
			Size:     100,
			Selected: true,
			Metadata: map[string]string{
				"package_name": "orphan-pkg",
			},
		},
		{
			ID:       "aur-cache:clone:some-aur-app",
			Path:     aurPkgDir,
			Size:     50,
			Selected: true,
		},
	}

	var executedCommands [][]string
	runner := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmdLine := append([]string{name}, args...)
		executedCommands = append(executedCommands, cmdLine)
		return []byte("ok"), nil
	}

	m := New()
	m.SetCacheDir(cacheDir)
	m.SetCommandRunner(runner)

	ctx := context.Background()

	// 1. Dry-run test: no files or pacman commands should execute
	dryRes, err := m.Clean(ctx, items, core.CleanOptions{DryRun: true})
	if err != nil {
		t.Fatalf("DryRun returned error: %v", err)
	}
	if len(dryRes.CleanedItems) != len(items) {
		t.Errorf("expected %d dry-run cleaned items, got %d", len(items), len(dryRes.CleanedItems))
	}
	if dryRes.FreedBytes != 170 {
		t.Errorf("expected 170 freed bytes, got %d", dryRes.FreedBytes)
	}
	if len(executedCommands) != 0 {
		t.Errorf("DryRun should not execute any shell commands, executed: %v", executedCommands)
	}
	if _, err := os.Stat(oldPkg); os.IsNotExist(err) {
		t.Errorf("DryRun should not delete files")
	}

	// 2. Real clean test
	cleanRes, err := m.Clean(ctx, items, core.CleanOptions{DryRun: false})
	if err != nil {
		t.Fatalf("Clean returned error: %v", err)
	}
	if len(cleanRes.CleanedItems) != len(items) {
		t.Errorf("expected %d cleaned items, got %d", len(items), len(cleanRes.CleanedItems))
	}
	if len(cleanRes.Errors) != 0 {
		t.Errorf("unexpected errors in clean: %v", cleanRes.Errors)
	}

	// Verify old package file deleted
	if _, err := os.Stat(oldPkg); !os.IsNotExist(err) {
		t.Errorf("expected %s to be deleted", oldPkg)
	}
	// Verify signature deleted
	if _, err := os.Stat(oldSig); !os.IsNotExist(err) {
		t.Errorf("expected %s to be deleted", oldSig)
	}
	// Verify AUR dir deleted
	if _, err := os.Stat(aurPkgDir); !os.IsNotExist(err) {
		t.Errorf("expected %s to be deleted", aurPkgDir)
	}

	// Verify pacman -Rns executed
	if len(executedCommands) != 1 {
		t.Fatalf("expected 1 command executed, got %d", len(executedCommands))
	}
	cmd := executedCommands[0]
	if cmd[0] != "pacman" || cmd[1] != "-Rns" || cmd[2] != "--noconfirm" || cmd[3] != "orphan-pkg" {
		t.Errorf("unexpected command: %v", cmd)
	}
}
