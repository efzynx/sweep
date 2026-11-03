package usercache

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/efzynx/sweep/internal/core"
)

const (
	ModuleID       = "usercache"
	ModuleName     = "Desktop & Browser Caches"
	ModuleCategory = "user_cache"
	ModuleDesc     = "Clean desktop trash, thumbnail caches, browser caches (Firefox, Chromium, Chrome), and idle application logs"

	DefaultLogIdleDays = 30
)

// Module implements core.Module for user desktop caches, browser caches, and logs.
type Module struct {
	mu                 sync.RWMutex
	homeDir            string
	trashDir           string
	thumbDir           string
	firefoxCacheDir    string
	firefoxProfileDirs []string
	chromiumCacheDir   string
	chromiumConfigDir  string
	chromeCacheDir     string
	chromeConfigDir    string
	stateLogDir        string
	stateDir           string
	logIdleDays        int
}

// New creates and returns a new User & Desktop Cache module initialized with current user paths.
func New() *Module {
	home, err := core.GetUserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("HOME")
	}
	return NewWithHome(home)
}

// NewWithHome creates a new User Cache module rooted at a custom home directory.
func NewWithHome(home string) *Module {
	if home == "" {
		home = "."
	}
	return &Module{
		homeDir:         home,
		trashDir:        filepath.Join(home, ".local", "share", "Trash"),
		thumbDir:        filepath.Join(home, ".cache", "thumbnails"),
		firefoxCacheDir: filepath.Join(home, ".cache", "mozilla", "firefox"),
		firefoxProfileDirs: []string{
			filepath.Join(home, ".mozilla", "firefox"),
			filepath.Join(home, ".config", "mozilla", "firefox"),
			filepath.Join(home, ".cache", "mozilla", "firefox"),
		},
		chromiumCacheDir:  filepath.Join(home, ".cache", "chromium"),
		chromiumConfigDir: filepath.Join(home, ".config", "chromium"),
		chromeCacheDir:    filepath.Join(home, ".cache", "google-chrome"),
		chromeConfigDir:   filepath.Join(home, ".config", "google-chrome"),
		stateLogDir:       filepath.Join(home, ".local", "state", "log"),
		stateDir:          filepath.Join(home, ".local", "state"),
		logIdleDays:       DefaultLogIdleDays,
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
	return true
}

func (m *Module) RequiresElevation() bool {
	return false
}

// SetLogIdleDays overrides the retention period for idle log files.
func (m *Module) SetLogIdleDays(days int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if days > 0 {
		m.logIdleDays = days
	}
}

// Scan performs a read-only inspection of trash, thumbnails, browser caches, and idle application logs.
func (m *Module) Scan(ctx context.Context) (core.ScanResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := core.ScanResult{
		Items:     make([]core.CleanableItem, 0),
		ScannedAt: time.Now(),
	}

	// 1. Scan Trash Can (~/.local/share/Trash)
	if trashItems, err := m.scanTrash(ctx); err == nil {
		result.Items = append(result.Items, trashItems...)
	} else if ctx.Err() != nil {
		return result, ctx.Err()
	}

	// 2. Scan Desktop Thumbnails (~/.cache/thumbnails)
	if thumbItems, err := m.scanThumbnails(ctx); err == nil {
		result.Items = append(result.Items, thumbItems...)
	} else if ctx.Err() != nil {
		return result, ctx.Err()
	}

	// 3. Scan Mozilla Firefox Caches (~/.cache/mozilla/firefox/*.default*/cache2)
	if ffItems, err := m.scanFirefox(ctx); err == nil {
		result.Items = append(result.Items, ffItems...)
	} else if ctx.Err() != nil {
		return result, ctx.Err()
	}

	// 4. Scan Chromium & Google Chrome Caches
	if chromeItems, err := m.scanChromiumFamily(ctx); err == nil {
		result.Items = append(result.Items, chromeItems...)
	} else if ctx.Err() != nil {
		return result, ctx.Err()
	}

	// 5. Scan General Application Logs (~/.local/state/log and idle *.log)
	if logItems, err := m.scanLogs(ctx); err == nil {
		result.Items = append(result.Items, logItems...)
	} else if ctx.Err() != nil {
		return result, ctx.Err()
	}

	result.RecalculateTotal()
	return result, nil
}

// scanTrash inspects ~/.local/share/Trash/files and info.
func (m *Module) scanTrash(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	filesDir := filepath.Join(m.trashDir, "files")
	infoDir := filepath.Join(m.trashDir, "info")

	size1, count1, mod1, err1 := dirStats(ctx, filesDir)
	if err1 != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	size2, count2, mod2, err2 := dirStats(ctx, infoDir)
	if err2 != nil && ctx.Err() != nil {
		return nil, ctx.Err()
	}

	totalSize := size1 + size2
	totalCount := count1 + count2
	latestMod := mod1
	if mod2.After(latestMod) {
		latestMod = mod2
	}

	if totalCount > 0 || totalSize > 0 {
		items = append(items, core.CleanableItem{
			ID:          "usercache:trash",
			Name:        "Desktop Trash Can",
			Description: fmt.Sprintf("Deleted files and metadata in ~/.local/share/Trash (%d items)", count1),
			Path:        m.trashDir,
			Size:        totalSize,
			Selected:    true,
			Safety:      core.SafetySafe,
			ModTime:     latestMod,
			Metadata: map[string]string{
				"type":       "trash",
				"item_count": fmt.Sprintf("%d", count1),
				"files_dir":  filesDir,
				"info_dir":   infoDir,
			},
		})
	}

	return items, nil
}

// scanThumbnails inspects ~/.cache/thumbnails subdirectories (normal, large, x-large, fail).
func (m *Module) scanThumbnails(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	if _, err := os.Stat(m.thumbDir); err != nil {
		return items, nil
	}

	subdirs := []string{"normal", "large", "x-large", "fail"}
	var totalSize int64
	var totalCount int
	var latestMod time.Time

	for _, sub := range subdirs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		subPath := filepath.Join(m.thumbDir, sub)
		size, count, mod, err := dirStats(ctx, subPath)
		if err != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		totalSize += size
		totalCount += count
		if mod.After(latestMod) {
			latestMod = mod
		}
	}

	// Also check any extra subdirectories under thumbnailsDir (e.g. xx-large)
	if entries, err := os.ReadDir(m.thumbDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			isStandard := false
			for _, sub := range subdirs {
				if name == sub {
					isStandard = true
					break
				}
			}
			if !isStandard {
				subPath := filepath.Join(m.thumbDir, name)
				size, count, mod, _ := dirStats(ctx, subPath)
				totalSize += size
				totalCount += count
				if mod.After(latestMod) {
					latestMod = mod
				}
			}
		}
	}

	if totalCount > 0 || totalSize > 0 {
		items = append(items, core.CleanableItem{
			ID:          "usercache:thumbnails",
			Name:        "Desktop Thumbnails",
			Description: fmt.Sprintf("Cached image and video thumbnails in ~/.cache/thumbnails (%d files)", totalCount),
			Path:        m.thumbDir,
			Size:        totalSize,
			Selected:    true,
			Safety:      core.SafetySafe,
			ModTime:     latestMod,
			Metadata: map[string]string{
				"type":       "thumbnails",
				"file_count": fmt.Sprintf("%d", totalCount),
			},
		})
	}

	return items, nil
}

