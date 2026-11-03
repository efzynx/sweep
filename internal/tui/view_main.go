package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/efzynx/sweep/internal/core"
)

// formatCategoryTitle formats a subsystem category into a clean Nordic category header.
func formatCategoryTitle(cat string) string {
	switch strings.ToLower(strings.TrimSpace(cat)) {
	case "package_manager", "sandboxed_apps", "pkg", "pacman", "flatpak", "apt", "dnf":
		return "PACKAGE MANAGERS"
	case "containers", "docker", "podman":
		return "CONTAINERS & VIRTUALIZATION"
	case "snapshots", "filesystem", "system", "snapper", "btrfs":
		return "FILESYSTEM & SYSTEM"
	default:
		c := strings.ReplaceAll(strings.ToUpper(cat), "_", " ")
		if c == "" {
			return "OTHER SUBSYSTEMS"
		}
		return c
	}
}

// renderHeader generates the ultra-compact Nordic Minimalist (Telescope-style) statusline.
// Maximum vertical height: 2 rows (statusline + divider), or 3 rows on narrow screens (< 90 cols).
func (m *Model) renderHeader() string {
	topWidth := m.width
	if topWidth < 40 {
		topWidth = 40
	}

	distroStr := "Linux"
	if m.distroInfo != nil {
		if m.distroInfo.PrettyName != "" {
			distroStr = m.distroInfo.PrettyName
		}
		if m.distroInfo.Family != "" && m.distroInfo.Family != "unknown" {
			famRaw := string(m.distroInfo.Family)
			famTitle := strings.ToUpper(famRaw[:1]) + strings.ToLower(famRaw[1:])
			distroStr = fmt.Sprintf("%s (%s)", distroStr, famTitle)
		}
	}

	// Left segment: SWEEP // <Distro> // by Efzyn
	sweepText := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("SWEEP")
	slashDim := lipgloss.NewStyle().Foreground(ColorSubtle).Render("//")
	distroStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorFgWhite).Render(distroStr)
	authorDim := lipgloss.NewStyle().Foreground(ColorMuted).Render("by Efzyn")
	leftSegment := fmt.Sprintf("%s %s %s %s %s", sweepText, slashDim, distroStyled, slashDim, authorDim)

	// Center segment: Root (/): <Pct>% Used [Total: <Total> • Sisa: <Free>]
	rootLabel := lipgloss.NewStyle().Foreground(ColorMuted).Render("Root (/): ")
	usedPct := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(fmt.Sprintf("%.1f%% Used", m.diskStats.UsedPercent))
	totalStr := core.FormatBytes(int64(m.diskStats.TotalBytes))
	freeStr := core.FormatBytes(int64(m.diskStats.FreeBytes))
	totalPart := lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf(" [Total: %s • Sisa: ", totalStr))
	freePart := lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Render(freeStr)
	closeBracket := lipgloss.NewStyle().Foreground(ColorMuted).Render("]")
	centerSegment := fmt.Sprintf("%s%s%s%s%s", rootLabel, usedPct, totalPart, freePart, closeBracket)

	// Right segment: [BTRFS SAFEGUARD: ACTIVE] badge (or INACTIVE if Snapper unavailable)
	var rightBadge string
	if m.snapperAvailable {
		rightBadge = SafeguardActiveBadge
	} else {
		rightBadge = SafeguardInactiveBadge
	}

	divider := lipgloss.NewStyle().Foreground(ColorNordicBorder).Render(strings.Repeat("─", topWidth))

	leftW := lipgloss.Width(leftSegment)
	centerW := lipgloss.Width(centerSegment)
	rightW := lipgloss.Width(rightBadge)

	// Wide screens: single-line statusline + divider (strictly 2 rows)
	if topWidth >= 90 && leftW+centerW+rightW+4 <= topWidth {
		gapTotal := topWidth - leftW - centerW - rightW
		gap1 := gapTotal / 2
		gap2 := gapTotal - gap1
		statusline := fmt.Sprintf("%s%s%s%s%s", leftSegment, strings.Repeat(" ", gap1), centerSegment, strings.Repeat(" ", gap2), rightBadge)
		return lipgloss.JoinVertical(lipgloss.Left, statusline, divider)
	}

	// Narrow screens (< 90 cols or when statusline exceeds width): 2 status rows + divider (strictly 3 rows)
	gapLine1 := topWidth - leftW - rightW
	if gapLine1 < 1 {
		gapLine1 = 1
	}
	line1 := fmt.Sprintf("%s%s%s", leftSegment, strings.Repeat(" ", gapLine1), rightBadge)
	if lipgloss.Width(line1) > topWidth {
		line1 = lipgloss.NewStyle().MaxWidth(topWidth).Render(line1)
	}
	line2 := centerSegment
	if lipgloss.Width(line2) > topWidth {
		line2 = lipgloss.NewStyle().MaxWidth(topWidth).Render(line2)
	}

	return lipgloss.JoinVertical(lipgloss.Left, line1, line2, divider)
}

