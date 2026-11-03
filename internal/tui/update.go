package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/efzynx/sweep/internal/core"
)

// Init initializes the Bubbletea application by starting the spinner and scanning modules.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.scanAllCmd(),
		m.refreshDiskStatsCmd(),
	)
}

// Update processes incoming Bubbletea messages and drives state transitions.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		progWidth := msg.Width - 14
		if progWidth < 10 {
			progWidth = 10
		}
		m.progress.Width = progWidth
		m.adjustModuleScroll()
		if active := m.ActiveModule(); active != nil {
			m.adjustItemScroll(active)
		}
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case msgScanResult:
		m.scanningAll = false
		for _, mod := range m.modules {
			mod.Scanning = false
			if res, ok := msg.results[mod.Module.ID()]; ok {
				mod.ScanRes = res
				mod.Error = res.Error
				if mod.Cursor >= len(mod.ScanRes.Items) {
					if len(mod.ScanRes.Items) > 0 {
						mod.Cursor = len(mod.ScanRes.Items) - 1
					} else {
						mod.Cursor = 0
					}
				}
				// Safe by default: select safe items initially, and tag storage scope
				for i := range mod.ScanRes.Items {
					core.TagItemStorage(&mod.ScanRes.Items[i], m.storageInfo)
					if mod.ScanRes.Items[i].Safety == core.SafetySafe {
						mod.ScanRes.Items[i].Selected = true
					}
				}
			}
		}
		return m, nil

	case msgDiskStats:
		if msg.err == nil {
			m.diskStats = msg.stats
			if msg.storageInfo.Root.TotalBytes > 0 {
				m.storageInfo = msg.storageInfo
			}
		}
		return m, nil

	case msgCleanStep:
		m.freedBytes += msg.freed
		m.cleanedItems += msg.cleaned
		if msg.err != nil {
			m.cleanErrors = append(m.cleanErrors, fmt.Sprintf("%s: %v", msg.modName, msg.err))
			m.AppendLog(fmt.Sprintf("[ERROR] %s: %v", msg.modName, msg.err))
		} else {
			m.AppendLog(fmt.Sprintf("[OK] %s: cleaned %d items (%s)", msg.modName, msg.cleaned, core.FormatBytes(msg.freed)))
		}

		m.executingStepIdx++
		if m.executingStepIdx < len(m.executionQueue) {
			// Advance to next module in the queue
			return m, m.executeStepCmd(m.executingStepIdx)
		}

		// All steps complete
		m.duration = time.Since(m.startTime)
		m.AppendLog(fmt.Sprintf("[DONE] Finished cleanup: %d items cleaned, %s freed", m.cleanedItems, core.FormatBytes(m.freedBytes)))
		m.viewMode = ViewModeDone
		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	default:
		if m.viewMode == ViewModeCustomRuleForm {
			var cmd tea.Cmd
			switch m.formFocusIndex {
			case 0:
				m.inputRuleName, cmd = m.inputRuleName.Update(msg)
			case 1:
				m.inputRulePath, cmd = m.inputRulePath.Update(msg)
			case 2:
				m.inputRulePattern, cmd = m.inputRulePattern.Update(msg)
			}
			return m, cmd
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch m.viewMode {
	case ViewModeMain:
		return m.handleMainKeys(key)

	case ViewModeAbout:
		return m.handleAboutKeys(key)

	case ViewModeModal:
		return m.handleModalKeys(key)

	case ViewModeCustomRuleForm:
		return m.handleCustomFormKeys(msg)

	case ViewModeExecuting:
		if key == "ctrl+c" {
			m.cancel()
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil

	case ViewModeDone:
		switch key {
		case "enter", "esc", "q", " ":
			// Return to main view and rescan
			m.viewMode = ViewModeMain
			m.scanningAll = true
			for _, mod := range m.modules {
				mod.Scanning = true
			}
			return m, tea.Batch(
				m.scanAllCmd(),
				m.refreshDiskStatsCmd(),
			)
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m Model) handleMainKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case "tab":
		if m.focus == FocusLeft {
			m.focus = FocusRight
		} else {
			m.focus = FocusLeft
		}
		return m, nil

	case "shift+tab":
		if m.focus == FocusRight {
			m.focus = FocusLeft
		} else {
			m.focus = FocusRight
		}
		return m, nil

	case "h", "left":
		m.focus = FocusLeft
		return m, nil

	case "l", "right":
		m.focus = FocusRight
		return m, nil

	case "j", "down":
		if m.focus == FocusLeft {
			if m.activeModuleIdx < len(m.modules)-1 {
				m.activeModuleIdx++
				m.adjustModuleScroll()
			}
		} else {
			active := m.ActiveModule()
			if active != nil {
				filtered := m.FilteredItemIndices(active)
				if len(filtered) > 0 && active.Cursor < len(filtered)-1 {
					active.Cursor++
					m.adjustItemScroll(active)
				}
			}
		}
		return m, nil

	case "k", "up":
		if m.focus == FocusLeft {
			if m.activeModuleIdx > 0 {
				m.activeModuleIdx--
				m.adjustModuleScroll()
			}
		} else {
			active := m.ActiveModule()
			if active != nil {
				filtered := m.FilteredItemIndices(active)
				if len(filtered) > 0 && active.Cursor > 0 {
					active.Cursor--
					m.adjustItemScroll(active)
				}
			}
		}
		return m, nil

	case "g", "home":
		if m.focus == FocusLeft {
			m.activeModuleIdx = 0
			m.moduleScrollOff = 0
		} else {
			active := m.ActiveModule()
			if active != nil {
				active.Cursor = 0
				active.ScrollOffset = 0
			}
		}
		return m, nil

	case "G", "end":
		if m.focus == FocusLeft {
			if len(m.modules) > 0 {
				m.activeModuleIdx = len(m.modules) - 1
				m.adjustModuleScroll()
			}
		} else {
			active := m.ActiveModule()
			if active != nil {
				filtered := m.FilteredItemIndices(active)
				if len(filtered) > 0 {
					active.Cursor = len(filtered) - 1
					m.adjustItemScroll(active)
				}
			}
		}
		return m, nil

	case " ": // Space toggles item or module selection
		if m.focus == FocusLeft {
			active := m.ActiveModule()
			if active != nil {
				m.ToggleSelectFilteredModule(active)
			}
		} else {
			active := m.ActiveModule()
			if active != nil {
				filtered := m.FilteredItemIndices(active)
				if len(filtered) > 0 && active.Cursor >= 0 && active.Cursor < len(filtered) {
					origIdx := filtered[active.Cursor]
					active.ScanRes.Items[origIdx].Selected = !active.ScanRes.Items[origIdx].Selected
				}
			}
		}
		return m, nil

	case "a": // Select / Deselect all globally
		m.ToggleSelectAllGlobal()
		return m, nil

	case "p", "P": // Storage target filter cycle (All -> Root -> Home -> All)
		m.cycleStorageFilter()
		return m, nil

	case "s": // Rescan all modules
		m.scanningAll = true
		for _, mod := range m.modules {
			mod.Scanning = true
		}
		return m, tea.Batch(
			m.scanAllCmd(),
			m.refreshDiskStatsCmd(),
		)

	case "d": // Dry-Run confirmation modal
		m.modalType = ModalTypeDryRun
		m.viewMode = ViewModeModal
		return m, nil

	case "enter": // Clean confirmation modal
		m.modalType = ModalTypeClean
		m.viewMode = ViewModeModal
		return m, nil

	case "?", "i": // About & System Info modal
		m.viewMode = ViewModeAbout
		return m, nil

	case "n", "N":
		active := m.ActiveModule()
		if active != nil && (active.Module.ID() == "custom" || active.Module.Category() == "custom") {
			m.initCustomRuleForm()
			cmd := m.inputRuleName.Focus()
			return m, cmd
		}
		return m, nil

	case "x", "delete":
		active := m.ActiveModule()
		if active != nil && (active.Module.ID() == "custom" || active.Module.Category() == "custom") {
			if len(active.ScanRes.Items) > 0 {
				cursor := active.Cursor
				if cursor < 0 {
					cursor = 0
				}
				if cursor >= len(active.ScanRes.Items) {
					cursor = len(active.ScanRes.Items) - 1
				}
				item := active.ScanRes.Items[cursor]
				ruleID := ""
				if item.Metadata != nil {
					ruleID = item.Metadata["rule_id"]
				}
				if ruleID == "" && strings.HasPrefix(item.ID, "custom:") {
					parts := strings.Split(item.ID, ":")
					if len(parts) >= 2 {
						ruleID = parts[1]
					}
				}
				if ruleID != "" && m.cfg != nil {
					_ = m.cfg.RemoveCustomRule(ruleID)
					if m.reg != nil {
						for _, mod := range m.reg.Modules() {
							if cm, ok := mod.(interface{ SetConfig(*core.Config) }); ok {
								cm.SetConfig(m.cfg)
							}
						}
					}
					for _, mi := range m.modules {
						if cm, ok := mi.Module.(interface{ SetConfig(*core.Config) }); ok {
							cm.SetConfig(m.cfg)
						}
					}
					m.scanningAll = true
					for _, mod := range m.modules {
						mod.Scanning = true
					}
					return m, tea.Batch(
						m.scanAllCmd(),
						m.refreshDiskStatsCmd(),
					)
				}
			}
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleCustomFormKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case "esc":
		m.viewMode = ViewModeMain
		m.formError = ""
		return m, nil

	case "tab", "down":
		m.formFocusIndex = (m.formFocusIndex + 1) % 6
		m.updateFormFocus()
		return m, nil

	case "shift+tab", "up":
		m.formFocusIndex = (m.formFocusIndex - 1 + 6) % 6
		m.updateFormFocus()
		return m, nil

	case " ":
		if m.formFocusIndex == 3 {
			m.formRecursive = !m.formRecursive
			return m, nil
		}
		if m.formFocusIndex == 4 {
			switch m.formSafety {
			case core.SafetySafe:
				m.formSafety = core.SafetyCaution
			case core.SafetyCaution:
				m.formSafety = core.SafetyHigh
			default:
				m.formSafety = core.SafetySafe
			}
			return m, nil
		}
		if m.formFocusIndex == 5 {
			return m.submitCustomRuleForm()
		}

	case "enter":
		return m.submitCustomRuleForm()
	}

	// Route keys to currently focused text input
	var cmd tea.Cmd
	switch m.formFocusIndex {
	case 0:
		m.inputRuleName, cmd = m.inputRuleName.Update(msg)
	case 1:
		m.inputRulePath, cmd = m.inputRulePath.Update(msg)
	case 2:
		m.inputRulePattern, cmd = m.inputRulePattern.Update(msg)
	}
	return m, cmd
}

func (m Model) submitCustomRuleForm() (tea.Model, tea.Cmd) {
	name := strings.TrimSpace(m.inputRuleName.Value())
	path := strings.TrimSpace(m.inputRulePath.Value())

	if name == "" {
		m.formError = "Nama aturan tidak boleh kosong."
		m.formFocusIndex = 0
		m.updateFormFocus()
		return m, nil
	}
	if path == "" {
		m.formError = "Path direktori tidak boleh kosong."
		m.formFocusIndex = 1
		m.updateFormFocus()
		return m, nil
	}

	pattern := strings.TrimSpace(m.inputRulePattern.Value())
	if pattern == "" {
		pattern = "*"
	}

	safety := m.formSafety
	if safety == "" {
		safety = core.SafetySafe
	}

	rule := core.CustomScanRule{
		Name:      name,
		Path:      path,
		Pattern:   pattern,
		Recursive: m.formRecursive,
		Safety:    safety,
	}

	if m.cfg != nil {
		if err := m.cfg.AddCustomRule(rule); err != nil {
			m.formError = fmt.Sprintf("Gagal menyimpan aturan: %v", err)
			return m, nil
		}
	}

	// Update custom module in registry and model items
	if m.reg != nil {
		for _, mod := range m.reg.Modules() {
			if cm, ok := mod.(interface{ SetConfig(*core.Config) }); ok {
				cm.SetConfig(m.cfg)
			}
		}
	}
	for _, mi := range m.modules {
		if cm, ok := mi.Module.(interface{ SetConfig(*core.Config) }); ok {
			cm.SetConfig(m.cfg)
		}
	}

	m.viewMode = ViewModeMain
	m.formError = ""
	m.scanningAll = true
	for _, mod := range m.modules {
		mod.Scanning = true
	}
	return m, tea.Batch(
		m.scanAllCmd(),
		m.refreshDiskStatsCmd(),
	)
}

func (m Model) handleAboutKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q", "?", "i", "enter", " ":
		m.viewMode = ViewModeMain
		return m, nil

	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	}

	return m, nil
}