// scanFirefox inspects ~/.cache/mozilla/firefox/*.default*/cache2 and other profile caches.
func (m *Module) scanFirefox(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	if _, err := os.Stat(m.firefoxCacheDir); err != nil {
		return items, nil
	}

	entries, err := os.ReadDir(m.firefoxCacheDir)
	if err != nil {
		return items, nil
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() {
			continue
		}

		profileName := entry.Name()
		cache2Dir := filepath.Join(m.firefoxCacheDir, profileName, "cache2")
		fi, err := os.Stat(cache2Dir)
		if err != nil || !fi.IsDir() {
			continue
		}

		size, count, mod, err := dirStats(ctx, cache2Dir)
		if err != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if count > 0 || size > 0 {
			locked := m.isFirefoxProfileLocked(profileName)

			items = append(items, core.CleanableItem{
				ID:          fmt.Sprintf("usercache:firefox:%s", profileName),
				Name:        fmt.Sprintf("Firefox Cache (%s)", profileName),
				Description: fmt.Sprintf("Firefox HTTP cache2 for profile %s (%d files)", profileName, count),
				Path:        cache2Dir,
				Size:        size,
				Selected:    !locked,
				Safety:      core.SafetySafe,
				ModTime:     mod,
				Metadata: map[string]string{
					"type":        "browser_cache",
					"browser":     "firefox",
					"profile":     profileName,
					"locked":      fmt.Sprintf("%t", locked),
					"cache2_path": cache2Dir,
				},
			})
		}
	}

	return items, nil
}

