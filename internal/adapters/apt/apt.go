package apt

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/core/distro"
)

const (
	ModuleID       = "apt"
	ModuleName     = "APT Package Cache"
	ModuleCategory = "package_manager"
	ModuleDesc     = "Clean APT archive cache and orphan packages on Debian/Ubuntu"

	defaultAptCachePath = "/var/cache/apt/archives"
)

// CommandRunner defines the function signature for running external CLI commands.
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Module implements core.Module for Debian/Ubuntu APT package manager.
type Module struct {
	cachePath     string
	commandRunner CommandRunner
}

// New creates and returns a new APT module instance.
func New() *Module {
	return &Module{
		cachePath: defaultAptCachePath,
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

func (m *Module) IsAvailable() bool {
	if distro.HasApt() {
		return true
	}
	info := distro.Current()
	return info != nil && info.Family == distro.FamilyDebian
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

	// 1. Read-only inspection of package archives directory (/var/cache/apt/archives)
	cacheItems, err := m.scanArchiveCache(ctx)
	if err != nil && ctx.Err() != nil {
		return result, err
	}
	result.Items = append(result.Items, cacheItems...)

	if err := ctx.Err(); err != nil {
		return result, err
	}

	// 2. Scan orphan packages via apt-get --dry-run autoremove
	orphanItems, err := m.scanOrphans(ctx)
	if err != nil && ctx.Err() != nil {
		return result, err
	}
	result.Items = append(result.Items, orphanItems...)

	result.RecalculateTotal()
	return result, nil
}

// scanArchiveCache inspects the apt archives directory, excluding lock and partial.
func (m *Module) scanArchiveCache(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	entries, err := os.ReadDir(m.cachePath)
	if err != nil {
		if os.IsNotExist(err) || os.IsPermission(err) {
			return items, nil
		}
		return items, err
	}

	var archiveSize int64
	var count int

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return items, ctx.Err()
		default:
		}

		name := entry.Name()
		// Exclude directories (such as "partial") and lock files
		if entry.IsDir() || name == "lock" || name == "lock-frontend" || !strings.HasSuffix(name, ".deb") {
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
		items = append(items, core.CleanableItem{
			ID:          "apt-archives",
			Name:        fmt.Sprintf("Cached .deb packages (%d files)", count),
			Description: fmt.Sprintf("Downloaded package archives in %s", m.cachePath),
			Path:        m.cachePath,
			Size:        archiveSize,
			Selected:    true,
			Safety:      core.SafetySafe,
			Metadata: map[string]string{
				"type":  "archives",
				"count": strconv.Itoa(count),
			},
		})
	}

	return items, nil
}

// scanOrphans queries unneeded dependency packages using `apt-get --dry-run autoremove`.
func (m *Module) scanOrphans(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	if m.commandRunner == nil {
		return items, nil
	}

	out, err := m.commandRunner(ctx, "apt-get", "--dry-run", "autoremove")
	if err != nil {
		// Non-zero return from apt-get or missing command
		return items, nil
	}

	orphanPkgs, totalFreed := ParseAptAutoremoveOutput(string(out))
	if len(orphanPkgs) == 0 {
		return items, nil
	}

	perItemSize := int64(0)
	if totalFreed > 0 {
		perItemSize = totalFreed / int64(len(orphanPkgs))
	}

	for i, pkg := range orphanPkgs {
		itemSize := perItemSize
		// Add remainder to the last item so the total matches exactly
		if i == len(orphanPkgs)-1 && totalFreed > 0 {
			itemSize = totalFreed - perItemSize*int64(len(orphanPkgs)-1)
		}

		items = append(items, core.CleanableItem{
			ID:          "apt-orphan:" + pkg,
			Name:        fmt.Sprintf("Orphan package: %s", pkg),
			Description: fmt.Sprintf("Unneeded dependency package (%s)", pkg),
			Size:        itemSize,
			Selected:    true,
			Safety:      core.SafetyCaution,
			Metadata: map[string]string{
				"type":    "orphan",
				"package": pkg,
			},
		})
	}

	return items, nil
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

	var hasArchives bool
	var archiveItem core.CleanableItem
	var orphanItems []core.CleanableItem

	for _, item := range items {
		select {
		case <-ctx.Done():
			cleanRes.Errors = append(cleanRes.Errors, ctx.Err())
			cleanRes.Duration = time.Since(start)
			return cleanRes, ctx.Err()
		default:
		}

		if item.ID == "apt-archives" || item.Metadata["type"] == "archives" {
			hasArchives = true
			archiveItem = item
		} else if strings.HasPrefix(item.ID, "apt-orphan:") || item.Metadata["type"] == "orphan" {
			orphanItems = append(orphanItems, item)
		}
	}

	// 1. Clean package archives via apt-get clean & apt-get autoclean
	if hasArchives {
		var cleanErr error
		if m.commandRunner != nil {
			_, err1 := m.commandRunner(ctx, "apt-get", "clean")
			if err1 != nil {
				cleanErr = err1
			}
			_, _ = m.commandRunner(ctx, "apt-get", "autoclean")
		}

		// Also remove files in test or non-default cachePath directly
		if m.cachePath != defaultAptCachePath {
			_ = removeDebFiles(m.cachePath)
		}

		if cleanErr != nil && m.cachePath == defaultAptCachePath {
			cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("apt-get clean failed: %w", cleanErr))
		} else {
			cleanRes.CleanedItems = append(cleanRes.CleanedItems, archiveItem)
			cleanRes.FreedBytes += archiveItem.Size
		}
	}

	// 2. Remove orphan packages via apt-get autoremove -y
	if len(orphanItems) > 0 {
		var autoremoveErr error
		if m.commandRunner != nil {
			_, autoremoveErr = m.commandRunner(ctx, "apt-get", "autoremove", "-y")
		}

		if autoremoveErr != nil {
			cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("apt-get autoremove failed: %w", autoremoveErr))
		} else {
			for _, orphan := range orphanItems {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, orphan)
				cleanRes.FreedBytes += orphan.Size
			}
		}
	}

	cleanRes.Duration = time.Since(start)
	return cleanRes, nil
}

