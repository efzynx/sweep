package flatpak

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/core/distro"
)

const (
	ModuleID       = "flatpak"
	ModuleName     = "Flatpak Packages"
	ModuleCategory = "sandboxed_apps"
	ModuleDesc     = "Clean unused Flatpak runtimes and application cache directories"
)

// CommandRunner defines the function signature for running external CLI commands.
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Module implements core.Module for Flatpak sandboxed packages and caches.
type Module struct {
	varAppDir         string
	commandRunner     CommandRunner
	availableOverride *bool
}

// New creates and returns a new Flatpak module instance.
func New() *Module {
	home, _ := core.GetUserHomeDir()
	if home == "" {
		home = os.Getenv("HOME")
	}
	varAppDir := ""
	if home != "" {
		varAppDir = filepath.Join(home, ".var", "app")
	}

	return &Module{
		varAppDir: varAppDir,
		commandRunner: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
	}
}

func init() {
	_ = core.Register(New())
}

func (m *Module) ID() string {
	return ModuleID
}

func (m *Module) Name() string {
	return ModuleName
}

func (m *Module) Category() string {
	return ModuleCategory
}

func (m *Module) Description() string {
	return ModuleDesc
}

// SetAvailable overrides IsAvailable for testing purposes.
func (m *Module) SetAvailable(available bool) {
	m.availableOverride = &available
}

func (m *Module) IsAvailable() bool {
	if m.availableOverride != nil {
		return *m.availableOverride
	}
	return distro.HasFlatpak()
}

func (m *Module) RequiresElevation() bool {
	return false
}

// Scan performs read-only detection of unused Flatpak runtimes and cache directories.
func (m *Module) Scan(ctx context.Context) (core.ScanResult, error) {
	result := core.ScanResult{
		Items:     make([]core.CleanableItem, 0),
		ScannedAt: time.Now(),
	}

	if !m.IsAvailable() {
		return result, nil
	}

	// 1. Scan unused runtimes
	runtimeItems, err := m.scanUnusedRuntimes(ctx)
	if err == nil {
		result.Items = append(result.Items, runtimeItems...)
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	// 2. Scan Flatpak app cache directories (~/.var/app/*/cache)
	cacheItems, err := m.scanAppCaches(ctx)
	if err == nil {
		result.Items = append(result.Items, cacheItems...)
	}

	result.RecalculateTotal()
	return result, nil
}

func (m *Module) scanUnusedRuntimes(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	// Attempt standard invocation: flatpak list --unused --columns=application,ref,size,options
	out, err := m.commandRunner(ctx, "flatpak", "list", "--unused", "--columns=application,ref,size,options")
	if err == nil && len(out) > 0 {
		return parseFlatpakListOutput(out), nil
	}

	// Fallback for newer Flatpak versions where --unused is an option for uninstall rather than list:
	// Query full runtime list with sizes, then identify candidates.
	listOut, listErr := m.commandRunner(ctx, "flatpak", "list", "--columns=application,ref,size,options")
	if listErr != nil {
		return items, nil
	}

	allRuntimes := parseFlatpakListOutput(listOut)
	if len(allRuntimes) == 0 {
		return items, nil
	}

	// Execute non-destructive dry-check via flatpak uninstall --unused with EOF
	// When run without -y or confirmation, flatpak outputs candidates before prompting
	uninstCmd := exec.CommandContext(ctx, "flatpak", "uninstall", "--unused")
	uninstCmd.Stdin = strings.NewReader("n\n")
	uninstOut, _ := uninstCmd.CombinedOutput()

	candidateNames := extractUnusedCandidateNames(string(uninstOut))
	if len(candidateNames) == 0 {
		return items, nil
	}

	for _, rt := range allRuntimes {
		app := rt.Metadata["application"]
		ref := rt.Metadata["ref"]
		matched := false
		for _, cand := range candidateNames {
			if cand == app || strings.HasPrefix(ref, cand) || strings.Contains(ref, cand) {
				matched = true
				break
			}
		}
		if matched {
			rt.ID = "flatpak-runtime:" + ref
			rt.Name = fmt.Sprintf("Unused Flatpak runtime: %s", app)
			rt.Description = fmt.Sprintf("Unused Flatpak runtime ref %s", ref)
			items = append(items, rt)
		}
	}

	return items, nil
}

func (m *Module) scanAppCaches(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	if m.varAppDir == "" {
		return items, nil
	}

	entries, err := os.ReadDir(m.varAppDir)
	if err != nil {
		return items, nil
	}

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return items, ctx.Err()
		default:
		}

		if !entry.IsDir() {
			continue
		}

		appName := entry.Name()
		cachePath := filepath.Join(m.varAppDir, appName, "cache")

		info, err := os.Stat(cachePath)
		if err != nil || !info.IsDir() {
			continue
		}

		sizeBytes := calculateFlatpakDirSize(cachePath)
		if sizeBytes == 0 {
			continue
		}

		items = append(items, core.CleanableItem{
			ID:          "flatpak-cache:" + appName,
			Name:        fmt.Sprintf("Flatpak cache: %s", appName),
			Description: fmt.Sprintf("Application cache for %s in %s", appName, cachePath),
			Path:        cachePath,
			Size:        sizeBytes,
			Selected:    true,
			Safety:      core.SafetySafe,
			ModTime:     info.ModTime(),
			Metadata: map[string]string{
				"type": "cache",
				"app":  appName,
			},
		})
	}

	return items, nil
}

