package tui

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/core/distro"
)

// CategoryPriority returns ordinal sorting rank for subsystem categories.
func CategoryPriority(cat string) int {
	title := formatCategoryTitle(cat)
	switch title {
	case "PACKAGE MANAGERS":
		return 1
	case "CONTAINERS & VIRTUALIZATION":
		return 2
	case "DEVELOPER CACHES":
		return 3
	case "SYSTEM & DESKTOP":
		return 4
	case "FILESYSTEM & SNAPSHOTS":
		return 5
	case "USER CUSTOM RULES":
		return 6
	default:
		return 7
	}
}

// leftPaneVisibleRows computes the available vertical content rows for the left tree pane.
func (m *Model) leftPaneVisibleRows() int {
	headerHeight := lipgloss.Height(m.renderHeader())
	footerHeight := lipgloss.Height(m.renderFooter())
	mainHeight := m.height - headerHeight - footerHeight
	if mainHeight < 4 {
		mainHeight = 4
	}
	paneContentHeight := mainHeight - 2
	if paneContentHeight < 1 {
		paneContentHeight = 1
	}
	avail := paneContentHeight - 1 // 1 row for SUBSYSTEMS title
	if avail < 1 {
		avail = 1
	}
	return avail
}

// ViewMode represents the currently active full-screen or overlay mode.
type ViewMode int

const (
	ViewModeMain ViewMode = iota
	ViewModeModal
	ViewModeAbout
	ViewModeCustomRuleForm
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

	// Custom Rule Form State
	inputRuleName    textinput.Model
	inputRulePath    textinput.Model
	inputRulePattern textinput.Model
	formRecursive    bool
	formSafety       core.SafetyLevel
	formFocusIndex   int // 0: Name, 1: Path, 2: Pattern, 3: Recursive, 4: Safety, 5: Submit
	formError        string

	// Distro & System Metrics
	distroInfo    *distro.DistroInfo
	diskStats     DiskStats
	storageInfo   core.SystemStorageInfo
	storageFilter core.StorageScope

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
	sInfo, _ := core.DetectSystemStorage()

	for _, mod := range reg.Modules() {
		if cm, ok := mod.(interface{ SetConfig(*core.Config) }); ok {
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

	inName := textinput.New()
	inName.Placeholder = "Temporary Build Artifacts"
	inName.CharLimit = 80
	inName.Width = 44

	inPath := textinput.New()
	inPath.Placeholder = "~/tmp/builds or /var/tmp/build"
	inPath.CharLimit = 120
	inPath.Width = 44

	inPat := textinput.New()
	inPat.Placeholder = "* or *.tmp"
	inPat.SetValue("*")
	inPat.CharLimit = 60
	inPat.Width = 44

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
		inputRuleName:            inName,
		inputRulePath:            inPath,
		inputRulePattern:         inPat,
		formRecursive:            true,
		formSafety:               core.SafetySafe,
		formFocusIndex:           0,
		formError:                "",
		distroInfo:               dInfo,
		diskStats:                dStats,
		storageInfo:              sInfo,
		storageFilter:            core.StorageScopeAll,
		spinner:                  s,
		progress:                 prog,
		logs:                     make([]string, 0),
		cleanErrors:              make([]string, 0),
	}
}

// initCustomRuleForm resets and initializes the custom rule modal input fields.
func (m *Model) initCustomRuleForm() {
	m.inputRuleName = textinput.New()
	m.inputRuleName.Placeholder = "Temporary Build Artifacts"
	m.inputRuleName.Prompt = "> "
	m.inputRuleName.PromptStyle = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	m.inputRuleName.TextStyle = lipgloss.NewStyle().Foreground(ColorFgWhite)
	m.inputRuleName.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorSubtle)
	m.inputRuleName.CharLimit = 80
	m.inputRuleName.Width = 44
	m.inputRuleName.Focus()

	m.inputRulePath = textinput.New()
	m.inputRulePath.Placeholder = "~/tmp/builds or /var/tmp/build"
	m.inputRulePath.Prompt = "  "
	m.inputRulePath.PromptStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	m.inputRulePath.TextStyle = lipgloss.NewStyle().Foreground(ColorFgWhite)
	m.inputRulePath.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorSubtle)
	m.inputRulePath.CharLimit = 120
	m.inputRulePath.Width = 44

	m.inputRulePattern = textinput.New()
	m.inputRulePattern.Placeholder = "* or *.tmp"
	m.inputRulePattern.SetValue("*")
	m.inputRulePattern.Prompt = "  "
	m.inputRulePattern.PromptStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	m.inputRulePattern.TextStyle = lipgloss.NewStyle().Foreground(ColorFgWhite)
	m.inputRulePattern.PlaceholderStyle = lipgloss.NewStyle().Foreground(ColorSubtle)
	m.inputRulePattern.CharLimit = 60
	m.inputRulePattern.Width = 44

	m.formRecursive = true
	m.formSafety = core.SafetySafe
	m.formFocusIndex = 0
	m.formError = ""
	m.viewMode = ViewModeCustomRuleForm
}

