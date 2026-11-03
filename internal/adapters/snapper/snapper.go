package snapper

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/core/distro"
)

const (
	ModuleID       = "snapper"
	ModuleName     = "Btrfs & Snapper Snapshots"
	ModuleCategory = "snapshots"
	ModuleDesc     = "Manage and clean Snapper Btrfs filesystem snapshots with safeguard snapshot protection"

	defaultMountsPath = "/proc/mounts"

	SnapperPermissionDeniedMessage = "Snapper requires root privileges or ALLOW_USERS in /etc/snapper/configs/root. Run with sudo to manage snapshots."
)

var (
	// ErrPermissionDenied is returned when Snapper cannot access snapshots due to missing privileges.
	ErrPermissionDenied = errors.New(SnapperPermissionDeniedMessage)
)

// CommandRunner defines the signature for executing external commands.
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Module implements core.Module for Btrfs Snapper snapshot management.
type Module struct {
	mountsPath        string
	commandRunner     CommandRunner
	configs           []string
	availableOverride *bool
}

// New creates and returns a new Snapper module instance.
func New() *Module {
	return &Module{
		mountsPath: defaultMountsPath,
		configs:    []string{"root", "home"},
		commandRunner: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if core.IsPrivileged() {
				return exec.CommandContext(ctx, name, args...).Output()
			}
			return core.RunElevated(ctx, name, args...)
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
	if !distro.HasSnapper() {
		return false
	}
	return m.hasBtrfsMounts()
}

func (m *Module) RequiresElevation() bool {
	return true
}

func (m *Module) hasBtrfsMounts() bool {
	data, err := os.ReadFile(m.mountsPath)
	if err != nil {
		return false
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 && fields[2] == "btrfs" {
			return true
		}
	}
	return false
}

// CreateSafeguardSnapshot executes a pre-clean safeguard snapshot for the root subvolume.
func (m *Module) CreateSafeguardSnapshot(desc string) error {
	if desc == "" {
		desc = "Sweep safeguard snapshot"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := m.commandRunner(ctx, "snapper", "-c", "root", "create", "-d", desc)
	return err
}

// Scan performs read-only detection of Snapper snapshots across configured subvolumes.
func (m *Module) Scan(ctx context.Context) (core.ScanResult, error) {
	result := core.ScanResult{
		Items:     make([]core.CleanableItem, 0),
		Metadata:  make(map[string]string),
		ScannedAt: time.Now(),
	}

	if !m.IsAvailable() {
		return result, nil
	}

	// Discover accessible configurations if possible
	configs := m.discoverConfigs(ctx)

	for _, cfg := range configs {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		snapItems, err := m.scanConfigSnapshots(ctx, cfg)
		if err != nil {
			if errors.Is(err, ErrPermissionDenied) || isSnapperPermissionDenied(err) {
				result.Metadata["permission_denied"] = SnapperPermissionDeniedMessage
			}
		} else {
			result.Items = append(result.Items, snapItems...)
		}
	}

	result.RecalculateTotal()
	return result, nil
}

func (m *Module) discoverConfigs(ctx context.Context) []string {
	out, err := m.commandRunner(ctx, "snapper", "--csvout", "list-configs", "--columns", "config")
	if err != nil || len(out) == 0 {
		return m.configs
	}

	var found []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.EqualFold(line, "config") {
			continue
		}
		// Clean quotes or commas
		line = strings.Trim(line, "\", 	")
		if line != "" {
			found = append(found, line)
		}
	}

	if len(found) > 0 {
		return found
	}
	return m.configs
}

func (m *Module) scanConfigSnapshots(ctx context.Context, config string) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	// Run with --csvout to obtain structured machine-readable fields
	out, err := m.commandRunner(ctx, "snapper", "--csvout", "-c", config, "list", "--columns", "number,default,active,date,type,description,used-space")
	if err != nil {
		// If CSV fails or permission denied, try standard output or exit with diagnostic error
		outTable, errTable := m.commandRunner(ctx, "snapper", "-c", config, "list")
		if errTable != nil {
			if isSnapperPermissionDenied(errTable) || isSnapperPermissionDenied(err) || isSnapperPermissionOutput(outTable) || isSnapperPermissionOutput(out) {
				return items, ErrPermissionDenied
			}
			return items, fmt.Errorf("snapper list failed for config %q: %w", config, errTable)
		}
		if isSnapperPermissionOutput(outTable) {
			return items, ErrPermissionDenied
		}
		return parseSnapperTableOutput(outTable, config), nil
	}

	if isSnapperPermissionOutput(out) {
		return items, ErrPermissionDenied
	}

	return parseSnapperCSVOutput(out, config), nil
}