// Clean uninstalls unused runtimes and removes application caches.
func (m *Module) Clean(ctx context.Context, items []core.CleanableItem, opts core.CleanOptions) (core.CleanResult, error) {
	start := time.Now()
	cleanRes := core.CleanResult{
		CleanedItems: make([]core.CleanableItem, 0),
		Errors:       make([]error, 0),
	}

	if len(items) == 0 {
		cleanRes.Duration = time.Since(start)
		return cleanRes, nil
	}

	if opts.DryRun {
		for _, item := range items {
			cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
			cleanRes.FreedBytes += item.Size
		}
		cleanRes.Duration = time.Since(start)
		return cleanRes, nil
	}

	var hasRuntimes bool
	var runtimeItems []core.CleanableItem

	for _, item := range items {
		select {
		case <-ctx.Done():
			cleanRes.Errors = append(cleanRes.Errors, ctx.Err())
			cleanRes.Duration = time.Since(start)
			return cleanRes, ctx.Err()
		default:
		}

		itemType := item.Metadata["type"]
		switch {
		case itemType == "runtime" || strings.HasPrefix(item.ID, "flatpak-runtime:"):
			hasRuntimes = true
			runtimeItems = append(runtimeItems, item)

		case itemType == "cache" || strings.HasPrefix(item.ID, "flatpak-cache:"):
			// Remove the cache directory contents
			if err := cleanDirContents(item.Path); err != nil && !os.IsNotExist(err) {
				cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to clean flatpak cache %s: %w", item.Path, err))
			} else {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
				cleanRes.FreedBytes += item.Size
			}
		}
	}

	// Uninstall unused runtimes in one parameterized call
	if hasRuntimes {
		_, err := m.commandRunner(ctx, "flatpak", "uninstall", "--unused", "-y")
		if err != nil {
			cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to uninstall unused flatpak runtimes: %w", err))
		} else {
			for _, rt := range runtimeItems {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, rt)
				cleanRes.FreedBytes += rt.Size
			}
		}
	}

	cleanRes.Duration = time.Since(start)
	return cleanRes, nil
}

// SetVarAppDir sets the flatpak user data directory for testing.
func (m *Module) SetVarAppDir(path string) {
	m.varAppDir = filepath.Clean(path)
}

// SetCommandRunner overrides the command runner for testing.
func (m *Module) SetCommandRunner(runner CommandRunner) {
	m.commandRunner = runner
}

// parseFlatpakListOutput parses tab-separated output from `flatpak list --columns=application,ref,size,options`.
func parseFlatpakListOutput(output []byte) []core.CleanableItem {
	var items []core.CleanableItem
	scanner := bufio.NewScanner(bytes.NewReader(output))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}

		app := strings.TrimSpace(parts[0])
		ref := strings.TrimSpace(parts[1])

		var sizeBytes int64
		if len(parts) >= 3 {
			sizeStr := strings.TrimSpace(parts[2])
			sizeStr = strings.ReplaceAll(sizeStr, "\u00a0", " ")
			if parsed, err := core.ParseBytes(sizeStr); err == nil {
				sizeBytes = parsed
			}
		}

		items = append(items, core.CleanableItem{
			ID:          "flatpak-runtime:" + ref,
			Name:        fmt.Sprintf("Unused Flatpak runtime: %s", app),
			Description: fmt.Sprintf("Flatpak runtime %s", ref),
			Size:        sizeBytes,
			Selected:    true,
			Safety:      core.SafetySafe,
			Metadata: map[string]string{
				"type":        "runtime",
				"application": app,
				"ref":         ref,
			},
		})
	}

	return items
}

// extractUnusedCandidateNames parses candidate runtime names from `flatpak uninstall --unused` confirmation prompt.
func extractUnusedCandidateNames(output string) []string {
	var candidates []string
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Look for lines like:
		//  1.    org.kde.Platform  6.10  r
		if strings.Contains(line, ".") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && strings.HasSuffix(fields[0], ".") {
				candidates = append(candidates, fields[1])
			}
		}
	}

	return candidates
}

// calculateFlatpakDirSize calculates the total bytes of all files in a directory.
func calculateFlatpakDirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// cleanDirContents removes all files and subdirectories inside dir without deleting dir itself.
func cleanDirContents(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		subPath := filepath.Join(dir, entry.Name())
		if err := os.RemoveAll(subPath); err != nil {
			return err
		}
	}
	return nil
}
