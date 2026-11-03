package tui

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/core/distro"
)

// mockTestModule implements core.Module for unit testing TUI state transitions.
type mockTestModule struct {
	id          string
	name        string
	category    string
	items       []core.CleanableItem
	scanErr     error
	cleanErr    error
	cleaned     []core.CleanableItem
	cleanedOpts core.CleanOptions
}

func (m *mockTestModule) ID() string              { return m.id }
func (m *mockTestModule) Name() string            { return m.name }
func (m *mockTestModule) Category() string        { return m.category }
func (m *mockTestModule) Description() string     { return "Mock description for " + m.name }
func (m *mockTestModule) IsAvailable() bool       { return true }
func (m *mockTestModule) RequiresElevation() bool { return false }

func (m *mockTestModule) Scan(ctx context.Context) (core.ScanResult, error) {
	if m.scanErr != nil {
		return core.ScanResult{Error: m.scanErr}, m.scanErr
	}
	var total int64
	for _, it := range m.items {
		total += it.Size
	}
	return core.ScanResult{
		Items:     m.items,
		TotalSize: total,
		ScannedAt: time.Now(),
	}, nil
}

func (m *mockTestModule) Clean(ctx context.Context, items []core.CleanableItem, opts core.CleanOptions) (core.CleanResult, error) {
	m.cleaned = items
	m.cleanedOpts = opts
	if m.cleanErr != nil {
		return core.CleanResult{Errors: []error{m.cleanErr}}, m.cleanErr
	}
	var freed int64
	for _, it := range items {
		freed += it.Size
	}
	return core.CleanResult{
		CleanedItems: items,
		FreedBytes:   freed,
		Duration:     10 * time.Millisecond,
	}, nil
}

func setupTestRegistry() (*core.Registry, *mockTestModule, *mockTestModule) {
	reg := core.NewRegistry()

	mod1 := &mockTestModule{
		id:       "pacman_test",
		name:     "Pacman Test",
		category: "pkg",
		items: []core.CleanableItem{
			{ID: "pkg-1", Name: "linux-firmware-old", Size: 200 * 1024 * 1024, Safety: core.SafetySafe, Path: "/var/cache/pacman/pkg/linux.pkg"},
			{ID: "pkg-2", Name: "nvidia-utils-old", Size: 150 * 1024 * 1024, Safety: core.SafetyCaution, Path: "/var/cache/pacman/pkg/nvidia.pkg"},
			{ID: "pkg-3", Name: "system-root-state", Size: 50 * 1024 * 1024, Safety: core.SafetyHigh, Path: "/var/cache/pacman/pkg/root.pkg"},
		},
	}

	mod2 := &mockTestModule{
		id:       "docker_test",
		name:     "Docker Test",
		category: "containers",
		items: []core.CleanableItem{
			{ID: "img-1", Name: "sha256:dangling1", Size: 300 * 1024 * 1024, Safety: core.SafetySafe, Description: "Dangling image"},
			{ID: "vol-1", Name: "unused-vol-data", Size: 500 * 1024 * 1024, Safety: core.SafetySafe, Description: "Orphaned volume"},
		},
	}

	_ = reg.Register(mod1)
	_ = reg.Register(mod2)

	return reg, mod1, mod2
}

func TestTUI_InitAndScan(t *testing.T) {
	reg, _, _ := setupTestRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)
	if len(m.modules) != 2 {
		t.Fatalf("expected 2 registered modules, got %d", len(m.modules))
	}

	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected Init() to return a batch command")
	}

	// Simulate scan completion message
	results, err := reg.ScanAll(context.Background())
	if err != nil {
		t.Fatalf("ScanAll failed: %v", err)
	}

	newM, _ := m.Update(msgScanResult{results: results})
	updatedModel := newM.(Model)

	// In safe-by-default, Safe items are selected
	if updatedModel.TotalSelectedCount() != 3 {
		t.Errorf("expected 3 safe items selected by default, got %d", updatedModel.TotalSelectedCount())
	}

	if updatedModel.TotalScannedItems() != 5 {
		t.Errorf("expected 5 total scanned items, got %d", updatedModel.TotalScannedItems())
	}

	expectedScannedBytes := int64((200 + 150 + 50 + 300 + 500) * 1024 * 1024)
	if updatedModel.TotalScannedBytes() != expectedScannedBytes {
		t.Errorf("expected %d scanned bytes, got %d", expectedScannedBytes, updatedModel.TotalScannedBytes())
	}
}

func TestTUI_NavigationAndVimKeys(t *testing.T) {
	reg, _, _ := setupTestRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)
	results, _ := reg.ScanAll(context.Background())
	newM, _ := m.Update(msgScanResult{results: results})
	m = newM.(Model)

	// Focus initially on Left (Modules)
	if m.focus != FocusLeft {
		t.Fatalf("expected initial focus to be FocusLeft, got %v", m.focus)
	}

	// Vim down (j) on left pane
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = newM.(Model)
	if m.activeModuleIdx != 1 {
		t.Errorf("expected activeModuleIdx to be 1 after 'j', got %d", m.activeModuleIdx)
	}

	// Vim up (k) on left pane
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = newM.(Model)
	if m.activeModuleIdx != 0 {
		t.Errorf("expected activeModuleIdx to be 0 after 'k', got %d", m.activeModuleIdx)
	}

	// Tab switches focus to right pane
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = newM.(Model)
	if m.focus != FocusRight {
		t.Fatalf("expected focus to switch to FocusRight on Tab, got %v", m.focus)
	}

	// Vim down (j) inside right pane (Items)
	activeMod := m.ActiveModule()
	if activeMod.Cursor != 0 {
		t.Fatalf("expected initial item cursor to be 0, got %d", activeMod.Cursor)
	}

	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = newM.(Model)
	activeMod = m.ActiveModule()
	if activeMod.Cursor != 1 {
		t.Errorf("expected item cursor to be 1 after 'j' on items pane, got %d", activeMod.Cursor)
	}

	// Vim 'h' switches focus back to Left pane
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = newM.(Model)
	if m.focus != FocusLeft {
		t.Errorf("expected focus to switch back to FocusLeft on 'h', got %v", m.focus)
	}

	// Vim 'l' switches focus back to Right pane
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = newM.(Model)
	if m.focus != FocusRight {
		t.Errorf("expected focus to switch to FocusRight on 'l', got %v", m.focus)
	}
}

