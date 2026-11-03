package tui

import (
	"context"
	"fmt"

	"github.com/efzynx/sweep/internal/core"
)

// Run launches the interactive Bubbletea TUI application.
// This is the presentation entrypoint to be expanded by the TUI phase.
func Run(ctx context.Context, reg *core.Registry, cfg *core.Config) error {
	fmt.Println("Sweep - Modern Multi-Distro Linux System Cleaner")
	fmt.Printf("Config loaded: DryRun=%v, SnapshotSafeguard=%v\n", cfg.DryRunDefault, cfg.SnapshotSafeguard)
	fmt.Println("Run with --scan-only to inspect cleanable items or --help for options.")
	return nil
}
