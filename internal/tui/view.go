package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/efzynx/sweep/internal/core"
)

// View renders the current state of the TUI application to terminal output.
func (m Model) View() string {
	if m.quitting {
		return "Exiting Sweep...\n"
	}

	// Graceful compact fallback for small terminal windows
	if m.width < 55 || m.height < 14 {
		return m.renderCompact()
	}

	switch m.viewMode {
	case ViewModeModal:
		return m.renderModal()
	case ViewModeExecuting:
		return m.renderExecuting()
	case ViewModeDone:
		return m.renderDone()
	default:
		return m.renderMainSplitView()
	}
}

// renderMainSplitView renders the full standard desktop split view.
func (m *Model) renderMainSplitView() string {
	header := m.renderHeader()
	footer := m.renderFooter()

	headerHeight := lipgloss.Height(header)
	footerHeight := lipgloss.Height(footer)

	mainHeight := m.height - headerHeight - footerHeight
	if mainHeight < 6 {
		mainHeight = 6
	}

	// Calculate responsive pane widths
	leftWidth := (m.width * 32) / 100
	if leftWidth < 28 {
		leftWidth = 28
	}
	if leftWidth > 38 {
		leftWidth = 38
	}

	rightWidth := m.width - leftWidth - 2
	if rightWidth < 25 {
		rightWidth = 25
	}

	leftPane := m.renderLeftPane(leftWidth, mainHeight)
	rightPane := m.renderRightPane(rightWidth, mainHeight)

	split := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)

	return lipgloss.JoinVertical(lipgloss.Left, header, split, footer)
}

// renderCompact provides a clean, single-column fallback view when terminal dimensions are constrained.
func (m *Model) renderCompact() string {
	title := BannerStyle.Render("SWEEP")
	status := lipgloss.NewStyle().Foreground(ColorWarning).Render(
		fmt.Sprintf("Terminal window is compact (%dx%d). Please enlarge for split view.", m.width, m.height),
	)

	summary := fmt.Sprintf("Modules: %d | Items Selected: %d | Reclaimable: %s",
		len(m.modules),
		m.TotalSelectedCount(),
		core.FormatBytes(m.TotalSelectedBytes()),
	)

	prompt := lipgloss.NewStyle().Foreground(ColorMuted).Render("[d] Dry-run  [Enter] Clean  [q] Quit")

	return strings.Join([]string{title, status, summary, prompt}, "\n\n")
}
