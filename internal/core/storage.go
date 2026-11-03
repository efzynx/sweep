package core

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// StorageScope represents the targeted storage scope for disk metrics and cleanable items.
type StorageScope string

const (
	// StorageScopeAll encompasses all storage devices/filesystems.
	StorageScopeAll StorageScope = "all"

	// StorageScopeRoot targets items located on the root filesystem ("/").
	StorageScopeRoot StorageScope = "root"

	// StorageScopeHome targets items located on the user home filesystem ("/home").
	StorageScopeHome StorageScope = "home"
)

// MountStats represents filesystem capacity, device, and usage metrics for a specific mount path.
type MountStats struct {
	Path        string  `json:"path"`
	Device      string  `json:"device"`
	FSType      string  `json:"fs_type"`
	TotalBytes  uint64  `json:"total_bytes"`
	UsedBytes   uint64  `json:"used_bytes"`
	FreeBytes   uint64  `json:"free_bytes"`
	UsedPercent float64 `json:"used_percent"`
	DevID       uint64  `json:"dev_id"`
}

// SystemStorageInfo encapsulates storage metrics for root and home, indicating
// whether home resides on a dedicated filesystem or partition.
type SystemStorageInfo struct {
	Root            MountStats `json:"root"`
	Home            MountStats `json:"home"`
	HasSeparateHome bool       `json:"has_separate_home"`
}

// Internal hook for testing mount point parsing
var mountsFilePath = "/proc/mounts"

type mountRecord struct {
	device     string
	mountPoint string
	fsType     string
}

// unescapeMount unescapes octal sequences (e.g. \040 for space) in /proc/mounts.
func unescapeMount(s string) string {
	if !strings.Contains(s, "\\") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if oct, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(oct))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// findMountEntry finds the device and filesystem type for targetPath from /proc/mounts.
// It prioritizes exact matches, falling back to the longest mountpoint prefix.
func findMountEntry(targetPath string) (string, string) {
	cleanTarget := filepath.Clean(targetPath)

	file, err := os.Open(mountsFilePath)
	if err != nil {
		return "", ""
	}
	defer file.Close()

	var exactMatch *mountRecord
	var bestPrefixMatch *mountRecord
	bestPrefixLen := -1

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		dev := unescapeMount(fields[0])
		mountPoint := filepath.Clean(unescapeMount(fields[1]))
		fsType := fields[2]

		rec := mountRecord{
			device:     dev,
			mountPoint: mountPoint,
			fsType:     fsType,
		}

		if mountPoint == cleanTarget {
			exactMatch = &rec
		}

		// Prefix match check: root "/" is prefix of everything, or "/foo" is prefix of "/foo/bar"
		if mountPoint == "/" || strings.HasPrefix(cleanTarget, mountPoint+"/") || cleanTarget == mountPoint {
			if len(mountPoint) > bestPrefixLen {
				bestPrefixLen = len(mountPoint)
				bestPrefixMatch = &rec
			}
		}
	}

	if exactMatch != nil {
		return exactMatch.device, exactMatch.fsType
	}
	if bestPrefixMatch != nil {
		return bestPrefixMatch.device, bestPrefixMatch.fsType
	}

	return "", ""
}

// queryMountStats retrieves the capacity and device metadata for a mount path.
func queryMountStats(path string) (MountStats, error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return MountStats{}, err
	}

	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		return MountStats{}, err
	}

	total := fs.Blocks * uint64(fs.Bsize)
	free := fs.Bavail * uint64(fs.Bsize)
	var used uint64
	if total >= free {
		used = total - free
	}

	var pct float64
	if total > 0 {
		pct = (float64(used) / float64(total)) * 100.0
	}

	dev, fsType := findMountEntry(path)
	if fsType == "" {
		switch uint32(fs.Type) {
		case unix.BTRFS_SUPER_MAGIC:
			fsType = "btrfs"
		case 0xef53: // EXT4_SUPER_MAGIC / EXT2 / EXT3
			fsType = "ext4"
		case 0x58465342: // XFS_SUPER_MAGIC
			fsType = "xfs"
		case 0x01021994: // TMPFS_MAGIC
			fsType = "tmpfs"
		default:
			fsType = "unknown"
		}
	}

	return MountStats{
		Path:        path,
		Device:      dev,
		FSType:      fsType,
		TotalBytes:  total,
		UsedBytes:   used,
		FreeBytes:   free,
		UsedPercent: pct,
		DevID:       st.Dev,
	}, nil
}

