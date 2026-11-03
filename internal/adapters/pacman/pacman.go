package pacman

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/core/distro"
)

const (
	ModuleID       = "pacman"
	ModuleName     = "Pacman & AUR"
	ModuleCategory = "package_manager"
	ModuleDesc     = "Clean Pacman package caches, orphan packages, incomplete downloads, and AUR build caches"

	defaultPacmanCacheDir = "/var/cache/pacman/pkg"
)

// CommandRunner defines the function signature for running external CLI commands.
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Module implements core.Module for Arch Linux / CachyOS Pacman and AUR package caches.
type Module struct {
	cacheDir      string
	aurDirs       []string
	commandRunner CommandRunner
}

// New creates and returns a new Pacman module initialized with default system paths.
func New() *Module {
	home := os.Getenv("HOME")
	var aurDirs []string
	if home != "" {
		aurDirs = []string{
			filepath.Join(home, ".cache", "paru", "clone"),
			filepath.Join(home, ".cache", "yay"),
		}
	}

	return &Module{
		cacheDir: defaultPacmanCacheDir,
		aurDirs:  aurDirs,
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
	return distro.HasPacman() || distro.Current().Family == distro.FamilyArch
}

func (m *Module) RequiresElevation() bool {
	return true
}

// pkgFileMeta tracks parsed metadata for package cache files.
type pkgFileMeta struct {
	name        string
	pkgName     string
	fullPath    string
	size        int64
	modTime     time.Time
	sigPath     string
	sigSize     int64
	isCandidate bool
}

// Scan performs read-only detection of old package versions, broken downloads,
// orphan packages, and AUR build caches without requiring elevated privileges.
func (m *Module) Scan(ctx context.Context) (core.ScanResult, error) {
	result := core.ScanResult{
		Items:     make([]core.CleanableItem, 0),
		ScannedAt: time.Now(),
	}

	if !m.IsAvailable() {
		return result, nil
	}

	// 1. Scan /var/cache/pacman/pkg for old versions and broken downloads
	cacheItems, err := m.scanCacheDir(ctx)
	if err == nil {
		result.Items = append(result.Items, cacheItems...)
	}

	// Check context cancellation
	if err := ctx.Err(); err != nil {
		return result, err
	}

	// 2. Scan orphan packages via pacman -Qtdq
	orphanItems, err := m.scanOrphans(ctx)
	if err == nil {
		result.Items = append(result.Items, orphanItems...)
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	// 3. Scan AUR clone caches (Paru, Yay)
	aurItems, err := m.scanAUR(ctx)
	if err == nil {
		result.Items = append(result.Items, aurItems...)
	}

	result.RecalculateTotal()
	return result, nil
}

// scanCacheDir inspects the pacman package cache directory for old package versions
// and incomplete downloads. Non-root readable by default.
func (m *Module) scanCacheDir(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	entries, err := os.ReadDir(m.cacheDir)
	if err != nil {
		// Directory might not exist or be inaccessible in restricted test environments
		return items, nil
	}

	pkgMap := make(map[string][]*pkgFileMeta)
	sigMap := make(map[string]fs.FileInfo)

	// First pass: index signatures and broken downloads
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return items, ctx.Err()
		default:
		}

		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		info, err := entry.Info()
		if err != nil {
			continue
		}

		fullPath := filepath.Join(m.cacheDir, name)

		// Check for broken / incomplete downloads
		if strings.HasPrefix(name, "download-") || strings.HasSuffix(name, ".part") {
			items = append(items, core.CleanableItem{
				ID:          "pacman-broken:" + name,
				Name:        fmt.Sprintf("Broken download: %s", name),
				Description: "Incomplete or broken download artifact in pacman cache",
				Path:        fullPath,
				Size:        info.Size(),
				Selected:    true,
				Safety:      core.SafetySafe,
				ModTime:     info.ModTime(),
			})
			continue
		}

		if strings.HasSuffix(name, ".sig") {
			basePkg := strings.TrimSuffix(name, ".sig")
			sigMap[basePkg] = info
			continue
		}

		pkgName, ok := parseArchPackageName(name)
		if !ok {
			continue
		}

		meta := &pkgFileMeta{
			name:     name,
			pkgName:  pkgName,
			fullPath: fullPath,
			size:     info.Size(),
			modTime:  info.ModTime(),
		}
		pkgMap[pkgName] = append(pkgMap[pkgName], meta)
	}

	// Second pass: associate signatures and identify older versions (keep 1 version by default)
	for pkgName, versions := range pkgMap {
		select {
		case <-ctx.Done():
			return items, ctx.Err()
		default:
		}

		// Associate .sig files
		for _, v := range versions {
			if sigInfo, ok := sigMap[v.name]; ok {
				v.sigPath = filepath.Join(m.cacheDir, sigInfo.Name())
				v.sigSize = sigInfo.Size()
			}
		}

		if len(versions) <= 1 {
			// Only 1 version exists; keep it by default
			continue
		}

		// Sort versions by ModTime descending (newest first)
		sort.Slice(versions, func(i, j int) bool {
			return versions[i].modTime.After(versions[j].modTime)
		})

		// Index 0 is the newest version; prune indices 1..N
		for i := 1; i < len(versions); i++ {
			oldPkg := versions[i]
			totalSize := oldPkg.size + oldPkg.sigSize

			metadata := map[string]string{
				"package": pkgName,
				"file":    oldPkg.name,
			}
			if oldPkg.sigPath != "" {
				metadata["sig_file"] = filepath.Base(oldPkg.sigPath)
			}

			items = append(items, core.CleanableItem{
				ID:          "pacman-cache:" + oldPkg.name,
				Name:        fmt.Sprintf("Old cached package: %s", oldPkg.name),
				Description: fmt.Sprintf("Older version of %s in /var/cache/pacman/pkg", pkgName),
				Path:        oldPkg.fullPath,
				Size:        totalSize,
				Selected:    true,
				Safety:      core.SafetyCaution,
				ModTime:     oldPkg.modTime,
				Metadata:    metadata,
			})
		}
	}

	return items, nil
}