func TestTUI_SelectionToggles(t *testing.T) {
	reg, _, _ := setupTestRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)
	results, _ := reg.ScanAll(context.Background())
	newM, _ := m.Update(msgScanResult{results: results})
	m = newM.(Model)

	// Toggle global select all ('a')
	// Currently 3 items are selected out of 5
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = newM.(Model)
	if m.TotalSelectedCount() != 5 {
		t.Errorf("expected all 5 items selected after 'a', got %d", m.TotalSelectedCount())
	}
	if !m.AllSelected() {
		t.Error("expected AllSelected() to be true")
	}

	// Pressing 'a' again should deselect all
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = newM.(Model)
	if m.TotalSelectedCount() != 0 {
		t.Errorf("expected 0 items selected after second 'a', got %d", m.TotalSelectedCount())
	}

	// Focus is Left. Pressing Space toggles all items in active module (pacman_test: 3 items)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = newM.(Model)
	if m.modules[0].SelectedCount() != 3 {
		t.Errorf("expected all 3 pacman items selected after space on module, got %d", m.modules[0].SelectedCount())
	}

	// Switch to right pane and toggle first item with Space
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = newM.(Model)
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = newM.(Model)
	if m.modules[0].ScanRes.Items[0].Selected {
		t.Errorf("expected first item to be unselected after space toggle")
	}
}

func TestTUI_DryRunModalAndExecution(t *testing.T) {
	reg, mod1, _ := setupTestRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)
	results, _ := reg.ScanAll(context.Background())
	newM, _ := m.Update(msgScanResult{results: results})
	m = newM.(Model)

	// Press 'd' to open Dry-Run modal
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = newM.(Model)

	if m.viewMode != ViewModeModal {
		t.Fatalf("expected viewMode to be ViewModeModal, got %v", m.viewMode)
	}
	if m.modalType != ModalTypeDryRun {
		t.Fatalf("expected modalType to be ModalTypeDryRun, got %v", m.modalType)
	}

	// Modal view output check
	viewStr := m.View()
	if !strings.Contains(viewStr, "DRY-RUN SIMULATION") {
		t.Errorf("expected modal view to display DRY-RUN SIMULATION, got:\n%s", viewStr)
	}

	// Snapper snapshot toggle ('t')
	m.snapperAvailable = true
	m.snapperSafeguardSnapshot = true
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = newM.(Model)
	if m.snapperSafeguardSnapshot {
		t.Error("expected snapperSafeguardSnapshot to toggle to false")
	}

	// Confirm with 'y' -> starts execution
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	m = newM.(Model)

	if m.viewMode != ViewModeExecuting {
		t.Fatalf("expected viewMode to transition to ViewModeExecuting, got %v", m.viewMode)
	}
	if !m.executingDryRun {
		t.Errorf("expected executingDryRun to be true")
	}
	if cmd == nil {
		t.Fatal("expected execution cmd to be returned")
	}

	// Execute steps to completion
	stepMsg := cmd().(msgCleanStep)
	newM, nextCmd := m.Update(stepMsg)
	m = newM.(Model)

	for nextCmd != nil {
		msg := nextCmd()
		if done, ok := msg.(msgCleanDone); ok {
			newM, _ = m.Update(done)
			m = newM.(Model)
			break
		}
		newM, nextCmd = m.Update(msg)
		m = newM.(Model)
	}

	if m.viewMode != ViewModeDone {
		t.Fatalf("expected viewMode to be ViewModeDone after all steps, got %v", m.viewMode)
	}

	// Verify that Clean was called with DryRun = true
	if !mod1.cleanedOpts.DryRun {
		t.Errorf("expected mod1 to be cleaned with DryRun=true")
	}

	// Verify View contains FINISHED and clean summary
	doneView := m.View()
	if !strings.Contains(doneView, "FINISHED") {
		t.Errorf("expected done view to contain FINISHED, got:\n%s", doneView)
	}

	// Pressing Enter in ViewModeDone returns to ViewModeMain and triggers rescan
	newM, rescanCmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)
	if m.viewMode != ViewModeMain {
		t.Errorf("expected viewMode to return to ViewModeMain after Enter on done view, got %v", m.viewMode)
	}
	if rescanCmd == nil {
		t.Error("expected rescan command on returning to main view")
	}
}

func TestTUI_CleanConfirmationModal(t *testing.T) {
	reg, _, _ := setupTestRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)
	results, _ := reg.ScanAll(context.Background())
	newM, _ := m.Update(msgScanResult{results: results})
	m = newM.(Model)

	// Press Enter to open Clean modal
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = newM.(Model)

	if m.viewMode != ViewModeModal {
		t.Fatalf("expected viewMode to be ViewModeModal, got %v", m.viewMode)
	}
	if m.modalType != ModalTypeClean {
		t.Fatalf("expected modalType to be ModalTypeClean, got %v", m.modalType)
	}

	modalStr := m.View()
	if !strings.Contains(modalStr, "CLEANUP CONFIRMATION") {
		t.Errorf("expected view to contain CLEANUP CONFIRMATION, got:\n%s", modalStr)
	}

	// Press 'n' to cancel
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = newM.(Model)
	if m.viewMode != ViewModeMain {
		t.Errorf("expected cancelling modal with 'n' to return to ViewModeMain, got %v", m.viewMode)
	}
}

func TestTUI_WindowResizeAndCompact(t *testing.T) {
	reg, _, _ := setupTestRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)

	// Standard desktop size
	newM, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = newM.(Model)
	if m.width != 120 || m.height != 40 {
		t.Errorf("expected dimensions 120x40, got %dx%d", m.width, m.height)
	}

	view := m.View()
	if !strings.Contains(view, "SWEEP") || !strings.Contains(view, "SUBSYSTEMS") {
		t.Errorf("expected desktop split view, got:\n%s", view)
	}

	// Compact terminal size
	newM, _ = m.Update(tea.WindowSizeMsg{Width: 50, Height: 12})
	m = newM.(Model)
	compactView := m.View()
	if !strings.Contains(compactView, "compact") {
		t.Errorf("expected compact fallback warning in view, got:\n%s", compactView)
	}
}