// isFirefoxProfileLocked checks if Firefox has an active lockfile for the given profile.
func (m *Module) isFirefoxProfileLocked(profileName string) bool {
	lockNames := []string{"lock", "parent.lock", ".parentlock"}

	for _, pDir := range m.firefoxProfileDirs {
		targetDir := filepath.Join(pDir, profileName)
		for _, lockName := range lockNames {
			lockPath := filepath.Join(targetDir, lockName)
			if _, err := os.Lstat(lockPath); err == nil {
				return true
			}
		}
	}

	// Also check directly inside cache profile dir
	for _, lockName := range lockNames {
		lockPath := filepath.Join(m.firefoxCacheDir, profileName, lockName)
		if _, err := os.Lstat(lockPath); err == nil {
			return true
		}
	}

	return false
}

// scanChromiumFamily scans Chromium and Google Chrome cache directories.
func (m *Module) scanChromiumFamily(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	targets := []struct {
		browser   string
		name      string
		cacheRoot string
		configDir string
	}{
		{
			browser:   "chromium",
			name:      "Chromium",
			cacheRoot: m.chromiumCacheDir,
			configDir: m.chromiumConfigDir,
		},
		{
			browser:   "google-chrome",
			name:      "Google Chrome",
			cacheRoot: m.chromeCacheDir,
			configDir: m.chromeConfigDir,
		},
	}

	for _, t := range targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if _, err := os.Stat(t.cacheRoot); err != nil {
			continue
		}

		locked := m.isChromiumFamilyLocked(t.configDir)

		// Discover profile subdirectories in cache (e.g. Default, Profile 1, etc.)
		profileDirs := []string{"Default"}
		if entries, err := os.ReadDir(t.cacheRoot); err == nil {
			for _, e := range entries {
				if e.IsDir() && e.Name() != "Default" && (strings.HasPrefix(e.Name(), "Profile ") || e.Name() == "System Profile") {
					profileDirs = append(profileDirs, e.Name())
				}
			}
		}

		for _, p := range profileDirs {
			cachePath := filepath.Join(t.cacheRoot, p, "Cache")
			if fi, err := os.Stat(cachePath); err == nil && fi.IsDir() {
				size, count, mod, err := dirStats(ctx, cachePath)
				if err != nil && ctx.Err() != nil {
					return nil, ctx.Err()
				}

				if count > 0 || size > 0 {
					items = append(items, core.CleanableItem{
						ID:          fmt.Sprintf("usercache:%s:%s", t.browser, strings.ToLower(p)),
						Name:        fmt.Sprintf("%s Cache (%s)", t.name, p),
						Description: fmt.Sprintf("%s HTTP cache (%s, %d files)", t.name, cachePath, count),
						Path:        cachePath,
						Size:        size,
						Selected:    !locked,
						Safety:      core.SafetySafe,
						ModTime:     mod,
						Metadata: map[string]string{
							"type":       "browser_cache",
							"browser":    t.browser,
							"profile":    p,
							"locked":     fmt.Sprintf("%t", locked),
							"cache_path": cachePath,
						},
					})
				}
			}
		}
	}

	return items, nil
}