// scanOrphans queries orphan packages via pacman -Qtdq.
func (m *Module) scanOrphans(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	out, err := m.commandRunner(ctx, "pacman", "-Qtdq")
	if err != nil {
		// pacman returns exit code 1 if no orphan packages are found
		return items, nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	var orphanNames []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			orphanNames = append(orphanNames, line)
		}
	}

	if len(orphanNames) == 0 {
		return items, nil
	}

	// Query package size for each orphan
	for _, name := range orphanNames {
		select {
		case <-ctx.Done():
			return items, ctx.Err()
		default:
		}

		var size int64
		qiOut, qiErr := m.commandRunner(ctx, "pacman", "-Qi", name)
		if qiErr == nil {
			size = parsePacmanInstalledSize(string(qiOut))
		}

		items = append(items, core.CleanableItem{
			ID:          "pacman-orphan:" + name,
			Name:        fmt.Sprintf("Orphan package: %s", name),
			Description: fmt.Sprintf("Unneeded dependency package (%s)", name),
			Size:        size,
			Selected:    true,
			Safety:      core.SafetyCaution,
			Metadata: map[string]string{
				"package_name": name,
			},
		})
	}

	return items, nil
}

// scanAUR inspects ~/.cache/paru/clone and ~/.cache/yay directories.
func (m *Module) scanAUR(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	for _, aurDir := range m.aurDirs {
		select {
		case <-ctx.Done():
			return items, ctx.Err()
		default:
		}

		entries, err := os.ReadDir(aurDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			subDirPath := filepath.Join(aurDir, entry.Name())
			dirBytes := calculateDirSize(subDirPath)
			if dirBytes == 0 {
				continue
			}

			items = append(items, core.CleanableItem{
				ID:          fmt.Sprintf("aur-cache:%s:%s", filepath.Base(aurDir), entry.Name()),
				Name:        fmt.Sprintf("AUR build cache: %s (%s)", entry.Name(), filepath.Base(aurDir)),
				Description: fmt.Sprintf("AUR build workspace in %s", subDirPath),
				Path:        subDirPath,
				Size:        dirBytes,
				Selected:    true,
				Safety:      core.SafetySafe,
				Metadata: map[string]string{
					"aur_helper": filepath.Base(aurDir),
					"package":    entry.Name(),
				},
			})
		}
	}

	return items, nil
}

