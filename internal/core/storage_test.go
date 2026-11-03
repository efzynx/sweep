package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectSystemStorage_Host(t *testing.T) {
	info, err := DetectSystemStorage()
	if err != nil {
		t.Fatalf("DetectSystemStorage failed: %v", err)
	}

	// Verify Root
	if info.Root.Path != "/" {
		t.Errorf("expected Root.Path to be '/', got %q", info.Root.Path)
	}
	if info.Root.TotalBytes == 0 {
		t.Errorf("expected Root.TotalBytes > 0")
	}
	if info.Root.FreeBytes == 0 {
		t.Errorf("expected Root.FreeBytes > 0")
	}
	if info.Root.DevID == 0 {
		t.Errorf("expected Root.DevID != 0")
	}

	// Verify Home
	if info.Home.Path != "/home" {
		t.Errorf("expected Home.Path to be '/home', got %q", info.Home.Path)
	}
	if info.Home.TotalBytes == 0 {
		t.Errorf("expected Home.TotalBytes > 0")
	}
	if info.Home.DevID == 0 {
		t.Errorf("expected Home.DevID != 0")
	}

	// Acceptance criteria verification on this host
	t.Logf("Root: dev=%s fstype=%s devID=%d total=%s free=%s usedPct=%.1f%%",
		info.Root.Device, info.Root.FSType, info.Root.DevID,
		FormatBytes(int64(info.Root.TotalBytes)), FormatBytes(int64(info.Root.FreeBytes)), info.Root.UsedPercent)
	t.Logf("Home: dev=%s fstype=%s devID=%d total=%s free=%s usedPct=%.1f%%",
		info.Home.Device, info.Home.FSType, info.Home.DevID,
		FormatBytes(int64(info.Home.TotalBytes)), FormatBytes(int64(info.Home.FreeBytes)), info.Home.UsedPercent)
	t.Logf("HasSeparateHome: %v", info.HasSeparateHome)

	if !info.HasSeparateHome {
		t.Errorf("expected HasSeparateHome to be true on this host (/dev/sda4 root vs /dev/sda5 home)")
	}
	if info.Root.Device != "/dev/sda4" {
		t.Errorf("expected Root.Device to be /dev/sda4, got %q", info.Root.Device)
	}
	if info.Home.Device != "/dev/sda5" {
		t.Errorf("expected Home.Device to be /dev/sda5, got %q", info.Home.Device)
	}
	if info.Root.FSType != "btrfs" {
		t.Errorf("expected Root.FSType to be btrfs, got %q", info.Root.FSType)
	}
	if info.Home.FSType != "btrfs" {
		t.Errorf("expected Home.FSType to be btrfs, got %q", info.Home.FSType)
	}
}

func TestClassifyPathStorage(t *testing.T) {
	info, err := DetectSystemStorage()
	if err != nil {
		t.Fatalf("DetectSystemStorage failed: %v", err)
	}

	homeDir, _ := GetUserHomeDir()
	if homeDir == "" {
		homeDir = "/home/efzyn"
	}

	tests := []struct {
		name     string
		path     string
		expected StorageScope
	}{
		{
			name:     "empty path defaults to root",
			path:     "",
			expected: StorageScopeRoot,
		},
		{
			name:     "root pacman cache",
			path:     "/var/cache/pacman",
			expected: StorageScopeRoot,
		},
		{
			name:     "tmp path",
			path:     "/tmp",
			expected: StorageScopeRoot,
		},
		{
			name:     "system config path",
			path:     "/etc/sweep/config.yaml",
			expected: StorageScopeRoot,
		},
		{
			name:     "root filesystem itself",
			path:     "/",
			expected: StorageScopeRoot,
		},
		{
			name:     "user npm cache with explicit home path",
			path:     filepath.Join(homeDir, ".npm"),
			expected: StorageScopeHome,
		},
		{
			name:     "tilde cache expansion",
			path:     "~/.cache",
			expected: StorageScopeHome,
		},
		{
			name:     "tilde go cache expansion",
			path:     "~/go/pkg/mod",
			expected: StorageScopeHome,
		},
		{
			name:     "home directory itself",
			path:     "/home",
			expected: StorageScopeHome,
		},
		{
			name:     "user home directory itself",
			path:     homeDir,
			expected: StorageScopeHome,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyPathStorage(tc.path, info)
			if got != tc.expected {
				t.Errorf("ClassifyPathStorage(%q) = %q, want %q", tc.path, got, tc.expected)
			}
		})
	}
}

