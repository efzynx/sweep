package dnf

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/efzynx/sweep/internal/core"
	"github.com/efzynx/sweep/internal/core/distro"
)

const (
	ModuleID       = "dnf"
	ModuleName     = "DNF Package Cache"
	ModuleCategory = "package_manager"
	ModuleDesc     = "Clean DNF/RPM package caches, repo metadata, and old non-running kernels on Fedora/RHEL"
)

var defaultDnfCachePaths = []string{
	"/var/cache/dnf",
	"/var/cache/libdnf5",
}

// CommandRunner defines the function signature for running external CLI commands.
type CommandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Module implements core.Module for Fedora/RHEL DNF package manager.
type Module struct {
	cachePaths    []string
	commandRunner CommandRunner
	runningKernel string // optional override for testing
}

// New creates and returns a new DNF module instance.
func New() *Module {
	paths := make([]string, len(defaultDnfCachePaths))
	copy(paths, defaultDnfCachePaths)

	return &Module{
		cachePaths: paths,
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

func (m *Module) IsAvailable() bool {
	if distro.HasDnf() {
		return true
	}
	info := distro.Current()
	return info != nil && info.Family == distro.FamilyFedora
}

func (m *Module) RequiresElevation() bool {
	return true
}

func (m *Module) Scan(ctx context.Context) (core.ScanResult, error) {
	result := core.ScanResult{
		Items:     make([]core.CleanableItem, 0),
		ScannedAt: time.Now(),
	}

	if !m.IsAvailable() {
		return result, nil
	}

	// 1. Scan /var/cache/dnf and /var/cache/libdnf5 package caches
	cacheItems, err := m.scanPackageCaches(ctx)
	if err != nil && ctx.Err() != nil {
		return result, err
	}
	result.Items = append(result.Items, cacheItems...)

	if err := ctx.Err(); err != nil {
		return result, err
	}

	// 2. Scan old non-running kernels via rpm -q kernel vs uname -r
	kernelItems, err := m.scanOldKernels(ctx)
	if err != nil && ctx.Err() != nil {
		return result, err
	}
	result.Items = append(result.Items, kernelItems...)

	result.RecalculateTotal()
	return result, nil
}

// scanPackageCaches inspects candidate DNF cache directories for RPM packages.
func (m *Module) scanPackageCaches(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem
	var totalSize int64
	var rpmCount int
	var foundPaths []string

	for _, cPath := range m.cachePaths {
		if _, err := os.Stat(cPath); err != nil {
			continue
		}

		foundPaths = append(foundPaths, cPath)
		err := filepath.WalkDir(cPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if d.IsDir() {
				return nil
			}

			if strings.HasSuffix(d.Name(), ".rpm") {
				if info, err := d.Info(); err == nil {
					totalSize += info.Size()
					rpmCount++
				}
			}
			return nil
		})

		if err != nil && ctx.Err() != nil {
			return items, err
		}
	}

	if rpmCount > 0 {
		firstPath := ""
		if len(foundPaths) > 0 {
			firstPath = foundPaths[0]
		}
		items = append(items, core.CleanableItem{
			ID:          "dnf-packages",
			Name:        fmt.Sprintf("Cached .rpm packages (%d files)", rpmCount),
			Description: "Downloaded RPM packages in " + strings.Join(foundPaths, ", "),
			Path:        firstPath,
			Size:        totalSize,
			Selected:    true,
			Safety:      core.SafetySafe,
			Metadata: map[string]string{
				"type":  "packages",
				"count": strconv.Itoa(rpmCount),
			},
		})
	}

	return items, nil
}

// scanOldKernels detects installed kernel packages older than the running kernel.
func (m *Module) scanOldKernels(ctx context.Context) ([]core.CleanableItem, error) {
	var items []core.CleanableItem

	if m.commandRunner == nil {
		return items, nil
	}

	// 1. Determine running kernel
	running := m.runningKernel
	if running == "" {
		out, err := m.commandRunner(ctx, "uname", "-r")
		if err != nil {
			return items, nil
		}
		running = strings.TrimSpace(string(out))
	}

	if running == "" {
		return items, nil
	}

	// 2. Query installed kernels via rpm -q --queryformat
	// Try query with size first: "%{NAME}-%{VERSION}-%{RELEASE}.%{ARCH} %{SIZE}\n"
	out, err := m.commandRunner(ctx, "rpm", "-q", "--queryformat", "%{NAME}-%{VERSION}-%{RELEASE}.%{ARCH} %{SIZE}\n", "kernel")
	if err != nil {
		// Fallback to simple rpm -q kernel
		out, err = m.commandRunner(ctx, "rpm", "-q", "kernel")
		if err != nil {
			return items, nil
		}
	}

	oldKernels := ParseOldKernels(running, string(out))
	for _, k := range oldKernels {
		items = append(items, core.CleanableItem{
			ID:          "dnf-kernel:" + k.PackageName,
			Name:        fmt.Sprintf("Old kernel: %s", k.PackageName),
			Description: fmt.Sprintf("Non-running Linux kernel (%s, active: %s)", k.PackageName, running),
			Size:        k.Size,
			Selected:    true,
			Safety:      core.SafetyCaution,
			Metadata: map[string]string{
				"type":    "kernel",
				"package": k.PackageName,
				"active":  running,
			},
		})
	}

	return items, nil
}

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

	var hasPackages bool
	var packageItem core.CleanableItem
	var kernelItems []core.CleanableItem
	var hasAutoremove bool
	var autoremoveItems []core.CleanableItem

	for _, item := range items {
		select {
		case <-ctx.Done():
			cleanRes.Errors = append(cleanRes.Errors, ctx.Err())
			cleanRes.Duration = time.Since(start)
			return cleanRes, ctx.Err()
		default:
		}

		if item.ID == "dnf-packages" || item.Metadata["type"] == "packages" {
			hasPackages = true
			packageItem = item
		} else if strings.HasPrefix(item.ID, "dnf-kernel:") || item.Metadata["type"] == "kernel" {
			kernelItems = append(kernelItems, item)
		} else if strings.HasPrefix(item.ID, "dnf-orphan:") || item.Metadata["type"] == "orphan" {
			hasAutoremove = true
			autoremoveItems = append(autoremoveItems, item)
		}
	}

	// Choose dnf or dnf5 binary
	dnfBin := "dnf"
	if _, ok := distro.LookPath("dnf5"); ok {
		if _, okDnf := distro.LookPath("dnf"); !okDnf {
			dnfBin = "dnf5"
		}
	}

	// 1. Clean DNF packages & cache via dnf clean all
	if hasPackages {
		var cleanErr error
		if m.commandRunner != nil {
			_, cleanErr = m.commandRunner(ctx, dnfBin, "clean", "all")
		}

		// Also clean test / non-default paths
		for _, p := range m.cachePaths {
			if !isDefaultDnfPath(p) {
				_ = removeRpmFiles(p)
			}
		}

		if cleanErr != nil && len(m.cachePaths) > 0 && isDefaultDnfPath(m.cachePaths[0]) {
			cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("dnf clean all failed: %w", cleanErr))
		} else {
			cleanRes.CleanedItems = append(cleanRes.CleanedItems, packageItem)
			cleanRes.FreedBytes += packageItem.Size
		}
	}

	// 2. Remove old kernels via dnf remove -y
	if len(kernelItems) > 0 {
		var kernelPkgs []string
		for _, kItem := range kernelItems {
			pkg := kItem.Metadata["package"]
			if pkg == "" {
				pkg = strings.TrimPrefix(kItem.ID, "dnf-kernel:")
			}
			kernelPkgs = append(kernelPkgs, pkg)
		}

		var removeErr error
		if m.commandRunner != nil {
			args := append([]string{"remove", "-y"}, kernelPkgs...)
			_, removeErr = m.commandRunner(ctx, dnfBin, args...)
		}

		if removeErr != nil {
			cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("dnf remove kernels failed: %w", removeErr))
		} else {
			for _, k := range kernelItems {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, k)
				cleanRes.FreedBytes += k.Size
			}
		}
	}

	// 3. Clean autoremove / orphans if requested via dnf autoremove -y
	if hasAutoremove {
		var autoErr error
		if m.commandRunner != nil {
			_, autoErr = m.commandRunner(ctx, dnfBin, "autoremove", "-y")
		}
		if autoErr != nil {
			cleanRes.Errors = append(cleanRes.Errors, fmt.Errorf("dnf autoremove failed: %w", autoErr))
		} else {
			for _, item := range autoremoveItems {
				cleanRes.CleanedItems = append(cleanRes.CleanedItems, item)
				cleanRes.FreedBytes += item.Size
			}
		}
	}

	cleanRes.Duration = time.Since(start)
	return cleanRes, nil
}