// Clean removes the designated cleanable items. Supports DryRun.
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

	var orphansToRemove []core.CleanableItem

	for _, item := range items {
		select {
		case <-ctx.Done():
			cleanRes.Errors = append(cleanRes.Errors, ctx.Err())
			cleanRes.Duration = time.Since(start)
			return cleanRes, ctx.Err()
		default:
		}

		switch {
		case strings.HasPrefix(item.ID, "pacman-cache:"), strings.HasPrefix(item.ID, "pacman-broken:"):
			if err := os.Remove(item.Path); err != nil && !os.IsNotExist(err) {
				cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to remove %s: %w", item.Path, err))
			} else {
				// Also remove signature file if present
				if sigFile := item.Metadata["sig_file"]; sigFile != "" {
					_ = os.Remove(filepath.Join(m.cacheDir, sigFile))
				}
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
				cleanRes.FreedBytes += item.Size
			}

		case strings.HasPrefix(item.ID, "pacman-orphan:"):
			orphansToRemove = append(orphansToRemove, item)

		case strings.HasPrefix(item.ID, "aur-cache:"):
			if err := os.RemoveAll(item.Path); err != nil && !os.IsNotExist(err) {
				cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to remove AUR cache %s: %w", item.Path, err))
			} else {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
				cleanRes.FreedBytes += item.Size
			}
		}
	}

	// Remove orphans in a single parameterized pacman -Rns call
	if len(orphansToRemove) > 0 {
		args := []string{"-Rns", "--noconfirm"}
		for _, orphan := range orphansToRemove {
			pkg := orphan.Metadata["package_name"]
			if pkg == "" {
				pkg = strings.TrimPrefix(orphan.ID, "pacman-orphan:")
			}
			args = append(args, pkg)
		}

		_, err := m.commandRunner(ctx, "pacman", args...)
		if err != nil {
			cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to remove orphan packages: %w", err))
		} else {
			for _, orphan := range orphansToRemove {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, orphan)
				cleanRes.FreedBytes += orphan.Size
			}
		}
	}

	cleanRes.Duration = time.Since(start)
	return cleanRes, nil
}

// SetCacheDir sets the cache directory for testing.
func (m *Module) SetCacheDir(path string) {
	m.cacheDir = filepath.Clean(path)
}

// SetAURDirs sets the AUR cache directories for testing.
func (m *Module) SetAURDirs(dirs []string) {
	m.aurDirs = dirs
}

// SetCommandRunner overrides the command execution handler for testing.
func (m *Module) SetCommandRunner(runner CommandRunner) {
	m.commandRunner = runner
}

// parseArchPackageName parses an ALPM package filename (e.g., 7zip-26.05-1-x86_64.pkg.tar.zst)
// and extracts the base package name.
func parseArchPackageName(filename string) (string, bool) {
	extensions := []string{
		".pkg.tar.zst",
		".pkg.tar.xz",
		".pkg.tar.gz",
		".pkg.tar.bz2",
		".pkg.tar.lzo",
		".pkg.tar.lrz",
		".pkg.tar.Z",
	}

	trimmed := filename
	hasExt := false
	for _, ext := range extensions {
		if strings.HasSuffix(filename, ext) {
			trimmed = strings.TrimSuffix(filename, ext)
			hasExt = true
			break
		}
	}

	if !hasExt {
		return "", false
	}

	// Format: <pkgname>-<pkgver>-<pkgrel>-<arch>
	// Split by "-" from the right:
	// arch is last, pkgrel is second last, pkgver is third last.
	parts := strings.Split(trimmed, "-")
	if len(parts) < 4 {
		return "", false
	}

	// pkgname is everything before the third-to-last part
	pkgName := strings.Join(parts[:len(parts)-3], "-")
	return pkgName, pkgName != ""
}

// parsePacmanInstalledSize extracts and parses "Installed Size  : 6.47 MiB" from pacman -Qi output.
func parsePacmanInstalledSize(qiOutput string) int64 {
	scanner := bufio.NewScanner(strings.NewReader(qiOutput))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Installed Size") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				sizeStr := strings.TrimSpace(parts[1])
				// Normalize any non-breaking spaces
				sizeStr = strings.ReplaceAll(sizeStr, "\u00a0", " ")
				if size, err := core.ParseBytes(sizeStr); err == nil {
					return size
				}
			}
		}
	}
	return 0
}

// calculateDirSize calculates total size in bytes of all files in a directory.
func calculateDirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}