func TestClassifyPathStorage_SinglePartition(t *testing.T) {
	// Simulate single partition system where Root and Home share DevID
	singlePartitionInfo := SystemStorageInfo{
		Root: MountStats{
			Path:   "/",
			Device: "/dev/sda1",
			FSType: "ext4",
			DevID:  100,
		},
		Home: MountStats{
			Path:   "/home",
			Device: "/dev/sda1",
			FSType: "ext4",
			DevID:  100,
		},
		HasSeparateHome: false,
	}

	homeDir, _ := GetUserHomeDir()
	if homeDir == "" {
		homeDir = "/home/efzyn"
	}

	// Home paths should still classify as Home
	if got := ClassifyPathStorage("~/test", singlePartitionInfo); got != StorageScopeHome {
		t.Errorf("expected ~/test to classify as home, got %q", got)
	}
	if got := ClassifyPathStorage(filepath.Join(homeDir, "cache"), singlePartitionInfo); got != StorageScopeHome {
		t.Errorf("expected user home path to classify as home, got %q", got)
	}
	if got := ClassifyPathStorage("/home/otheruser/.cache", singlePartitionInfo); got != StorageScopeHome {
		t.Errorf("expected /home path to classify as home, got %q", got)
	}

	// Root paths should classify as Root
	if got := ClassifyPathStorage("/var/log", singlePartitionInfo); got != StorageScopeRoot {
		t.Errorf("expected /var/log to classify as root, got %q", got)
	}
	if got := ClassifyPathStorage("/etc/os-release", singlePartitionInfo); got != StorageScopeRoot {
		t.Errorf("expected /etc/os-release to classify as root, got %q", got)
	}
}

func TestStorageTagging(t *testing.T) {
	info, err := DetectSystemStorage()
	if err != nil {
		t.Fatalf("DetectSystemStorage failed: %v", err)
	}

	// Test tagging root item
	rootItem := CleanableItem{
		ID:       "pacman-pkg-cache",
		Name:     "Pacman Package Cache",
		Path:     "/var/cache/pacman/pkg",
		Metadata: map[string]string{"type": "package"},
	}
	TagItemStorage(&rootItem, info)
	if rootItem.Metadata["storage"] != string(StorageScopeRoot) {
		t.Errorf("expected root item metadata['storage'] = %q, got %q", StorageScopeRoot, rootItem.Metadata["storage"])
	}
	if rootItem.Metadata["type"] != "package" {
		t.Errorf("expected existing metadata to be preserved, got %v", rootItem.Metadata)
	}
	if scope := ItemStorageScope(rootItem); scope != StorageScopeRoot {
		t.Errorf("expected ItemStorageScope(rootItem) = %q, got %q", StorageScopeRoot, scope)
	}

	// Test tagging home item with nil Metadata initially
	homeDir, _ := GetUserHomeDir()
	homeItem := CleanableItem{
		ID:   "user-thumbnail-cache",
		Name: "User Thumbnails",
		Path: filepath.Join(homeDir, ".cache/thumbnails"),
	}
	TagItemStorage(&homeItem, info)
	if homeItem.Metadata["storage"] != string(StorageScopeHome) {
		t.Errorf("expected home item metadata['storage'] = %q, got %q", StorageScopeHome, homeItem.Metadata["storage"])
	}
	if scope := ItemStorageScope(homeItem); scope != StorageScopeHome {
		t.Errorf("expected ItemStorageScope(homeItem) = %q, got %q", StorageScopeHome, scope)
	}

	// Test untagged item defaults to root
	untaggedItem := CleanableItem{
		ID:   "untagged",
		Name: "Untagged Item",
	}
	if scope := ItemStorageScope(untaggedItem); scope != StorageScopeRoot {
		t.Errorf("expected untagged item to default to %q, got %q", StorageScopeRoot, scope)
	}

	// Test item with empty storage key defaults to root
	emptyTagItem := CleanableItem{
		ID:       "empty-tag",
		Name:     "Empty Tag",
		Metadata: map[string]string{"storage": ""},
	}
	if scope := ItemStorageScope(emptyTagItem); scope != StorageScopeRoot {
		t.Errorf("expected empty storage key to default to %q, got %q", StorageScopeRoot, scope)
	}

	// Test nil item handling
	TagItemStorage(nil, info) // should not panic
}

