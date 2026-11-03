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

	var output string
	switch m.viewMode {
	case ViewModeModal:
		output = m.renderModal()
	case ViewModeAbout:
		output = m.renderAboutModal()
	case ViewModeExecuting:
		output = m.renderExecuting()
	case ViewModeDone:
		output = m.renderDone()
	default:
		output = m.renderMainSplitView()
	}

	// Strict viewport height clamping guarantee across all modes
	if m.height > 0 && lipgloss.Height(output) > m.height {
		lines := strings.Split(output, "\n")
		if len(lines) > m.height {
			output = strings.Join(lines[:m.height], "\n")
		}
	}

	return output
}

// renderMainSplitView renders the full standard desktop split view.
func (m *Model) renderMainSplitView() string {
	header := m.renderHeader()
	footer := m.renderFooter()

	headerHeight := lipgloss.Height(header)
	footerHeight := lipgloss.Height(footer)

	mainHeight := m.height - headerHeight - footerHeight
	if mainHeight < 4 {
		mainHeight = 4
	}

	// Account for 1 top border row + 1 bottom border row on split panes
	paneContentHeight := mainHeight - 2
	if paneContentHeight < 1 {
		paneContentHeight = 1
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

	leftPane := m.renderLeftPane(leftWidth, paneContentHeight)
	rightPane := m.renderRightPane(rightWidth, paneContentHeight)

	split := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
	if lipgloss.Height(split) > mainHeight {
		splitLines := strings.Split(split, "\n")
		if len(splitLines) > mainHeight {
			split = strings.Join(splitLines[:mainHeight], "\n")
		}
	}

	view := lipgloss.JoinVertical(lipgloss.Left, header, split, footer)

	// Invariant Clamping: ensure total rendered lines NEVER exceed m.height
	if m.height > 0 && lipgloss.Height(view) > m.height {
		excess := lipgloss.Height(view) - m.height
		splitLines := strings.Split(split, "\n")
		if len(splitLines) > excess {
			newSplitHeight := len(splitLines) - excess
			if newSplitHeight < 1 {
				newSplitHeight = 1
			}
			split = strings.Join(splitLines[:newSplitHeight], "\n")
			view = lipgloss.JoinVertical(lipgloss.Left, header, split, footer)
		}
		// Final safety guarantee
		viewLines := strings.Split(view, "\n")
		if len(viewLines) > m.height {
			view = strings.Join(viewLines[:m.height], "\n")
		}
	}

	return view
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

	prompt := lipgloss.NewStyle().Foreground(ColorMuted).Render("[d] Dry-run  [Enter] Clean  [?] Info  [q] Quit")

	sep := "\n\n"
	if m.height < 8 {
		sep = "\n"
	}
	output := strings.Join([]string{title, status, summary, prompt}, sep)
	if m.height > 0 && lipgloss.Height(output) > m.height {
		lines := strings.Split(output, "\n")
		if len(lines) > m.height {
			output = strings.Join(lines[:m.height], "\n")
		}
	}
	return output
}