func (m Model) handleModalKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "n", "N", "q":
		m.viewMode = ViewModeMain
		return m, nil

	case "t", "T":
		if m.snapperAvailable {
			m.snapperSafeguardSnapshot = !m.snapperSafeguardSnapshot
		}
		return m, nil

	case "y", "Y", "enter":
		if m.TotalSelectedCount() == 0 {
			// Nothing selected, cancel back to main
			m.viewMode = ViewModeMain
			return m, nil
		}

		dryRun := (m.modalType == ModalTypeDryRun)
		return m.startExecution(dryRun)
	}

	return m, nil
}

func (m *Model) adjustModuleScroll() {
	availRows := m.leftPaneVisibleRows()
	entries, activeIdx := m.buildLeftPaneEntries(30)
	if len(entries) <= availRows {
		m.moduleScrollOff = 0
		return
	}

	if activeIdx < m.moduleScrollOff {
		m.moduleScrollOff = activeIdx
		if activeIdx > 0 && entries[activeIdx-1].isHeader {
			m.moduleScrollOff = activeIdx - 1
		}
	} else if activeIdx >= m.moduleScrollOff+availRows {
		m.moduleScrollOff = activeIdx - availRows + 1
	}

	maxScroll := len(entries) - availRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.moduleScrollOff > maxScroll {
		m.moduleScrollOff = maxScroll
	}
	if m.moduleScrollOff < 0 {
		m.moduleScrollOff = 0
	}
}

