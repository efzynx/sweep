package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/efzynx/sweep/internal/core"
)

// renderHeader generates the top application banner, distro badge, and disk meter.
func (m *Model) renderHeader() string {
	topWidth := m.width
	if topWidth < 40 {
		topWidth = 40
	}

	// Top Line: SWEEP banner, author credit, and distro badge
	banner := BannerStyle.Render("SWEEP")
	sub := BannerSubStyle.Render("SYSTEM CLEANER")
	author := AuthorStyle.Render("• By Ahmad Fauzan Adiman (Efzyn)")

	distroStr := "Unknown Linux"
	if m.distroInfo != nil {
		if m.distroInfo.Family != "" && m.distroInfo.Family != "unknown" {
			distroStr = fmt.Sprintf("%s (%s)", m.distroInfo.PrettyName, strings.ToUpper(string(m.distroInfo.Family)))
		} else {
			distroStr = m.distroInfo.PrettyName
		}
	}
	distroBadge := DistroBadgeStyle.Render(fmt.Sprintf("[Distro: %s]", distroStr))

	var topLine string
	var authorLine string
	if topWidth >= 90 {
		appTitle := lipgloss.JoinHorizontal(lipgloss.Center, banner, sub, lipgloss.NewStyle().SetString(" ").String(), author)
		gapWidth := topWidth - lipgloss.Width(appTitle) - lipgloss.Width(distroBadge) - 2
		if gapWidth < 1 {
			gapWidth = 1
		}
		topLine = lipgloss.JoinHorizontal(lipgloss.Center, appTitle, strings.Repeat(" ", gapWidth), distroBadge)
	} else {
		appTitle := lipgloss.JoinHorizontal(lipgloss.Center, banner, sub)
		gapWidth := topWidth - lipgloss.Width(appTitle) - lipgloss.Width(distroBadge) - 2
		if gapWidth < 1 {
			gapWidth = 1
		}
		topLine = lipgloss.JoinHorizontal(lipgloss.Center, appTitle, strings.Repeat(" ", gapWidth), distroBadge)
		authorLine = AuthorStyle.Render("By Ahmad Fauzan Adiman (Efzyn)")
	}

	// Target notification line
	targetBadge := TargetBadgeStyle.Render("[TARGET: ROOT FILESYSTEM (/)]")
	targetNotice := TargetTextStyle.Render(" • Mode Optimasi & Pemeliharaan Sistem Linux")
	targetLine := lipgloss.JoinHorizontal(lipgloss.Center, targetBadge, targetNotice)

	// Disk usage meter and storage metrics for root filesystem
	diskBarWidth := 20
	if m.width > 95 {
		diskBarWidth = 28
	}
	visualBar, statsLine := RenderRootDiskStats(m.diskStats, diskBarWidth)
	diskLabel := DiskInfoStyle.Render("Root (/) Disk Usage: ")
	diskMeter := DiskMeterStyle.Render(visualBar)
	diskStatsStyled := lipgloss.NewStyle().Foreground(ColorMuted).Render(statsLine)

	var diskSection string
	if topWidth >= 105 {
		diskSection = fmt.Sprintf("%s%s   %s", diskLabel, diskMeter, diskStatsStyled)
	} else {
		diskSection = fmt.Sprintf("%s%s\n%s", diskLabel, diskMeter, diskStatsStyled)
	}

	divider := lipgloss.NewStyle().Foreground(ColorSubtle).Render(strings.Repeat("─", topWidth))

	var headerParts []string
	headerParts = append(headerParts, topLine)
	if authorLine != "" {
		headerParts = append(headerParts, authorLine)
	}
	headerParts = append(headerParts, targetLine, diskSection, divider)

	return lipgloss.JoinVertical(lipgloss.Left, headerParts...)
}

