package gui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/efzynx/sweep/internal/adapters"
	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/core/distro"
)

// GUIModuleSummary provides frontend-ready metadata about a registered subsystem module.
type GUIModuleSummary struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Category          string `json:"category"`
	Description       string `json:"description"`
	Available         bool   `json:"available"`
	RequiresElevation bool   `json:"requires_elevation"`
	Enabled           bool   `json:"enabled"`
}

// GUICleanResult encapsulates execution results with stringified errors for reliable JSON serialization over Wails.
type GUICleanResult struct {
	CleanedItems []core.CleanableItem `json:"cleaned_items"`
	FreedBytes   int64                `json:"freed_bytes"`
	Errors       []string             `json:"errors"`
	DurationMs   int64                `json:"duration_ms"`
	Success      bool                 `json:"success"`
}

// App is the Go desktop application bridge connecting the Sweep Core Engine
// to the Wails presentation layer.
type App struct {
	ctx         context.Context
	cancel      context.CancelFunc
	reg         *core.Registry
	cfg         *core.Config
	storageInfo core.SystemStorageInfo
	version     string
	mu          sync.RWMutex
}

// NewApp creates a new uninitialized App instance.
func NewApp() *App {
	return &App{
		version: "1.0.1-beta",
	}
}

// SetVersion sets the app version string displayed in the UI.
func (a *App) SetVersion(v string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.version = v
}

// GetAppVersion returns the application release version.
func (a *App) GetAppVersion() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.version != "" {
		return a.version
	}
	return "1.0.1-beta"
}

// NewAppWithDependencies creates a new App instance pre-populated with a registry and config.
func NewAppWithDependencies(reg *core.Registry, cfg *core.Config) *App {
	return &App{
		reg: reg,
		cfg: cfg,
	}
}

// Startup is called at Wails application startup. It detects storage metrics,
// loads system/user configuration, and registers default adapters.
func (a *App) Startup(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.ctx, a.cancel = context.WithCancel(ctx)

	if a.reg == nil {
		a.reg = core.NewRegistry()
		adapters.RegisterDefaultAdapters(a.reg)
	}

	if a.cfg == nil {
		cfg, err := core.LoadConfig("")
		if err != nil {
			cfg = core.DefaultConfig()
		}
		a.cfg = cfg
	}

	for _, mod := range a.reg.Modules() {
		if cm, ok := mod.(interface{ SetConfig(*core.Config) }); ok {
			cm.SetConfig(a.cfg)
		}
	}

	sInfo, err := core.DetectSystemStorage()
	if err == nil {
		a.storageInfo = sInfo
	}
}

// Shutdown is called at Wails application shutdown for clean cancellation.
func (a *App) Shutdown(ctx context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
}

// GetStorageInfo returns the detected storage allocation metrics for root and home.
func (a *App) GetStorageInfo() (core.SystemStorageInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	info, err := core.DetectSystemStorage()
	if err == nil {
		a.storageInfo = info
		return info, nil
	}

	if a.storageInfo.Root.Path != "" {
		return a.storageInfo, nil
	}

	return a.storageInfo, err
}

// GetDistroInfo returns detected Linux distribution family and release metadata.
func (a *App) GetDistroInfo() (*distro.DistroInfo, error) {
	info := distro.Current()
	if info != nil {
		return info, nil
	}
	return distro.Detect()
}

// GetModules returns summaries of all registered subsystem modules.
func (a *App) GetModules() ([]GUIModuleSummary, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.reg == nil {
		return nil, errors.New("module registry not initialized")
	}

	mods := a.reg.Modules()
	summaries := make([]GUIModuleSummary, 0, len(mods))
	for _, m := range mods {
		enabled := true
		if a.cfg != nil {
			enabled = a.cfg.IsModuleEnabled(m.ID())
		}
		summaries = append(summaries, GUIModuleSummary{
			ID:                m.ID(),
			Name:              m.Name(),
			Category:          m.Category(),
			Description:       m.Description(),
			Available:         m.IsAvailable(),
			RequiresElevation: m.RequiresElevation(),
			Enabled:           enabled,
		})
	}

	return summaries, nil
}

