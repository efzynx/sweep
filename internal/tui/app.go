package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/efzynx/sweep/internal/core"
)

// Run launches the interactive Bubbletea TUI application.
func Run(ctx context.Context, reg *core.Registry, cfg *core.Config) error {
	m := NewModel(ctx, reg, cfg)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