// renderLeftPane renders the list of detected modules with statuses, counts, and sizes.
func (m *Model) renderLeftPane(width, height int) string {
	titleStyle := PaneTitleInactive
	borderStyle := InactivePaneBorder
	if m.focus == FocusLeft {
		titleStyle = PaneTitleActive
		borderStyle = ActivePaneBorder
	}

	title := titleStyle.Render(fmt.Sprintf("Modules [%d]", len(m.modules)))

	var lines []string
	lines = append(lines, title)

	visibleRows := height - 3
	if visibleRows < 1 {
		visibleRows = 1
	}

	start := m.moduleScrollOff
	end := start + visibleRows
	if end > len(m.modules) {
		end = len(m.modules)
	}

	for i := start; i < end; i++ {
		mod := m.modules[i]
		isActive := (i == m.activeModuleIdx)

		// Checkbox indicator
		cb := "[ ]"
		if mod.IsAllSelected() {
			cb = "[x]"
		} else if mod.IsAnySelected() {
			cb = "[-]"
		}

		cursorPrefix := "  "
		if isActive {
			cursorPrefix = "> "
		}

		catBadge := RenderCategoryBadge(mod.Module.Category())

		var statusBadge string
		if mod.Scanning {
			statusBadge = lipgloss.NewStyle().Foreground(ColorWarning).Render("[Scanning...]")
		} else if mod.Error != nil {
			statusBadge = lipgloss.NewStyle().Foreground(ColorDanger).Render("[!]")
		} else {
			statusBadge = SizeBadgeStyle.Render(core.FormatBytes(mod.TotalBytes()))
		}

		name := mod.Module.Name()
		// Compute max available width for name
		availForName := width - len(cursorPrefix) - len(cb) - lipgloss.Width(catBadge) - lipgloss.Width(statusBadge) - 6
		if availForName < 4 {
			availForName = 4
		}
		if len(name) > availForName {
			name = name[:availForName-1] + "…"
		}

		rowContent := fmt.Sprintf("%s%s %s %s %s", cursorPrefix, cb, name, catBadge, statusBadge)

		if isActive {
			rowStyle := CursorRowStyle.Width(width - 2)
			lines = append(lines, rowStyle.Render(rowContent))
		} else {
			rowStyle := NormalRowStyle.Width(width - 2)
			lines = append(lines, rowStyle.Render(rowContent))
		}
	}

	for len(lines) < height {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")
	return borderStyle.Width(width).Height(height).Render(content)
}

// renderRightPane renders the cleanable items table for the currently active module.
func (m *Model) renderRightPane(width, height int) string {
	titleStyle := PaneTitleInactive
	borderStyle := InactivePaneBorder
	if m.focus == FocusRight {
		titleStyle = PaneTitleActive
		borderStyle = ActivePaneBorder
	}

	active := m.ActiveModule()
	titleText := "Item Inspector"
	if active != nil {
		titleText = fmt.Sprintf("Inspector: %s (%d/%d selected, %s)",
			active.Module.Name(),
			active.SelectedCount(),
			active.ItemCount(),
			core.FormatBytes(active.SelectedBytes()),
		)
	}
	title := titleStyle.Render(titleText)

	var lines []string
	lines = append(lines, title)

	if active == nil || len(active.ScanRes.Items) == 0 {
		var emptyMsg string
		if active != nil && active.Scanning {
			emptyMsg = lipgloss.NewStyle().Foreground(ColorWarning).Render("  Scanning module for cleanable items...")
		} else if active != nil && active.Error != nil {
			emptyMsg = lipgloss.NewStyle().Foreground(ColorDanger).Render(fmt.Sprintf("  Scan error: %v", active.Error))
		} else {
			emptyMsg = lipgloss.NewStyle().Foreground(ColorMuted).Render("  No cleanable items discovered for this module.")
		}
		lines = append(lines, emptyMsg)
		for len(lines) < height {
			lines = append(lines, "")
		}
		return borderStyle.Width(width).Height(height).Render(strings.Join(lines, "\n"))
	}

	// Table column widths
	colCheck := 5   // " [x] "
	colSize := 11   // " 1024.0 MB "
	colSafety := 11 // " [CAUTION] "
	colName := 28
	if width < 80 {
		colName = 18
	}
	colPath := width - colCheck - colName - colSize - colSafety - 6
	if colPath < 10 {
		colPath = 10
	}

	// Table Header
	headerFmt := fmt.Sprintf("  %-*s %-*s %-*s %-*s %s",
		colCheck-2, "SEL",
		colName, "NAME / ID",
		colSize, "SIZE",
		colSafety, "SAFETY",
		"PATH / DETAILS",
	)
	lines = append(lines, TableHeaderStyle.Width(width-2).Render(headerFmt))

	visibleRows := height - 4
	if visibleRows < 1 {
		visibleRows = 1
	}

	items := active.ScanRes.Items
	start := active.ScrollOffset
	end := start + visibleRows
	if end > len(items) {
		end = len(items)
	}

	for i := start; i < end; i++ {
		it := items[i]
		isCursor := (i == active.Cursor)

		cursorMark := " "
		if isCursor {
			cursorMark = ">"
		}

		cb := "[ ]"
		if it.Selected {
			cb = "[x]"
		}

		name := it.Name
		if len(name) > colName {
			name = name[:colName-1] + "…"
		}

		sizeStr := core.FormatBytes(it.Size)
		safetyBadge := RenderSafetyBadge(it.Safety)

		detail := it.Path
		if detail == "" {
			detail = it.Description
		}
		if len(detail) > colPath {
			detail = detail[:colPath-1] + "…"
		}

		rowContent := fmt.Sprintf("%s %s %-*s %-*s %s %s",
			cursorMark,
			cb,
			colName, name,
			colSize, sizeStr,
			safetyBadge,
			detail,
		)

		if isCursor {
			lines = append(lines, CursorRowStyle.Width(width-2).Render(rowContent))
		} else {
			lines = append(lines, NormalRowStyle.Width(width-2).Render(rowContent))
		}
	}

	for len(lines) < height {
		lines = append(lines, "")
	}

	return borderStyle.Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

// renderFooter produces the keybinding command legend and total selection summary.
func (m *Model) renderFooter() string {
	keys := []string{
		RenderKeyHelp("[Tab]", "pane"),
		RenderKeyHelp("[Space]", "toggle"),
		RenderKeyHelp("[a]", "all"),
		RenderKeyHelp("[s]", "rescan"),
		RenderKeyHelp("[d]", "dry-run"),
		RenderKeyHelp("[Enter]", "clean"),
		RenderKeyHelp("[?]", "info"),
		RenderKeyHelp("[q]", "quit"),
	}

	legend := strings.Join(keys, KeyDivider.String())

	summary := fmt.Sprintf("Selected: %s / %s (%d items)",
		core.FormatBytes(m.TotalSelectedBytes()),
		core.FormatBytes(m.TotalScannedBytes()),
		m.TotalSelectedCount(),
	)
	summaryStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(summary)

	footWidth := m.width
	if footWidth < 40 {
		footWidth = 40
	}

	gap := footWidth - lipgloss.Width(legend) - lipgloss.Width(summaryStyled) - 4
	if gap < 1 {
		gap = 1
	}

	content := lipgloss.JoinHorizontal(lipgloss.Center, legend, strings.Repeat(" ", gap), summaryStyled)
	return FooterContainer.Width(footWidth).Render(content)
}
