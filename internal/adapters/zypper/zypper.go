package zypper

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/core/distro"
)

const (
	ModuleID       = "zypper"
	ModuleName     = "Zypper Package Cache"
	ModuleCategory = "package_manager"
	ModuleDesc     = "Clean Zypper package caches, repo metadata, and unneeded packages on openSUSE/SUSE"
)

var defaultZypperCachePaths = []string{
	"/var/cache/zypp/packages",
	"/var/cache/zypp/raw",
	"/var/cache/zypp/solv",
}

// CommandRunner defines the function signature for running external CLI commands.
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// UnneededPackage holds information about an orphan/unneeded package identified by Zypper.
type UnneededPackage struct {
	Name    string
	Version string
	Repo    string
	Arch    string
	Size    int64
}

// Module implements core.Module for openSUSE/SUSE Zypper package manager.
type Module struct {
	cachePaths        []string
	commandRunner     CommandRunner
	availableOverride *bool
}

// New creates and returns a new Zypper module instance.
func New() *Module {
	paths := make([]string, len(defaultZypperCachePaths))
	copy(paths, defaultZypperCachePaths)

	return &Module{
		cachePaths: paths,
		commandRunner: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
	}
}

func init() {
	_ = core.Register(New())
}

func (m *Module) ID() string {
	return ModuleID
}

func (m *Module) Name() string {
	return ModuleName
}

func (m *Module) Category() string {
	return ModuleCategory
}

func (m *Module) Description() string {
	return ModuleDesc
}

// SetAvailable overrides IsAvailable for testing purposes.
func (m *Module) SetAvailable(available bool) {
	m.availableOverride = &available
}

// SetCachePaths overrides the candidate cache directories for testing purposes.
func (m *Module) SetCachePaths(paths []string) {
	m.cachePaths = paths
}

// SetCommandRunner overrides the command runner for testing purposes.
func (m *Module) SetCommandRunner(runner CommandRunner) {
	m.commandRunner = runner
}

func (m *Module) IsAvailable() bool {
	if m.availableOverride != nil {
		return *m.availableOverride
	}
	if distro.HasZypper() {
		return true
	}
	info := distro.Current()
	return info != nil && info.IsSuse()
}

func (m *Module) RequiresElevation() bool {
	return true
}

func (m *Module) Scan(ctx context.Context) (core.ScanResult, error) {
	result := core.ScanResult{
		Items:     make([]core.CleanableItem, 0),
		ScannedAt: time.Now(),
	}

	if !m.IsAvailable() {
		return result, nil
	}

	// 1. Scan candidate Zypper cache directories (/var/cache/zypp/packages, raw, solv)
	cacheItems, err := m.scanPackageCaches(ctx)
	if err != nil && ctx.Err() != nil {
		return result, err
	}
	result.Items = append(result.Items, cacheItems...)

	if err := ctx.Err(); err != nil {
		return result, err
	}

	// 2. Scan unneeded packages via zypper packages --unneeded
	unneededItems, err := m.scanUnneededPackages(ctx)
	if err != nil && ctx.Err() != nil {
		return result, err
	}
	result.Items = append(result.Items, unneededItems...)

	result.RecalculateTotal()
	return result, nil
}

// scanPackageCaches inspects candidate Zypper cache directories for cached RPM packages.
func (m *Module) scanPackageCaches(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem
	var totalSize int64
	var rpmCount int
	var foundPaths []string

	for _, cPath := range m.cachePaths {
		if _, err := os.Stat(cPath); err != nil {
			continue
		}

		foundPaths = append(foundPaths, cPath)
		err := filepath.WalkDir(cPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if d.IsDir() {
				return nil
			}

			if strings.HasSuffix(d.Name(), ".rpm") {
				if info, err := d.Info(); err == nil {
					totalSize += info.Size()
					rpmCount++
				}
			}
			return nil
		})

		if err != nil && ctx.Err() != nil {
			return items, err
		}
	}

	if rpmCount > 0 {
		firstPath := ""
		if len(foundPaths) > 0 {
			firstPath = foundPaths[0]
		}
		items = append(items, core.CleanableItem{
			ID:          "zypper-packages",
			Name:        fmt.Sprintf("Cached .rpm packages (%d files)", rpmCount),
			Description: "Downloaded RPM packages in " + strings.Join(foundPaths, ", "),
			Path:        firstPath,
			Size:        totalSize,
			Selected:    true,
			Safety:      core.SafetySafe,
			Metadata: map[string]string{
				"type":   "packages",
				"count":  strconv.Itoa(rpmCount),
				"module": ModuleID,
			},
		})
	}

	return items, nil
}