// updateFormFocus updates the focus state and prompts of text input fields based on formFocusIndex.
func (m *Model) updateFormFocus() {
	switch m.formFocusIndex {
	case 0:
		m.inputRuleName.Focus()
		m.inputRuleName.Prompt = "> "
		m.inputRuleName.PromptStyle = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)

		m.inputRulePath.Blur()
		m.inputRulePath.Prompt = "  "
		m.inputRulePath.PromptStyle = lipgloss.NewStyle().Foreground(ColorMuted)

		m.inputRulePattern.Blur()
		m.inputRulePattern.Prompt = "  "
		m.inputRulePattern.PromptStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	case 1:
		m.inputRuleName.Blur()
		m.inputRuleName.Prompt = "  "
		m.inputRuleName.PromptStyle = lipgloss.NewStyle().Foreground(ColorMuted)

		m.inputRulePath.Focus()
		m.inputRulePath.Prompt = "> "
		m.inputRulePath.PromptStyle = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)

		m.inputRulePattern.Blur()
		m.inputRulePattern.Prompt = "  "
		m.inputRulePattern.PromptStyle = lipgloss.NewStyle().Foreground(ColorMuted)

	case 2:
		m.inputRuleName.Blur()
		m.inputRuleName.Prompt = "  "
		m.inputRuleName.PromptStyle = lipgloss.NewStyle().Foreground(ColorMuted)

		m.inputRulePath.Blur()
		m.inputRulePath.Prompt = "  "
		m.inputRulePath.PromptStyle = lipgloss.NewStyle().Foreground(ColorMuted)

		m.inputRulePattern.Focus()
		m.inputRulePattern.Prompt = "> "
		m.inputRulePattern.PromptStyle = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)

	default:
		m.inputRuleName.Blur()
		m.inputRuleName.Prompt = "  "
		m.inputRuleName.PromptStyle = lipgloss.NewStyle().Foreground(ColorMuted)

		m.inputRulePath.Blur()
		m.inputRulePath.Prompt = "  "
		m.inputRulePath.PromptStyle = lipgloss.NewStyle().Foreground(ColorMuted)

		m.inputRulePattern.Blur()
		m.inputRulePattern.Prompt = "  "
		m.inputRulePattern.PromptStyle = lipgloss.NewStyle().Foreground(ColorMuted)
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