func TestTUI_ZeroEmojisValidation(t *testing.T) {
	reg, _, _ := setupTestRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)
	results, _ := reg.ScanAll(context.Background())
	newM, _ := m.Update(msgScanResult{results: results})
	m = newM.(Model)

	// Check badges
	safeBadge := RenderSafetyBadge(core.SafetySafe)
	cautionBadge := RenderSafetyBadge(core.SafetyCaution)
	highBadge := RenderSafetyBadge(core.SafetyHigh)

	for _, badge := range []string{safeBadge, cautionBadge, highBadge, BadgeOK, BadgeWarn, BadgeError, BadgeInfo} {
		for _, r := range badge {
			// Emojis typically live in unicode blocks: 0x1F300-0x1FAFF, 0x2600-0x27BF
			if (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF) {
				t.Fatalf("forbidden emoji rune found in badge: %c (U+%X)", r, r)
			}
		}
	}

	// Check entire rendered main view
	rendered := m.View()
	for _, r := range rendered {
		if (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF) {
			t.Fatalf("forbidden emoji rune found in rendered TUI: %c (U+%X)", r, r)
		}
	}

	// Check About modal view
	m.viewMode = ViewModeAbout
	aboutRendered := m.View()
	for _, r := range aboutRendered {
		if (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF) {
			t.Fatalf("forbidden emoji rune found in rendered About modal: %c (U+%X)", r, r)
		}
	}

	// Check Modal view
	m.viewMode = ViewModeModal
	modalRendered := m.View()
	for _, r := range modalRendered {
		if (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF) || (r >= 0x1F900 && r <= 0x1F9FF) {
			t.Fatalf("forbidden emoji rune found in rendered Confirmation modal: %c (U+%X)", r, r)
		}
	}

	// Check Executing view
	m.viewMode = ViewModeExecuting
	m.logs = []string{"Cleaning dangling containers...", "Done."}
	execRendered := m.View()
	for _, r := range execRendered {
		if (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF) || (r >= 0x1F900 && r <= 0x1F9FF) {
			t.Fatalf("forbidden emoji rune found in rendered Executing view: %c (U+%X)", r, r)
		}
	}

	// Check Done view
	m.viewMode = ViewModeDone
	m.cleanedItems = 5
	m.freedBytes = 1024 * 1024 * 50
	m.duration = time.Second
	doneRendered := m.View()
	for _, r := range doneRendered {
		if (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF) || (r >= 0x1F900 && r <= 0x1F9FF) {
			t.Fatalf("forbidden emoji rune found in rendered Done view: %c (U+%X)", r, r)
		}
	}
}

func TestTUI_DiskBarFormatting(t *testing.T) {
	stats := DiskStats{
		TotalBytes:  100 * 1024 * 1024 * 1024,
		UsedBytes:   40 * 1024 * 1024 * 1024,
		FreeBytes:   60 * 1024 * 1024 * 1024,
		UsedPercent: 40.0,
	}

	bar := RenderDiskUsageBar(stats, 20)
	if !strings.Contains(bar, "40.0%") {
		t.Errorf("expected 40.0%% in bar, got: %s", bar)
	}
	if !strings.Contains(bar, "40.00 GB") || !strings.Contains(bar, "100.00 GB") {
		t.Errorf("expected byte sizes in bar, got: %s", bar)
	}
}

func TestTUI_RootDiskStatsFormatting(t *testing.T) {
	stats := DiskStats{
		TotalBytes:  500 * 1024 * 1024 * 1024,
		UsedBytes:   345 * 1024 * 1024 * 1024,
		FreeBytes:   155 * 1024 * 1024 * 1024,
		UsedPercent: 69.0,
	}

	visualBar, statsLine := RenderRootDiskStats(stats, 20)

	// Verify visual bar contains progress blocks and percentage
	if !strings.Contains(visualBar, "69.0%") {
		t.Errorf("expected 69.0%% in visualBar, got: %s", visualBar)
	}
	if !strings.Contains(visualBar, "█") || !strings.Contains(visualBar, "░") {
		t.Errorf("expected block characters in visualBar, got: %s", visualBar)
	}

	// Verify statsLine explicitly displays Total, Free (Tersisa), and Used (Terpakai)
	if !strings.Contains(statsLine, "Total:") {
		t.Errorf("expected 'Total:' in statsLine, got: %s", statsLine)
	}
	if !strings.Contains(statsLine, "Tersisa (Free):") {
		t.Errorf("expected 'Tersisa (Free):' in statsLine, got: %s", statsLine)
	}
	if !strings.Contains(statsLine, "Terpakai (Used):") {
		t.Errorf("expected 'Terpakai (Used):' in statsLine, got: %s", statsLine)
	}
	if !strings.Contains(statsLine, "500.00 GB") || !strings.Contains(statsLine, "155.00 GB") || !strings.Contains(statsLine, "345.00 GB") {
		t.Errorf("expected formatted byte sizes in statsLine, got: %s", statsLine)
	}
}

func TestTUI_HeaderAndModalRootNotices(t *testing.T) {
	reg, _, _ := setupTestRegistry()
	cfg := core.DefaultConfig()
	m := NewModel(context.Background(), reg, cfg)
	m.width = 110
	m.height = 30
	m.diskStats = DiskStats{
		TotalBytes:  100 * 1024 * 1024 * 1024,
		UsedBytes:   50 * 1024 * 1024 * 1024,
		FreeBytes:   50 * 1024 * 1024 * 1024,
		UsedPercent: 50.0,
	}

	// 1. Verify Header output (Concept 2 Nordic Minimalist statusline)
	header := m.renderHeader()
	if !strings.Contains(header, "SWEEP //") {
		t.Errorf("expected 'SWEEP //' in header, got:\n%s", header)
	}
	if !strings.Contains(header, "by Efzyn") {
		t.Errorf("expected 'by Efzyn' in header, got:\n%s", header)
	}
	if !strings.Contains(header, "Root (/):") {
		t.Errorf("expected 'Root (/):' in header, got:\n%s", header)
	}
	if !strings.Contains(header, "50.0% Used") {
		t.Errorf("expected '50.0%% Used' in header, got:\n%s", header)
	}
	if !strings.Contains(header, "Total:") || !strings.Contains(header, "Sisa:") {
		t.Errorf("expected 'Total:' and 'Sisa:' in header, got:\n%s", header)
	}
	if !strings.Contains(header, "[BTRFS SAFEGUARD:") {
		t.Errorf("expected '[BTRFS SAFEGUARD:' in header, got:\n%s", header)
	}

	// 2. Verify Confirmation Modal output
	m.viewMode = ViewModeModal
	m.modalType = ModalTypeDryRun
	modal := m.renderModal()
	if !strings.Contains(modal, "Target: Root Filesystem (/)") || !strings.Contains(modal, "Operations clean system packages") {
		t.Errorf("expected target notice in modal, got:\n%s", modal)
	}

	// 3. Verify Footer contains ? info and Total Reclaimable:
	footer := m.renderFooter()
	if !strings.Contains(footer, "?") || !strings.Contains(footer, "info") {
		t.Errorf("expected '?' and 'info' in footer, got:\n%s", footer)
	}
	if !strings.Contains(footer, "Total Reclaimable:") {
		t.Errorf("expected 'Total Reclaimable:' in footer, got:\n%s", footer)
	}
}