// ParseUnneededPackages parses output from `zypper packages --unneeded`.
func ParseUnneededPackages(output string) []UnneededPackage {
	var list []UnneededPackage

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "--") {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}

		status := strings.TrimSpace(parts[0])
		// Check for installed unneeded indicator: "i" or "i+"
		if status != "i" && status != "i+" {
			continue
		}

		repo := strings.TrimSpace(parts[1])
		name := strings.TrimSpace(parts[2])
		version := strings.TrimSpace(parts[3])
		arch := ""
		if len(parts) >= 5 {
			arch = strings.TrimSpace(parts[4])
		}

		if name == "" || strings.EqualFold(name, "name") {
			continue
		}

		list = append(list, UnneededPackage{
			Name:    name,
			Version: version,
			Repo:    repo,
			Arch:    arch,
			Size:    20 * 1024 * 1024, // 20 MB estimated fallback per package
		})
	}

	return list
}

// scanUnneededPackages detects packages installed as dependencies that are no longer needed.
func (m *Module) scanUnneededPackages(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	if m.commandRunner == nil {
		return items, nil
	}

	out, err := m.commandRunner(ctx, "zypper", "--no-refresh", "packages", "--unneeded")
	if err != nil {
		return items, nil
	}

	unneededList := ParseUnneededPackages(string(out))
	for _, pkg := range unneededList {
		pkgSize := pkg.Size
		// Try to query exact size via rpm if available
		if sizeOut, sizeErr := m.commandRunner(ctx, "rpm", "-q", "--queryformat", "%{SIZE}\n", pkg.Name); sizeErr == nil {
			if s, err := strconv.ParseInt(strings.TrimSpace(string(sizeOut)), 10, 64); err == nil && s > 0 {
				pkgSize = s
			}
		}

		items = append(items, core.CleanableItem{
			ID:          "zypper-unneeded:" + pkg.Name,
			Name:        fmt.Sprintf("Unneeded package: %s (%s)", pkg.Name, pkg.Version),
			Description: fmt.Sprintf("Orphan package not required by any installed software (%s)", pkg.Repo),
			Size:        pkgSize,
			Selected:    false,
			Safety:      core.SafetyCaution,
			Metadata: map[string]string{
				"package_name": pkg.Name,
				"version":      pkg.Version,
				"repo":         pkg.Repo,
				"module":       ModuleID,
			},
		})
	}

	return items, nil
}

// Clean removes cached RPMs or unneeded packages.
func (m *Module) Clean(ctx context.Context, items []core.CleanableItem, opts core.CleanOptions) (core.CleanResult, error) {
	start := time.Now()
	cleanRes := core.CleanResult{
		CleanedItems: make([]core.CleanableItem, 0),
		Errors:       make([]error, 0),
	}

	if len(items) == 0 {
		cleanRes.Duration = time.Since(start)
		return cleanRes, nil
	}

	if opts.DryRun {
		for _, item := range items {
			cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
			cleanRes.FreedBytes += item.Size
		}
		cleanRes.Duration = time.Since(start)
		return cleanRes, nil
	}

	var hasPackageCache bool
	var unneededToRemove []core.CleanableItem

	for _, item := range items {
		select {
		case <-ctx.Done():
			cleanRes.Errors = append(cleanRes.Errors, ctx.Err())
			cleanRes.Duration = time.Since(start)
			return cleanRes, ctx.Err()
		default:
		}

		if item.ID == "zypper-packages" || strings.HasPrefix(item.ID, "zypper-cache:") {
			hasPackageCache = true
			cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
			cleanRes.FreedBytes += item.Size
		} else if strings.HasPrefix(item.ID, "zypper-unneeded:") {
			unneededToRemove = append(unneededToRemove, item)
		}
	}

	// 1. Clean zypper caches
	if hasPackageCache {
		var cleanErr error
		if m.commandRunner != nil {
			_, cleanErr = m.commandRunner(ctx, "zypper", "clean", "--all")
		}
		if cleanErr != nil {
			cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("zypper clean failed: %w", cleanErr))
		}
	}

	// 2. Remove unneeded packages
	if len(unneededToRemove) > 0 {
		var pkgNames []string
		for _, item := range unneededToRemove {
			name := item.Metadata["package_name"]
			if name == "" {
				name = strings.TrimPrefix(item.ID, "zypper-unneeded:")
			}
			pkgNames = append(pkgNames, name)
		}

		args := append([]string{"remove", "--clean-deps", "-y"}, pkgNames...)
		var removeErr error
		if m.commandRunner != nil {
			_, removeErr = m.commandRunner(ctx, "zypper", args...)
		}
		if removeErr != nil {
			cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to remove unneeded packages: %w", removeErr))
		} else {
			for _, item := range unneededToRemove {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
				cleanRes.FreedBytes += item.Size
			}
		}
	}

	cleanRes.Duration = time.Since(start)
	return cleanRes, nil
}