// removeDebFiles removes .deb files from the given directory (used for tests or fallback).
func removeDebFiles(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".deb") {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}

// SetCachePath overrides the default cache directory (used for testing).
func (m *Module) SetCachePath(path string) {
	m.cachePath = filepath.Clean(path)
}

// SetCommandRunner overrides the CLI runner for testing.
func (m *Module) SetCommandRunner(runner CommandRunner) {
	m.commandRunner = runner
}

var (
	// Matches lines like: Remv libfoo [1.0] or Remv libbar-dev
	remvRegex = regexp.MustCompile(`^Remv\s+([^\s:]+)`)
	// Matches lines like: After this operation, 12.3 MB disk space will be freed.
	freedRegex = regexp.MustCompile(`(?i)After this operation,\s*([0-9.,]+)\s*([a-zA-Z]+)\s*(?:of additional\s*)?disk space will be freed`)
)

// ParseAptAutoremoveOutput parses package names and estimated freed size from apt-get --dry-run autoremove.
func ParseAptAutoremoveOutput(output string) ([]string, int64) {
	var packages []string
	pkgSet := make(map[string]struct{})
	var totalFreed int64

	scanner := bufio.NewScanner(strings.NewReader(output))
	inRemovedBlock := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			inRemovedBlock = false
			continue
		}

		// Check for Remv lines: "Remv libfoo [1.0-1] ..."
		if match := remvRegex.FindStringSubmatch(line); len(match) > 1 {
			pkg := match[1]
			if _, exists := pkgSet[pkg]; !exists {
				pkgSet[pkg] = struct{}{}
				packages = append(packages, pkg)
			}
			continue
		}

		// Check for header "The following packages will be REMOVED:"
		if strings.HasPrefix(line, "The following packages will be REMOVED:") {
			inRemovedBlock = true
			continue
		}

		if inRemovedBlock {
			if strings.Contains(line, "upgraded,") || strings.Contains(line, "to remove") {
				inRemovedBlock = false
			} else {
				fields := strings.Fields(line)
				for _, f := range fields {
					cleanName := strings.Trim(f, "*: ")
					if cleanName != "" && !strings.Contains(cleanName, "...") {
						if _, exists := pkgSet[cleanName]; !exists {
							pkgSet[cleanName] = struct{}{}
							packages = append(packages, cleanName)
						}
					}
				}
			}
		}

		// Check for freed space line
		if match := freedRegex.FindStringSubmatch(line); len(match) > 2 {
			totalFreed = parseDiskSize(match[1], match[2])
		}
	}

	return packages, totalFreed
}

// parseDiskSize converts human-readable size strings (e.g. "12.3", "MB") to bytes.
func parseDiskSize(amountStr, unitStr string) int64 {
	cleanAmount := strings.ReplaceAll(amountStr, ",", "")
	val, err := strconv.ParseFloat(cleanAmount, 64)
	if err != nil {
		return 0
	}

	unit := strings.ToLower(strings.TrimSpace(unitStr))
	var multiplier float64

	switch unit {
	case "b", "bytes":
		multiplier = 1
	case "k", "kb", "kib":
		multiplier = 1024
	case "m", "mb", "mib":
		multiplier = 1024 * 1024
	case "g", "gb", "gib":
		multiplier = 1024 * 1024 * 1024
	case "t", "tb", "tib":
		multiplier = 1024 * 1024 * 1024 * 1024
	default:
		multiplier = 1
	}

	return int64(val * multiplier)
}
