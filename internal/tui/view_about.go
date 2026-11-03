package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderAboutModal renders a centered modal dialog with app info, author, system scope, and keyboard reference.
func (m *Model) renderAboutModal() string {
	modalWidth := 74
	if m.width < 78 {
		modalWidth = m.width - 4
	}
	if modalWidth < 50 {
		modalWidth = 50
	}

	title := ModalTitleStyle.Render("[ ABOUT & SYSTEM INFO ]")

	var lines []string
	lines = append(lines, title)

	// App info section
	appName := lipgloss.NewStyle().Bold(true).Foreground(ColorFgWhite).Render("Sweep - Modern Multi-Distro Linux System Cleaner")
	lines = append(lines, appName)
	lines = append(lines, fmt.Sprintf("%s %s", lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("Author:"), lipgloss.NewStyle().Foreground(ColorFgWhite).Render("Ahmad Fauzan Adiman (Efzyn) <me@efzyn.my.id>")))
	lines = append(lines, fmt.Sprintf("%s %s", lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("Repository:"), lipgloss.NewStyle().Foreground(ColorAccent).Render("github.com/efzynx/sweep")))
	lines = append(lines, fmt.Sprintf("%s %s", lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("License:"), lipgloss.NewStyle().Foreground(ColorFgWhite).Render("MIT License")))

	if m.height >= 24 {
		lines = append(lines, "")
	}

	// System Scope notice
	scopeTitle := lipgloss.NewStyle().Bold(true).Foreground(ColorWarning).Render("System Scope:")
	scopeDesc := lipgloss.NewStyle().Foreground(ColorMuted).Width(modalWidth - 6).Render("Targeting root filesystem (/) system caches, package managers, containers, flatpak, and snapshots")
	lines = append(lines, fmt.Sprintf("%s %s", scopeTitle, scopeDesc))

	if m.height >= 24 {
		lines = append(lines, "")
	}

	// Keyboard Navigation Reference Table
	kbTitle := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Render("Keyboard Navigation Reference:")
	lines = append(lines, kbTitle)

	shortcuts := []struct {
		keys string
		desc string
	}{
		{"[Tab] / [h]/[l]", "Switch pane"},
		{"[j]/[k] / [Down]/[Up]", "Navigate rows"},
		{"[Space]", "Toggle selection"},
		{"[a]", "Select/deselect all"},
		{"[s]", "Rescan modules"},
		{"[d]", "Dry-run simulation"},
		{"[Enter]", "Execute clean"},
		{"[?] / [i]", "Toggle this About & Info modal"},
		{"[q] / [Ctrl+C]", "Quit"},
	}

	for _, sc := range shortcuts {
		keyStyled := lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).Width(24).Render(sc.keys)
		descStyled := lipgloss.NewStyle().Foreground(ColorFgWhite).Render(sc.desc)
		lines = append(lines, fmt.Sprintf("  %s %s", keyStyled, descStyled))
	}

	if m.height >= 24 {
		lines = append(lines, "")
	}
	lines = append(lines, lipgloss.NewStyle().Foreground(ColorSubtle).Render(strings.Repeat("─", modalWidth-6)))

	dismissHint := lipgloss.NewStyle().Bold(true).Foreground(ColorMuted).Render("Press [Esc], [?], [q], or [Enter] to return")
	lines = append(lines, dismissHint)

	content := strings.Join(lines, "\n")
	box := ModalOverlay.Width(modalWidth).Render(content)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
