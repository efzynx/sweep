package distro

import (
	"errors"
	"testing"
)

func TestLookPathReal(t *testing.T) {
	ResetLookupCache()
	defer ResetLookupCache()

	// "go" binary is installed and on PATH in this environment
	path, ok := LookPath("go")
	if !ok || path == "" {
		t.Fatalf("expected LookPath(\"go\") to succeed, got ok=%v, path=%q", ok, path)
	}

	// Verify caching returns same path
	path2, ok2 := LookPath("go")
	if !ok2 || path2 != path {
		t.Errorf("expected cached lookup to match: got (%q, %v), want (%q, %v)", path2, ok2, path, ok)
	}

	// Non-existent binary
	nonExistent, ok := LookPath("non_existent_binary_xyz_12345")
	if ok || nonExistent != "" {
		t.Errorf("expected non-existent binary to fail, got ok=%v, path=%q", ok, nonExistent)
	}
}

func TestSubsystemAvailabilityRealHost(t *testing.T) {
	ResetLookupCache()
	defer ResetLookupCache()

	// The host system has pacman, docker, flatpak, snapper installed
	if !HasPacman() {
		t.Error("expected HasPacman() to be true on host system")
	}
	if !HasDocker() {
		t.Error("expected HasDocker() to be true on host system")
	}
	if !HasFlatpak() {
		t.Error("expected HasFlatpak() to be true on host system")
	}
	if !HasSnapper() {
		t.Error("expected HasSnapper() to be true on host system")
	}

	// The host system does NOT have apt, dnf, zypper installed
	if HasApt() {
		t.Error("expected HasApt() to be false on CachyOS host")
	}
	if HasDnf() {
		t.Error("expected HasDnf() to be false on CachyOS host")
	}
	if HasZypper() {
		t.Error("expected HasZypper() to be false on CachyOS host")
	}
}

func TestSubsystemAvailabilityWithMock(t *testing.T) {
	defer ResetLookPathMock()

	mockBinaries := map[string]string{
		"apt-get": "/usr/bin/apt-get",
		"dnf":     "/usr/bin/dnf",
		"zypper":  "/usr/bin/zypper",
	}

	SetLookPathMock(func(binary string) (string, error) {
		if path, found := mockBinaries[binary]; found {
			return path, nil
		}
		return "", errors.New("command not found")
	})

	if !HasApt() {
		t.Error("expected HasApt() to be true with mock")
	}
	if !HasDnf() {
		t.Error("expected HasDnf() to be true with mock")
	}
	if !HasZypper() {
		t.Error("expected HasZypper() to be true with mock")
	}
	if HasPacman() {
		t.Error("expected HasPacman() to be false with mock")
	}
	if HasDocker() {
		t.Error("expected HasDocker() to be false with mock")
	}
	if HasFlatpak() {
		t.Error("expected HasFlatpak() to be false with mock")
	}
	if HasSnapper() {
		t.Error("expected HasSnapper() to be false with mock")
	}
}
