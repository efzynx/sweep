package distro

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const archReleaseFixture = `
NAME="Arch Linux"
PRETTY_NAME="Arch Linux"
ID=arch
BUILD_ID=rolling
ANSI_COLOR="38;2;23;147;209"
HOME_URL="https://archlinux.org/"
DOCUMENTATION_URL="https://wiki.archlinux.org/"
SUPPORT_URL="https://bbs.archlinux.org/"
LOGO=archlinux-logo
`

const cachyOSReleaseFixture = `
NAME="CachyOS Linux"
PRETTY_NAME="CachyOS"
ID=cachyos
ID_LIKE=arch
BUILD_ID=rolling
ANSI_COLOR="38;2;23;147;209"
HOME_URL="https://cachyos.org/"
`

const ubuntuReleaseFixture = `
PRETTY_NAME="Ubuntu 24.04 LTS"
NAME="Ubuntu"
VERSION_ID="24.04"
VERSION="24.04 LTS (Noble Numbat)"
VERSION_CODENAME=noble
ID=ubuntu
ID_LIKE=debian
HOME_URL="https://www.ubuntu.com/"
SUPPORT_URL="https://help.ubuntu.com/"
BUG_REPORT_URL="https://bugs.launchpad.net/ubuntu/"
UBUNTU_CODENAME=noble
LOGO=ubuntu-logo
`

const fedoraReleaseFixture = `
NAME="Fedora Linux"
VERSION="40 (Workstation Edition)"
ID=fedora
VERSION_ID=40
VERSION_CODENAME=""
PLATFORM_ID="platform:f40"
PRETTY_NAME="Fedora Linux 40 (Workstation Edition)"
ANSI_COLOR="0;38;2;60;110;180"
LOGO=fedora-logo-icon
CPE_NAME="cpe:/o:fedoraproject:fedora:40"
HOME_URL="https://fedoraproject.org/"
`

const suseTumbleweedReleaseFixture = `
NAME="openSUSE Tumbleweed"
# Simulated comment line
ID="opensuse-tumbleweed"
ID_LIKE="opensuse suse"
VERSION_ID="20240401"
PRETTY_NAME="openSUSE Tumbleweed"
ANSI_COLOR="0;32"
HOME_URL="https://www.opensuse.org/"
`

const suseLeapReleaseFixture = `
NAME="openSUSE Leap"
VERSION="15.5"
ID="opensuse-leap"
ID_LIKE="suse opensuse"
VERSION_ID="15.5"
PRETTY_NAME="openSUSE Leap 15.5"
ANSI_COLOR="0;32"
HOME_URL="https://www.opensuse.org/"
`

const debianReleaseFixture = `
PRETTY_NAME="Debian GNU/Linux 12 (bookworm)"
NAME="Debian GNU/Linux"
VERSION_ID="12"
VERSION="12 (bookworm)"
VERSION_CODENAME=bookworm
ID=debian
HOME_URL="https://www.debian.org/"
`

const rockyReleaseFixture = `
NAME="Rocky Linux"
VERSION="9.3 (Blue Onyx)"
ID="rocky"
ID_LIKE="rhel centos fedora"
VERSION_ID="9.3"
PRETTY_NAME="Rocky Linux 9.3 (Blue Onyx)"
`

const manjaroReleaseFixture = `
NAME="Manjaro Linux"
PRETTY_NAME="Manjaro Linux"
ID=manjaro
ID_LIKE=arch
BUILD_ID=rolling
`

const mintReleaseFixture = `
NAME="Linux Mint"
VERSION="21.3 (Virginia)"
ID=linuxmint
ID_LIKE="ubuntu debian"
VERSION_ID="21.3"
PRETTY_NAME="Linux Mint 21.3"
`