func TestTUI_AboutModalRenderingAndKeys(t *testing.T) {
	reg, _, _ := setupTestRegistry()
	cfg := core.DefaultConfig()
	m := NewModel(context.Background(), reg, cfg)
	m.width = 100
	m.height = 30

	// 1. Test key '?' opens About modal
	newM, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = newM.(Model)
	if m.viewMode != ViewModeAbout {
		t.Fatalf("expected viewMode ViewModeAbout after '?', got %v", m.viewMode)
	}

	// 2. Test rendering of About modal content
	viewOutput := m.View()
	expectedSubstrings := []string{
		"ABOUT & SYSTEM INFO",
		"Sweep - Modern Multi-Distro Linux System Cleaner",
		"Ahmad Fauzan Adiman (Efzyn)",
		"<me@efzyn.my.id>",
		"github.com/efzynx/sweep",
		"MIT License",
		"Targeting root filesystem (/)",
		"containers, flatpak, and snapshots",
		"[Tab] / [h]/[l]",
		"[j]/[k] / [Down]/[Up]",
		"[Space]",
		"[a]",
		"[s]",
		"[d]",
		"[Enter]",
		"[?] / [i]",
		"[q] / [Ctrl+C]",
		"Press [Esc], [?], [q], or [Enter] to return",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(viewOutput, sub) {
			t.Errorf("expected %q in About modal view output, but not found", sub)
		}
	}

	// 3. Test dismissal keys return to ViewModeMain
	dismissKeys := []string{"esc", "q", "?", "i", "enter", " "}
	for _, dk := range dismissKeys {
		m.viewMode = ViewModeAbout
		var msg tea.KeyMsg
		switch dk {
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case " ":
			msg = tea.KeyMsg{Type: tea.KeySpace}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(dk)}
		}

		newM, _ = m.Update(msg)
		resM := newM.(Model)
		if resM.viewMode != ViewModeMain {
			t.Errorf("expected dismissing with %q to return to ViewModeMain, got %v", dk, resM.viewMode)
		}
	}

	// 4. Test key 'i' also opens About modal
	m.viewMode = ViewModeMain
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = newM.(Model)
	if m.viewMode != ViewModeAbout {
		t.Fatalf("expected viewMode ViewModeAbout after 'i', got %v", m.viewMode)
	}

	// 5. Test ctrl+c quits
	newM, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = newM.(Model)
	if !m.quitting {
		t.Errorf("expected m.quitting to be true on ctrl+c in About modal")
	}
	if cmd == nil {
		t.Errorf("expected tea.Quit command on ctrl+c in About modal")
	}
}

func TestTUI_GetRootDiskStatsReal(t *testing.T) {
	stats, err := GetRootDiskStats()
	if err != nil {
		t.Fatalf("unexpected error from GetRootDiskStats: %v", err)
	}
	if stats.TotalBytes == 0 {
		t.Errorf("expected TotalBytes > 0")
	}
	if stats.FreeBytes == 0 {
		t.Errorf("expected FreeBytes > 0")
	}
	if stats.TotalBytes < stats.FreeBytes {
		t.Errorf("TotalBytes (%d) < FreeBytes (%d)", stats.TotalBytes, stats.FreeBytes)
	}
	if stats.UsedBytes != stats.TotalBytes-stats.FreeBytes {
		t.Errorf("UsedBytes (%d) != TotalBytes (%d) - FreeBytes (%d)", stats.UsedBytes, stats.TotalBytes, stats.FreeBytes)
	}
	if stats.UsedPercent <= 0 || stats.UsedPercent > 100 {
		t.Errorf("unexpected UsedPercent: %f", stats.UsedPercent)
	}

	// Test non-existent path returns error
	_, err = GetDiskStats("/path/that/does/not/exist/sweep/test")
	if err == nil {
		t.Errorf("expected error for non-existent path, got nil")
	}
}

func TestTUI_StrictViewportHeightConstraint(t *testing.T) {
	reg, _, _ := setupTestRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)
	results, _ := reg.ScanAll(context.Background())
	newM, _ := m.Update(msgScanResult{results: results})
	m = newM.(Model)
	m.diskStats = DiskStats{
		TotalBytes:  100 * 1024 * 1024 * 1024,
		UsedBytes:   50 * 1024 * 1024 * 1024,
		FreeBytes:   50 * 1024 * 1024 * 1024,
		UsedPercent: 50.0,
	}

	heights := []int{20, 24, 30, 40, 60}
	widths := []int{60, 80, 100, 120, 160}

	for _, h := range heights {
		for _, w := range widths {
			m.width = w
			m.height = h

			header := m.renderHeader()
			footer := m.renderFooter()
			hH := lipgloss.Height(header)
			fH := lipgloss.Height(footer)
			view := m.View()
			gotHeight := lipgloss.Height(view)
			if (w == 120 && h == 40) || (w == 60 && h == 20) || (w == 80 && h == 24) {
				t.Logf("w=%d h=%d -> headerH=%d footerH=%d totalH=%d", w, h, hH, fH, gotHeight)
			}
			if gotHeight > h {
				t.Errorf("dimension %dx%d: rendered height %d exceeds window height %d", w, h, gotHeight, h)
			}

			lines := strings.Split(view, "\n")
			if len(lines) == 0 {
				t.Fatalf("dimension %dx%d: empty view output", w, h)
			}

			// First line must contain "SWEEP"
			if !strings.Contains(lines[0], "SWEEP") {
				t.Errorf("dimension %dx%d: first line does not contain 'SWEEP': %q", w, h, lines[0])
			}

			// Must contain disk stats
			if !strings.Contains(view, "Root (/)") && !strings.Contains(view, "Disk") && !strings.Contains(view, "50.0%") {
				t.Errorf("dimension %dx%d: view missing disk stats", w, h)
			}
		}
	}
}

func TestTUI_AllViewModes_HeightInvariant(t *testing.T) {
	reg, _, _ := setupTestRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)
	results, _ := reg.ScanAll(context.Background())
	newM, _ := m.Update(msgScanResult{results: results})
	m = newM.(Model)
	m.diskStats = DiskStats{
		TotalBytes:  200 * 1024 * 1024 * 1024,
		UsedBytes:   120 * 1024 * 1024 * 1024,
		FreeBytes:   80 * 1024 * 1024 * 1024,
		UsedPercent: 60.0,
	}

	modes := []struct {
		name     string
		setupMod func(m Model) Model
	}{
		{
			name: "MainSplitView",
			setupMod: func(m Model) Model {
				m.viewMode = ViewModeMain
				return m
			},
		},
		{
			name: "ModalDryRun",
			setupMod: func(m Model) Model {
				m.viewMode = ViewModeModal
				m.modalType = ModalTypeDryRun
				return m
			},
		},
		{
			name: "ModalClean",
			setupMod: func(m Model) Model {
				m.viewMode = ViewModeModal
				m.modalType = ModalTypeClean
				return m
			},
		},
		{
			name: "AboutModal",
			setupMod: func(m Model) Model {
				m.viewMode = ViewModeAbout
				return m
			},
		},
		{
			name: "Executing",
			setupMod: func(m Model) Model {
				m.viewMode = ViewModeExecuting
				m.executingDryRun = false
				m.executionQueue = []int{0, 1}
				m.executingStepIdx = 1
				m.logs = []string{"Cleaning cache...", "Removing orphans...", "Done."}
				return m
			},
		},
		{
			name: "Done",
			setupMod: func(m Model) Model {
				m.viewMode = ViewModeDone
				m.cleanedItems = 42
				m.freedBytes = 1024 * 1024 * 500
				m.duration = 2 * time.Second
				m.logs = []string{"Task completed successfully."}
				return m
			},
		},
	}

	testDimensions := []struct {
		w, h int
		desc string
	}{
		{54, 13, "Compact threshold boundary"},
		{55, 14, "Split view minimum boundary"},
		{60, 20, "Small terminal"},
		{80, 24, "Standard terminal (80x24)"},
		{80, 26, "Header constraint boundary high (26 rows)"},
		{80, 27, "Header unconstrained boundary low (27 rows)"},
		{120, 30, "Widescreen terminal (120x30)"},
		{160, 45, "Ultra-wide terminal"},
		{200, 60, "4K high-resolution terminal"},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			modeModel := mode.setupMod(m)
			for _, dim := range testDimensions {
				modeModel.width = dim.w
				modeModel.height = dim.h

				view := modeModel.View()
				gotH := lipgloss.Height(view)

				if gotH > dim.h {
					t.Errorf("[%s] %s (%dx%d): height %d exceeds max %d", mode.name, dim.desc, dim.w, dim.h, gotH, dim.h)
				}

				// Check trailing newline: view must not end with extra empty lines exceeding height
				lines := strings.Split(view, "\n")
				if len(lines) > dim.h {
					t.Errorf("[%s] %s (%dx%d): line count %d exceeds height %d", mode.name, dim.desc, dim.w, dim.h, len(lines), dim.h)
				}
			}
		})
	}
}

