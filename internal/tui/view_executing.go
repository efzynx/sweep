package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/efzynx/sweep/internal/core"
)

// renderExecuting displays the real-time execution screen with spinner, progress bar, and log viewport.
func (m *Model) renderExecuting() string {
	header := m.renderHeader()

	boxWidth := m.width - 4
	if boxWidth < 40 {
		boxWidth = 40
	}

	modeStr := "CLEANUP IN PROGRESS"
	if m.executingDryRun {
		modeStr = "DRY-RUN SIMULATION IN PROGRESS"
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorFgWhite).
		Background(ColorPrimary).
		Padding(0, 2).
		Render(fmt.Sprintf("[ %s ]", modeStr))

	totalSteps := len(m.executionQueue)
	currentStep := m.executingStepIdx
	if currentStep > totalSteps {
		currentStep = totalSteps
	}

	var pct float64
	if totalSteps > 0 {
		pct = float64(currentStep) / float64(totalSteps)
	}

	progressBar := m.progress.ViewAs(pct)
	stepInfo := lipgloss.NewStyle().Foreground(ColorMuted).Render(
		fmt.Sprintf("Module %d of %d (%.0f%%)", currentStep, totalSteps, pct*100),
	)

	statusLine := lipgloss.JoinHorizontal(lipgloss.Center,
		m.spinner.View(),
		" ",
		lipgloss.NewStyle().Bold(true).Foreground(ColorFgWhite).Render("Executing subsystem cleaner adapters..."),
		"  ",
		stepInfo,
	)

	// Log Window Viewport
	logHeight := m.height - 14
	if logHeight < 5 {
		logHeight = 5
	}

	startLog := 0
	if len(m.logs) > logHeight {
		startLog = len(m.logs) - logHeight
	}

	visibleLogs := m.logs[startLog:]
	var logLines []string
	for _, l := range visibleLogs {
		logLines = append(logLines, LogTextStyle.Render(l))
	}
	for len(logLines) < logHeight {
		logLines = append(logLines, "")
	}

	logContent := strings.Join(logLines, "\n")
	logBox := LogContainerStyle.Width(boxWidth).Height(logHeight).Render(logContent)

	cancelPrompt := lipgloss.NewStyle().Foreground(ColorMuted).Render("Press [Ctrl+C] to abort")

	content := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		statusLine,
		progressBar,
		"",
		lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("Live Execution Logs:"),
		logBox,
		"",
		cancelPrompt,
	)

	return lipgloss.JoinVertical(lipgloss.Left, header, content)
}

// renderDone displays the post-execution summary and metrics screen.
func (m *Model) renderDone() string {
	header := m.renderHeader()

	boxWidth := m.width - 4
	if boxWidth < 40 {
		boxWidth = 40
	}

	titleText := "CLEANUP OPERATION FINISHED"
	titleStyle := ModalSuccessBox
	if m.executingDryRun {
		titleText = "DRY-RUN SIMULATION FINISHED"
		titleStyle = ModalWarningBox
	}
	if len(m.cleanErrors) > 0 {
		titleText = "CLEANUP FINISHED WITH WARNINGS / ERRORS"
		titleStyle = ModalDangerBox
	}

	titleBanner := titleStyle.Width(boxWidth).Render(fmt.Sprintf("[ %s ]", titleText))

	// Summary Cards
	summaryLines := []string{
		fmt.Sprintf("Cleaned Items:     %d items", m.cleanedItems),
		fmt.Sprintf("Reclaimed Space:   %s", SizeBadgeStyle.Render(core.FormatBytes(m.freedBytes))),
		fmt.Sprintf("Execution Time:    %s", m.duration.Round(1000000).String()),
	}

	if len(m.cleanErrors) > 0 {
		summaryLines = append(summaryLines, "")
		summaryLines = append(summaryLines, lipgloss.NewStyle().Bold(true).Foreground(ColorDanger).Render("Encountered Errors:"))
		for _, errStr := range m.cleanErrors {
			summaryLines = append(summaryLines, lipgloss.NewStyle().Foreground(ColorDanger).Render(fmt.Sprintf("  [!] %s", errStr)))
		}
	}

	// Recent logs
	logHeight := m.height - 18
	if logHeight < 4 {
		logHeight = 4
	}
	startLog := 0
	if len(m.logs) > logHeight {
		startLog = len(m.logs) - logHeight
	}
	visibleLogs := m.logs[startLog:]
	var logLines []string
	for _, l := range visibleLogs {
		logLines = append(logLines, LogTextStyle.Render(l))
	}
	for len(logLines) < logHeight {
		logLines = append(logLines, "")
	}
	logBox := LogContainerStyle.Width(boxWidth).Height(logHeight).Render(strings.Join(logLines, "\n"))

	returnPrompt := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorFgWhite).
		Background(ColorPrimary).
		Padding(0, 2).
		Render("[Enter] / [Esc] / [q] Return to Main Inspector")

	content := lipgloss.JoinVertical(lipgloss.Left,
		titleBanner,
		"",
		strings.Join(summaryLines, "\n"),
		"",
		lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("Final Execution Logs:"),
		logBox,
		"",
		returnPrompt,
	)

	return lipgloss.JoinVertical(lipgloss.Left, header, content)
}