func isSnapperPermissionOutput(out []byte) bool {
	if len(out) == 0 {
		return false
	}
	s := strings.ToLower(string(out))
	return strings.Contains(s, "no permission") ||
		strings.Contains(s, "permission denied") ||
		strings.Contains(s, "allow_users") ||
		strings.Contains(s, "root privileges")
}

func isSnapperPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrPermissionDenied) {
		return true
	}
	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "permission") ||
		strings.Contains(errStr, "denied") ||
		strings.Contains(errStr, "root privileges") {
		return true
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		stderrStr := strings.ToLower(string(exitErr.Stderr))
		if strings.Contains(stderrStr, "permission") ||
			strings.Contains(stderrStr, "denied") ||
			strings.Contains(stderrStr, "root privileges") {
			return true
		}
	}
	// Snapper requires root privileges by default on Linux
	if os.Geteuid() != 0 {
		return true
	}
	return false
}

// Clean deletes the designated snapshot IDs from Snapper.
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

	// Filter out any protected snapshots (snapshot 0 or default boot snapshot)
	var safeToDelete []core.CleanableItem
	for _, it := range items {
		snapID := it.Metadata["snapshot_id"]
		if snapID == "0" || it.Metadata["protected"] == "true" {
			cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("refusing to delete protected snapshot %s", it.ID))
			continue
		}
		safeToDelete = append(safeToDelete, it)
	}

	if opts.DryRun {
		for _, item := range safeToDelete {
			cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
			cleanRes.FreedBytes += item.Size
		}
		cleanRes.Duration = time.Since(start)
		return cleanRes, nil
	}

	// Trigger optional pre-clean safeguard snapshot if requested
	if opts.SnapshotSafeguard {
		if err := m.CreateSafeguardSnapshot("Sweep pre-clean safeguard snapshot"); err != nil {
			cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to create safeguard snapshot: %w", err))
		}
	}

	// Group snapshot IDs by config
	configGroups := make(map[string][]core.CleanableItem)
	for _, it := range safeToDelete {
		cfg := it.Metadata["config"]
		if cfg == "" {
			cfg = "root"
		}
		configGroups[cfg] = append(configGroups[cfg], it)
	}

	for cfg, cfgItems := range configGroups {
		select {
		case <-ctx.Done():
			cleanRes.Errors = append(cleanRes.Errors, ctx.Err())
			cleanRes.Duration = time.Since(start)
			return cleanRes, ctx.Err()
		default:
		}

		var idArgs []string
		for _, it := range cfgItems {
			id := it.Metadata["snapshot_id"]
			if id != "" && id != "0" {
				idArgs = append(idArgs, id)
			}
		}

		if len(idArgs) == 0 {
			continue
		}

		args := append([]string{"-c", cfg, "delete"}, idArgs...)
		_, err := m.commandRunner(ctx, "snapper", args...)
		if err != nil {
			cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("failed to delete snapshots for config %s: %w", cfg, err))
		} else {
			for _, it := range cfgItems {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, it)
				cleanRes.FreedBytes += it.Size
			}
		}
	}

	cleanRes.Duration = time.Since(start)
	return cleanRes, nil
}

// SetMountsPath sets the mounts file path for testing.
func (m *Module) SetMountsPath(path string) {
	m.mountsPath = path
}

// SetConfigs sets the config list for testing.
func (m *Module) SetConfigs(cfgs []string) {
	m.configs = cfgs
}

// SetCommandRunner overrides the command runner for testing.
func (m *Module) SetCommandRunner(runner CommandRunner) {
	m.commandRunner = runner
}

