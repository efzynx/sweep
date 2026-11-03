package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/efzynx/sweep/internal/core"
)

// formatCategoryTitle formats a subsystem category into a clean Nordic category header.
func formatCategoryTitle(cat string) string {
	switch strings.ToLower(strings.TrimSpace(cat)) {
	case "package_manager", "sandboxed_apps", "pkg", "pacman", "flatpak", "snap", "apt", "dnf", "aur":
		return "PACKAGE MANAGERS"
	case "containers", "docker", "podman", "virtualization":
		return "CONTAINERS & VIRTUALIZATION"
	case "devcache", "dev_cache", "developer", "developer_cache", "npm", "go", "rust", "cargo", "pip", "uv", "build_cache":
		return "DEVELOPER CACHES"
	case "user_cache", "usercache", "desktop", "browser", "trash", "thumbnails", "system_logs", "logs", "journal", "system":
		return "SYSTEM & DESKTOP"
	case "snapshots", "snapshot", "filesystem", "snapper", "btrfs":
		return "FILESYSTEM & SNAPSHOTS"
	case "custom", "user_custom", "rules", "custom_rules", "paths":
		return "USER CUSTOM RULES"
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

	// Right segment: [BTRFS SAFEGUARD: ACTIVE] badge (or INACTIVE if Snapper unavailable)
	var rightBadge string
	if m.snapperAvailable {
		rightBadge = SafeguardActiveBadge
	} else {
		rightBadge = SafeguardInactiveBadge
	}

	var centerSegment string
	var rightSegment string

	var targetBadge string
	switch m.storageFilter {
	case core.StorageScopeRoot:
		targetBadge = TargetBadgeStyle.Render("[TARGET: ROOT /]")
	case core.StorageScopeHome:
		targetBadge = TargetBadgeStyle.Render("[TARGET: HOME /home]")
	default:
		targetBadge = TargetBadgeStyle.Render("[TARGET: ALL]")
	}

	if m.storageInfo.HasSeparateHome {
		rootLabel := lipgloss.NewStyle().Foreground(ColorMuted).Render("Root (/): ")
		rootPct := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(fmt.Sprintf("%.1f%%", m.storageInfo.Root.UsedPercent))
		rootSisa := lipgloss.NewStyle().Foreground(ColorMuted).Render(" [Sisa: ") +
			lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Render(core.FormatBytes(int64(m.storageInfo.Root.FreeBytes))) +
			lipgloss.NewStyle().Foreground(ColorMuted).Render("]")

		bullet := lipgloss.NewStyle().Foreground(ColorSubtle).Render(" • ")

		homeLabel := lipgloss.NewStyle().Foreground(ColorMuted).Render("Home (/home): ")
		homePct := lipgloss.NewStyle().Bold(true).Foreground(ColorAccent).Render(fmt.Sprintf("%.1f%%", m.storageInfo.Home.UsedPercent))
		homeSisa := lipgloss.NewStyle().Foreground(ColorMuted).Render(" [Sisa: ") +
			lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Render(core.FormatBytes(int64(m.storageInfo.Home.FreeBytes))) +
			lipgloss.NewStyle().Foreground(ColorMuted).Render("]")

		centerSegment = fmt.Sprintf("%s%s%s%s%s%s%s", rootLabel, rootPct, rootSisa, bullet, homeLabel, homePct, homeSisa)
		rightSegment = fmt.Sprintf("%s  %s", targetBadge, rightBadge)
	} else {
		// Single filesystem
		rootLabel := lipgloss.NewStyle().Foreground(ColorMuted).Render("Root (/): ")
		usedPct := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(fmt.Sprintf("%.1f%% Used", m.diskStats.UsedPercent))
		totalStr := core.FormatBytes(int64(m.diskStats.TotalBytes))
		freeStr := core.FormatBytes(int64(m.diskStats.FreeBytes))
		totalPart := lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf(" [Total: %s • Sisa: ", totalStr))
		freePart := lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Render(freeStr)
		closeBracket := lipgloss.NewStyle().Foreground(ColorMuted).Render("]")
		centerSegment = fmt.Sprintf("%s%s%s%s%s", rootLabel, usedPct, totalPart, freePart, closeBracket)

		if m.storageFilter != core.StorageScopeAll {
			rightSegment = fmt.Sprintf("%s  %s", targetBadge, rightBadge)
		} else {
			rightSegment = rightBadge
		}
	}

	divider := lipgloss.NewStyle().Foreground(ColorNordicBorder).Render(strings.Repeat("─", topWidth))

	leftW := lipgloss.Width(leftSegment)
	centerW := lipgloss.Width(centerSegment)
	rightW := lipgloss.Width(rightSegment)

	// Wide screens: single-line statusline + divider (strictly 2 rows)
	if topWidth >= 90 && leftW+centerW+rightW+4 <= topWidth {
		gapTotal := topWidth - leftW - centerW - rightW
		gap1 := gapTotal / 2
		gap2 := gapTotal - gap1
		statusline := fmt.Sprintf("%s%s%s%s%s", leftSegment, strings.Repeat(" ", gap1), centerSegment, strings.Repeat(" ", gap2), rightSegment)
		return lipgloss.JoinVertical(lipgloss.Left, statusline, divider)
	}

	// Narrow screens (< 90 cols or when statusline exceeds width): 2 status rows + divider (strictly 3 rows)
	gapLine1 := topWidth - leftW - rightW
	if gapLine1 < 1 {
		gapLine1 = 1
	}
	line1 := fmt.Sprintf("%s%s%s", leftSegment, strings.Repeat(" ", gapLine1), rightSegment)
	if lipgloss.Width(line1) > topWidth {
		line1 = lipgloss.NewStyle().MaxWidth(topWidth).Render(line1)
	}
	line2 := centerSegment
	if lipgloss.Width(line2) > topWidth {
		line2 = lipgloss.NewStyle().MaxWidth(topWidth).Render(line2)
	}

	return lipgloss.JoinVertical(lipgloss.Left, line1, line2, divider)
}