func (m *Model) adjustItemScroll(active *ModuleItem) {
	visibleRows := m.height - 9
	if visibleRows < 4 {
		visibleRows = 4
	}
	filteredLen := len(m.FilteredItemIndices(active))
	if active.Cursor >= filteredLen {
		if filteredLen > 0 {
			active.Cursor = filteredLen - 1
		} else {
			active.Cursor = 0
		}
	}
	if active.Cursor < 0 {
		active.Cursor = 0
	}
	if active.Cursor < active.ScrollOffset {
		active.ScrollOffset = active.Cursor
	} else if active.Cursor >= active.ScrollOffset+visibleRows {
		active.ScrollOffset = active.Cursor - visibleRows + 1
	}
	if active.ScrollOffset < 0 {
		active.ScrollOffset = 0
	}
}

func (m Model) startExecution(dryRun bool) (tea.Model, tea.Cmd) {
	m.viewMode = ViewModeExecuting
	m.executingDryRun = dryRun
	m.executingStepIdx = 0
	m.cleanedItems = 0
	m.freedBytes = 0
	m.cleanErrors = make([]string, 0)
	m.logs = make([]string, 0)
	m.startTime = time.Now()

	// Build queue of modules that have selected items matching storageFilter
	m.executionQueue = make([]int, 0)
	for i, mod := range m.modules {
		if m.ModuleFilteredSelectedCount(mod) > 0 {
			m.executionQueue = append(m.executionQueue, i)
		}
	}

	modeStr := "Cleanup Operation"
	if dryRun {
		modeStr = "Dry-Run Simulation"
	}
	m.AppendLog(fmt.Sprintf("[INIT] Starting %s across %d modules", modeStr, len(m.executionQueue)))

	// Snapper safeguard snapshot handling
	if m.snapperSafeguardSnapshot && m.snapperAvailable {
		if dryRun {
			m.AppendLog("[SNAP] Snapper safeguard snapshot simulated [OK]")
		} else {
			if mod, ok := m.reg.Get("snapper"); ok {
				if sm, ok := mod.(interface{ CreateSafeguardSnapshot(string) error }); ok {
					m.AppendLog("[SNAP] Creating Snapper pre-clean safeguard snapshot...")
					if err := sm.CreateSafeguardSnapshot("Sweep pre-clean safeguard snapshot"); err != nil {
						m.AppendLog(fmt.Sprintf("[WARN] Failed to create safeguard snapshot: %v", err))
					} else {
						m.AppendLog("[SNAP] Safeguard snapshot created successfully [OK]")
					}
				}
			}
		}
	}

	if len(m.executionQueue) == 0 {
		m.viewMode = ViewModeDone
		m.duration = time.Since(m.startTime)
		return m, nil
	}

	return m, m.executeStepCmd(0)
}