// renderLeftPane renders the tree-structured subsystems list grouped by category.
func (m *Model) renderLeftPane(width, height int) string {
	titleStyle := PaneTitleInactive
	borderStyle := InactivePaneBorder
	if m.focus == FocusLeft {
		titleStyle = PaneTitleActive
		borderStyle = ActivePaneBorder
	}

	titleText := fmt.Sprintf("SUBSYSTEMS [%d DETECTED]", len(m.modules))
	title := titleStyle.Render(titleText)

	var lines []string
	lines = append(lines, title)

	if len(m.modules) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(ColorMuted).Render("  No subsystems detected."))
		for len(lines) < height {
			lines = append(lines, "")
		}
		if len(lines) > height {
			lines = lines[:height]
		}
		content := strings.Join(lines, "\n")
		return borderStyle.Width(width).Height(height).Render(content)
	}

	// Group module indices by category preserving order
	var categoryOrder []string
	catModules := make(map[string][]int)

	for i, mod := range m.modules {
		catTitle := formatCategoryTitle(mod.Module.Category())
		if len(catModules[catTitle]) == 0 {
			categoryOrder = append(categoryOrder, catTitle)
		}
		catModules[catTitle] = append(catModules[catTitle], i)
	}

	for groupIdx, catTitle := range categoryOrder {
		modIndices := catModules[catTitle]
		if len(modIndices) == 0 {
			continue
		}

		// Subtle spacing between categories if vertical height allows
		if groupIdx > 0 && len(lines) < height-len(modIndices)-1 {
			lines = append(lines, "")
		}

		catHeader := lipgloss.NewStyle().Foreground(ColorMuted).Bold(true).Render("  " + catTitle)
		lines = append(lines, catHeader)

		for itemIdx, modIdx := range modIndices {
			mod := m.modules[modIdx]
			isActive := (modIdx == m.activeModuleIdx)

			isLastInGroup := (itemIdx == len(modIndices)-1)
			treeGlyph := "├─ "
			if isLastInGroup {
				treeGlyph = "└─ "
			}

			cb := "[ ] "
			if mod.IsAllSelected() {
				cb = "[x] "
			} else if mod.IsAnySelected() {
				cb = "[-] "
			}

			cursor := "  "
			if isActive {
				cursor = "> "
			}

			name := mod.Module.Name()

			var statusBadge string
			if mod.Scanning {
				statusBadge = lipgloss.NewStyle().Foreground(ColorWarning).Render("[Scanning...]")
			} else if mod.Error != nil {
				statusBadge = lipgloss.NewStyle().Foreground(ColorDanger).Render("[!]")
			} else {
				bytes := mod.TotalBytes()
				if bytes > 0 {
					if isActive {
						statusBadge = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(core.FormatBytes(bytes))
					} else {
						statusBadge = lipgloss.NewStyle().Foreground(ColorSuccess).Render(core.FormatBytes(bytes))
					}
				} else {
					statusBadge = lipgloss.NewStyle().Foreground(ColorMuted).Render("0 B")
				}
			}

			avail := width - 2
			prefix := cursor + treeGlyph + cb
			prefixW := lipgloss.Width(prefix)
			badgeW := lipgloss.Width(statusBadge)

			maxNameW := avail - prefixW - badgeW - 1
			if maxNameW < 4 {
				maxNameW = 4
			}
			if len(name) > maxNameW {
				name = name[:maxNameW-1] + "…"
			}

			gap := avail - prefixW - len(name) - badgeW
			if gap < 1 {
				gap = 1
			}

			rowContent := fmt.Sprintf("%s%s%s%s", prefix, name, strings.Repeat(" ", gap), statusBadge)

			var renderedRow string
			if isActive {
				renderedRow = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Background(ColorHighlight).Render(rowContent)
			} else {
				renderedRow = NormalRowStyle.Render(rowContent)
			}
			lines = append(lines, renderedRow)
		}
	}

	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	content := strings.Join(lines, "\n")
	rendered := borderStyle.Width(width).Height(height).Render(content)

	renderedLines := strings.Split(rendered, "\n")
	expectedHeight := height + 2
	if len(renderedLines) > expectedHeight {
		top := renderedLines[0]
		bottom := renderedLines[len(renderedLines)-1]
		mid := renderedLines[1 : expectedHeight-1]
		renderedLines = append(append([]string{top}, mid...), bottom)
		rendered = strings.Join(renderedLines, "\n")
	}

	return rendered
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
	var leftText string
	if active != nil {
		leftText = fmt.Sprintf("%s • %d ITEMS TERDETEKSI", strings.ToUpper(active.Module.Name()), active.ItemCount())
	} else {
		leftText = "ITEM INSPECTOR • 0 ITEMS TERDETEKSI"
	}

	leftTitle := titleStyle.Render(leftText)
	tabHint := lipgloss.NewStyle().Foreground(ColorPrimary).Render("[Tab] switch pane")

	availTitle := width - 2
	gap := availTitle - lipgloss.Width(leftTitle) - lipgloss.Width(tabHint)
	var titleLine string
	if gap >= 1 {
		titleLine = leftTitle + strings.Repeat(" ", gap) + tabHint
	} else {
		titleLine = leftTitle
	}

	var lines []string
	lines = append(lines, titleLine)

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
		if len(lines) > height {
			lines = lines[:height]
		}
		content := strings.Join(lines, "\n")
		rendered := borderStyle.Width(width).Height(height).Render(content)
		renderedLines := strings.Split(rendered, "\n")
		expectedHeight := height + 2
		if len(renderedLines) > expectedHeight {
			top := renderedLines[0]
			bottom := renderedLines[len(renderedLines)-1]
			mid := renderedLines[1 : expectedHeight-1]
			renderedLines = append(append([]string{top}, mid...), bottom)
			rendered = strings.Join(renderedLines, "\n")
		}
		return rendered
	}

	// Table column widths
	colSel := 6
	colName := 28
	if width < 85 {
		colName = 18
	}
	colSize := 12
	colSafety := 14
	colPath := width - colSel - colName - colSize - colSafety - 4
	if colPath < 8 {
		colPath = 8
	}

	// Table Header
	headerFmt := fmt.Sprintf(" %-*s %-*s %-*s %-*s %s",
		colSel, "SEL",
		colName, "NAMA ITEM",
		colSize, "UKURAN",
		colSafety, "TINGKAT BAHAYA",
		"DESKRIPSI RINGKAS / PATH",
	)
	thRendered := TableHeaderStyle.Width(width - 2).Render(headerFmt)
	for _, thLine := range strings.Split(thRendered, "\n") {
		lines = append(lines, thLine)
	}

	visibleRows := height - len(lines)
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
		selCol := fmt.Sprintf("%s %s", cursorMark, cb)

		name := it.Name
		if len(name) > colName {
			name = name[:colName-1] + "…"
		}

		sizeStr := core.FormatBytes(it.Size)
		safetyBadge := RenderSafetyBadge(it.Safety)

		detail := it.Description
		if detail == "" {
			detail = it.Path
		}
		if len(detail) > colPath {
			detail = detail[:colPath-1] + "…"
		}

		rowContent := fmt.Sprintf(" %-*s %-*s %-*s %-*s %s",
			colSel, selCol,
			colName, name,
			colSize, sizeStr,
			colSafety, safetyBadge,
			detail,
		)

		var renderedRow string
		if isCursor {
			renderedRow = CursorRowStyle.Render(rowContent)
		} else {
			renderedRow = NormalRowStyle.Render(rowContent)
		}
		rowParts := strings.Split(renderedRow, "\n")
		lines = append(lines, rowParts[0])
	}

	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	content := strings.Join(lines, "\n")
	rendered := borderStyle.Width(width).Height(height).Render(content)

	renderedLines := strings.Split(rendered, "\n")
	expectedHeight := height + 2
	if len(renderedLines) > expectedHeight {
		top := renderedLines[0]
		bottom := renderedLines[len(renderedLines)-1]
		mid := renderedLines[1 : expectedHeight-1]
		renderedLines = append(append([]string{top}, mid...), bottom)
		rendered = strings.Join(renderedLines, "\n")
	}

	return rendered
}