type treeEntry struct {
	text     string
	modIdx   int // module index in m.modules, or -1 for category header
	isHeader bool
}

// buildLeftPaneEntries formats all categories and module tree nodes for the subsystems pane.
func (m *Model) buildLeftPaneEntries(width int) ([]treeEntry, int) {
	var entries []treeEntry
	activeEntryIdx := 0

	if len(m.modules) == 0 {
		return entries, 0
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

	for _, catTitle := range categoryOrder {
		modIndices := catModules[catTitle]
		if len(modIndices) == 0 {
			continue
		}

		catHeader := lipgloss.NewStyle().Foreground(ColorMuted).Bold(true).Render("  " + catTitle)
		entries = append(entries, treeEntry{
			text:     catHeader,
			modIdx:   -1,
			isHeader: true,
		})

		for itemIdx, modIdx := range modIndices {
			mod := m.modules[modIdx]
			isActive := (modIdx == m.activeModuleIdx)
			if isActive {
				activeEntryIdx = len(entries)
			}

			isLastInGroup := (itemIdx == len(modIndices)-1)
			treeGlyph := "├─ "
			if isLastInGroup {
				treeGlyph = "└─ "
			}

			cb := "[ ] "
			if m.isModuleFilteredAllSelected(mod) {
				cb = "[x] "
			} else if m.isModuleFilteredAnySelected(mod) {
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
				bytes := m.ModuleFilteredBytes(mod)
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
			name = truncateString(name, maxNameW)

			gap := avail - prefixW - lipgloss.Width(name) - badgeW
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

			entries = append(entries, treeEntry{
				text:     renderedRow,
				modIdx:   modIdx,
				isHeader: false,
			})
		}
	}

	return entries, activeEntryIdx
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

	var lines []string

	if len(m.modules) == 0 {
		title := titleStyle.Render(titleText)
		lines = append(lines, title)
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

	entries, activeIdx := m.buildLeftPaneEntries(width)

	availRows := height - 1
	if availRows < 1 {
		availRows = 1
	}

	if len(entries) <= availRows {
		m.moduleScrollOff = 0
	} else {
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

		hasAbove := m.moduleScrollOff > 0
		hasBelow := m.moduleScrollOff+availRows < len(entries)
		if hasAbove && hasBelow {
			titleText = fmt.Sprintf("SUBSYSTEMS [%d DETECTED] [↑↓]", len(m.modules))
		} else if hasAbove {
			titleText = fmt.Sprintf("SUBSYSTEMS [%d DETECTED] [↑]", len(m.modules))
		} else if hasBelow {
			titleText = fmt.Sprintf("SUBSYSTEMS [%d DETECTED] [↓]", len(m.modules))
		}
	}

	title := titleStyle.Render(titleText)
	lines = append(lines, title)

	startIdx := m.moduleScrollOff
	endIdx := startIdx + availRows
	if endIdx > len(entries) {
		endIdx = len(entries)
	}
	if startIdx < len(entries) {
		for i := startIdx; i < endIdx; i++ {
			lines = append(lines, entries[i].text)
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

// formatItemDetail produces a clean description string combining description and path.
func formatItemDetail(it core.CleanableItem) string {
	desc := strings.TrimSpace(it.Description)
	path := strings.TrimSpace(it.Path)

	if desc != "" && path != "" {
		if strings.Contains(desc, path) {
			return desc
		}
		return fmt.Sprintf("%s — %s", desc, path)
	}
	if desc != "" {
		return desc
	}
	if path != "" {
		return path
	}
	return "-"
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
	var filteredIndices []int
	if active != nil {
		filteredIndices = m.FilteredItemIndices(active)
		leftText = fmt.Sprintf("%s • %d ITEMS TERDETEKSI", strings.ToUpper(active.Module.Name()), len(filteredIndices))
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

	if active == nil || len(filteredIndices) == 0 {
		wrapText := func(text string, style lipgloss.Style) []string {
			maxW := width - 6
			if maxW < 20 {
				maxW = 20
			}
			wrapped := lipgloss.NewStyle().Width(maxW).Render(text)
			var out []string
			for _, line := range strings.Split(wrapped, "\n") {
				out = append(out, "  "+style.Render(line))
			}
			return out
		}

		var emptyLines []string
		if active != nil && active.Scanning {
			emptyLines = append(emptyLines, wrapText("Memindai modul untuk item yang dapat dibersihkan...", lipgloss.NewStyle().Foreground(ColorWarning))...)
		} else if active != nil && active.Error != nil {
			emptyLines = append(emptyLines, wrapText(fmt.Sprintf("Scan error: %v", active.Error), lipgloss.NewStyle().Foreground(ColorDanger))...)
		} else if active != nil && len(active.ScanRes.Items) > 0 && len(filteredIndices) == 0 {
			emptyLines = append(emptyLines, wrapText(fmt.Sprintf("Tidak ada item untuk target [%s] pada modul ini.", strings.ToUpper(string(m.storageFilter))), lipgloss.NewStyle().Foreground(ColorMuted))...)
			emptyLines = append(emptyLines, "")
			emptyLines = append(emptyLines, wrapText("Tekan [p] untuk mengubah target penyimpanan (All / Root / Home).", lipgloss.NewStyle().Foreground(ColorPrimary))...)
		} else if active != nil && (active.Module.ID() == "custom" || active.Module.Category() == "custom") {
			if m.cfg == nil || len(m.cfg.CustomRules) == 0 {
				emptyLines = append(emptyLines, wrapText("Belum ada aturan custom path yang dikonfigurasi.", lipgloss.NewStyle().Foreground(ColorFgWhite).Bold(true))...)
				emptyLines = append(emptyLines, "")
				emptyLines = append(emptyLines, wrapText("Tekan [n] untuk menambah aturan path kustom baru langsung dari sini.", lipgloss.NewStyle().Foreground(ColorPrimary))...)
			} else {
				emptyLines = append(emptyLines, wrapText("Tidak ditemukan item yang cocok dengan aturan custom path aktif.", lipgloss.NewStyle().Foreground(ColorFgWhite).Bold(true))...)
				emptyLines = append(emptyLines, "")
				emptyLines = append(emptyLines, wrapText("Tekan [n] untuk menambah aturan path kustom baru langsung dari sini.", lipgloss.NewStyle().Foreground(ColorPrimary))...)
			}
		} else if active != nil && active.Module.ID() == "snapper" {
			var hasPermDenied bool
			if active.ScanRes.Metadata != nil {
				_, hasPermDenied = active.ScanRes.Metadata["permission_denied"]
			}
			isUnprivileged := os.Geteuid() != 0
			if hasPermDenied || isUnprivileged {
				emptyLines = append(emptyLines, wrapText("Snapper memerlukan izin root untuk membaca daftar snapshot.", lipgloss.NewStyle().Foreground(ColorWarning).Bold(true))...)
				emptyLines = append(emptyLines, "")
				emptyLines = append(emptyLines, wrapText("Petunjuk: Jalankan 'sudo sweep' untuk memindai snapshot, atau tambahkan user ke /etc/snapper/configs/root.", lipgloss.NewStyle().Foreground(ColorFgWhite))...)
				emptyLines = append(emptyLines, wrapText("Catatan: Snapshot aktif (#0) dan snapshot < 3 hari otomatis dilindungi demi keamanan.", lipgloss.NewStyle().Foreground(ColorMuted))...)
			} else {
				emptyLines = append(emptyLines, wrapText("Sistem Anda bersih! Tidak ditemukan item yang memenuhi kriteria pembersihan untuk modul ini.", lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true))...)
				emptyLines = append(emptyLines, "")
				emptyLines = append(emptyLines, wrapText("Catatan: Snapshot aktif (#0) dan snapshot < 3 hari otomatis dilindungi demi keamanan.", lipgloss.NewStyle().Foreground(ColorMuted))...)
			}
		} else {
			emptyLines = append(emptyLines, wrapText("Sistem Anda bersih! Tidak ditemukan item yang memenuhi kriteria pembersihan untuk modul ini.", lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true))...)
		}

		lines = append(lines, "")
		for _, el := range emptyLines {
			lines = append(lines, el)
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

	// Table column widths
	colSel := 6
	colStorage := 5
	colName := 28
	if width >= 105 {
		colName = 34
	} else if width >= 85 {
		colName = 32
	} else if width < 70 {
		colName = 20
	} else if width < 80 {
		colName = 24
	}
	colSize := 11
	colSafety := 13
	colPath := width - colSel - colStorage - colName - colSize - colSafety - 5
	if colPath < 8 {
		colPath = 8
	}

	// Table Header
	headerFmt := fmt.Sprintf(" %-*s %-*s %-*s %-*s %-*s %s",
		colSel, "SEL",
		colStorage, "STOR",
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

	start := active.ScrollOffset
	end := start + visibleRows
	if end > len(filteredIndices) {
		end = len(filteredIndices)
	}

	for i := start; i < end; i++ {
		origIdx := filteredIndices[i]
		it := active.ScanRes.Items[origIdx]
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

		scope := core.ItemStorageScope(it)
		storageBadge := RenderStorageBadge(scope)
		storageCol := storageBadge + strings.Repeat(" ", colStorage-lipgloss.Width(storageBadge))

		name := truncateString(it.Name, colName)

		sizeStr := core.FormatBytes(it.Size)
		safetyBadge := RenderSafetyBadge(it.Safety)

		detail := formatItemDetail(it)
		detail = truncateString(detail, colPath)

		rowContent := fmt.Sprintf(" %-*s %s %-*s %-*s %-*s %s",
			colSel, selCol,
			storageCol,
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

	isCustom := false
	if active := m.ActiveModule(); active != nil && (active.Module.ID() == "custom" || active.Module.Category() == "custom") {
		isCustom = true
	}

	var keys []string
	if footWidth >= 130 {
		keys = []string{
			RenderKeyHelp("j/k", "navigasi"),
			RenderKeyHelp("Space", "toggle"),
			RenderKeyHelp("a", "all"),
		}
		if m.storageInfo.HasSeparateHome || m.storageFilter != core.StorageScopeAll {
			keys = append(keys, RenderKeyHelp("p", "storage target (all/root/home)"))
		}
		keys = append(keys,
			RenderKeyHelp("s", "rescan"),
			RenderKeyHelp("d", "dry-run"),
			RenderKeyHelp("Enter", "clean"),
		)
		if isCustom {
			keys = append(keys, RenderKeyHelp("n", "tambah"), RenderKeyHelp("x", "hapus"))
		}
		keys = append(keys,
			RenderKeyHelp("?", "info"),
			RenderKeyHelp("q", "quit"),
		)
	} else if footWidth >= 100 {
		keys = []string{
			RenderKeyHelp("j/k", "nav"),
			RenderKeyHelp("Space", "toggle"),
			RenderKeyHelp("a", "all"),
		}
		if m.storageInfo.HasSeparateHome || m.storageFilter != core.StorageScopeAll {
			keys = append(keys, RenderKeyHelp("p", "storage target"))
		}
		keys = append(keys,
			RenderKeyHelp("d", "dry-run"),
			RenderKeyHelp("Enter", "clean"),
		)
		if isCustom {
			keys = append(keys, RenderKeyHelp("n", "tambah"))
		}
		keys = append(keys,
			RenderKeyHelp("?", "info"),
			RenderKeyHelp("q", "quit"),
		)
	} else {
		keys = []string{
			RenderKeyHelp("Space", "toggle"),
		}
		if m.storageInfo.HasSeparateHome || m.storageFilter != core.StorageScopeAll {
			keys = append(keys, RenderKeyHelp("p", "target"))
		}
		keys = append(keys,
			RenderKeyHelp("d", "dry"),
			RenderKeyHelp("Enter", "clean"),
		)
		if isCustom {
			keys = append(keys, RenderKeyHelp("n", "add"))
		}
		keys = append(keys,
			RenderKeyHelp("?", "info"),
			RenderKeyHelp("q", "quit"),
		)
	}

	legend := strings.Join(keys, bullet)

	var summaryStyled string
	availInner := footWidth - 4
	if availInner < 10 {
		availInner = 10
	}

	if m.storageInfo.HasSeparateHome {
		homeSel, rootSel := m.SelectedStorageBytes()
		totSel := homeSel + rootSel

		selLabel := lipgloss.NewStyle().Foreground(ColorMuted).Render("Selected: ")
		totSelStr := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(core.FormatBytes(totSel))
		openParen := lipgloss.NewStyle().Foreground(ColorMuted).Render(" (Home: ")
		homeStr := lipgloss.NewStyle().Bold(true).Foreground(ColorAccent).Render(core.FormatBytes(homeSel))
		bulletSep := lipgloss.NewStyle().Foreground(ColorSubtle).Render(" • ")
		rootLabel := lipgloss.NewStyle().Foreground(ColorMuted).Render("Root: ")
		rootStr := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(core.FormatBytes(rootSel))
		closeParen := lipgloss.NewStyle().Foreground(ColorMuted).Render(")")

		fullSummary := fmt.Sprintf("%s%s%s%s%s%s%s%s", selLabel, totSelStr, openParen, homeStr, bulletSep, rootLabel, rootStr, closeParen)
		compactSummary := fmt.Sprintf("%s%s%s%s%s%s", totSelStr, openParen, homeStr, bulletSep, rootStr, closeParen)

		if lipgloss.Width(legend)+lipgloss.Width(fullSummary)+2 <= availInner {
			summaryStyled = fullSummary
		} else if lipgloss.Width(legend)+lipgloss.Width(compactSummary)+2 <= availInner {
			summaryStyled = compactSummary
		} else {
			summaryStyled = fullSummary
		}
	} else {
		reclaimableLabel := lipgloss.NewStyle().Foreground(ColorMuted).Render("Total Reclaimable: ")
		selBytes := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(core.FormatBytes(m.TotalSelectedBytes()))
		slash := lipgloss.NewStyle().Foreground(ColorMuted).Render(" / ")
		totBytes := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(core.FormatBytes(m.TotalScannedBytes()))

		if lipgloss.Width(legend)+lipgloss.Width(reclaimableLabel)+lipgloss.Width(selBytes)+lipgloss.Width(slash)+lipgloss.Width(totBytes)+2 <= availInner {
			summaryStyled = fmt.Sprintf("%s%s%s%s", reclaimableLabel, selBytes, slash, totBytes)
		} else {
			summaryStyled = fmt.Sprintf("%s%s%s", selBytes, slash, totBytes)
		}
	}

	if lipgloss.Width(legend)+lipgloss.Width(summaryStyled)+1 > availInner {
		content := fmt.Sprintf("%s\n%s", legend, summaryStyled)
		return FooterContainer.Width(footWidth).Render(content)
	}

	gap := availInner - lipgloss.Width(legend) - lipgloss.Width(summaryStyled)
	if gap < 1 {
		gap = 1
	}

	content := fmt.Sprintf("%s%s%s", legend, strings.Repeat(" ", gap), summaryStyled)
	return FooterContainer.Width(footWidth).Render(content)
}