// DetectSystemStorage queries filesystem telemetry for Root ("/") and Home ("/home").
// It compares device IDs to identify if Home has a separate filesystem partition.
func DetectSystemStorage() (SystemStorageInfo, error) {
	var info SystemStorageInfo

	rootStats, err := queryMountStats("/")
	if err != nil {
		return info, fmt.Errorf("statfs root / failed: %w", err)
	}
	info.Root = rootStats

	homeStats, err := queryMountStats("/home")
	if err != nil {
		// If /home cannot be stat'd (e.g., minimal container without /home),
		// fall back to root filesystem metrics.
		info.Home = rootStats
		info.Home.Path = "/home"
		info.HasSeparateHome = false
		return info, nil
	}
	info.Home = homeStats

	// Check if device ID of / != device ID of /home
	if info.Root.DevID != 0 && info.Home.DevID != 0 && info.Root.DevID != info.Home.DevID {
		info.HasSeparateHome = true
	} else {
		info.HasSeparateHome = false
	}

	return info, nil
}

// ClassifyPathStorage classifies a file/directory path into StorageScopeRoot or StorageScopeHome.
//
// Classification logic:
// 1. If path is empty, return StorageScopeRoot.
// 2. Expand path with ExpandPath (handling ~ and env vars).
// 3. Match against the current user's home directory prefix.
// 4. Match against generic /home prefix.
// 5. If separate home partition exists, stat the path and compare stat.Dev with Home.DevID.
// 6. Otherwise return StorageScopeRoot.
func ClassifyPathStorage(path string, info SystemStorageInfo) StorageScope {
	if path == "" {
		return StorageScopeRoot
	}

	expanded := ExpandPath(path)
	if expanded == "" {
		return StorageScopeRoot
	}
	cleanPath := filepath.Clean(expanded)

	// Check user home directory
	userHome, err := GetUserHomeDir()
	if err == nil && userHome != "" {
		cleanUserHome := filepath.Clean(userHome)
		if cleanPath == cleanUserHome || strings.HasPrefix(cleanPath, cleanUserHome+string(filepath.Separator)) {
			return StorageScopeHome
		}
	}

	// Check generic /home hierarchy
	if cleanPath == "/home" || strings.HasPrefix(cleanPath, "/home/") {
		return StorageScopeHome
	}

	// If separate home exists and path exists on disk, check device ID
	if info.HasSeparateHome && info.Home.DevID != 0 {
		var st unix.Stat_t
		if err := unix.Stat(cleanPath, &st); err == nil {
			if st.Dev == info.Home.DevID {
				return StorageScopeHome
			}
		}
	}

	return StorageScopeRoot
}

// TagItemStorage sets the "storage" metadata key on a CleanableItem based on its Path.
func TagItemStorage(item *CleanableItem, info SystemStorageInfo) {
	if item == nil {
		return
	}
	if item.Metadata == nil {
		item.Metadata = make(map[string]string)
	}
	scope := ClassifyPathStorage(item.Path, info)
	item.Metadata["storage"] = string(scope)
}

// ItemStorageScope reads the "storage" metadata key from a CleanableItem,
// defaulting to StorageScopeRoot if not set.
func ItemStorageScope(item CleanableItem) StorageScope {
	if item.Metadata != nil {
		if s, ok := item.Metadata["storage"]; ok && s != "" {
			return StorageScope(s)
		}
	}
	return StorageScopeRoot
}
