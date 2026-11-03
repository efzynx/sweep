package distro

import (
	"os/exec"
	"sync"
)

type pathCacheEntry struct {
	path  string
	found bool
}

var (
	lookupMu       sync.RWMutex
	lookupCache    = make(map[string]pathCacheEntry)
	lookPathMockFn func(binary string) (string, error)
)

// LookPath searches for an executable binary in the directories named by the PATH environment variable.
// Results are cached in-memory for fast repeated lookups.
func LookPath(binary string) (string, bool) {
	lookupMu.RLock()
	if entry, exists := lookupCache[binary]; exists {
		lookupMu.RUnlock()
		return entry.path, entry.found
	}
	mockFn := lookPathMockFn
	lookupMu.RUnlock()

	var path string
	var err error

	if mockFn != nil {
		path, err = mockFn(binary)
	} else {
		path, err = exec.LookPath(binary)
	}

	found := (err == nil && path != "")
	lookupMu.Lock()
	lookupCache[binary] = pathCacheEntry{path: path, found: found}
	lookupMu.Unlock()

	return path, found
}

// ResetLookupCache clears the cached LookPath results.
func ResetLookupCache() {
	lookupMu.Lock()
	defer lookupMu.Unlock()
	lookupCache = make(map[string]pathCacheEntry)
}

// SetLookPathMock overrides LookPath resolution for testing.
func SetLookPathMock(fn func(binary string) (string, error)) {
	lookupMu.Lock()
	defer lookupMu.Unlock()
	lookPathMockFn = fn
	lookupCache = make(map[string]pathCacheEntry)
}

// ResetLookPathMock removes the LookPath testing mock.
func ResetLookPathMock() {
	lookupMu.Lock()
	defer lookupMu.Unlock()
	lookPathMockFn = nil
	lookupCache = make(map[string]pathCacheEntry)
}

// HasPacman reports whether the Arch Linux pacman package manager is installed on PATH.
func HasPacman() bool {
	_, ok := LookPath("pacman")
	return ok
}

// HasApt reports whether Debian/Ubuntu APT (apt-get or apt) is installed on PATH.
func HasApt() bool {
	if _, ok := LookPath("apt-get"); ok {
		return true
	}
	_, ok := LookPath("apt")
	return ok
}

// HasDnf reports whether Fedora/RHEL DNF (dnf or dnf5) is installed on PATH.
func HasDnf() bool {
	if _, ok := LookPath("dnf"); ok {
		return true
	}
	_, ok := LookPath("dnf5")
	return ok
}

// HasZypper reports whether openSUSE Zypper package manager is installed on PATH.
func HasZypper() bool {
	_, ok := LookPath("zypper")
	return ok
}

// HasFlatpak reports whether the Flatpak sandboxed application CLI is installed on PATH.
func HasFlatpak() bool {
	_, ok := LookPath("flatpak")
	return ok
}

// HasDocker reports whether the Docker container runtime CLI is installed on PATH.
func HasDocker() bool {
	_, ok := LookPath("docker")
	return ok
}

// HasSnapper reports whether the Snapper Btrfs snapshot management utility is installed on PATH.
func HasSnapper() bool {
	_, ok := LookPath("snapper")
	return ok
}

// HasSnap reports whether the Snap sandboxed application CLI is installed on PATH.
func HasSnap() bool {
	_, ok := LookPath("snap")
	return ok
}
