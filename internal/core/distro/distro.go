package distro

import (
	"bufio"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
)

// DistroFamily identifies the major Linux distribution family.
type DistroFamily string

const (
	// FamilyArch represents Arch Linux and its derivatives (CachyOS, EndeavourOS, Manjaro, Artix, etc.).
	FamilyArch DistroFamily = "arch"

	// FamilyDebian represents Debian and its derivatives (Ubuntu, Linux Mint, Pop!_OS, Kali, etc.).
	FamilyDebian DistroFamily = "debian"

	// FamilyFedora represents Fedora, Red Hat Enterprise Linux, CentOS, Rocky Linux, AlmaLinux, etc.
	FamilyFedora DistroFamily = "fedora"

	// FamilySuse represents openSUSE Tumbleweed, openSUSE Leap, SUSE Linux Enterprise, etc.
	FamilySuse DistroFamily = "suse"

	// FamilyUnknown represents an unrecognized or unclassified distribution family.
	FamilyUnknown DistroFamily = "unknown"
)

// DistroInfo holds parsed distribution metadata and classified family.
type DistroInfo struct {
	ID         string            `json:"id"`
	IDLike     []string          `json:"id_like"`
	Name       string            `json:"name"`
	PrettyName string            `json:"pretty_name"`
	VersionID  string            `json:"version_id"`
	Family     DistroFamily      `json:"family"`
	Raw        map[string]string `json:"raw,omitempty"`
}

var (
	// Standard paths for distribution identification files.
	defaultReleasePaths = []string{
		"/etc/os-release",
		"/usr/lib/os-release",
	}

	currentOnce sync.Once
	currentInfo *DistroInfo
)

// Current returns the detected distribution information for the current system.
// The detection result is cached after the first call.
func Current() *DistroInfo {
	currentOnce.Do(func() {
		info, err := Detect()
		if err != nil || info == nil {
			currentInfo = &DistroInfo{
				ID:         "unknown",
				IDLike:     nil,
				Name:       "Unknown Linux",
				PrettyName: "Unknown Linux",
				VersionID:  "",
				Family:     FamilyUnknown,
				Raw:        make(map[string]string),
			}
			return
		}
		currentInfo = info
	})
	return currentInfo
}

// ResetCurrentCache clears the cached distribution info (primarily used for testing).
func ResetCurrentCache() {
	currentInfo = nil
	currentOnce = sync.Once{}
}

// Detect inspects standard release files (/etc/os-release, /usr/lib/os-release)
// and returns the parsed distribution metadata.
func Detect() (*DistroInfo, error) {
	return DetectFromPaths(defaultReleasePaths...)
}

// DetectFromPaths attempts to parse the first accessible release file from the candidate paths.
func DetectFromPaths(paths ...string) (*DistroInfo, error) {
	var lastErr error
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			lastErr = err
			continue
		}
		defer f.Close()

		info, err := ParseOSRelease(f)
		if err != nil {
			lastErr = err
			continue
		}
		return info, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("no os-release file found")
}

// ParseOSRelease parses KEY=VALUE contents following the os-release standard specification.
func ParseOSRelease(r io.Reader) (*DistroInfo, error) {
	raw := make(map[string]string)
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := unquoteValue(strings.TrimSpace(parts[1]))
		raw[key] = val
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	info := &DistroInfo{
		ID:         raw["ID"],
		Name:       raw["NAME"],
		PrettyName: raw["PRETTY_NAME"],
		VersionID:  raw["VERSION_ID"],
		Raw:        raw,
	}

	// Fallback for PrettyName and Name if omitted
	if info.PrettyName == "" {
		if info.Name != "" {
			info.PrettyName = info.Name
		} else if info.ID != "" {
			info.PrettyName = info.ID
		}
	}
	if info.Name == "" {
		info.Name = info.PrettyName
	}

	// Parse ID_LIKE space-separated list
	if idLikeRaw, ok := raw["ID_LIKE"]; ok && idLikeRaw != "" {
		fields := strings.Fields(idLikeRaw)
		info.IDLike = fields
	}

	info.Family = ClassifyFamily(info.ID, info.IDLike)
	return info, nil
}

// ParseOSReleaseFile reads and parses a specific os-release file from the filesystem.
func ParseOSReleaseFile(path string) (*DistroInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseOSRelease(f)
}

// ClassifyFamily classifies the distribution into a DistroFamily enum based on ID and ID_LIKE values.
func ClassifyFamily(id string, idLike []string) DistroFamily {
	normID := strings.ToLower(strings.TrimSpace(id))
	var normIDLike []string
	for _, l := range idLike {
		normIDLike = append(normIDLike, strings.ToLower(strings.TrimSpace(l)))
	}

	// 1. Direct ID evaluation
	switch normID {
	case "arch", "cachyos", "endeavouros", "manjaro", "artix", "garuda", "arco", "blackarch", "parabola":
		return FamilyArch
	case "debian", "ubuntu", "linuxmint", "mint", "pop", "kali", "elementary", "zorin", "raspbian", "devuan", "deepin":
		return FamilyDebian
	case "fedora", "rhel", "centos", "rocky", "almalinux", "amzn", "ol", "scientific":
		return FamilyFedora
	case "opensuse", "opensuse-tumbleweed", "opensuse-leap", "suse", "sles", "sled":
		return FamilySuse
	}

	// 2. ID_LIKE evaluations
	for _, like := range normIDLike {
		switch like {
		case "arch":
			return FamilyArch
		case "debian", "ubuntu":
			return FamilyDebian
		case "fedora", "rhel", "centos":
			return FamilyFedora
		case "opensuse", "suse":
			return FamilySuse
		}
	}

	// 3. Fallback heuristic for composite or unusual IDs
	if strings.Contains(normID, "arch") {
		return FamilyArch
	}
	if strings.Contains(normID, "debian") || strings.Contains(normID, "ubuntu") {
		return FamilyDebian
	}
	if strings.Contains(normID, "fedora") || strings.Contains(normID, "rhel") {
		return FamilyFedora
	}
	if strings.Contains(normID, "suse") {
		return FamilySuse
	}

	return FamilyUnknown
}

// IsArch returns true if the distribution belongs to the Arch family.
func (d *DistroInfo) IsArch() bool {
	return d != nil && d.Family == FamilyArch
}

// IsDebian returns true if the distribution belongs to the Debian family.
func (d *DistroInfo) IsDebian() bool {
	return d != nil && d.Family == FamilyDebian
}

// IsFedora returns true if the distribution belongs to the Fedora family.
func (d *DistroInfo) IsFedora() bool {
	return d != nil && d.Family == FamilyFedora
}

// IsSuse returns true if the distribution belongs to the SUSE family.
func (d *DistroInfo) IsSuse() bool {
	return d != nil && d.Family == FamilySuse
}

// HasLike returns true if ID matches target or any ID_LIKE item matches target (case-insensitive).
func (d *DistroInfo) HasLike(target string) bool {
	if d == nil {
		return false
	}
	target = strings.ToLower(strings.TrimSpace(target))
	if strings.ToLower(d.ID) == target {
		return true
	}
	for _, like := range d.IDLike {
		if strings.ToLower(like) == target {
			return true
		}
	}
	return false
}

// unquoteValue strips outer double or single quotes and handles common escape sequences.
func unquoteValue(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
	}

	// Unescape common escape characters
	replacer := strings.NewReplacer(
		`\"`, `"`,
		`\'`, `'`,
		`\\`, `\`,
		`\$`, `$`,
		`\t`, "\t",
		`\n`, "\n",
	)
	return replacer.Replace(v)
}