// ToggleSelectAllGlobal toggles selection state across all modules, respecting active storageFilter.
func (m *Model) ToggleSelectAllGlobal() {
	if m.storageFilter == core.StorageScopeAll {
		allSel := m.AllSelected()
		newVal := !allSel
		for _, mod := range m.modules {
			mod.ScanRes.SetAllSelected(newVal)
		}
		return
	}
	allSel := m.AllFilteredSelected()
	newVal := !allSel
	for _, mod := range m.modules {
		indices := m.FilteredItemIndices(mod)
		for _, idx := range indices {
			mod.ScanRes.Items[idx].Selected = newVal
		}
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

// StorageFilter returns the active storage filter scope.
func (m *Model) StorageFilter() core.StorageScope {
	return m.storageFilter
}

// SetStorageFilter sets the active storage filter scope.
func (m *Model) SetStorageFilter(scope core.StorageScope) {
	m.storageFilter = scope
}

// StorageInfo returns the system storage metrics.
func (m *Model) StorageInfo() core.SystemStorageInfo {
	return m.storageInfo
}

// SetStorageInfo sets the system storage metrics.
func (m *Model) SetStorageInfo(info core.SystemStorageInfo) {
	m.storageInfo = info
}

// cycleStorageFilter cycles the cleanup target between All, Root, and Home.
// ScopeAll -> ScopeRoot -> ScopeHome -> ScopeAll
func (m *Model) cycleStorageFilter() {
	switch m.storageFilter {
	case core.StorageScopeAll:
		m.storageFilter = core.StorageScopeRoot
	case core.StorageScopeRoot:
		m.storageFilter = core.StorageScopeHome
	case core.StorageScopeHome:
		m.storageFilter = core.StorageScopeAll
	default:
		m.storageFilter = core.StorageScopeAll
	}

	if active := m.ActiveModule(); active != nil {
		m.adjustItemScroll(active)
	}
}

// FilteredItemIndices returns indices of items in mod.ScanRes.Items that match storageFilter.
func (m *Model) FilteredItemIndices(mod *ModuleItem) []int {
	if mod == nil {
		return nil
	}
	var indices []int
	for i := range mod.ScanRes.Items {
		item := &mod.ScanRes.Items[i]
		if item.Metadata == nil || item.Metadata["storage"] == "" {
			core.TagItemStorage(item, m.storageInfo)
		}
		scope := core.ItemStorageScope(*item)
		if m.storageFilter == core.StorageScopeAll || scope == m.storageFilter {
			indices = append(indices, i)
		}
	}
	return indices
}

// ModuleFilteredBytes computes the total reclaimable bytes of items matching storageFilter in mod.
func (m *Model) ModuleFilteredBytes(mod *ModuleItem) int64 {
	if mod == nil {
		return 0
	}
	indices := m.FilteredItemIndices(mod)
	var total int64
	for _, idx := range indices {
		total += mod.ScanRes.Items[idx].Size
	}
	return total
}

// ModuleFilteredCount returns the number of items matching storageFilter in mod.
func (m *Model) ModuleFilteredCount(mod *ModuleItem) int {
	return len(m.FilteredItemIndices(mod))
}

// ModuleFilteredSelectedBytes computes selected bytes matching storageFilter in mod.
func (m *Model) ModuleFilteredSelectedBytes(mod *ModuleItem) int64 {
	if mod == nil {
		return 0
	}
	indices := m.FilteredItemIndices(mod)
	var total int64
	for _, idx := range indices {
		if mod.ScanRes.Items[idx].Selected {
			total += mod.ScanRes.Items[idx].Size
		}
	}
	return total
}

// ModuleFilteredSelectedCount returns the number of selected items matching storageFilter in mod.
func (m *Model) ModuleFilteredSelectedCount(mod *ModuleItem) int {
	if mod == nil {
		return 0
	}
	indices := m.FilteredItemIndices(mod)
	var count int
	for _, idx := range indices {
		if mod.ScanRes.Items[idx].Selected {
			count++
		}
	}
	return count
}

// isModuleFilteredAllSelected reports whether all items matching storageFilter in mod are selected.
func (m *Model) isModuleFilteredAllSelected(mod *ModuleItem) bool {
	indices := m.FilteredItemIndices(mod)
	if len(indices) == 0 {
		return false
	}
	for _, idx := range indices {
		if !mod.ScanRes.Items[idx].Selected {
			return false
		}
	}
	return true
}

// isModuleFilteredAnySelected reports whether at least one item matching storageFilter in mod is selected.
func (m *Model) isModuleFilteredAnySelected(mod *ModuleItem) bool {
	indices := m.FilteredItemIndices(mod)
	for _, idx := range indices {
		if mod.ScanRes.Items[idx].Selected {
			return true
		}
	}
	return false
}

// ToggleSelectFilteredModule toggles selection for all items matching storageFilter in mod.
func (m *Model) ToggleSelectFilteredModule(mod *ModuleItem) {
	if mod == nil {
		return
	}
	indices := m.FilteredItemIndices(mod)
	if len(indices) == 0 {
		return
	}
	allSel := true
	for _, idx := range indices {
		if !mod.ScanRes.Items[idx].Selected {
			allSel = false
			break
		}
	}
	for _, idx := range indices {
		mod.ScanRes.Items[idx].Selected = !allSel
	}
}

// AllFilteredSelected reports whether all filtered cleanable items are selected across all modules.
func (m *Model) AllFilteredSelected() bool {
	scanned := 0
	selected := 0
	for _, mod := range m.modules {
		indices := m.FilteredItemIndices(mod)
		scanned += len(indices)
		for _, idx := range indices {
			if mod.ScanRes.Items[idx].Selected {
				selected++
			}
		}
	}
	if scanned == 0 {
		return false
	}
	return selected == scanned
}

// SelectedStorageBytes computes the breakdown of selected bytes for Home and Root filesystems.
func (m *Model) SelectedStorageBytes() (homeBytes int64, rootBytes int64) {
	for _, mod := range m.modules {
		for i := range mod.ScanRes.Items {
			it := &mod.ScanRes.Items[i]
			if it.Selected {
				if it.Metadata == nil || it.Metadata["storage"] == "" {
					core.TagItemStorage(it, m.storageInfo)
				}
				scope := core.ItemStorageScope(*it)
				if scope == core.StorageScopeHome {
					homeBytes += it.Size
				} else {
					rootBytes += it.Size
				}
			}
		}
	}
	return homeBytes, rootBytes
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
	stats       DiskStats
	storageInfo core.SystemStorageInfo
	err         error
}
