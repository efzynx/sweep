package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/efzynx/sweep/internal/core"
)

// renderModal generates the centered confirmation dialog for Dry-Run or Clean operations.
func (m *Model) renderModal() string {
	modalWidth := 74
	if m.width < 78 {
		modalWidth = m.width - 4
	}
	if modalWidth < 50 {
		modalWidth = 50
	}

	titleText := "DRY-RUN SIMULATION"
	actionPrompt := "[y] Run Simulation      [n] / [Esc] Cancel"
	if m.modalType == ModalTypeClean {
		titleText = "CLEANUP CONFIRMATION"
		actionPrompt = "[y] Execute Deletion     [n] / [Esc] Cancel"
	}

	title := ModalTitleStyle.Render(fmt.Sprintf("[ %s ]", titleText))

	var lines []string
	lines = append(lines, title)

	targetNotice := lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Width(modalWidth - 6).
		Render("Target: Root Filesystem (/) — Operations clean system packages, containers, runtimes, and snapshots.")
	lines = append(lines, targetNotice)
	lines = append(lines, "")

	selCount := m.TotalSelectedCount()
	selBytes := m.TotalSelectedBytes()

	// Selected modules count
	activeMods := 0
	for _, mod := range m.modules {
		if mod.SelectedCount() > 0 {
			activeMods++
		}
	}

	if selCount == 0 {
		warn := ModalWarningBox.Width(modalWidth - 6).Render("[!] No items currently selected.\nSelect items with [Space] or [a] before proceeding.")
		lines = append(lines, warn)
		actionPrompt = "[n] / [Esc] Return to Inspector"
	} else {
		// Summary stats
		lines = append(lines, fmt.Sprintf("Target Modules:     %d modules", activeMods))
		lines = append(lines, fmt.Sprintf("Cleanable Items:    %d items", selCount))
		lines = append(lines, fmt.Sprintf("Reclaimable Space:  %s", SizeBadgeStyle.Render(core.FormatBytes(selBytes))))

		// Safety Warning Box
		highestSafety := m.HighestSelectedSafety()
		switch highestSafety {
		case core.SafetyHigh:
			warnText := "[HIGH RISK] Attention: High-risk items selected!\nIncludes sensitive system or container states."
			lines = append(lines, ModalDangerBox.Width(modalWidth-6).Render(warnText))
		case core.SafetyCaution:
			warnText := "[CAUTION] Notice: Caution items selected.\nCaches may need to be re-downloaded if needed later."
			lines = append(lines, ModalWarningBox.Width(modalWidth-6).Render(warnText))
		default:
			safeText := "[SAFE] All selected items are safe to clean."
			lines = append(lines, ModalSuccessBox.Width(modalWidth-6).Render(safeText))
		}

		// Snapper Safeguard Snapshot line
		var snapperLine string
		if m.snapperAvailable {
			toggleState := "[ ] DISABLED"
			if m.snapperSafeguardSnapshot {
				toggleState = "[x] ENABLED "
			}
			snapperLine = fmt.Sprintf("Btrfs Snapper Snapshot: %s  %s",
				lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(toggleState),
				lipgloss.NewStyle().Foreground(ColorMuted).Render("(Press [t] to toggle)"),
			)
		} else {
			snapperLine = fmt.Sprintf("Btrfs Snapper Snapshot: %s",
				lipgloss.NewStyle().Foreground(ColorMuted).Render("[N/A] (Snapper not detected)"),
			)
		}
		lines = append(lines, snapperLine)
	}

	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(ColorSubtle).Render(strings.Repeat("─", modalWidth-6)))
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(ColorFgWhite).Render(actionPrompt))

	modalContent := strings.Join(lines, "\n")
	box := ModalOverlay.Width(modalWidth).Render(modalContent)

	// Place centered on screen
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
