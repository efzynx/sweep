package devcache

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/efzynx/sweep/internal/core"
)

const (
	ModuleID       = "devcache"
	ModuleName     = "Developer Build Caches"
	ModuleCategory = "devcache"
	ModuleDesc     = "Clean build caches, package registries, and compiler artifacts for Node.js, Go, Rust, and Python"
)

// Target represents a specific developer cache location and its metadata.
type Target struct {
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Ecosystem   string           `json:"ecosystem"`
	Path        string           `json:"path"`
	Safety      core.SafetyLevel `json:"safety"`
	Description string           `json:"description"`
}

// Module implements core.Module for developer build and package caches.
type Module struct {
	mu      sync.RWMutex
	homeDir string
	targets []Target
	cfg     *core.Config
}

// New creates and returns a new Developer Build Caches module with default system targets.
func New() *Module {
	home := os.Getenv("HOME")
	if home == "" {
		if uHome, err := os.UserHomeDir(); err == nil {
			home = uHome
		}
	}

	m := &Module{
		homeDir: home,
	}
	m.targets = m.defaultTargets(home)
	return m
}

// NewWithConfig creates a new devcache module initialized with configuration.
func NewWithConfig(cfg *core.Config) *Module {
	m := New()
	m.SetConfig(cfg)
	return m
}

func init() {
	_ = core.Register(New())
}