// isChromiumFamilyLocked checks if Chromium or Chrome SingletonLock is present.
func (m *Module) isChromiumFamilyLocked(configDir string) bool {
	lockFiles := []string{
		filepath.Join(configDir, "SingletonLock"),
		filepath.Join(configDir, "Default", "SingletonLock"),
		filepath.Join(configDir, "SingletonCookie"),
	}
	for _, lf := range lockFiles {
		if _, err := os.Lstat(lf); err == nil {
			return true
		}
	}
	return false
}

// scanLogs inspects ~/.local/state/log and idle *.log files > 30 days old.
func (m *Module) scanLogs(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	idleThreshold := time.Duration(m.logIdleDays) * 24 * time.Hour
	cutoff := time.Now().Add(-idleThreshold)

	seenPaths := make(map[string]bool)

	// 1. Check ~/.local/state/log specifically
	if fi, err := os.Stat(m.stateLogDir); err == nil && fi.IsDir() {
		_ = filepath.WalkDir(m.stateLogDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if info, err := d.Info(); err == nil {
				if info.ModTime().Before(cutoff) {
					seenPaths[p] = true
					items = append(items, core.CleanableItem{
						ID:          fmt.Sprintf("usercache:log:%s", filepath.Base(p)),
						Name:        fmt.Sprintf("Idle Log: %s", filepath.Base(p)),
						Description: fmt.Sprintf("Application log idle > %d days (%s)", m.logIdleDays, p),
						Path:        p,
						Size:        info.Size(),
						Selected:    false,
						Safety:      core.SafetyCaution,
						ModTime:     info.ModTime(),
						Metadata: map[string]string{
							"type":      "app_log",
							"idle_days": fmt.Sprintf("%d", m.logIdleDays),
						},
					})
				}
			}
			return nil
		})
	}

	// 2. Check ~/.local/state for idle *.log files (bounded depth to avoid unbounded traversal)
	if fi, err := os.Stat(m.stateDir); err == nil && fi.IsDir() {
		stateDepth := strings.Count(m.stateDir, string(filepath.Separator))
		_ = filepath.WalkDir(m.stateDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			currentDepth := strings.Count(p, string(filepath.Separator))
			if d.IsDir() {
				if currentDepth-stateDepth > 3 {
					return filepath.SkipDir
				}
				return nil
			}

			if strings.HasSuffix(d.Name(), ".log") && !seenPaths[p] {
				if info, err := d.Info(); err == nil {
					if info.ModTime().Before(cutoff) {
						seenPaths[p] = true
						items = append(items, core.CleanableItem{
							ID:          fmt.Sprintf("usercache:log:%s", filepath.Base(p)),
							Name:        fmt.Sprintf("Idle Log: %s", filepath.Base(p)),
							Description: fmt.Sprintf("Application log idle > %d days (%s)", m.logIdleDays, p),
							Path:        p,
							Size:        info.Size(),
							Selected:    false,
							Safety:      core.SafetyCaution,
							ModTime:     info.ModTime(),
							Metadata: map[string]string{
								"type":      "app_log",
								"idle_days": fmt.Sprintf("%d", m.logIdleDays),
							},
						})
					}
				}
			}
			return nil
		})
	}

	return items, nil
}

