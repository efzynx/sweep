package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/efzynx/sweep/internal/core"
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
	if !strings.Contains(view, "SWEEP") || !strings.Contains(view, "Modules") {
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
		if (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF) {
			t.Fatalf("forbidden emoji rune found in rendered Confirmation modal: %c (U+%X)", r, r)
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

	// 1. Verify Header output
	header := m.renderHeader()
	if !strings.Contains(header, "Ahmad Fauzan Adiman (Efzyn)") {
		t.Errorf("expected author in header, got:\n%s", header)
	}
	if !strings.Contains(header, "[TARGET: ROOT FILESYSTEM (/)]") {
		t.Errorf("expected target root badge in header, got:\n%s", header)
	}
	if !strings.Contains(header, "Mode Optimasi & Pemeliharaan Sistem Linux") {
		t.Errorf("expected mode text in header, got:\n%s", header)
	}
	if !strings.Contains(header, "Total:") || !strings.Contains(header, "Tersisa (Free):") || !strings.Contains(header, "Terpakai (Used):") {
		t.Errorf("expected detailed disk stats in header, got:\n%s", header)
	}

	// 2. Verify Confirmation Modal output
	m.viewMode = ViewModeModal
	m.modalType = ModalTypeDryRun
	modal := m.renderModal()
	if !strings.Contains(modal, "Target: Root Filesystem (/)") || !strings.Contains(modal, "Operations clean system packages") {
		t.Errorf("expected target notice in modal, got:\n%s", modal)
	}

	// 3. Verify Footer contains [?] info
	footer := m.renderFooter()
	if !strings.Contains(footer, "[?]") || !strings.Contains(footer, "info") {
		t.Errorf("expected '[?]' and 'info' in footer, got:\n%s", footer)
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