// parseSnapperCSVOutput parses CSV output from `snapper --csvout -c <config> list --columns ...`.
func parseSnapperCSVOutput(output []byte, config string) []core.CleanableItem {
	var items []core.CleanableItem
	scanner := bufio.NewScanner(bytes.NewReader(output))

	now := time.Now()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}

		// Header check
		if strings.EqualFold(strings.Trim(parts[0], "\" "), "number") || strings.EqualFold(strings.Trim(parts[0], "\" "), "#") {
			continue
		}

		for i := range parts {
			parts[i] = strings.Trim(parts[i], "\" \t")
		}

		number := parts[0]
		if number == "" {
			continue
		}

		defaultFlag := false
		activeFlag := false
		dateStr := ""
		snapType := "single"
		description := ""
		usedSpaceStr := ""

		if len(parts) >= 2 {
			defaultFlag = parseSnapperBool(parts[1])
		}
		if len(parts) >= 3 {
			activeFlag = parseSnapperBool(parts[2])
		}
		if len(parts) >= 4 {
			dateStr = parts[3]
		}
		if len(parts) >= 5 && parts[4] != "" {
			snapType = strings.ToLower(parts[4])
		}
		if len(parts) >= 6 {
			description = parts[5]
		}
		if len(parts) >= 7 {
			usedSpaceStr = parts[6]
		}

		snapTime := parseSnapperTime(dateStr)
		ageCategory, safety, selected, isProtected := classifySnapshot(number, defaultFlag, activeFlag, snapTime, now)

		var sizeBytes int64
		if usedSpaceStr != "" {
			usedSpaceStr = strings.ReplaceAll(usedSpaceStr, "\u00a0", " ")
			if parsed, err := core.ParseBytes(usedSpaceStr); err == nil {
				sizeBytes = parsed
			}
		}

		desc := description
		if desc == "" {
			desc = fmt.Sprintf("Btrfs snapshot (%s)", snapType)
		}

		metadata := map[string]string{
			"config":       config,
			"snapshot_id":  number,
			"type":         snapType,
			"age_category": ageCategory,
			"protected":    fmt.Sprintf("%t", isProtected),
		}
		if dateStr != "" {
			metadata["date"] = dateStr
		}

		items = append(items, core.CleanableItem{
			ID:          fmt.Sprintf("snapper:%s:%s", config, number),
			Name:        fmt.Sprintf("Snapshot #%s [%s] (%s, %s)", number, config, snapType, ageCategory),
			Description: desc,
			Size:        sizeBytes,
			Selected:    selected,
			Safety:      safety,
			ModTime:     snapTime,
			Metadata:    metadata,
		})
	}

	return items
}

// parseSnapperTableOutput parses plain-text tabular output from `snapper list`.
func parseSnapperTableOutput(output []byte, config string) []core.CleanableItem {
	var items []core.CleanableItem
	scanner := bufio.NewScanner(bytes.NewReader(output))
	now := time.Now()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "─") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		number := fields[0]
		// Skip non-numeric first field (like headers)
		if number == "#" || number == "Type" {
			continue
		}

		defaultFlag := false
		activeFlag := false
		snapType := "single"

		for _, f := range fields {
			if f == "default" || f == "yes" {
				defaultFlag = true
			}
			if f == "active" || f == "+" {
				activeFlag = true
			}
			switch strings.ToLower(f) {
			case "single", "pre", "post", "timeline":
				snapType = strings.ToLower(f)
			}
		}

		ageCategory, safety, selected, isProtected := classifySnapshot(number, defaultFlag, activeFlag, time.Time{}, now)

		metadata := map[string]string{
			"config":       config,
			"snapshot_id":  number,
			"type":         snapType,
			"age_category": ageCategory,
			"protected":    fmt.Sprintf("%t", isProtected),
		}

		items = append(items, core.CleanableItem{
			ID:          fmt.Sprintf("snapper:%s:%s", config, number),
			Name:        fmt.Sprintf("Snapshot #%s [%s] (%s, %s)", number, config, snapType, ageCategory),
			Description: fmt.Sprintf("Btrfs snapshot (%s)", snapType),
			Selected:    selected,
			Safety:      safety,
			Metadata:    metadata,
		})
	}

	return items
}

func parseSnapperBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "yes" || s == "true" || s == "1" || s == "+" || s == "default" || s == "active"
}

func parseSnapperTime(s string) time.Time {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05 MST",
		time.RFC3339,
		"Mon 02 Jan 2006 03:04:05 PM MST",
		"Mon Jan 2 15:04:05 2006",
	}

	s = strings.TrimSpace(s)
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// classifySnapshot determines safety level, age category, and selection state.
func classifySnapshot(number string, isDefault, isActive bool, snapTime, now time.Time) (ageCategory string, safety core.SafetyLevel, selected bool, isProtected bool) {
	// Rule 1: Never delete snapshot 0 (current subvolume) or active/default boot snapshot
	if number == "0" || isDefault || isActive {
		return "current", core.SafetyHigh, false, true
	}

	if snapTime.IsZero() {
		// Age unknown, default to safe caution but unselected
		return ">7d", core.SafetyCaution, false, false
	}

	age := now.Sub(snapTime)
	switch {
	case age > 30*24*time.Hour:
		// Older than 30 days: safe to prune
		return ">30d", core.SafetySafe, true, false
	case age > 7*24*time.Hour:
		// 7 - 30 days: caution, unselected by default
		return ">7d", core.SafetyCaution, false, false
	case age > 3*24*time.Hour:
		// 3 - 7 days: caution, unselected by default
		return ">3d", core.SafetyCaution, false, false
	default:
		// Recent snapshots <= 3 days: high caution, unselected
		return "<=3d", core.SafetyHigh, false, false
	}
}
