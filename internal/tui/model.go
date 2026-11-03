package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/core/distro"
)

// CategoryPriority returns ordinal sorting rank for subsystem categories.
func CategoryPriority(cat string) int {
	switch strings.ToLower(strings.TrimSpace(cat)) {
	case "package_manager", "sandboxed_apps", "pkg", "pacman", "flatpak", "apt", "dnf":
		return 1
	case "containers", "docker", "podman":
		return 2
	case "snapshots", "filesystem", "system", "snapper", "btrfs":
		return 3
	default:
		return 4
	}
}

// ViewMode represents the currently active full-screen or overlay mode.
type ViewMode int

const (
	ViewModeMain ViewMode = iota
	ViewModeModal
	ViewModeAbout
	ViewModeExecuting
	ViewModeDone
)

// PaneFocus indicates which pane currently has keyboard focus in the split view.
type PaneFocus int

const (
	FocusLeft  PaneFocus = iota // Left pane: Modules list
	FocusRight                  // Right pane: Items inspector
)

// ModalType specifies the confirmation context when ViewModeModal is active.
type ModalType int

const (
	ModalTypeDryRun ModalType = iota
	ModalTypeClean
)

// ModuleItem represents a registered module and its scanned state in the TUI.
type ModuleItem struct {
	Module       core.Module
	ScanRes      core.ScanResult
	Scanning     bool
	Error        error
	Cursor       int // Active item cursor index in right pane
	ScrollOffset int // Viewport vertical scroll offset
}

// TotalBytes returns the total reclaimable bytes discovered for this module.
func (mi *ModuleItem) TotalBytes() int64 {
	return mi.ScanRes.TotalSize
}

// SelectedBytes returns the byte sum of currently selected items in this module.
func (mi *ModuleItem) SelectedBytes() int64 {
	return mi.ScanRes.SelectedSize()
}

// SelectedCount returns the number of selected items in this module.
func (mi *ModuleItem) SelectedCount() int {
	var count int
	for _, it := range mi.ScanRes.Items {
		if it.Selected {
			count++
		}
	}
	return count
}

// ItemCount returns total items found for this module.
func (mi *ModuleItem) ItemCount() int {
	return len(mi.ScanRes.Items)
}

// IsAllSelected returns true if all items in this module are selected.
func (mi *ModuleItem) IsAllSelected() bool {
	if len(mi.ScanRes.Items) == 0 {
		return false
	}
	for _, it := range mi.ScanRes.Items {
		if !it.Selected {
			return false
		}
	}
	return true
}

// IsAnySelected returns true if at least one item is selected.
func (mi *ModuleItem) IsAnySelected() bool {
	for _, it := range mi.ScanRes.Items {
		if it.Selected {
			return true
		}
	}
	return false
}

// ToggleSelectAll toggles selection of all items in this module.
func (mi *ModuleItem) ToggleSelectAll() {
	allSel := mi.IsAllSelected()
	mi.ScanRes.SetAllSelected(!allSel)
}

// Model encapsulates the complete interactive state for Sweep TUI.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc
	reg    *core.Registry
	cfg    *core.Config

	// Window dimensions
	width  int
	height int

	// Navigation & Focus
	viewMode        ViewMode
	focus           PaneFocus
	activeModuleIdx int
	moduleScrollOff int
	modules         []*ModuleItem
	scanningAll     bool

	// Modals & Options
	modalType                ModalType
	snapperAvailable         bool
	snapperSafeguardSnapshot bool

	// Distro & System Metrics
	distroInfo *distro.DistroInfo
	diskStats  DiskStats

	// Execution & Logging
	spinner          spinner.Model
	progress         progress.Model
	executingDryRun  bool
	executingStepIdx int
	executionQueue   []int // module indices with selected items
	logs             []string
	cleanedItems     int
	freedBytes       int64
	cleanErrors      []string
	startTime        time.Time
	duration         time.Duration
	quitting         bool
}