func TestTUI_HeaderVisibilityAndBanner(t *testing.T) {
	reg, _, _ := setupTestRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)
	results, _ := reg.ScanAll(context.Background())
	newM, _ := m.Update(msgScanResult{results: results})
	m = newM.(Model)
	m.diskStats = DiskStats{
		TotalBytes:  500 * 1024 * 1024 * 1024,
		UsedBytes:   250 * 1024 * 1024 * 1024,
		FreeBytes:   250 * 1024 * 1024 * 1024,
		UsedPercent: 50.0,
	}

	dimensions := [][2]int{
		{60, 20},
		{80, 24},
		{120, 30},
		{100, 25},
		{160, 40},
	}

	for _, dim := range dimensions {
		w, h := dim[0], dim[1]
		m.width = w
		m.height = h

		view := m.View()
		lines := strings.Split(view, "\n")

		if len(lines) < 4 {
			t.Fatalf("dimension %dx%d: view has too few lines (%d)", w, h, len(lines))
		}

		// Line 0 MUST contain "SWEEP"
		if !strings.Contains(lines[0], "SWEEP") {
			t.Errorf("dimension %dx%d: line 0 missing SWEEP banner: %q", w, h, lines[0])
		}

		// Lines 0..3 MUST contain "Root (/)" disk usage meter
		headerText := strings.Join(lines[:4], "\n")
		if !strings.Contains(headerText, "Root (/)") {
			t.Errorf("dimension %dx%d: lines 0..3 missing 'Root (/)': %q", w, h, headerText)
		}
		if !strings.Contains(headerText, "50.0%") {
			t.Errorf("dimension %dx%d: lines 0..3 missing '50.0%%': %q", w, h, headerText)
		}
	}
}

func TestTUI_ConcurrentResizeAndRenderRace(t *testing.T) {
	reg, _, _ := setupTestRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)
	results, _ := reg.ScanAll(context.Background())
	newM, _ := m.Update(msgScanResult{results: results})
	m = newM.(Model)

	// Rapidly send window size updates and render View() concurrently
	var wg sync.WaitGroup
	dimensions := [][2]int{
		{80, 24}, {120, 30}, {60, 20}, {100, 25}, {160, 45}, {55, 14}, {70, 22},
	}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			dim := dimensions[idx%len(dimensions)]
			currM := m
			updatedM, _ := currM.Update(tea.WindowSizeMsg{Width: dim[0], Height: dim[1]})
			view := updatedM.(Model).View()
			if lipgloss.Height(view) > dim[1] {
				t.Errorf("concurrent test %dx%d: rendered height %d > %d", dim[0], dim[1], lipgloss.Height(view), dim[1])
			}
		}(i)
	}
	wg.Wait()
}

func TestTUI_Concept2_NordicStatuslineAndTreePane(t *testing.T) {
	reg, _, _ := setupTestRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)
	results, _ := reg.ScanAll(context.Background())
	newM, _ := m.Update(msgScanResult{results: results})
	m = newM.(Model)
	m.width = 135
	m.height = 35
	m.diskStats = DiskStats{
		TotalBytes:  100 * 1024 * 1024 * 1024,
		UsedBytes:   69 * 1024 * 1024 * 1024,
		FreeBytes:   31 * 1024 * 1024 * 1024,
		UsedPercent: 69.0,
	}

	// 1. Test Statusline components & 2-row invariant on wide screens
	header := m.renderHeader()
	headerLines := strings.Split(header, "\n")
	if len(headerLines) != 2 {
		t.Fatalf("expected header on 135 cols to be strictly 2 rows, got %d:\n%s", len(headerLines), header)
	}
	if !strings.Contains(headerLines[0], "SWEEP //") {
		t.Errorf("line 0 missing 'SWEEP //': %q", headerLines[0])
	}
	if !strings.Contains(headerLines[0], "by Efzyn") {
		t.Errorf("line 0 missing 'by Efzyn': %q", headerLines[0])
	}
	if !strings.Contains(headerLines[0], "Root (/):") || !strings.Contains(headerLines[0], "69.0% Used") {
		t.Errorf("line 0 missing 'Root (/): 69.0%% Used': %q", headerLines[0])
	}
	if !strings.Contains(headerLines[0], "Total:") || !strings.Contains(headerLines[0], "Sisa:") {
		t.Errorf("line 0 missing 'Total:' and 'Sisa:': %q", headerLines[0])
	}
	if !strings.Contains(headerLines[0], "[BTRFS SAFEGUARD:") {
		t.Errorf("line 0 missing '[BTRFS SAFEGUARD:': %q", headerLines[0])
	}
	if !strings.Contains(headerLines[1], "─") {
		t.Errorf("line 1 missing divider line '─': %q", headerLines[1])
	}

	// 2. Test Tree Subsystems Pane
	leftPane := m.renderLeftPane(40, 20)
	if !strings.Contains(leftPane, "SUBSYSTEMS [2 DETECTED]") {
		t.Errorf("left pane missing 'SUBSYSTEMS [2 DETECTED]':\n%s", leftPane)
	}
	if !strings.Contains(leftPane, "PACKAGE MANAGERS") {
		t.Errorf("left pane missing 'PACKAGE MANAGERS':\n%s", leftPane)
	}
	if !strings.Contains(leftPane, "CONTAINERS & VIRTUALIZATION") {
		t.Errorf("left pane missing 'CONTAINERS & VIRTUALIZATION':\n%s", leftPane)
	}
	if (!strings.Contains(leftPane, "└─") && !strings.Contains(leftPane, "├─")) || !strings.Contains(leftPane, "Pacman Test") {
		t.Errorf("left pane missing box-drawing tree glyph for Pacman Test:\n%s", leftPane)
	}
	if (!strings.Contains(leftPane, "└─") && !strings.Contains(leftPane, "├─")) || !strings.Contains(leftPane, "Docker Test") {
		t.Errorf("left pane missing box-drawing tree glyph for Docker Test:\n%s", leftPane)
	}
	// Initial focus on left pane: active cursor marker '>'
	if !strings.Contains(leftPane, "> ") {
		t.Errorf("left pane missing active cursor '> ':\n%s", leftPane)
	}

	// 3. Test Right Pane Item Inspector
	rightPane := m.renderRightPane(80, 20)
	if !strings.Contains(rightPane, "PACMAN TEST • 3 ITEMS TERDETEKSI") {
		t.Errorf("right pane missing module title & item count:\n%s", rightPane)
	}
	if !strings.Contains(rightPane, "[Tab] switch pane") {
		t.Errorf("right pane missing '[Tab] switch pane' hint:\n%s", rightPane)
	}
	if !strings.Contains(rightPane, "SEL") || !strings.Contains(rightPane, "NAMA ITEM") || !strings.Contains(rightPane, "UKURAN") || !strings.Contains(rightPane, "TINGKAT BAHAYA") {
		t.Errorf("right pane missing table column headers:\n%s", rightPane)
	}
	if !strings.Contains(rightPane, "[SAFE]") || !strings.Contains(rightPane, "[CAUTION]") || !strings.Contains(rightPane, "[HIGH]") {
		t.Errorf("right pane missing safety badges:\n%s", rightPane)
	}

	// 4. Test Minimalist Footer
	footer := m.renderFooter()
	if !strings.Contains(footer, "j/k navigasi") || !strings.Contains(footer, "Space toggle") || !strings.Contains(footer, "•") {
		t.Errorf("footer missing compact keybinding hints or bullet separator:\n%s", footer)
	}
	if !strings.Contains(footer, "Total Reclaimable:") {
		t.Errorf("footer missing 'Total Reclaimable:':\n%s", footer)
	}
}

