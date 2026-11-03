package apt

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
	ModuleID       = "apt"
	ModuleName     = "APT Package Cache"
	ModuleCategory = "package_manager"
	ModuleDesc     = "Clean APT archive cache and downloaded package archives on Debian/Ubuntu"

	defaultAptCachePath = "/var/cache/apt/archives"
)

// Module implements core.Module for Debian/Ubuntu APT package manager.
type Module struct {
	cachePath string
}

// New creates and returns a new APT module instance.
func New() *Module {
	return &Module{
		cachePath: defaultAptCachePath,
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
	return distro.HasApt()
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

	// Read-only inspection of package archives directory
	entries, err := os.ReadDir(m.cachePath)
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			// Inaccessible or non-existent cache directory
			return result, nil
		}
		return result, err
	}

	var archiveSize int64
	var count int

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".deb") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		archiveSize += info.Size()
		count++
	}

	if count > 0 {
		result.Items = append(result.Items, core.CleanableItem{
			ID:          "apt-archives",
			Name:        fmt.Sprintf("Cached .deb packages (%d files)", count),
			Description: "Downloaded package archives in /var/cache/apt/archives",
			Path:        m.cachePath,
			Size:        archiveSize,
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