// defaultTargets constructs the standard dev cache targets based on home and environment.
func (m *Module) defaultTargets(home string) []Target {
	if home == "" {
		return nil
	}

	// 1. Go targets
	goBuildPath := filepath.Join(home, ".cache", "go-build")
	if gocache := os.Getenv("GOCACHE"); gocache != "" && m.homeDir == home {
		goBuildPath = gocache
	}

	goModPath := filepath.Join(home, "go", "pkg", "mod", "cache")
	if gopath := os.Getenv("GOPATH"); gopath != "" && m.homeDir == home {
		goModPath = filepath.Join(gopath, "pkg", "mod", "cache")
	}

	// 2. Cargo / Rust targets
	cargoRegistry := filepath.Join(home, ".cargo", "registry", "cache")
	cargoGit := filepath.Join(home, ".cargo", "git", "db")
	if cargoHome := os.Getenv("CARGO_HOME"); cargoHome != "" && m.homeDir == home {
		cargoRegistry = filepath.Join(cargoHome, "registry", "cache")
		cargoGit = filepath.Join(cargoHome, "git", "db")
	}

	// 3. Python targets
	pipCache := filepath.Join(home, ".cache", "pip")
	uvCache := filepath.Join(home, ".cache", "uv")
	if uvCacheEnv := os.Getenv("UV_CACHE_DIR"); uvCacheEnv != "" && m.homeDir == home {
		uvCache = uvCacheEnv
	}

	return []Target{
		// Node.js & JavaScript
		{
			ID:          "devcache:npm",
			Name:        "npm Global Cache",
			Ecosystem:   "node",
			Path:        filepath.Join(home, ".npm", "_cacache"),
			Safety:      core.SafetySafe,
			Description: "npm package cache (~/.npm/_cacache)",
		},
		{
			ID:          "devcache:yarn",
			Name:        "Yarn Cache",
			Ecosystem:   "node",
			Path:        filepath.Join(home, ".yarn", "cache"),
			Safety:      core.SafetySafe,
			Description: "Yarn v2+ cache (~/.yarn/cache)",
		},
		{
			ID:          "devcache:yarn-classic",
			Name:        "Yarn Classic Cache",
			Ecosystem:   "node",
			Path:        filepath.Join(home, ".cache", "yarn"),
			Safety:      core.SafetySafe,
			Description: "Yarn v1 cache (~/.cache/yarn)",
		},
		{
			ID:          "devcache:pnpm",
			Name:        "pnpm Virtual Store",
			Ecosystem:   "node",
			Path:        filepath.Join(home, ".local", "share", "pnpm", "store"),
			Safety:      core.SafetySafe,
			Description: "pnpm store (~/.local/share/pnpm/store)",
		},
		{
			ID:          "devcache:pnpm-store",
			Name:        "pnpm Legacy Store",
			Ecosystem:   "node",
			Path:        filepath.Join(home, ".pnpm-store"),
			Safety:      core.SafetySafe,
			Description: "pnpm store (~/.pnpm-store)",
		},

		// Go
		{
			ID:          "devcache:go-build",
			Name:        "Go Build Cache",
			Ecosystem:   "go",
			Path:        goBuildPath,
			Safety:      core.SafetySafe,
			Description: "Go compiler & test build cache (~/.cache/go-build)",
		},
		{
			ID:          "devcache:go-mod",
			Name:        "Go Module Cache [CAUTION]",
			Ecosystem:   "go",
			Path:        goModPath,
			Safety:      core.SafetyCaution,
			Description: "Go module download cache (~/go/pkg/mod/cache)",
		},

		// Rust / Cargo
		{
			ID:          "devcache:cargo-registry",
			Name:        "Cargo Registry Cache",
			Ecosystem:   "rust",
			Path:        cargoRegistry,
			Safety:      core.SafetySafe,
			Description: "Cargo registry crate tarballs cache (~/.cargo/registry/cache)",
		},
		{
			ID:          "devcache:cargo-git",
			Name:        "Cargo Git Database",
			Ecosystem:   "rust",
			Path:        cargoGit,
			Safety:      core.SafetySafe,
			Description: "Cargo git repository checkouts database (~/.cargo/git/db)",
		},

		// Python
		{
			ID:          "devcache:pip",
			Name:        "pip Cache",
			Ecosystem:   "python",
			Path:        pipCache,
			Safety:      core.SafetySafe,
			Description: "pip download and wheel cache (~/.cache/pip)",
		},
		{
			ID:          "devcache:uv",
			Name:        "uv Cache",
			Ecosystem:   "python",
			Path:        uvCache,
			Safety:      core.SafetySafe,
			Description: "uv package and wheel cache (~/.cache/uv)",
		},
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

func (m *Module) RequiresElevation() bool {
	return false
}

// IsAvailable returns true if at least one dev cache directory exists
// or if any developer tool CLI is installed.
func (m *Module) IsAvailable() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, target := range m.targets {
		if fi, err := os.Stat(target.Path); err == nil && fi.IsDir() {
			return true
		}
	}

	clis := []string{"go", "npm", "yarn", "pnpm", "cargo", "pip", "pip3", "uv", "python", "python3"}
	for _, cli := range clis {
		if _, err := exec.LookPath(cli); err == nil {
			return true
		}
	}

	return false
}

// SetHomeDir overrides the base home directory and updates default targets.
func (m *Module) SetHomeDir(dir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.homeDir = dir
	m.targets = m.defaultTargets(dir)
}

// SetTargets directly overrides the active target list.
func (m *Module) SetTargets(targets []Target) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.targets = make([]Target, len(targets))
	copy(m.targets, targets)
}

// Targets returns a snapshot of the active devcache targets.
func (m *Module) Targets() []Target {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]Target, len(m.targets))
	copy(res, m.targets)
	return res
}

// SetConfig sets the configuration pointer for the module.
func (m *Module) SetConfig(cfg *core.Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
}