func TestParseOSReleaseFixtures(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		expectedID     string
		expectedFamily DistroFamily
		expectedName   string
		expectedPretty string
		expectedVer    string
		expectedLikes  []string
	}{
		{
			name:           "Arch Linux",
			content:        archReleaseFixture,
			expectedID:     "arch",
			expectedFamily: FamilyArch,
			expectedName:   "Arch Linux",
			expectedPretty: "Arch Linux",
			expectedVer:    "",
			expectedLikes:  nil,
		},
		{
			name:           "CachyOS",
			content:        cachyOSReleaseFixture,
			expectedID:     "cachyos",
			expectedFamily: FamilyArch,
			expectedName:   "CachyOS Linux",
			expectedPretty: "CachyOS",
			expectedVer:    "",
			expectedLikes:  []string{"arch"},
		},
		{
			name:           "Ubuntu 24.04",
			content:        ubuntuReleaseFixture,
			expectedID:     "ubuntu",
			expectedFamily: FamilyDebian,
			expectedName:   "Ubuntu",
			expectedPretty: "Ubuntu 24.04 LTS",
			expectedVer:    "24.04",
			expectedLikes:  []string{"debian"},
		},
		{
			name:           "Fedora 40",
			content:        fedoraReleaseFixture,
			expectedID:     "fedora",
			expectedFamily: FamilyFedora,
			expectedName:   "Fedora Linux",
			expectedPretty: "Fedora Linux 40 (Workstation Edition)",
			expectedVer:    "40",
			expectedLikes:  nil,
		},
		{
			name:           "openSUSE Tumbleweed",
			content:        suseTumbleweedReleaseFixture,
			expectedID:     "opensuse-tumbleweed",
			expectedFamily: FamilySuse,
			expectedName:   "openSUSE Tumbleweed",
			expectedPretty: "openSUSE Tumbleweed",
			expectedVer:    "20240401",
			expectedLikes:  []string{"opensuse", "suse"},
		},
		{
			name:           "openSUSE Leap",
			content:        suseLeapReleaseFixture,
			expectedID:     "opensuse-leap",
			expectedFamily: FamilySuse,
			expectedName:   "openSUSE Leap",
			expectedPretty: "openSUSE Leap 15.5",
			expectedVer:    "15.5",
			expectedLikes:  []string{"suse", "opensuse"},
		},
		{
			name:           "Debian 12",
			content:        debianReleaseFixture,
			expectedID:     "debian",
			expectedFamily: FamilyDebian,
			expectedName:   "Debian GNU/Linux",
			expectedPretty: "Debian GNU/Linux 12 (bookworm)",
			expectedVer:    "12",
			expectedLikes:  nil,
		},
		{
			name:           "Rocky Linux 9",
			content:        rockyReleaseFixture,
			expectedID:     "rocky",
			expectedFamily: FamilyFedora,
			expectedName:   "Rocky Linux",
			expectedPretty: "Rocky Linux 9.3 (Blue Onyx)",
			expectedVer:    "9.3",
			expectedLikes:  []string{"rhel", "centos", "fedora"},
		},
		{
			name:           "Manjaro Linux",
			content:        manjaroReleaseFixture,
			expectedID:     "manjaro",
			expectedFamily: FamilyArch,
			expectedName:   "Manjaro Linux",
			expectedPretty: "Manjaro Linux",
			expectedVer:    "",
			expectedLikes:  []string{"arch"},
		},
		{
			name:           "Linux Mint 21",
			content:        mintReleaseFixture,
			expectedID:     "linuxmint",
			expectedFamily: FamilyDebian,
			expectedName:   "Linux Mint",
			expectedPretty: "Linux Mint 21.3",
			expectedVer:    "21.3",
			expectedLikes:  []string{"ubuntu", "debian"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info, err := ParseOSRelease(strings.NewReader(tc.content))
			if err != nil {
				t.Fatalf("unexpected error parsing %s: %v", tc.name, err)
			}
			if info.ID != tc.expectedID {
				t.Errorf("ID mismatch: got %q, want %q", info.ID, tc.expectedID)
			}
			if info.Family != tc.expectedFamily {
				t.Errorf("Family mismatch: got %q, want %q", info.Family, tc.expectedFamily)
			}
			if info.Name != tc.expectedName {
				t.Errorf("Name mismatch: got %q, want %q", info.Name, tc.expectedName)
			}
			if info.PrettyName != tc.expectedPretty {
				t.Errorf("PrettyName mismatch: got %q, want %q", info.PrettyName, tc.expectedPretty)
			}
			if info.VersionID != tc.expectedVer {
				t.Errorf("VersionID mismatch: got %q, want %q", info.VersionID, tc.expectedVer)
			}
			if len(info.IDLike) != len(tc.expectedLikes) {
				t.Errorf("IDLike length mismatch: got %v, want %v", info.IDLike, tc.expectedLikes)
			} else {
				for i, l := range tc.expectedLikes {
					if info.IDLike[i] != l {
						t.Errorf("IDLike[%d] mismatch: got %q, want %q", i, info.IDLike[i], l)
					}
				}
			}
		})
	}
}