func TestTUI_Concept2_NarrowScreenStatusline(t *testing.T) {
	reg, _, _ := setupTestRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)
	m.width = 75
	m.height = 24
	m.diskStats = DiskStats{
		TotalBytes:  100 * 1024 * 1024 * 1024,
		UsedBytes:   50 * 1024 * 1024 * 1024,
		FreeBytes:   50 * 1024 * 1024 * 1024,
		UsedPercent: 50.0,
	}

	header := m.renderHeader()
	lines := strings.Split(header, "\n")
	// On narrow screens (< 90 cols), header vertical height must not exceed 3 rows
	if len(lines) > 3 {
		t.Errorf("narrow screen header should be at most 3 rows, got %d:\n%s", len(lines), header)
	}
	if !strings.Contains(header, "SWEEP //") {
		t.Errorf("narrow header missing 'SWEEP //':\n%s", header)
	}
	if !strings.Contains(header, "Root (/):") {
		t.Errorf("narrow header missing 'Root (/):':\n%s", header)
	}
}

func TestTUI_Concept2_ComprehensiveQA(t *testing.T) {
	// 1. Test CategoryPriority sorting
	p1 := CategoryPriority("pacman")
	p2 := CategoryPriority("docker")
	p3 := CategoryPriority("snapper")
	p4 := CategoryPriority("unknown_subsystem")
	if !(p1 < p2 && p2 < p3 && p3 < p4) {
		t.Fatalf("expected CategoryPriority pacman(%d) < docker(%d) < snapper(%d) < other(%d)", p1, p2, p3, p4)
	}

	reg, _, _ := setupTestRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)
	results, _ := reg.ScanAll(context.Background())
	newM, _ := m.Update(msgScanResult{results: results})
	m = newM.(Model)

	// 2. Test Distro fallbacks in Statusline
	// Case A: nil distroInfo
	m.distroInfo = nil
	m.width = 120
	hA := m.renderHeader()
	if !strings.Contains(hA, "SWEEP // Linux // by Efzyn") {
		t.Errorf("expected 'SWEEP // Linux // by Efzyn' for nil distro, got:\n%s", hA)
	}

	// Case B: family only
	m.distroInfo = &distro.DistroInfo{Family: distro.FamilyArch}
	hB := m.renderHeader()
	if !strings.Contains(hB, "SWEEP // Linux (Arch) // by Efzyn") {
		t.Errorf("expected 'SWEEP // Linux (Arch) // by Efzyn' for family only, got:\n%s", hB)
	}

	// Case C: prettyName + family
	m.distroInfo = &distro.DistroInfo{PrettyName: "CachyOS", Family: distro.FamilyArch}
	hC := m.renderHeader()
	if !strings.Contains(hC, "SWEEP // CachyOS (Arch) // by Efzyn") {
		t.Errorf("expected 'SWEEP // CachyOS (Arch) // by Efzyn', got:\n%s", hC)
	}

	// 3. Test Safeguard Badge Toggle
	m.snapperAvailable = true
	if !strings.Contains(m.renderHeader(), "[BTRFS SAFEGUARD: ACTIVE]") {
		t.Errorf("expected ACTIVE safeguard badge when snapper is available")
	}
	m.snapperAvailable = false
	if !strings.Contains(m.renderHeader(), "[BTRFS SAFEGUARD: INACTIVE]") {
		t.Errorf("expected INACTIVE safeguard badge when snapper is unavailable")
	}

	// 4. Test Footer Responsiveness at 3 width breakpoints
	// Breakpoint 1: >= 130
	m.width = 140
	fWide := m.renderFooter()
	if !strings.Contains(fWide, "j/k navigasi") || !strings.Contains(fWide, "Space toggle") || !strings.Contains(fWide, "•") {
		t.Errorf("wide footer missing full keys or bullet: %s", fWide)
	}

	// Breakpoint 2: 100..129
	m.width = 110
	fMed := m.renderFooter()
	if !strings.Contains(fMed, "j/k nav") || !strings.Contains(fMed, "Space toggle") {
		t.Errorf("medium footer missing abbreviated keys: %s", fMed)
	}

	// Breakpoint 3: < 100
	m.width = 80
	fCompact := m.renderFooter()
	if !strings.Contains(fCompact, "Space toggle") || !strings.Contains(fCompact, "d dry") {
		t.Errorf("compact footer missing core keys: %s", fCompact)
	}
}