// Scan performs a read-only scan across developer build cache directories.
func (m *Module) Scan(ctx context.Context) (core.ScanResult, error) {
	result := core.ScanResult{
		Items:     make([]core.CleanableItem, 0),
		ScannedAt: time.Now(),
	}

	if !m.IsAvailable() {
		return result, nil
	}

	targets := m.Targets()
	if len(targets) == 0 {
		return result, nil
	}

	type scanOutput struct {
		item core.CleanableItem
		ok   bool
		err  error
		idx  int
	}

	outChan := make(chan scanOutput, len(targets))
	var wg sync.WaitGroup

	for i, target := range targets {
		if err := ctx.Err(); err != nil {
			return result, err
		}

		fi, err := os.Stat(target.Path)
		if err != nil || !fi.IsDir() {
			continue
		}

		wg.Add(1)
		go func(idx int, t Target, dirInfo os.FileInfo) {
			defer wg.Done()

			size, count, modTime, scanErr := scanDirStats(ctx, t.Path, dirInfo.ModTime())
			if scanErr != nil {
				outChan <- scanOutput{err: scanErr, idx: idx}
				return
			}

			if count == 0 && size == 0 {
				return
			}

			selected := t.Safety == core.SafetySafe

			item := core.CleanableItem{
				ID:          t.ID,
				Name:        t.Name,
				Description: fmt.Sprintf("%s (%d files)", t.Description, count),
				Path:        t.Path,
				Size:        size,
				Selected:    selected,
				Safety:      t.Safety,
				ModTime:     modTime,
				Metadata: map[string]string{
					"ecosystem": t.Ecosystem,
					"files":     strconv.Itoa(count),
					"path":      t.Path,
					"safety":    string(t.Safety),
				},
			}

			outChan <- scanOutput{item: item, ok: true, idx: idx}
		}(i, target, fi)
	}

	wg.Wait()
	close(outChan)

	var outputs []scanOutput
	for out := range outChan {
		if out.err != nil {
			if errors.Is(out.err, context.Canceled) || errors.Is(out.err, context.DeadlineExceeded) {
				return result, out.err
			}
			continue
		}
		if out.ok {
			outputs = append(outputs, out)
		}
	}

	// Sort items according to original target definition order
	sort.Slice(outputs, func(i, j int) bool {
		return outputs[i].idx < outputs[j].idx
	})

	for _, out := range outputs {
		result.Items = append(result.Items, out.item)
	}

	result.RecalculateTotal()
	return result, nil
}

// scanDirStats recursively calculates the total size and file count of a directory.
func scanDirStats(ctx context.Context, dir string, defaultMod time.Time) (int64, int, time.Time, error) {
	var totalSize int64
	var fileCount int
	latestMod := defaultMod

	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				totalSize += info.Size()
				fileCount++
				if info.ModTime().After(latestMod) {
					latestMod = info.ModTime()
				}
			}
		}
		return nil
	})

	return totalSize, fileCount, latestMod, err
}

// Clean unlinks the contents of selected developer cache directories.
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

	for _, item := range items {
		select {
		case <-ctx.Done():
			cleanRes.Errors = append(cleanRes.Errors, ctx.Err())
			cleanRes.Duration = time.Since(start)
			return cleanRes, ctx.Err()
		default:
		}

		if err := cleanTarget(item.Path); err != nil && !os.IsNotExist(err) {
			cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to clean %s (%s): %w", item.Name, item.Path, err))
		} else {
			cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
			cleanRes.FreedBytes += item.Size
		}
	}

	cleanRes.Duration = time.Since(start)
	return cleanRes, nil
}

// cleanTarget unlinks directory contents or deletes an individual file.
func cleanTarget(path string) error {
	_ = os.Chmod(path, 0o700)
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if !fi.IsDir() {
		return safeRemoveAll(path)
	}

	return cleanDirContents(path)
}

// cleanDirContents removes all files and subdirectories inside dir without deleting dir itself.
func cleanDirContents(dir string) error {
	_ = os.Chmod(dir, 0o700)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		subPath := filepath.Join(dir, entry.Name())
		if err := safeRemoveAll(subPath); err != nil {
			return err
		}
	}

	return nil
}

// safeRemoveAll removes path, proactively chmodding write permissions if read-only (such as Go mod cache).
func safeRemoveAll(path string) error {
	err := os.RemoveAll(path)
	if err == nil || os.IsNotExist(err) {
		return nil
	}

	// On permission error, recursively grant write and exec permissions, then retry removal.
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr == nil {
			_ = os.Chmod(p, 0o700)
		}
		return nil
	})

	return os.RemoveAll(path)
}