// ScanAll executes read-only inspection concurrently across all enabled modules.
func (a *App) ScanAll() (map[string]core.ScanResult, error) {
	a.mu.RLock()
	reg := a.reg
	cfg := a.cfg
	sInfo := a.storageInfo
	ctx := a.ctx
	a.mu.RUnlock()

	if reg == nil {
		return nil, errors.New("module registry not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	modules := reg.EnabledModules(cfg)
	results, err := reg.ScanModules(ctx, modules)
	if err != nil && ctx.Err() != nil {
		return nil, err
	}

	for modID, res := range results {
		for i := range res.Items {
			if res.Items[i].Metadata == nil {
				res.Items[i].Metadata = make(map[string]string)
			}
			res.Items[i].Metadata["module"] = modID
			core.TagItemStorage(&res.Items[i], sInfo)
			res.Items[i].Metadata["storage_target"] = res.Items[i].Metadata["storage"]
		}
		results[modID] = res
	}

	return results, nil
}

// ScanModule executes read-only inspection on a specific module by ID.
func (a *App) ScanModule(id string) (core.ScanResult, error) {
	a.mu.RLock()
	reg := a.reg
	sInfo := a.storageInfo
	ctx := a.ctx
	a.mu.RUnlock()

	if reg == nil {
		return core.ScanResult{}, errors.New("module registry not initialized")
	}
	mod, ok := reg.Get(id)
	if !ok {
		return core.ScanResult{}, fmt.Errorf("module not found: %s", id)
	}
	if !mod.IsAvailable() {
		return core.ScanResult{}, fmt.Errorf("module %s is not available on this system", id)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	res, err := mod.Scan(ctx)
	if err != nil && res.Error == nil {
		res.Error = err
	}
	if res.ScannedAt.IsZero() {
		res.ScannedAt = time.Now()
	}
	if res.TotalSize == 0 && len(res.Items) > 0 {
		res.RecalculateTotal()
	}

	for i := range res.Items {
		if res.Items[i].Metadata == nil {
			res.Items[i].Metadata = make(map[string]string)
		}
		res.Items[i].Metadata["module"] = id
		core.TagItemStorage(&res.Items[i], sInfo)
		res.Items[i].Metadata["storage_target"] = res.Items[i].Metadata["storage"]
	}

	return res, err
}

// CleanItems executes cleanup for the provided items, dispatching them to their respective modules.
func (a *App) CleanItems(items []core.CleanableItem, dryRun bool, snapshotSafeguard bool) (GUICleanResult, error) {
	a.mu.RLock()
	reg := a.reg
	ctx := a.ctx
	a.mu.RUnlock()

	if reg == nil {
		return GUICleanResult{}, errors.New("module registry not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	start := time.Now()
	combinedRes := core.CleanResult{
		CleanedItems: make([]core.CleanableItem, 0),
		Errors:       make([]error, 0),
	}

	if len(items) == 0 {
		return GUICleanResult{
			CleanedItems: []core.CleanableItem{},
			Errors:       []string{},
			DurationMs:   time.Since(start).Milliseconds(),
			Success:      true,
		}, nil
	}

	// 1. Snapper safeguard snapshot if requested and not in dry-run mode
	if snapshotSafeguard && !dryRun {
		if mod, ok := reg.Get("snapper"); ok && mod.IsAvailable() {
			if sm, ok := mod.(interface{ CreateSafeguardSnapshot(string) error }); ok {
				if err := sm.CreateSafeguardSnapshot("Sweep pre-clean safeguard snapshot"); err != nil {
					combinedRes.Errors = append(combinedRes.Errors, fmt.Errorf("snapper safeguard snapshot: %w", err))
				}
			}
		}
	}

	// 2. Group items by their responsible module
	itemsByModule := make(map[string][]core.CleanableItem)
	registeredMods := reg.Modules()

	for _, item := range items {
		modID := ""
		if item.Metadata != nil {
			modID = item.Metadata["module"]
		}
		if modID == "" {
			for _, m := range registeredMods {
				mid := m.ID()
				if item.ID == mid ||
					strings.HasPrefix(item.ID, mid+":") ||
					strings.HasPrefix(item.ID, mid+"-") ||
					strings.HasPrefix(item.ID, mid+"_") {
					modID = mid
					break
				}
			}
		}
		if modID == "" {
			if _, ok := reg.Get("custom"); ok {
				modID = "custom"
			} else {
				combinedRes.Errors = append(combinedRes.Errors, fmt.Errorf("unable to determine module for item: %s", item.ID))
				continue
			}
		}
		itemsByModule[modID] = append(itemsByModule[modID], item)
	}

	// 3. Clean items per module
	opts := core.CleanOptions{
		DryRun:            dryRun,
		SnapshotSafeguard: false, // Handled above globally
	}

	for modID, modItems := range itemsByModule {
		select {
		case <-ctx.Done():
			combinedRes.Errors = append(combinedRes.Errors, ctx.Err())
			errStrs := make([]string, 0, len(combinedRes.Errors))
			for _, e := range combinedRes.Errors {
				if e != nil {
					errStrs = append(errStrs, e.Error())
				}
			}
			return GUICleanResult{
				CleanedItems: combinedRes.CleanedItems,
				FreedBytes:   combinedRes.FreedBytes,
				Errors:       errStrs,
				DurationMs:   time.Since(start).Milliseconds(),
				Success:      false,
			}, ctx.Err()
		default:
		}

		mod, ok := reg.Get(modID)
		if !ok {
			combinedRes.Errors = append(combinedRes.Errors, fmt.Errorf("module %s not found in registry", modID))
			continue
		}

		res, err := mod.Clean(ctx, modItems, opts)
		if err != nil {
			combinedRes.Errors = append(combinedRes.Errors, fmt.Errorf("%s: %w", modID, err))
		}
		if len(res.Errors) > 0 {
			combinedRes.Errors = append(combinedRes.Errors, res.Errors...)
		}

		if len(res.CleanedItems) > 0 {
			combinedRes.CleanedItems = append(combinedRes.CleanedItems, res.CleanedItems...)
		} else if err == nil && len(res.Errors) == 0 {
			combinedRes.CleanedItems = append(combinedRes.CleanedItems, modItems...)
		}

		if res.FreedBytes > 0 {
			combinedRes.FreedBytes += res.FreedBytes
		} else if err == nil && len(res.Errors) == 0 {
			for _, it := range modItems {
				combinedRes.FreedBytes += it.Size
			}
		}
	}

	errStrs := make([]string, 0, len(combinedRes.Errors))
	for _, e := range combinedRes.Errors {
		if e != nil {
			errStrs = append(errStrs, e.Error())
		}
	}

	return GUICleanResult{
		CleanedItems: combinedRes.CleanedItems,
		FreedBytes:   combinedRes.FreedBytes,
		Errors:       errStrs,
		DurationMs:   time.Since(start).Milliseconds(),
		Success:      len(errStrs) == 0,
	}, nil
}

// AddCustomRule validates and adds a custom scan rule to configuration and persists it.
func (a *App) AddCustomRule(name, path, pattern string, recursive bool, safety string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cfg == nil {
		return errors.New("configuration not loaded")
	}

	rule := core.CustomScanRule{
		Name:      strings.TrimSpace(name),
		Path:      strings.TrimSpace(path),
		Pattern:   strings.TrimSpace(pattern),
		Recursive: recursive,
		Safety:    core.SafetyLevel(strings.ToUpper(strings.TrimSpace(safety))),
	}

	if err := a.cfg.AddCustomRule(rule); err != nil {
		return err
	}

	if a.reg != nil {
		if mod, ok := a.reg.Get("custom"); ok {
			if cm, ok := mod.(interface{ SetConfig(*core.Config) }); ok {
				cm.SetConfig(a.cfg)
			}
		}
	}

	return nil
}

// RemoveCustomRule deletes a custom scan rule by ID and persists the updated configuration.
func (a *App) RemoveCustomRule(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cfg == nil {
		return errors.New("configuration not loaded")
	}

	if err := a.cfg.RemoveCustomRule(id); err != nil {
		return err
	}

	if a.reg != nil {
		if mod, ok := a.reg.Get("custom"); ok {
			if cm, ok := mod.(interface{ SetConfig(*core.Config) }); ok {
				cm.SetConfig(a.cfg)
			}
		}
	}

	return nil
}

// GetConfig returns the active application configuration.
func (a *App) GetConfig() (*core.Config, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.cfg == nil {
		return nil, errors.New("configuration not loaded")
	}
	return a.cfg, nil
}

// UpdateConfig updates general and retention configuration options and persists to disk.
func (a *App) UpdateConfig(cfg core.Config) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cfg == nil {
		a.cfg = core.DefaultConfig()
	}

	a.cfg.General = cfg.General
	a.cfg.Retention = cfg.Retention
	a.cfg.DryRunDefault = cfg.General.DryRunDefault
	a.cfg.SnapshotSafeguard = cfg.General.SnapshotSafeguard
	if len(cfg.Modules) > 0 {
		a.cfg.Modules = cfg.Modules
	}

	if err := a.cfg.Save(""); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	if a.reg != nil {
		for _, mod := range a.reg.Modules() {
			if cm, ok := mod.(interface{ SetConfig(*core.Config) }); ok {
				cm.SetConfig(a.cfg)
			}
		}
	}

	return nil
}

// MinimizeWindow minimizes the application window.
func (a *App) MinimizeWindow() {
	a.mu.RLock()
	ctx := a.ctx
	a.mu.RUnlock()

	if ctx != nil {
		wailsruntime.WindowMinimise(ctx)
	}
}

// ToggleMaximizeWindow toggles between maximized and restored window states.
func (a *App) ToggleMaximizeWindow() {
	a.mu.RLock()
	ctx := a.ctx
	a.mu.RUnlock()

	if ctx != nil {
		wailsruntime.WindowToggleMaximise(ctx)
	}
}

// QuitApp cleanly terminates the desktop application.
func (a *App) QuitApp() {
	a.mu.RLock()
	ctx := a.ctx
	a.mu.RUnlock()

	if ctx != nil {
		wailsruntime.Quit(ctx)
	}
}
