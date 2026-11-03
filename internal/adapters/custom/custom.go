package custom

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/efzynx/sweep/internal/core"
)

const (
	ModuleID       = "custom"
	ModuleName     = "Custom Paths & Rules"
	ModuleCategory = "custom"
	ModuleDesc     = "Clean files in user-configured paths and custom scan rules"
)

// Module implements core.Module for user-defined scan paths and custom cleanup rules.
type Module struct {
	mu    sync.RWMutex
	cfg   *core.Config
	rules []core.CustomScanRule
}

// New creates and returns a new custom rules module.
func New() *Module {
	return &Module{
		rules: make([]core.CustomScanRule, 0),
	}
}

// NewWithConfig creates a new custom rules module initialized with the given configuration.
func NewWithConfig(cfg *core.Config) *Module {
	m := New()
	m.SetConfig(cfg)
	return m
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

// SetConfig sets or updates the configuration for the custom module.
func (m *Module) SetConfig(cfg *core.Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = cfg
	if cfg != nil {
		m.rules = make([]core.CustomScanRule, len(cfg.CustomRules))
		copy(m.rules, cfg.CustomRules)
	}
}

// SetRules directly overrides the custom scan rules.
func (m *Module) SetRules(rules []core.CustomScanRule) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rules = make([]core.CustomScanRule, len(rules))
	copy(m.rules, rules)
}

// Rules returns a copy of the active custom scan rules.
func (m *Module) Rules() []core.CustomScanRule {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.rules) == 0 && m.cfg != nil {
		return m.cfg.CustomRules
	}
	result := make([]core.CustomScanRule, len(m.rules))
	copy(result, m.rules)
	return result
}

// Scan iterates over all configured CustomScanRules and finds cleanable items.
func (m *Module) Scan(ctx context.Context) (core.ScanResult, error) {
	start := time.Now()
	result := core.ScanResult{
		Items:     make([]core.CleanableItem, 0),
		ScannedAt: start,
	}

	rules := m.Rules()
	if len(rules) == 0 {
		return result, nil
	}

	for _, rule := range rules {
		if err := ctx.Err(); err != nil {
			result.Error = err
			return result, err
		}

		items, err := rule.Scan(ctx)
		if err != nil {
			result.Error = err
			return result, err
		}

		result.Items = append(result.Items, items...)
	}

	result.RecalculateTotal()
	return result, nil
}

// Clean removes matched files with full dry-run support and path safety checks.
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

	home, _ := os.UserHomeDir()
	cleanHome := ""
	if home != "" {
		cleanHome = filepath.Clean(home)
	}

	for _, item := range items {
		select {
		case <-ctx.Done():
			cleanRes.Errors = append(cleanRes.Errors, ctx.Err())
			cleanRes.Duration = time.Since(start)
			return cleanRes, ctx.Err()
		default:
		}

		if item.Path == "" {
			continue
		}

		cleanPath := filepath.Clean(item.Path)
		// Safety safeguards: refuse to delete root, dot, or user home directory root
		if cleanPath == "/" || cleanPath == "." || cleanPath == ".." {
			cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("refusing to delete dangerous path: %s", cleanPath))
			continue
		}
		if cleanHome != "" && cleanPath == cleanHome {
			cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("refusing to delete user home directory: %s", cleanPath))
			continue
		}

		if err := os.Remove(cleanPath); err != nil {
			if !os.IsNotExist(err) {
				cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to remove %s: %w", cleanPath, err))
			}
		} else {
			cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
			cleanRes.FreedBytes += item.Size
		}
	}

	cleanRes.Duration = time.Since(start)
	return cleanRes, nil
}