func TestMockMountTableDetection(t *testing.T) {
	// Mock mounts file with separate / and /home, including escaped spaces
	mockContent := `/dev/nvme0n1p2 / btrfs rw,relatime 0 0
/dev/nvme0n1p3 /home ext4 rw,relatime 0 0
/dev/sda1 /media/My\040Passport ntfs rw 0 0
`
	tmpFile := filepath.Join(t.TempDir(), "mounts")
	if err := os.WriteFile(tmpFile, []byte(mockContent), 0644); err != nil {
		t.Fatalf("failed to write mock mounts: %v", err)
	}

	orig := mountsFilePath
	mountsFilePath = tmpFile
	defer func() { mountsFilePath = orig }()

	dev, fs := findMountEntry("/")
	if dev != "/dev/nvme0n1p2" || fs != "btrfs" {
		t.Errorf("findMountEntry('/') = (%q, %q), want (/dev/nvme0n1p2, btrfs)", dev, fs)
	}

	devHome, fsHome := findMountEntry("/home")
	if devHome != "/dev/nvme0n1p3" || fsHome != "ext4" {
		t.Errorf("findMountEntry('/home') = (%q, %q), want (/dev/nvme0n1p3, ext4)", devHome, fsHome)
	}

	// Test unescaped space in mount path
	devMedia, fsMedia := findMountEntry("/media/My Passport")
	if devMedia != "/dev/sda1" || fsMedia != "ntfs" {
		t.Errorf("findMountEntry('/media/My Passport') = (%q, %q), want (/dev/sda1, ntfs)", devMedia, fsMedia)
	}

	// Fallback when /home is not in mounts
	mockSingle := `/dev/sda2 / ext4 rw,relatime 0 0
`
	if err := os.WriteFile(tmpFile, []byte(mockSingle), 0644); err != nil {
		t.Fatalf("failed to write mock single mount: %v", err)
	}
	devHomeFallback, fsHomeFallback := findMountEntry("/home")
	if devHomeFallback != "/dev/sda2" || fsHomeFallback != "ext4" {
		t.Errorf("findMountEntry('/home') fallback = (%q, %q), want (/dev/sda2, ext4)", devHomeFallback, fsHomeFallback)
	}
}

func TestUnescapeMount(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/normal/path", "/normal/path"},
		{"/path/with\\040space", "/path/with space"},
		{"/path/with\\011tab", "/path/with\ttab"},
		{"/path/with\\134backslash", "/path/with\\backslash"},
	}

	for _, tt := range tests {
		got := unescapeMount(tt.input)
		if got != tt.expected {
			t.Errorf("unescapeMount(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestQueryMountStats_Error(t *testing.T) {
	_, err := queryMountStats("/non_existent_sweep_test_path_12345")
	if err == nil {
		t.Errorf("expected error querying non-existent path, got nil")
	}
}