func TestHostDistroDetection(t *testing.T) {
	// The host system running this test suite is CachyOS (an Arch derivative).
	info, err := Detect()
	if err != nil {
		t.Fatalf("Detect() returned unexpected error on host: %v", err)
	}

	if info == nil {
		t.Fatal("expected non-nil DistroInfo on host system")
	}

	if info.Family != FamilyArch {
		t.Errorf("expected host family to be FamilyArch, got %v (ID: %s, IDLike: %v)", info.Family, info.ID, info.IDLike)
	}

	if !info.IsArch() {
		t.Errorf("expected IsArch() to return true on host")
	}

	if !info.HasLike("arch") {
		t.Errorf("expected HasLike(\"arch\") to return true on host")
	}

	// Verify cached singleton
	ResetCurrentCache()
	cur := Current()
	if cur.ID != info.ID || cur.Family != info.Family {
		t.Errorf("Current() did not match Detect(): got %v, want %v", cur, info)
	}
}

func TestFamilyHelperMethods(t *testing.T) {
	archInfo := &DistroInfo{Family: FamilyArch, ID: "arch"}
	debInfo := &DistroInfo{Family: FamilyDebian, ID: "ubuntu", IDLike: []string{"debian"}}
	fedInfo := &DistroInfo{Family: FamilyFedora, ID: "fedora"}
	suseInfo := &DistroInfo{Family: FamilySuse, ID: "opensuse-leap"}

	if !archInfo.IsArch() || archInfo.IsDebian() || archInfo.IsFedora() || archInfo.IsSuse() {
		t.Error("archInfo helper check failed")
	}
	if !debInfo.IsDebian() || debInfo.IsArch() || debInfo.IsFedora() || debInfo.IsSuse() {
		t.Error("debInfo helper check failed")
	}
	if !fedInfo.IsFedora() || fedInfo.IsArch() || fedInfo.IsDebian() || fedInfo.IsSuse() {
		t.Error("fedInfo helper check failed")
	}
	if !suseInfo.IsSuse() || suseInfo.IsArch() || suseInfo.IsDebian() || suseInfo.IsFedora() {
		t.Error("suseInfo helper check failed")
	}

	if !debInfo.HasLike("debian") || !debInfo.HasLike("ubuntu") {
		t.Error("HasLike check failed on debInfo")
	}
	if debInfo.HasLike("arch") {
		t.Error("HasLike(\"arch\") should be false for debInfo")
	}
}

func TestUnquoteAndEscapes(t *testing.T) {
	input := `
# Comment at the top
EMPTY_LINE=
KEY_UNQUOTED=simple_value
KEY_DOUBLE="double \"quoted\" with \\slash"
KEY_SINGLE='single \'quoted\''
TRAILING_SPACES="spaces inside"   
`
	info, err := ParseOSRelease(strings.NewReader(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Raw["KEY_UNQUOTED"] != "simple_value" {
		t.Errorf("unquoted: got %q, want 'simple_value'", info.Raw["KEY_UNQUOTED"])
	}
	if info.Raw["KEY_DOUBLE"] != `double "quoted" with \slash` {
		t.Errorf("double quote unescape failed: got %q", info.Raw["KEY_DOUBLE"])
	}
	if info.Raw["KEY_SINGLE"] != `single 'quoted'` {
		t.Errorf("single quote unescape failed: got %q", info.Raw["KEY_SINGLE"])
	}
	if info.Raw["TRAILING_SPACES"] != "spaces inside" {
		t.Errorf("trailing spaces unescape failed: got %q", info.Raw["TRAILING_SPACES"])
	}
}

func TestDetectFromPathsFallback(t *testing.T) {
	tmpDir := t.TempDir()
	secondPath := filepath.Join(tmpDir, "second-os-release")
	content := "ID=ubuntu\nNAME=Ubuntu\n"
	if err := os.WriteFile(secondPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write tmp os-release: %v", err)
	}

	nonExistent := filepath.Join(tmpDir, "does-not-exist")
	info, err := DetectFromPaths(nonExistent, secondPath)
	if err != nil {
		t.Fatalf("DetectFromPaths failed: %v", err)
	}

	if info.ID != "ubuntu" || info.Family != FamilyDebian {
		t.Errorf("fallback failed: got ID %q, Family %v", info.ID, info.Family)
	}

	// All non-existent paths
	_, err = DetectFromPaths(nonExistent, filepath.Join(tmpDir, "another-missing"))
	if err == nil {
		t.Error("expected error when all paths are missing, got nil")
	}
}

func TestUnknownFamilyClassification(t *testing.T) {
	info := &DistroInfo{
		ID:     "customlinux",
		IDLike: []string{"bsd", "solaris"},
	}
	family := ClassifyFamily(info.ID, info.IDLike)
	if family != FamilyUnknown {
		t.Errorf("expected FamilyUnknown, got %v", family)
	}
}