func setupPhase2MultiCategoryRegistry() (*core.Registry, []*mockTestModule) {
	reg := core.NewRegistry()

	mods := []*mockTestModule{
		{
			id:       "pacman_mod",
			name:     "Pacman Cache & Orphans",
			category: "pkg",
			items: []core.CleanableItem{
				{ID: "pacman-1", Name: "linux-kernel-old", Size: 500 * 1024 * 1024, Safety: core.SafetySafe, Path: "/var/cache/pacman/pkg/linux.pkg"},
			},
		},
		{
			id:       "flatpak_mod",
			name:     "Flatpak Unused Runtimes",
			category: "sandboxed_apps",
			items: []core.CleanableItem{
				{ID: "flatpak-1", Name: "org.gnome.Platform//45", Size: 800 * 1024 * 1024, Safety: core.SafetySafe, Path: "/var/lib/flatpak/runtime/org.gnome.Platform"},
			},
		},
		{
			id:       "snap_mod",
			name:     "Snap Disabled Revisions",
			category: "snap",
			items: []core.CleanableItem{
				{ID: "snap-1", Name: "core20 (rev 1822)", Size: 120 * 1024 * 1024, Safety: core.SafetySafe, Path: "/var/lib/snapd/snaps/core20_1822.snap"},
			},
		},
		{
			id:       "apt_mod",
			name:     "APT Archives & Autoremove",
			category: "apt",
			items: []core.CleanableItem{
				{ID: "apt-1", Name: "deb-archives-cache", Size: 350 * 1024 * 1024, Safety: core.SafetySafe, Path: "/var/cache/apt/archives"},
			},
		},
		{
			id:       "docker_mod",
			name:     "Docker System Prune",
			category: "containers",
			items: []core.CleanableItem{
				{ID: "dock-1", Name: "dangling-images", Size: 1500 * 1024 * 1024, Safety: core.SafetySafe, Description: "Dangling build cache layers"},
			},
		},
		{
			id:       "devcache_mod",
			name:     "Developer Build Caches",
			category: "devcache",
			items: []core.CleanableItem{
				{
					ID:          "dev-1",
					Name:        "npm global cache (~/.npm/_cacache)",
					Size:        2500 * 1024 * 1024,
					Safety:      core.SafetySafe,
					Description: "npm package manager download and cache directory",
					Path:        "/home/efzyn/.npm/_cacache",
				},
				{
					ID:          "dev-2",
					Name:        "Go module cache",
					Size:        3500 * 1024 * 1024,
					Safety:      core.SafetyCaution,
					Description: "Go downloaded module dependencies cache (go env GOMODCACHE)",
					Path:        "/home/efzyn/go/pkg/mod",
				},
				{
					ID:          "dev-3",
					Name:        "Cargo cache tarballs",
					Size:        1500 * 1024 * 1024,
					Safety:      core.SafetySafe,
					Description: "Downloaded Cargo crates registry cache",
					Path:        "/home/efzyn/.cargo/registry/cache",
				},
			},
		},
		{
			id:       "usercache_mod",
			name:     "Desktop & Browser Caches",
			category: "user_cache",
			items: []core.CleanableItem{
				{
					ID:          "uc-1",
					Name:        "Firefox Cache (profile.default)",
					Size:        800 * 1024 * 1024,
					Safety:      core.SafetySafe,
					Description: "Firefox HTTP cache2 for profile profile.default (3450 files)",
					Path:        "/home/efzyn/.cache/mozilla/firefox/profile.default/cache2",
				},
				{
					ID:          "uc-2",
					Name:        "Desktop Trash Can",
					Size:        500 * 1024 * 1024,
					Safety:      core.SafetySafe,
					Description: "Deleted files in ~/.local/share/Trash (120 files)",
					Path:        "/home/efzyn/.local/share/Trash",
				},
			},
		},
		{
			id:       "journal_mod",
			name:     "Systemd Journal Logs",
			category: "system_logs",
			items: []core.CleanableItem{
				{
					ID:          "jour-1",
					Name:        "Systemd Journal Logs",
					Size:        1200 * 1024 * 1024,
					Safety:      core.SafetySafe,
					Description: "Archived systemd logs (>14d retention)",
					Path:        "/var/log/journal",
				},
			},
		},
		{
			id:       "snapper_mod",
			name:     "Snapper Btrfs Snapshots",
			category: "snapshots",
			items: []core.CleanableItem{
				{
					ID:          "snap-sub-1",
					Name:        "Timeline Snapshot #42",
					Size:        5500 * 1024 * 1024,
					Safety:      core.SafetySafe,
					Description: "Btrfs pre-system update safeguard snapshot",
					Path:        "/.snapshots/42/snapshot",
				},
			},
		},
		{
			id:       "custom_mod",
			name:     "User Custom Rules",
			category: "custom",
			items: []core.CleanableItem{
				{
					ID:          "cust-1",
					Name:        "Custom build artifacts",
					Size:        1500 * 1024 * 1024,
					Safety:      core.SafetySafe,
					Description: "Temporary build directory /home/efzyn/scratch/build",
					Path:        "/home/efzyn/scratch/build",
				},
			},
		},
	}

	for _, m := range mods {
		_ = reg.Register(m)
	}

	return reg, mods
}

func TestTUI_Phase2_MultiCategoryTreeHierarchy(t *testing.T) {
	reg, _ := setupPhase2MultiCategoryRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)
	results, err := reg.ScanAll(context.Background())
	if err != nil {
		t.Fatalf("ScanAll failed: %v", err)
	}
	newM, _ := m.Update(msgScanResult{results: results})
	m = newM.(Model)
	m.width = 120
	m.height = 35

	leftPane := m.renderLeftPane(38, 26)

	// Verify all 6 category titles appear in the tree
	expectedCategories := []string{
		"PACKAGE MANAGERS",
		"CONTAINERS & VIRTUALIZATION",
		"DEVELOPER CACHES",
		"SYSTEM & DESKTOP",
		"FILESYSTEM & SNAPSHOTS",
		"USER CUSTOM RULES",
	}

	for _, cat := range expectedCategories {
		if !strings.Contains(leftPane, cat) {
			t.Errorf("left pane missing category header %q:\n%s", cat, leftPane)
		}
	}

	// Verify total detected count in title
	if !strings.Contains(leftPane, "SUBSYSTEMS [10 DETECTED]") {
		t.Errorf("left pane missing 'SUBSYSTEMS [10 DETECTED]':\n%s", leftPane)
	}

	// Verify box-drawing glyphs ├─ and └─ are used
	if !strings.Contains(leftPane, "├─") {
		t.Errorf("left pane missing branch glyph '├─':\n%s", leftPane)
	}
	if !strings.Contains(leftPane, "└─") {
		t.Errorf("left pane missing leaf glyph '└─':\n%s", leftPane)
	}
}

