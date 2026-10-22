package detect

import (
	"os"
	"strings"
)

// DistributionFamily reads the D6 package-command family from os-release.
// Unknown distributions return an empty family; callers must not guess a
// package command from an unrelated executable on PATH.
func DistributionFamily(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return FamilyFromOSRelease(string(data))
}

// FamilyFromOSRelease is the pure parser used by DistributionFamily.
func FamilyFromOSRelease(contents string) string {
	values := make(map[string]string)
	for _, line := range strings.Split(contents, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[key] = strings.Trim(strings.TrimSpace(value), "\"'")
	}
	if family := familyID(values["ID"]); family != "" {
		return family
	}
	for _, id := range strings.Fields(values["ID_LIKE"]) {
		if family := familyID(id); family != "" {
			return family
		}
	}
	return ""
}

func familyID(id string) string {
	id = strings.ToLower(id)
	switch {
	case id == "arch" || id == "manjaro" || id == "endeavouros" || id == "artix":
		return "arch"
	case id == "debian" || id == "ubuntu" || id == "linuxmint" || id == "pop":
		return "debian"
	case id == "fedora" || id == "rhel" || id == "centos" || id == "rocky" || id == "almalinux":
		return "fedora"
	case strings.HasPrefix(id, "opensuse") || id == "suse" || id == "sles":
		return "suse"
	case id == "alpine":
		return "alpine"
	case id == "void":
		return "xbps"
	default:
		return ""
	}
}
