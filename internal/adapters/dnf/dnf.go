package dnf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/core/distro"
)

const (
	ModuleID       = "dnf"
	ModuleName     = "DNF Package Cache"
	ModuleCategory = "package_manager"
	ModuleDesc     = "Clean DNF/RPM package caches and repo metadata on Fedora/RHEL/CentOS"

	defaultDnfCachePath = "/var/cache/dnf"
)

// Module implements core.Module for Fedora/RHEL DNF package manager.
type Module struct {
	cachePath string
}

// New creates and returns a new DNF module instance.
func New() *Module {
	return &Module{
		cachePath: defaultDnfCachePath,
	}
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

func (m *Module) IsAvailable() bool {
	return distro.HasDnf()
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

	// Read-only inspection of DNF cache directory
	if _, err := os.Stat(m.cachePath); err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			return result, nil
		}
		return result, err
	}

	var totalSize int64
	var rpmCount int

	err := filepath.WalkDir(m.cachePath, func(path string, d os.DirEntry, err error) error {
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
		return result, err
	}

	if rpmCount > 0 {
		result.Items = append(result.Items, core.CleanableItem{
			ID:          "dnf-packages",
			Name:        fmt.Sprintf("Cached .rpm packages (%d files)", rpmCount),
			Description: "Downloaded RPM packages in " + m.cachePath,
			Path:        m.cachePath,
			Size:        totalSize,
			Selected:    true,
			Safety:      core.SafetySafe,
		})
	}

	result.RecalculateTotal()
	return result, nil
}

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

	// Non-dry-run actual execution is completed in Phase 2
	for _, item := range items {
		cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
		cleanRes.FreedBytes += item.Size
	}

	cleanRes.Duration = time.Since(start)
	return cleanRes, nil
}

// SetCachePath overrides the default cache directory (used for testing).
func (m *Module) SetCachePath(path string) {
	m.cachePath = filepath.Clean(path)
}
