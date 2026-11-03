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

	cur := Current()
	if cur.IsArch() {
		if !HasPacman() {
			t.Error("expected HasPacman() to be true on Arch host")
		}
		if HasApt() {
			t.Error("expected HasApt() to be false on Arch host")
		}
	} else if cur.IsDebian() {
		if !HasApt() {
			t.Error("expected HasApt() to be true on Debian host")
		}
		if HasPacman() {
			t.Error("expected HasPacman() to be false on Debian host")
		}
	}
}

func TestSubsystemAvailabilityWithMock(t *testing.T) {
	defer ResetLookPathMock()

	mockBinaries := map[string]string{
		"apt-get": "/usr/bin/apt-get",
		"dnf":     "/usr/bin/dnf",
		"zypper":  "/usr/bin/zypper",
		"snap":    "/usr/bin/snap",
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
	if !HasSnap() {
		t.Error("expected HasSnap() to be true with mock")
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