// Clean executes the cleanup of selected items respecting DryRun and browser lock guards.
func (m *Module) Clean(ctx context.Context, items []core.CleanableItem, opts core.CleanOptions) (core.CleanResult, error) {
	start := time.Now()
	cleanRes := core.CleanResult{
		CleanedItems: make([]core.CleanableItem, 0),
		Errors:       make([]error, 0),
	}

	// DryRun mode: simulate cleanup without deleting any files
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

		switch {
		case item.ID == "usercache:trash":
			filesDir := filepath.Join(m.trashDir, "files")
			infoDir := filepath.Join(m.trashDir, "info")
			expungedDir := filepath.Join(m.trashDir, "expunged")

			var cleanErr error
			if err := cleanDirContents(filesDir); err != nil && !os.IsNotExist(err) {
				cleanErr = err
			}
			if err := cleanDirContents(infoDir); err != nil && !os.IsNotExist(err) {
				cleanErr = err
			}
			if err := cleanDirContents(expungedDir); err != nil && !os.IsNotExist(err) {
				cleanErr = err
			}

			if cleanErr != nil {
				cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to clean trash: %w", cleanErr))
			} else {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
				cleanRes.FreedBytes += item.Size
			}

		case item.ID == "usercache:thumbnails":
			var cleanErr error
			if entries, err := os.ReadDir(m.thumbDir); err == nil {
				for _, e := range entries {
					if e.IsDir() {
						sub := filepath.Join(m.thumbDir, e.Name())
						if err := cleanDirContents(sub); err != nil && !os.IsNotExist(err) {
							cleanErr = err
						}
					}
				}
			}
			if cleanErr != nil {
				cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to clean thumbnails: %w", cleanErr))
			} else {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
				cleanRes.FreedBytes += item.Size
			}

		case strings.HasPrefix(item.ID, "usercache:firefox:"):
			profileName := item.Metadata["profile"]
			if m.isFirefoxProfileLocked(profileName) {
				cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("skipped Firefox cache for profile %q: browser is locked/running", profileName))
				continue
			}

			if err := cleanDirContents(item.Path); err != nil && !os.IsNotExist(err) {
				cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to clean Firefox cache %s: %w", item.Path, err))
			} else {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
				cleanRes.FreedBytes += item.Size
			}

		case strings.HasPrefix(item.ID, "usercache:chromium:"):
			if m.isChromiumFamilyLocked(m.chromiumConfigDir) {
				cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("skipped Chromium cache %s: browser is locked/running", item.Path))
				continue
			}

			if err := cleanDirContents(item.Path); err != nil && !os.IsNotExist(err) {
				cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to clean Chromium cache %s: %w", item.Path, err))
			} else {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
				cleanRes.FreedBytes += item.Size
			}

		case strings.HasPrefix(item.ID, "usercache:google-chrome:"):
			if m.isChromiumFamilyLocked(m.chromeConfigDir) {
				cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("skipped Google Chrome cache %s: browser is locked/running", item.Path))
				continue
			}

			if err := cleanDirContents(item.Path); err != nil && !os.IsNotExist(err) {
				cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to clean Chrome cache %s: %w", item.Path, err))
			} else {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
				cleanRes.FreedBytes += item.Size
			}

		case strings.HasPrefix(item.ID, "usercache:log:"):
			if err := os.Remove(item.Path); err != nil && !os.IsNotExist(err) {
				cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to remove log file %s: %w", item.Path, err))
			} else {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
				cleanRes.FreedBytes += item.Size
			}

		default:
			// Fallback generic file or directory cleaner
			fi, err := os.Lstat(item.Path)
			if err != nil {
				if os.IsNotExist(err) {
					cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
					cleanRes.FreedBytes += item.Size
				} else {
					cleanRes.Errors = append(cleanRes.Errors, err)
				}
				continue
			}

			if fi.IsDir() {
				if err := cleanDirContents(item.Path); err != nil {
					cleanRes.Errors = append(cleanRes.Errors, err)
				} else {
					cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
					cleanRes.FreedBytes += item.Size
				}
			} else {
				if err := os.Remove(item.Path); err != nil {
					cleanRes.Errors = append(cleanRes.Errors, err)
				} else {
					cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
					cleanRes.FreedBytes += item.Size
				}
			}
		}
	}

	cleanRes.Duration = time.Since(start)
	return cleanRes, nil
}

// cleanDirContents unlinks all contents of dir without deleting dir itself.
func cleanDirContents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var firstErr error
	for _, entry := range entries {
		subPath := filepath.Join(dir, entry.Name())
		if err := os.RemoveAll(subPath); err != nil && !os.IsNotExist(err) {
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	return firstErr
}

// dirStats calculates the total size and file count of a directory.
func dirStats(ctx context.Context, dir string) (int64, int, time.Time, error) {
	var totalSize int64
	var totalCount int
	var latestMod time.Time

	fi, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, latestMod, nil
		}
		return 0, 0, latestMod, err
	}
	latestMod = fi.ModTime()

	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				totalSize += info.Size()
				totalCount++
				if info.ModTime().After(latestMod) {
					latestMod = info.ModTime()
				}
			}
		}
		return nil
	})

	return totalSize, totalCount, latestMod, err
}
