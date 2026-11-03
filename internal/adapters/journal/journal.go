package journal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/efzynx/sweep/internal/core"
)

const (
	ModuleID               = "journal"
	ModuleName             = "Systemd Journal Logs"
	ModuleCategory         = "system_logs"
	ModuleDesc             = "Vacuum systemd journal logs by age retention or size threshold"
	DefaultRetentionDays   = 7
	DefaultVacuumSizeStr   = "50M"
	DefaultVacuumSizeBytes = int64(50 * 1024 * 1024)
)

var (
	// Matches systemd disk usage: "Archived and active journals take up 44.9M in the file system."
	// or "Journals take up 1.2G on disk."
	reDiskUsage = regexp.MustCompile(`(?i)(?:take up|taking up|takes up|journals take up)\s+([0-9]+(?:\.[0-9]+)?)\s*([A-Za-z]+)`)

	// Fallback pattern if journalctl output only gives number and unit
	reFallbackDiskUsage = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*([KMGTBkmgtb]i?[Bb]?)`)

	// Matches: "Vacuuming done, freed 12.0M of archived journals from /var/log/journal."
	reVacuumFreed = regexp.MustCompile(`(?i)freed\s+([0-9]+(?:\.[0-9]+)?)\s*([A-Za-z]+)`)
)

// CommandRunner defines the function signature for executing CLI binaries.
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Module implements core.Module for systemd journal log vacuuming.
type Module struct {
	mu               sync.RWMutex
	commandRunner    CommandRunner
	isSystemdRunning func() bool
	retentionDays    int
	vacuumSizeLimit  int64
	vacuumSizeStr    string
	journalDirs      []string
}

// New creates and returns a new systemd journal module instance.
func New() *Module {
	return &Module{
		commandRunner: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return exec.CommandContext(ctx, name, args...).Output()
		},
		isSystemdRunning: defaultIsSystemdRunning,
		retentionDays:    DefaultRetentionDays,
		vacuumSizeLimit:  DefaultVacuumSizeBytes,
		vacuumSizeStr:    DefaultVacuumSizeStr,
		journalDirs:      []string{"/var/log/journal", "/run/log/journal"},
	}
}

// NewWithConfig creates a new journal module initialized with sweep configuration.
func NewWithConfig(cfg *core.Config) *Module {
	m := New()
	m.SetConfig(cfg)
	return m
}

func init() {
	_ = core.Register(New())
}

func defaultIsSystemdRunning() bool {
	fi, err := os.Stat("/run/systemd/system")
	return err == nil && fi.IsDir()
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

func (m *Module) IsAvailable() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.isSystemdRunning != nil && !m.isSystemdRunning() {
		return false
	}
	_, err := exec.LookPath("journalctl")
	return err == nil
}

func (m *Module) RequiresElevation() bool {
	return true
}

// SetConfig updates retention parameters from the central sweep configuration.
func (m *Module) SetConfig(cfg *core.Config) {
	if cfg == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if cfg.Retention.JournalVacuumDays > 0 {
		m.retentionDays = cfg.Retention.JournalVacuumDays
	}
}

// SetCommandRunner overrides the command execution handler for unit testing.
func (m *Module) SetCommandRunner(runner CommandRunner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commandRunner = runner
}

// SetSystemdRunning overrides the systemd detection check for testing.
func (m *Module) SetSystemdRunning(fn func() bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isSystemdRunning = fn
}

// SetJournalDirs sets custom journal directory paths for testing.
func (m *Module) SetJournalDirs(dirs []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.journalDirs = dirs
}

// SetRetentionDays sets the log retention window in days.
func (m *Module) SetRetentionDays(days int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if days > 0 {
		m.retentionDays = days
	}
}

// SetVacuumSizeLimit sets the disk usage limit for journals.
func (m *Module) SetVacuumSizeLimit(bytes int64, str string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if bytes > 0 {
		m.vacuumSizeLimit = bytes
	}
	if str != "" {
		m.vacuumSizeStr = str
	}
}

// ParseBytes parses a numeric value and size unit into bytes.
func ParseBytes(numStr, unitStr string) (int64, error) {
	val, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q: %w", numStr, err)
	}

	u := strings.ToUpper(strings.TrimSpace(unitStr))
	var multiplier float64 = 1

	switch u {
	case "B", "BYTES", "":
		multiplier = 1
	case "K", "KB", "KIB":
		multiplier = 1024
	case "M", "MB", "MIB":
		multiplier = 1024 * 1024
	case "G", "GB", "GIB":
		multiplier = 1024 * 1024 * 1024
	case "T", "TB", "TIB":
		multiplier = 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown byte unit %q", unitStr)
	}

	return int64(val * multiplier), nil
}

// ParseDiskUsage parses the byte count from `journalctl --disk-usage` output.
func ParseDiskUsage(output string) (int64, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return 0, fmt.Errorf("empty disk usage output")
	}

	if strings.Contains(strings.ToLower(trimmed), "no journals found") {
		return 0, nil
	}

	matches := reDiskUsage.FindStringSubmatch(trimmed)
	if len(matches) == 3 {
		return ParseBytes(matches[1], matches[2])
	}

	// Fallback regex attempt
	matches = reFallbackDiskUsage.FindStringSubmatch(trimmed)
	if len(matches) == 3 {
		return ParseBytes(matches[1], matches[2])
	}

	return 0, fmt.Errorf("unable to parse journalctl disk usage from: %q", output)
}

// ParseVacuumFreed parses total freed bytes from `journalctl --vacuum-...` output lines.
func ParseVacuumFreed(output string) (int64, error) {
	matches := reVacuumFreed.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return 0, nil
	}

	var totalFreed int64
	for _, m := range matches {
		if len(m) == 3 {
			b, err := ParseBytes(m[1], m[2])
			if err == nil {
				totalFreed += b
			}
		}
	}

	return totalFreed, nil
}

// isArchivedJournal returns true if the filename matches an archived journal.
func isArchivedJournal(name string) bool {
	if strings.Contains(name, "@") && strings.HasSuffix(name, ".journal") {
		return true
	}
	if strings.HasSuffix(name, ".journal~") {
		return true
	}
	return false
}

// Scan queries journalctl disk usage and scans journal directories for vacuumable logs.
func (m *Module) Scan(ctx context.Context) (core.ScanResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := core.ScanResult{
		Items:     make([]core.CleanableItem, 0),
		ScannedAt: time.Now(),
	}

	if !m.IsAvailable() {
		return result, nil
	}

	out, err := m.commandRunner(ctx, "journalctl", "--disk-usage")
	if err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, fmt.Errorf("journalctl --disk-usage failed: %w", err)
	}

	totalUsage, err := ParseDiskUsage(string(out))
	if err != nil {
		return result, fmt.Errorf("failed to parse journalctl disk usage: %w", err)
	}

	if totalUsage <= 0 {
		return result, nil
	}

	cutoff := time.Now().AddDate(0, 0, -m.retentionDays)
	var oldJournalsSize int64
	var oldJournalCount int

	for _, jDir := range m.journalDirs {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		entries, err := os.ReadDir(jDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				subDir := filepath.Join(jDir, e.Name())
				subEntries, sErr := os.ReadDir(subDir)
				if sErr != nil {
					continue
				}
				for _, subE := range subEntries {
					if isArchivedJournal(subE.Name()) {
						if info, err := subE.Info(); err == nil {
							if info.ModTime().Before(cutoff) {
								oldJournalsSize += info.Size()
								oldJournalCount++
							}
						}
					}
				}
			} else if isArchivedJournal(e.Name()) {
				if info, err := e.Info(); err == nil {
					if info.ModTime().Before(cutoff) {
						oldJournalsSize += info.Size()
						oldJournalCount++
					}
				}
			}
		}
	}

	var sizeExcess int64
	if totalUsage > m.vacuumSizeLimit {
		sizeExcess = totalUsage - m.vacuumSizeLimit
	}

	vacuumableSize := oldJournalsSize
	if sizeExcess > vacuumableSize {
		vacuumableSize = sizeExcess
	}

	if vacuumableSize > totalUsage {
		vacuumableSize = totalUsage
	}

	if vacuumableSize > 0 {
		result.Items = append(result.Items, core.CleanableItem{
			ID:          "journal:vacuum",
			Name:        "Systemd Journal Logs",
			Description: fmt.Sprintf("Archived systemd logs (>%dd retention or >%s threshold; current total: %s)", m.retentionDays, m.vacuumSizeStr, formatBytes(totalUsage)),
			Path:        "/var/log/journal",
			Size:        vacuumableSize,
			Selected:    true,
			Safety:      core.SafetySafe,
			ModTime:     time.Now(),
			Metadata: map[string]string{
				"total_disk_usage": fmt.Sprintf("%d", totalUsage),
				"retention_days":   fmt.Sprintf("%d", m.retentionDays),
				"vacuum_time":      fmt.Sprintf("%dd", m.retentionDays),
				"vacuum_size":      m.vacuumSizeStr,
				"vacuumable_bytes": fmt.Sprintf("%d", vacuumableSize),
				"old_file_count":   fmt.Sprintf("%d", oldJournalCount),
			},
		})
	}

	result.RecalculateTotal()
	return result, nil
}

// Clean executes journalctl vacuuming respecting dryRun mode.
func (m *Module) Clean(ctx context.Context, items []core.CleanableItem, opts core.CleanOptions) (core.CleanResult, error) {
	start := time.Now()
	cleanRes := core.CleanResult{
		CleanedItems: make([]core.CleanableItem, 0),
		Errors:       make([]error, 0),
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

		if item.ID == "journal:vacuum" {
			args := []string{
				fmt.Sprintf("--vacuum-time=%dd", m.retentionDays),
				fmt.Sprintf("--vacuum-size=%s", m.vacuumSizeStr),
			}

			out, err := m.commandRunner(ctx, "journalctl", args...)
			if err != nil {
				cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("journalctl vacuum failed: %w", err))
				continue
			}

			freed, _ := ParseVacuumFreed(string(out))
			if freed <= 0 {
				freed = item.Size
			}

			cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
			cleanRes.FreedBytes += freed
		}
	}

	cleanRes.Duration = time.Since(start)
	return cleanRes, nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
