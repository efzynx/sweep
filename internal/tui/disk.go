package tui

import (
	"fmt"
	"strings"

	"github.com/efzynx/sweep/internal/core"
	"golang.org/x/sys/unix"
)

// DiskStats represents filesystem capacity and usage metrics.
type DiskStats struct {
	TotalBytes  uint64  `json:"total_bytes"`
	FreeBytes   uint64  `json:"free_bytes"`
	UsedBytes   uint64  `json:"used_bytes"`
	UsedPercent float64 `json:"used_percent"`
}

// GetRootDiskStats queries the root filesystem ("/") for storage metrics.
func GetRootDiskStats() (DiskStats, error) {
	return GetDiskStats("/")
}

// GetDiskStats queries the filesystem at path for storage metrics.
func GetDiskStats(path string) (DiskStats, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return DiskStats{}, err
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	var used uint64
	if total >= free {
		used = total - free
	}

	var pct float64
	if total > 0 {
		pct = (float64(used) / float64(total)) * 100.0
	}

	return DiskStats{
		TotalBytes:  total,
		FreeBytes:   free,
		UsedBytes:   used,
		UsedPercent: pct,
	}, nil
}

// RenderDiskUsageBar formats a visual ASCII progress meter for disk capacity.
// Example: [████████░░░░░░░░░░░░] 40.0% (40.0 GB / 100.0 GB)
// Zero emojis: uses standard block characters (█ and ░).
func RenderDiskUsageBar(stats DiskStats, barWidth int) string {
	if barWidth < 5 {
		barWidth = 10
	}

	fillWidth := int((stats.UsedPercent / 100.0) * float64(barWidth))
	if fillWidth > barWidth {
		fillWidth = barWidth
	}
	if fillWidth < 0 {
		fillWidth = 0
	}
	emptyWidth := barWidth - fillWidth

	filled := strings.Repeat("█", fillWidth)
	empty := strings.Repeat("░", emptyWidth)

	usedStr := core.FormatBytes(int64(stats.UsedBytes))
	totalStr := core.FormatBytes(int64(stats.TotalBytes))

	return fmt.Sprintf("[%s%s] %.1f%% (%s / %s)", filled, empty, stats.UsedPercent, usedStr, totalStr)
}