func TestTUI_Phase2_TreeScrolling_8PlusModules(t *testing.T) {
	reg, _ := setupPhase2MultiCategoryRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)
	results, _ := reg.ScanAll(context.Background())
	newM, _ := m.Update(msgScanResult{results: results})
	m = newM.(Model)

	// Set a constrained terminal size where 10 modules + 6 categories (16 rows) exceed visible height
	m.width = 100
	m.height = 18

	// Initial view at top (activeModuleIdx = 0)
	if m.activeModuleIdx != 0 {
		t.Fatalf("expected initial activeModuleIdx to be 0, got %d", m.activeModuleIdx)
	}

	leftPaneInitial := m.renderLeftPane(32, 11)
	if !strings.Contains(leftPaneInitial, "PACKAGE MANAGERS") {
		t.Errorf("expected top category 'PACKAGE MANAGERS' visible at initial scroll:\n%s", leftPaneInitial)
	}

	// Navigate down with 'j' to the very last module (index 9: User Custom Rules)
	for i := 0; i < 9; i++ {
		newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = newM.(Model)

		// Assert height invariant holds at every step
		v := m.View()
		if lipgloss.Height(v) > m.height {
			t.Fatalf("height overflow at step %d: got %d, max allowed %d", i, lipgloss.Height(v), m.height)
		}
	}

	if m.activeModuleIdx != 9 {
		t.Fatalf("expected activeModuleIdx to be 9 after 9 downs, got %d", m.activeModuleIdx)
	}

	// Render left pane at bottom: last module must be visible
	leftPaneBottom := m.renderLeftPane(32, 11)
	if !strings.Contains(leftPaneBottom, "User Custom") {
		t.Errorf("expected last module 'User Custom' to be visible when scrolled to bottom:\n%s", leftPaneBottom)
	}
	if !strings.Contains(leftPaneBottom, "> ") {
		t.Errorf("expected active cursor '> ' to be visible when scrolled to bottom:\n%s", leftPaneBottom)
	}

	// Test jump to top with 'g'
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = newM.(Model)
	if m.activeModuleIdx != 0 {
		t.Fatalf("expected activeModuleIdx=0 after 'g', got %d", m.activeModuleIdx)
	}
	leftPaneTop := m.renderLeftPane(32, 11)
	if !strings.Contains(leftPaneTop, "PACKAGE MANAGERS") {
		t.Errorf("expected 'PACKAGE MANAGERS' visible after jumping to top with 'g':\n%s", leftPaneTop)
	}

	// Test jump to bottom with 'G'
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = newM.(Model)
	if m.activeModuleIdx != 9 {
		t.Fatalf("expected activeModuleIdx=9 after 'G', got %d", m.activeModuleIdx)
	}
	leftPaneEnd := m.renderLeftPane(32, 11)
	if !strings.Contains(leftPaneEnd, "User Custom") {
		t.Errorf("expected 'User Custom' visible after jumping to bottom with 'G':\n%s", leftPaneEnd)
	}
}

func TestTUI_Phase2_ItemInspector_FormattingAndTruncation(t *testing.T) {
	reg, _ := setupPhase2MultiCategoryRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)
	results, _ := reg.ScanAll(context.Background())
	newM, _ := m.Update(msgScanResult{results: results})
	m = newM.(Model)

	// Find the devcache and usercache module indices
	var devIdx, usercacheIdx int
	for i, mod := range m.modules {
		if mod.Module.ID() == "devcache_mod" {
			devIdx = i
		}
		if mod.Module.ID() == "usercache_mod" {
			usercacheIdx = i
		}
	}

	// 1. Inspect Developer Caches
	m.activeModuleIdx = devIdx
	m.width = 120
	m.height = 30
	rightPaneDev := m.renderRightPane(90, 20)

	if !strings.Contains(rightPaneDev, "npm global cache") {
		t.Errorf("right pane missing npm item:\n%s", rightPaneDev)
	}
	if !strings.Contains(rightPaneDev, "Go module cache") {
		t.Errorf("right pane missing Go module item:\n%s", rightPaneDev)
	}
	if !strings.Contains(rightPaneDev, "[CAUTION]") {
		t.Errorf("right pane missing [CAUTION] badge for Go module cache:\n%s", rightPaneDev)
	}

	// 2. Inspect Desktop & Browser Caches
	m.activeModuleIdx = usercacheIdx
	rightPaneUser := m.renderRightPane(90, 20)

	if !strings.Contains(rightPaneUser, "Firefox Cache (profile.default)") {
		t.Errorf("right pane missing 'Firefox Cache (profile.default)':\n%s", rightPaneUser)
	}
	if !strings.Contains(rightPaneUser, "Desktop Trash Can") {
		t.Errorf("right pane missing 'Desktop Trash Can':\n%s", rightPaneUser)
	}

	// 3. Narrow pane width truncation check (width 55 cols)
	narrowPane := m.renderRightPane(55, 15)
	if !strings.Contains(narrowPane, "…") {
		t.Errorf("narrow pane should contain ellipsis '…' for truncated columns:\n%s", narrowPane)
	}
}

func TestTUI_Phase2_MultiGigabyteSelectionTotals(t *testing.T) {
	reg, _ := setupPhase2MultiCategoryRegistry()
	cfg := core.DefaultConfig()

	m := NewModel(context.Background(), reg, cfg)
	results, _ := reg.ScanAll(context.Background())
	newM, _ := m.Update(msgScanResult{results: results})
	m = newM.(Model)

	// Total scanned across all 10 modules:
	// 500M + 800M + 120M + 350M + 1500M + (2500M+3500M+1500M) + (800M+500M) + 1200M + 5500M + 1500M = 20270 MB (~20.27 GB)
	totScanned := m.TotalScannedBytes()
	if totScanned <= 15*1024*1024*1024 {
		t.Fatalf("expected scanned total to be multi-gigabyte (>15 GB), got %s", core.FormatBytes(totScanned))
	}

	// All Safe items are selected by default. Caution item (Go mod cache 3500MB) is unselected.
	selBytes := m.TotalSelectedBytes()
	if selBytes <= 10*1024*1024*1024 {
		t.Fatalf("expected selected total to be multi-gigabyte (>10 GB), got %s", core.FormatBytes(selBytes))
	}

	m.width = 120
	m.height = 30
	footer := m.renderFooter()

	if !strings.Contains(footer, "Total Reclaimable:") {
		t.Errorf("footer missing 'Total Reclaimable:':\n%s", footer)
	}
	if !strings.Contains(footer, "GB") {
		t.Errorf("footer missing multi-gigabyte 'GB' metric notation:\n%s", footer)
	}

	// Test global select-all toggle with key 'a'
	newM, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = newM.(Model)
	if !m.AllSelected() {
		t.Fatalf("expected all items to be selected after 'a'")
	}
	if m.TotalSelectedBytes() != m.TotalScannedBytes() {
		t.Errorf("expected selected bytes (%d) == scanned bytes (%d)", m.TotalSelectedBytes(), m.TotalScannedBytes())
	}
}
