package snap

import (
	"bufio"
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
	ModuleID       = "snap"
	ModuleName     = "Snap Packages"
	ModuleCategory = "sandboxed_apps"
	ModuleDesc     = "Clean disabled Snap package revisions and old version snapshots"

	defaultSnapsDir = "/var/lib/snapd/snaps"
)

// CommandRunner defines the function signature for running external CLI commands.
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Module implements core.Module for Snap sandboxed packages.
type Module struct {
	snapsDir          string
	commandRunner     CommandRunner
	availableOverride *bool
}

// New creates and returns a new Snap module instance.
func New() *Module {
	return &Module{
		snapsDir: defaultSnapsDir,
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
	return distro.HasSnap()
}

func (m *Module) RequiresElevation() bool {
	return true
}

// SnapRevision represents a parsed revision from snap list --all.
type SnapRevision struct {
	Name     string
	Version  string
	Revision string
	Disabled bool
}

// Scan performs read-only detection of disabled snap revisions.
func (m *Module) Scan(ctx context.Context) (core.ScanResult, error) {
	result := core.ScanResult{
		Items:     make([]core.CleanableItem, 0),
		ScannedAt: time.Now(),
	}

	if !m.IsAvailable() {
		return result, nil
	}

	if m.commandRunner == nil {
		return result, nil
	}

	out, err := m.commandRunner(ctx, "snap", "list", "--all")
	if err != nil {
		// Snap command returned error or snapd not running
		return result, nil
	}

	revisions := ParseSnapListOutput(string(out))

	for _, rev := range revisions {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		if !rev.Disabled {
			continue
		}

		// Calculate reclaimable size from /var/lib/snapd/snaps/<name>_<rev>.snap
		snapFile := filepath.Join(m.snapsDir, fmt.Sprintf("%s_%s.snap", rev.Name, rev.Revision))
		var size int64
		if fi, err := os.Stat(snapFile); err == nil {
			size = fi.Size()
		}

		result.Items = append(result.Items, core.CleanableItem{
			ID:          fmt.Sprintf("snap:%s:%s", rev.Name, rev.Revision),
			Name:        fmt.Sprintf("Snap disabled revision: %s (rev %s)", rev.Name, rev.Revision),
			Description: fmt.Sprintf("Disabled revision %s of snap %s (v%s)", rev.Revision, rev.Name, rev.Version),
			Path:        snapFile,
			Size:        size,
			Selected:    true,
			Safety:      core.SafetySafe,
			Metadata: map[string]string{
				"name":     rev.Name,
				"revision": rev.Revision,
				"version":  rev.Version,
			},
		})
	}

	result.RecalculateTotal()
	return result, nil
}

// Clean removes disabled snap revisions via parameterized `snap remove <name> --revision=<rev>`.
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

	for _, item := range items {
		select {
		case <-ctx.Done():
			cleanRes.Errors = append(cleanRes.Errors, ctx.Err())
			cleanRes.Duration = time.Since(start)
			return cleanRes, ctx.Err()
		default:
		}

		name := item.Metadata["name"]
		rev := item.Metadata["revision"]
		if name == "" || rev == "" {
			continue
		}

		// Parameterized execution: snap remove <name> --revision=<rev>
		var removeErr error
		if m.commandRunner != nil {
			_, removeErr = m.commandRunner(ctx, "snap", "remove", name, fmt.Sprintf("--revision=%s", rev))
		}

		// If test or custom snapsDir, also remove .snap file
		if m.snapsDir != defaultSnapsDir {
			_ = os.Remove(item.Path)
		}

		if removeErr != nil && m.snapsDir == defaultSnapsDir {
			cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to remove snap %s rev %s: %w", name, rev, removeErr))
		} else {
			cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
			cleanRes.FreedBytes += item.Size
		}
	}

	cleanRes.Duration = time.Since(start)
	return cleanRes, nil
}

// ParseSnapListOutput parses the tabular stdout of `snap list --all`.
func ParseSnapListOutput(output string) []SnapRevision {
	var revisions []SnapRevision

	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "Name") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		name := fields[0]
		version := fields[1]
		revision := fields[2]

		// Check if the Notes (last column) indicates disabled status
		notes := fields[len(fields)-1]
		disabled := strings.Contains(notes, "disabled")

		revisions = append(revisions, SnapRevision{
			Name:     name,
			Version:  version,
			Revision: revision,
			Disabled: disabled,
		})
	}

	return revisions
}

// SetSnapsDir sets the snaps directory for testing.
func (m *Module) SetSnapsDir(dir string) {
	m.snapsDir = filepath.Clean(dir)
}

// SetCommandRunner overrides the CLI runner for testing.
func (m *Module) SetCommandRunner(runner CommandRunner) {
	m.commandRunner = runner
}
