package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/efzynx/sweep/internal/core"
)

// renderCustomRuleForm renders a centered modal form dialog for adding new custom scan paths.
func (m *Model) renderCustomRuleForm() string {
	modalWidth := 74
	if m.width < 78 {
		modalWidth = m.width - 4
	}
	if modalWidth < 50 {
		modalWidth = 50
	}

	title := ModalTitleStyle.Render("[ TAMBAH ATURAN CUSTOM PATH BARU ]")

	var lines []string
	lines = append(lines, title)

	subtitle := lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Width(modalWidth - 6).
		Render("Konfigurasikan direktori dan pola kustom untuk dipindai oleh Sweep.")
	lines = append(lines, subtitle)

	if m.height >= 24 {
		lines = append(lines, "")
	}

	labelWidth := 22

	// Field 0: Nama Aturan
	var nameLabel string
	if m.formFocusIndex == 0 {
		nameLabel = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(fmt.Sprintf("▸ %-*s", labelWidth-2, "Nama Aturan:"))
	} else {
		nameLabel = lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("  %-*s", labelWidth-2, "Nama Aturan:"))
	}
	lines = append(lines, fmt.Sprintf("%s %s", nameLabel, m.inputRuleName.View()))

	// Field 1: Path Direktori
	var pathLabel string
	if m.formFocusIndex == 1 {
		pathLabel = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(fmt.Sprintf("▸ %-*s", labelWidth-2, "Path Direktori:"))
	} else {
		pathLabel = lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("  %-*s", labelWidth-2, "Path Direktori:"))
	}
	lines = append(lines, fmt.Sprintf("%s %s", pathLabel, m.inputRulePath.View()))

	// Field 2: Filter Pola (Glob)
	var patLabel string
	if m.formFocusIndex == 2 {
		patLabel = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(fmt.Sprintf("▸ %-*s", labelWidth-2, "Filter Pola (Glob):"))
	} else {
		patLabel = lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("  %-*s", labelWidth-2, "Filter Pola (Glob):"))
	}
	lines = append(lines, fmt.Sprintf("%s %s", patLabel, m.inputRulePattern.View()))

	if m.height >= 24 {
		lines = append(lines, "")
	}

	// Field 3: Pindai Rekursif
	recText := "[ ] Tidak"
	if m.formRecursive {
		recText = "[x] Ya"
	}
	var recLabel, recValue string
	if m.formFocusIndex == 3 {
		recLabel = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(fmt.Sprintf("▸ %-*s", labelWidth-2, "Pindai Rekursif:"))
		recValue = lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess).Render(recText) + "  " + lipgloss.NewStyle().Foreground(ColorMuted).Render("(tekan Space untuk toggle)")
	} else {
		recLabel = lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("  %-*s", labelWidth-2, "Pindai Rekursif:"))
		recValue = lipgloss.NewStyle().Foreground(ColorFgWhite).Render(recText)
	}
	lines = append(lines, fmt.Sprintf("%s %s", recLabel, recValue))

	// Field 4: Tingkat Bahaya
	var safetyBadge string
	switch m.formSafety {
	case core.SafetySafe:
		safetyBadge = BadgeSafe
	case core.SafetyCaution:
		safetyBadge = BadgeCaution
	case core.SafetyHigh:
		safetyBadge = BadgeHigh
	default:
		safetyBadge = BadgeSafe
	}
	var safetyLabel, safetyValue string
	if m.formFocusIndex == 4 {
		safetyLabel = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render(fmt.Sprintf("▸ %-*s", labelWidth-2, "Tingkat Bahaya:"))
		safetyValue = fmt.Sprintf("%s  %s", safetyBadge, lipgloss.NewStyle().Foreground(ColorMuted).Render("(tekan Space untuk ubah)"))
	} else {
		safetyLabel = lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("  %-*s", labelWidth-2, "Tingkat Bahaya:"))
		safetyValue = safetyBadge
	}
	lines = append(lines, fmt.Sprintf("%s %s", safetyLabel, safetyValue))

	// Field 5: Submit Button
	if m.height >= 24 {
		lines = append(lines, "")
	}
	var buttonStr string
	if m.formFocusIndex == 5 {
		buttonStr = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorFgWhite).
			Background(ColorPrimary).
			Padding(0, 2).
			Render("[ SIMPAN & PINDAI ]")
	} else {
		buttonStr = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorNordicBorder).
			Padding(0, 1).
			Render("[ SIMPAN & PINDAI ]")
	}
	lines = append(lines, fmt.Sprintf("  %s", buttonStr))

	// Error Notice
	if m.formError != "" {
		errBox := ModalDangerBox.Width(modalWidth - 6).Render(fmt.Sprintf("[!] %s", m.formError))
		lines = append(lines, errBox)
	}

	if m.height >= 24 {
		lines = append(lines, "")
	}
	lines = append(lines, lipgloss.NewStyle().Foreground(ColorNordicBorder).Render(strings.Repeat("─", modalWidth-6)))

	bullet := lipgloss.NewStyle().Foreground(ColorSubtle).Render(" • ")
	var hints string
	if modalWidth >= 80 {
		hints = fmt.Sprintf("%s%s%s%s%s%s%s",
			RenderKeyHelp("[Tab]/[Down]", "Field Berikutnya"),
			bullet,
			RenderKeyHelp("[Space]", "Toggle"),
			bullet,
			RenderKeyHelp("[Enter]", "Simpan & Pindai"),
			bullet,
			RenderKeyHelp("[Esc]", "Batal"),
		)
	} else {
		hints = fmt.Sprintf("%s%s%s%s%s%s%s",
			RenderKeyHelp("[Tab]", "Next"),
			bullet,
			RenderKeyHelp("[Space]", "Toggle"),
			bullet,
			RenderKeyHelp("[Enter]", "Simpan"),
			bullet,
			RenderKeyHelp("[Esc]", "Batal"),
		)
	}
	lines = append(lines, hints)

	modalContent := strings.Join(lines, "\n")
	box := ModalOverlay.Width(modalWidth).Render(modalContent)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