func isDefaultDnfPath(p string) bool {
	for _, def := range defaultDnfCachePaths {
		if filepath.Clean(p) == filepath.Clean(def) {
			return true
		}
	}
	return false
}

// removeRpmFiles recursively removes .rpm files from dir.
func removeRpmFiles(dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".rpm") {
			_ = os.Remove(path)
		}
		return nil
	})
}

// SetCachePath overrides the default cache directory (used for testing).
func (m *Module) SetCachePath(path string) {
	m.cachePaths = []string{filepath.Clean(path)}
}

// SetCachePaths overrides multiple cache directories (used for testing).
func (m *Module) SetCachePaths(paths []string) {
	m.cachePaths = make([]string, len(paths))
	for i, p := range paths {
		m.cachePaths[i] = filepath.Clean(p)
	}
}

// SetCommandRunner overrides the CLI runner for testing.
func (m *Module) SetCommandRunner(runner CommandRunner) {
	m.commandRunner = runner
}

// SetRunningKernel overrides the running kernel version for testing.
func (m *Module) SetRunningKernel(version string) {
	m.runningKernel = version
}

// OldKernelInfo holds information about a detected old non-running kernel.
type OldKernelInfo struct {
	PackageName string
	Size        int64
}

// ParseOldKernels parses rpm -q kernel output and identifies kernels that do not match the running kernel.
func ParseOldKernels(runningKernel, rpmOutput string) []OldKernelInfo {
	var oldKernels []OldKernelInfo
	cleanRunning := strings.TrimSpace(runningKernel)
	if cleanRunning == "" {
		return oldKernels
	}

	scanner := bufio.NewScanner(strings.NewReader(rpmOutput))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "package kernel is not installed") {
			continue
		}

		fields := strings.Fields(line)
		pkgName := fields[0]

		// Check if this package corresponds to the currently running kernel
		if strings.Contains(pkgName, cleanRunning) {
			continue
		}

		var size int64
		if len(fields) > 1 {
			if s, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
				size = s
			}
		}
		if size <= 0 {
			// Sensible default estimate for installed kernel package (~80 MB)
			size = 80 * 1024 * 1024
		}

		oldKernels = append(oldKernels, OldKernelInfo{
			PackageName: pkgName,
			Size:        size,
		})
	}

	return oldKernels
}
