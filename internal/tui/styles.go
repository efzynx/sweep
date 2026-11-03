package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/efzynx/sweep/internal/core"
)

var (
	// Theme Color Palette
	ColorPrimary   = lipgloss.Color("#06B6D4") // Cyan
	ColorAccent    = lipgloss.Color("#8B5CF6") // Purple
	ColorSuccess   = lipgloss.Color("#10B981") // Green
	ColorWarning   = lipgloss.Color("#F59E0B") // Amber / Yellow
	ColorDanger    = lipgloss.Color("#EF4444") // Red
	ColorMuted     = lipgloss.Color("#6B7280") // Gray
	ColorSubtle    = lipgloss.Color("#4B5563") // Dark Gray
	ColorHighlight = lipgloss.Color("#1E293B") // Slate Blue / Active row
	ColorBgDark    = lipgloss.Color("#0F172A") // Deep Background
	ColorFgWhite   = lipgloss.Color("#F8FAFC") // Bright White

	// Header Styles
	BannerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorFgWhite).
			Background(ColorPrimary).
			Padding(0, 1)

	BannerSubStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			Padding(0, 1)

	DistroBadgeStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorAccent).
				Background(lipgloss.Color("#2E1065")).
				Padding(0, 1)

	DiskInfoStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	DiskMeterStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	// Pane Borders & Containers
	ActivePaneBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPrimary)

	InactivePaneBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorSubtle)

	PaneTitleActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorFgWhite).
			Background(ColorPrimary).
			Padding(0, 1)

	PaneTitleInactive = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorMuted).
				Padding(0, 1)

	// List & Table Styles
	CursorRowStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorFgWhite).
			Background(ColorHighlight)

	NormalRowStyle = lipgloss.NewStyle().
			Foreground(ColorFgWhite)

	MutedRowStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	TableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorPrimary).
				Border(lipgloss.NormalBorder(), false, false, true, false).
				BorderForeground(ColorSubtle)

	// Badges - STRICT: Zero Emojis
	BadgeSafe = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSuccess).
			Render("[SAFE]")

	BadgeCaution = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWarning).
			Render("[CAUTION]")

	BadgeHigh = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorDanger).
			Render("[HIGH]")

	BadgeOK = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorSuccess).
		Render("[OK]")

	BadgeWarn = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorWarning).
			Render("[WARN]")

	BadgeError = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorDanger).
			Render("[ERROR]")

	BadgeInfo = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			Render("[INFO]")

	CategoryBadgeStyle = lipgloss.NewStyle().
				Foreground(ColorAccent).
				Bold(true)

	SizeBadgeStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true)

	// Footer & Keybinding Legend
	FooterContainer = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(ColorSubtle).
			Padding(0, 1)

	KeyBadge = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)

	KeyDesc = lipgloss.NewStyle().
		Foreground(ColorMuted)

	KeyDivider = lipgloss.NewStyle().
			Foreground(ColorSubtle).
			SetString(" | ")

	// Modal Dialog Styles
	ModalOverlay = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(ColorPrimary).
			Background(ColorBgDark).
			Padding(1, 2)

	ModalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorFgWhite).
			Background(ColorPrimary).
			Padding(0, 2).
			MarginBottom(1)

	ModalWarningBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorWarning).
			Foreground(ColorWarning).
			Bold(true).
			Padding(0, 1).
			MarginTop(1).
			MarginBottom(1)

	ModalDangerBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorDanger).
			Foreground(ColorDanger).
			Bold(true).
			Padding(0, 1).
			MarginTop(1).
			MarginBottom(1)

	ModalSuccessBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorSuccess).
			Foreground(ColorSuccess).
			Bold(true).
			Padding(0, 1).
			MarginTop(1).
			MarginBottom(1)

	// Execution & Log Styles
	LogContainerStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorPrimary).
				Padding(0, 1)

	LogTimeStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	LogTextStyle = lipgloss.NewStyle().
			Foreground(ColorFgWhite)
)

// RenderSafetyBadge returns the styled text badge for a core.SafetyLevel.
func RenderSafetyBadge(level core.SafetyLevel) string {
	switch level {
	case core.SafetySafe:
		return BadgeSafe
	case core.SafetyCaution:
		return BadgeCaution
	case core.SafetyHigh:
		return BadgeHigh
	default:
		return lipgloss.NewStyle().Foreground(ColorMuted).Render(fmt.Sprintf("[%s]", level))
	}
}

// RenderCategoryBadge formats a concise category identifier.
func RenderCategoryBadge(cat string) string {
	if cat == "" {
		cat = "misc"
	}
	formatted := strings.ToLower(cat)
	if len(formatted) > 10 {
		formatted = formatted[:10]
	}
	return CategoryBadgeStyle.Render(fmt.Sprintf("[%s]", formatted))
}

// RenderKeyHelp formats a single keybinding hint like "[Tab] switch pane".
func RenderKeyHelp(key, desc string) string {
	return fmt.Sprintf("%s %s", KeyBadge.Render(key), KeyDesc.Render(desc))
}
