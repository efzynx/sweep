package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/tui"

	_ "github.com/efzynx/sweep/internal/adapters"
)

var (
	// Version metadata populated at build time via ldflags.
	Version   = "0.1.0-dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func main() {
	var (
		versionFlag  bool
		dryRunFlag   bool
		scanOnlyFlag bool
		configPath   string
	)

	flag.BoolVar(&versionFlag, "version", false, "Print version information and exit")
	flag.BoolVar(&versionFlag, "v", false, "Print version information and exit (shorthand)")
	flag.BoolVar(&dryRunFlag, "dry-run", false, "Simulate cleaning of detected items without deleting them")
	flag.BoolVar(&scanOnlyFlag, "scan-only", false, "Scan system for cleanable items and print summary without cleaning")
	flag.StringVar(&configPath, "config", "", "Path to custom configuration file")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Sweep - Modern Multi-Distro Linux System Cleaner (%s)\n\n", Version)
		fmt.Fprintf(os.Stderr, "Usage:\n  sweep [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  sweep                  # Launch interactive TUI\n")
		fmt.Fprintf(os.Stderr, "  sweep --scan-only      # Perform read-only scan and display reclaimable space\n")
		fmt.Fprintf(os.Stderr, "  sweep --dry-run        # Perform scan and simulate cleanup\n")
		fmt.Fprintf(os.Stderr, "  sweep --version        # Display version details\n")
	}

	flag.Parse()

	if versionFlag {
		fmt.Printf("sweep version %s (commit: %s, built: %s)\n", Version, Commit, BuildDate)
		os.Exit(0)
	}

	cfg, err := core.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load configuration: %v (using defaults)\n", err)
		cfg = core.DefaultConfig()
	}

	if customMod, ok := core.DefaultRegistry.Get("custom"); ok {
		if cm, ok := customMod.(interface{ SetConfig(*core.Config) }); ok {
			cm.SetConfig(cfg)
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if scanOnlyFlag || dryRunFlag {
		runCLI(ctx, cfg, dryRunFlag)
		return
	}

	// Interactive TUI mode (default when no CLI action flags specified)
	if err := tui.Run(ctx, core.DefaultRegistry, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error running Sweep TUI: %v\n", err)
		os.Exit(1)
	}
}

func runCLI(ctx context.Context, cfg *core.Config, dryRun bool) {
	available := core.DefaultRegistry.EnabledModules(cfg)
	if len(available) == 0 {
		fmt.Println("No active modules detected on this system.")
		return
	}

	fmt.Printf("Scanning %d available modules...\n\n", len(available))
	start := time.Now()

	results, err := core.DefaultRegistry.ScanModules(ctx, available)
	if err != nil && ctx.Err() != nil {
		fmt.Fprintf(os.Stderr, "Scan cancelled: %v\n", err)
		os.Exit(1)
	}

	var grandTotal int64
	var totalItems int

	for _, mod := range available {
		res, ok := results[mod.ID()]
		if !ok {
			continue
		}

		fmt.Printf("[%s] %s (%s)\n", mod.ID(), mod.Name(), mod.Category())
		if res.Error != nil {
			fmt.Printf("  Scan error: %v\n\n", res.Error)
			continue
		}

		if len(res.Items) == 0 {
			fmt.Println("  No cleanable items found.")
			fmt.Println()
			continue
		}

		for _, item := range res.Items {
			fmt.Printf("  - [%s] %s (%s) - %s\n", item.Safety, item.Name, core.FormatBytes(item.Size), item.Description)
			if item.Path != "" {
				fmt.Printf("    Path: %s\n", item.Path)
			}
		}

		fmt.Printf("  Total: %s across %d items\n\n", core.FormatBytes(res.TotalSize), len(res.Items))
		grandTotal += res.TotalSize
		totalItems += len(res.Items)
	}

	fmt.Printf("Summary: Found %d items totaling %s in %s\n",
		totalItems, core.FormatBytes(grandTotal), time.Since(start).Round(time.Millisecond))

	if dryRun && totalItems > 0 {
		fmt.Println("\nSimulating Dry-Run Cleanup...")
		opts := core.CleanOptions{
			DryRun:            true,
			SnapshotSafeguard: cfg.SnapshotSafeguard,
		}

		var totalFreed int64
		for _, mod := range available {
			res, ok := results[mod.ID()]
			if !ok || len(res.Items) == 0 {
				continue
			}

			cleanRes, err := mod.Clean(ctx, res.Items, opts)
			if err != nil {
				fmt.Printf("[%s] Clean error: %v\n", mod.ID(), err)
				continue
			}

			fmt.Printf("[%s] Simulated freed: %s (%d items cleaned)\n",
				mod.ID(), core.FormatBytes(cleanRes.FreedBytes), len(cleanRes.CleanedItems))
			totalFreed += cleanRes.FreedBytes
		}

		fmt.Printf("\nDry-run complete. Total space reclaimable: %s\n", core.FormatBytes(totalFreed))
	}
}