func (m Model) executeStepCmd(stepIdx int) tea.Cmd {
	return func() tea.Msg {
		if stepIdx >= len(m.executionQueue) {
			return msgCleanDone{duration: time.Since(m.startTime)}
		}

		modIdx := m.executionQueue[stepIdx]
		modItem := m.modules[modIdx]

		var selected []core.CleanableItem
		for _, it := range modItem.ScanRes.Items {
			if it.Selected {
				scope := core.ItemStorageScope(it)
				if m.storageFilter == core.StorageScopeAll || scope == m.storageFilter {
					selected = append(selected, it)
				}
			}
		}

		opts := core.CleanOptions{
			DryRun:            m.executingDryRun,
			SnapshotSafeguard: false, // Handled globally at start of clean
		}

		res, err := modItem.Module.Clean(m.ctx, selected, opts)
		cleanedCount := len(res.CleanedItems)
		if cleanedCount == 0 && err == nil {
			cleanedCount = len(selected)
		}
		freed := res.FreedBytes
		if freed == 0 && err == nil {
			for _, it := range selected {
				freed += it.Size
			}
		}

		return msgCleanStep{
			moduleIdx: modIdx,
			modName:   modItem.Module.Name(),
			cleaned:   cleanedCount,
			freed:     freed,
			err:       err,
		}
	}
}

func (m Model) scanAllCmd() tea.Cmd {
	return func() tea.Msg {
		results, err := m.reg.ScanAll(m.ctx)
		return msgScanResult{results: results, err: err}
	}
}

func (m Model) refreshDiskStatsCmd() tea.Cmd {
	return func() tea.Msg {
		stats, err := GetRootDiskStats()
		sInfo, _ := core.DetectSystemStorage()
		return msgDiskStats{stats: stats, storageInfo: sInfo, err: err}
	}
}