// renderFooter produces the compact Nordic command bar and total reclaimable summary.
func (m *Model) renderFooter() string {
	footWidth := m.width
	if footWidth < 40 {
		footWidth = 40
	}

	bullet := lipgloss.NewStyle().Foreground(ColorSubtle).Render(" • ")

	var keys []string
	if footWidth >= 130 {
		keys = []string{
			RenderKeyHelp("j/k", "navigasi"),
			RenderKeyHelp("Space", "toggle"),
			RenderKeyHelp("a", "all"),
			RenderKeyHelp("s", "rescan"),
			RenderKeyHelp("d", "dry-run"),
			RenderKeyHelp("Enter", "clean"),
			RenderKeyHelp("?", "info"),
			RenderKeyHelp("q", "quit"),
		}
	} else if footWidth >= 100 {
		keys = []string{
			RenderKeyHelp("j/k", "nav"),
			RenderKeyHelp("Space", "toggle"),
			RenderKeyHelp("a", "all"),
			RenderKeyHelp("d", "dry-run"),
			RenderKeyHelp("Enter", "clean"),
			RenderKeyHelp("?", "info"),
			RenderKeyHelp("q", "quit"),
		}
	} else {
		keys = []string{
			RenderKeyHelp("Space", "toggle"),
			RenderKeyHelp("d", "dry"),
			RenderKeyHelp("Enter", "clean"),
			RenderKeyHelp("?", "info"),
			RenderKeyHelp("q", "quit"),
		}
	}

	legend := strings.Join(keys, bullet)

	var summaryStyled string
	reclaimableLabel := lipgloss.NewStyle().Foreground(ColorMuted).Render("Total Reclaimable: ")
	selBytes := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(core.FormatBytes(m.TotalSelectedBytes()))
	slash := lipgloss.NewStyle().Foreground(ColorMuted).Render(" / ")
	totBytes := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(core.FormatBytes(m.TotalScannedBytes()))

	availInner := footWidth - 4
	if availInner < 10 {
		availInner = 10
	}

	if lipgloss.Width(legend)+lipgloss.Width(reclaimableLabel)+lipgloss.Width(selBytes)+lipgloss.Width(slash)+lipgloss.Width(totBytes)+2 <= availInner {
		summaryStyled = fmt.Sprintf("%s%s%s%s", reclaimableLabel, selBytes, slash, totBytes)
	} else {
		summaryStyled = fmt.Sprintf("%s%s%s", selBytes, slash, totBytes)
	}

	gap := availInner - lipgloss.Width(legend) - lipgloss.Width(summaryStyled)
	if gap < 1 {
		gap = 1
	}

	content := fmt.Sprintf("%s%s%s", legend, strings.Repeat(" ", gap), summaryStyled)
	return FooterContainer.Width(footWidth).Render(content)
}