// NewModel instantiates a new TUI Model initialized with available modules and configuration.
func NewModel(ctx context.Context, reg *core.Registry, cfg *core.Config) Model {
	if reg == nil {
		reg = core.DefaultRegistry
	}
	if cfg == nil {
		cfg = core.DefaultConfig()
	}

	subCtx, cancel := context.WithCancel(ctx)

	s := spinner.New()
	s.Spinner = spinner.Line
	s.Style = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)

	prog := progress.New(
		progress.WithDefaultGradient(),
		progress.WithSolidFill("#06B6D4"),
		progress.WithoutPercentage(),
	)

	dInfo := distro.Current()
	hasSnapper := distro.HasSnapper()
	dStats, _ := GetRootDiskStats()

	if customMod, ok := reg.Get("custom"); ok {
		if cm, ok := customMod.(interface{ SetConfig(*core.Config) }); ok {
			cm.SetConfig(cfg)
		}
	}

	availMods := reg.EnabledModules(cfg)
	items := make([]*ModuleItem, 0, len(availMods))
	for _, mod := range availMods {
		items = append(items, &ModuleItem{
			Module: mod,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return CategoryPriority(items[i].Module.Category()) < CategoryPriority(items[j].Module.Category())
	})

	return Model{
		ctx:                      subCtx,
		cancel:                   cancel,
		reg:                      reg,
		cfg:                      cfg,
		width:                    100,
		height:                   30,
		viewMode:                 ViewModeMain,
		focus:                    FocusLeft,
		activeModuleIdx:          0,
		modules:                  items,
		scanningAll:              false,
		modalType:                ModalTypeDryRun,
		snapperAvailable:         hasSnapper,
		snapperSafeguardSnapshot: hasSnapper && cfg.General.SnapshotSafeguard,
		distroInfo:               dInfo,
		diskStats:                dStats,
		spinner:                  s,
		progress:                 prog,
		logs:                     make([]string, 0),
		cleanErrors:              make([]string, 0),
	}
}

// ActiveModule returns the currently selected module item or nil if none.
func (m *Model) ActiveModule() *ModuleItem {
	if len(m.modules) == 0 || m.activeModuleIdx < 0 || m.activeModuleIdx >= len(m.modules) {
		return nil
	}
	return m.modules[m.activeModuleIdx]
}

// TotalSelectedCount computes the total number of selected items across all modules.
func (m *Model) TotalSelectedCount() int {
	var total int
	for _, mod := range m.modules {
		total += mod.SelectedCount()
	}
	return total
}

// TotalSelectedBytes computes the total reclaimable bytes across all selected items.
func (m *Model) TotalSelectedBytes() int64 {
	var total int64
	for _, mod := range m.modules {
		total += mod.SelectedBytes()
	}
	return total
}

// TotalScannedBytes computes the total reclaimable bytes across all scanned items.
func (m *Model) TotalScannedBytes() int64 {
	var total int64
	for _, mod := range m.modules {
		total += mod.TotalBytes()
	}
	return total
}

// TotalScannedItems computes the total item count across all modules.
func (m *Model) TotalScannedItems() int {
	var total int
	for _, mod := range m.modules {
		total += mod.ItemCount()
	}
	return total
}

// AllSelected reports whether all cleanable items in all modules are selected.
func (m *Model) AllSelected() bool {
	scanned := m.TotalScannedItems()
	if scanned == 0 {
		return false
	}
	return m.TotalSelectedCount() == scanned
}

// ToggleSelectAllGlobal toggles selection state across all modules.
func (m *Model) ToggleSelectAllGlobal() {
	allSel := m.AllSelected()
	newVal := !allSel
	for _, mod := range m.modules {
		mod.ScanRes.SetAllSelected(newVal)
	}
}

// HighestSelectedSafety finds the highest risk level among selected items.
func (m *Model) HighestSelectedSafety() core.SafetyLevel {
	highest := core.SafetySafe
	for _, mod := range m.modules {
		for _, it := range mod.ScanRes.Items {
			if !it.Selected {
				continue
			}
			if it.Safety == core.SafetyHigh {
				return core.SafetyHigh
			}
			if it.Safety == core.SafetyCaution {
				highest = core.SafetyCaution
			}
		}
	}
	return highest
}

// AppendLog adds a timestamped log line to the execution log stream.
func (m *Model) AppendLog(line string) {
	ts := time.Now().Format("15:04:05")
	m.logs = append(m.logs, fmt.Sprintf("[%s] %s", ts, line))
}

// Messages used by the Bubbletea event loop
type msgScanResult struct {
	results map[string]core.ScanResult
	err     error
}

type msgCleanStep struct {
	moduleIdx int
	modName   string
	cleaned   int
	freed     int64
	err       error
}

type msgCleanDone struct {
	duration time.Duration
}

type msgDiskStats struct {
	stats DiskStats
	err   error
}
